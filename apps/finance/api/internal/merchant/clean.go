// Package merchant turns card-statement descriptors into merchants and
// categories, tuned for Singapore. The cascade (cheapest and most certain
// first): your own rules → special lines (transit, payments, fees) → curated
// SG brands → local directory (OpenStreetMap + ACRA) → optional Claude.
package merchant

import (
	"regexp"
	"strings"
	"unicode"
)

// Descriptor is a cleaned statement description.
type Descriptor struct {
	Raw     string `json:"raw"`
	Name    string `json:"name"`    // merchant part, upper case, ids and location removed
	Branch  string `json:"branch"`  // branch / location hint ("JEM", "VELOCITY@NOV")
	Prefix  string `json:"prefix"`  // processor or platform prefix ("SMP", "FP", "GRAB", "WL")
	Country string `json:"country"` // ISO-2 when printed (SG default)
	City    string `json:"city"`
	Foreign bool   `json:"foreign"`
	Special string `json:"special,omitempty"` // transit | payment | fee
	Key     string `json:"key"`               // stable key for your rules and caches
}

var (
	reSpace     = regexp.MustCompile(`\s+`)
	rePhone     = regexp.MustCompile(`^\+?\d{3}[-.]?\d{3}[-.]?\d{4}$`)
	reDigits    = regexp.MustCompile(`^\d+$`)
	reTransit   = regexp.MustCompile(`^(BUS/MRT|SIMPLYGO|TRANSIT ?LINK|EZ-?LINK|NETS ?FLASHPAY)\b`)
	rePayment   = regexp.MustCompile(`^(PYMT|PAYMENT|PAYMT|GIRO PAYMENT|BILL PAYMENT|FAST PAYMENT|IBG PAYMENT)\b|THANK ?YOU$`)
	reFee       = regexp.MustCompile(`^(DCC FEE|CCY CONVERSION FEE|CURRENCY CONVERSION FEE|FOREIGN (TXN|TRANSACTION) FEE|FCY FEE|LATE (PAYMENT )?(CHARGE|FEE)|FINANCE CHARGE|INTEREST CHARGE|ANNUAL FEE|CASH ADVANCE FEE|OVERLIMIT FEE|PURCHASE INTEREST)`)
	reParen     = regexp.MustCompile(`\(([^)]*)\)`)
	reStarSplit = regexp.MustCompile(`^([A-Z0-9 .]{1,12}?)\s?\*\s*(.+)$`)
	reSlash     = regexp.MustCompile(`^([A-Z]{1,4})[/_](.+)$`)
)

// US states, for descriptors like "WL *STEAM PURCHASE 425-889-9642 WA".
var usStates = set("AL AK AZ AR CA CO CT DE FL GA HI ID IL IN IA KS KY LA ME MD MA MI MN MS MO MT NE NV NH NJ NM NY NC ND OH OK OR PA RI SC SD TN TX UT VT VA WA WV WI WY DC")

// Countries that appear as trailing ISO codes on SG statements.
var countries = set("SG MY ID TH VN PH HK TW CN JP KR IN AU NZ US CA GB IE FR DE NL BE LU CH AT IT ES PT SE NO DK FI AE IL MO")

// Processor / platform prefixes. A brand prefix names the merchant itself
// (fp* = foodpanda); otherwise the merchant follows the prefix.
var prefixBrand = map[string]string{
	"FP": "FOODPANDA", "GRAB": "GRAB", "GOJEK": "GOJEK", "AMZN": "AMAZON", "AMAZON MAR": "AMAZON",
	"AMAZON": "AMAZON", "SHOPEE": "SHOPEE", "LAZADA": "LAZADA", "APPLE.COM": "APPLE", "GOOGLE": "GOOGLE",
	"STEAM": "STEAM", "NETFLIX.COM": "NETFLIX", "SPOTIFY": "SPOTIFY", "DELIVEROO": "DELIVEROO",
}

func set(s string) map[string]bool {
	m := map[string]bool{}
	for _, f := range strings.Fields(s) {
		m[f] = true
	}
	return m
}

// isRefID reports tokens that are references, not names: hex hashes,
// mixed letter/digit codes, long numbers.
func isRefID(tok string) bool {
	t := strings.Trim(tok, "#:.,")
	if reDigits.MatchString(t) {
		return len(t) >= 5
	}
	letters, digits := 0, 0
	for _, r := range t {
		switch {
		case unicode.IsDigit(r):
			digits++
		case unicode.IsLetter(r):
			letters++
		case r == '-':
		default:
			return false
		}
	}
	return len(t) >= 8 && digits >= 2 && letters >= 1 && digits+letters >= 8
}

// Clean parses a raw descriptor.
func Clean(raw string) Descriptor {
	d := Descriptor{Raw: raw, Country: "SG"}
	s := strings.ToUpper(strings.TrimSpace(raw))
	s = strings.NewReplacer("—", " ", "–", " ", "|", " ").Replace(s)
	s = reSpace.ReplaceAllString(s, " ")
	s = strings.TrimLeft(s, "_=-~. ")
	s = strings.TrimPrefix(s, "WWW.")
	// Cross-border checkout processors put their name first.
	for _, p := range []string{"GLOBALE ", "GLOBAL-E "} {
		s = strings.TrimPrefix(s, p)
	}
	s = strings.ReplaceAll(s, " N/A ", " ")

	switch {
	case reTransit.MatchString(s):
		d.Special, d.Name = "transit", "BUS/MRT"
		d.Key = "transit"
		return d
	case rePayment.MatchString(s):
		d.Special, d.Name = "payment", "CARD PAYMENT"
		d.Key = "payment"
		return d
	case reFee.MatchString(s):
		d.Special = "fee"
		d.Name = reFee.FindString(s)
		d.Key = "fee|" + strings.ToLower(Squash(d.Name))
		return d
	}

	toks := strings.Fields(s)
	// Trailing location. A country code only counts as the very last token
	// ("… Vicenza IT"); earlier ones are part of the name ("(S) PT" = PTE).
	if n := len(toks); n > 1 && countries[toks[n-1]] {
		if last := toks[n-1]; last != "SG" {
			d.Country, d.Foreign = last, true
			if prev := toks[n-2]; n > 2 && isWord(prev) && !strings.Contains(prev, "*") && prev != "SINGAPORE" {
				d.City = prev
				toks = toks[:n-1]
			}
		}
		toks = toks[:len(toks)-1]
	}
	for len(toks) > 1 {
		last := toks[len(toks)-1]
		switch {
		case last == "SG" || last == "SINGAPORE" || last == "SINGAPOR":
		case usStates[last] && len(toks) > 2 && rePhone.MatchString(toks[len(toks)-2]):
			d.Country, d.Foreign = "US", true
		case rePhone.MatchString(last), reDigits.MatchString(last) && len(last) <= 4:
		default:
			goto doneLoc
		}
		toks = toks[:len(toks)-1]
	}
doneLoc:
	s = strings.Join(toks, " ")

	// Parenthetical: "(TPS)" is a branch, "(S)" a legal marker.
	if m := reParen.FindStringSubmatch(s); m != nil {
		inner := strings.TrimSpace(m[1])
		if inner != "S" && inner != "SG" && d.Branch == "" {
			d.Branch = inner
		}
		s = strings.TrimSpace(reParen.ReplaceAllString(s, " "))
	}

	// Prefixes: "SMP*NAME", "FP*FOOD PANDA", "WL *STEAM", "TS/STEAK 99", "SMP_GAO JI".
	if m := reStarSplit.FindStringSubmatch(s); m != nil {
		d.Prefix = strings.TrimSpace(m[1])
		s = strings.TrimSpace(strings.ReplaceAll(m[2], "*", " "))
	} else if m := reSlash.FindStringSubmatch(s); m != nil {
		d.Prefix, s = m[1], strings.TrimSpace(m[2])
	}

	// Branch: "KOPITIAM@JEM", "SONG FA - VELOCITY@NOV", "LONGJING-SUNTEC".
	if i := strings.Index(s, " - "); i > 0 {
		d.Branch = joinNonEmpty(strings.TrimSpace(s[i+3:]), d.Branch)
		s = strings.TrimSpace(s[:i])
	} else if i := strings.Index(s, "@"); i > 0 {
		d.Branch = joinNonEmpty(strings.TrimSpace(s[i+1:]), d.Branch)
		s = strings.TrimSpace(s[:i])
	} else if i := strings.IndexByte(s, '-'); i > 2 && !strings.Contains(s[:i], " ") {
		// "LONGJING-SUNTEC", "HEYTEA-SG-STORE"; not "7-ELEVEN".
		if rest := strings.TrimSpace(s[i+1:]); len(rest) >= 2 && !reDigits.MatchString(s[:i]) {
			d.Branch = joinNonEmpty(rest, d.Branch)
			s = strings.TrimSpace(s[:i])
		}
	}

	// Reference ids and trailing location abbreviations.
	var kept []string
	for _, t := range strings.Fields(s) {
		if isRefID(t) {
			continue
		}
		kept = append(kept, t)
	}
	// Truncated legal suffixes: "GAO JI FOOD (S) PT" (PTE), "… PTE LTD".
	for len(kept) > 1 {
		last, prev := kept[len(kept)-1], kept[len(kept)-2]
		if last == "PTE" || last == "LTD" || last == "PTE." || last == "LTD." || last == "PL" ||
			last == "PT" && !placeWords[prev] && placeAbbr[prev] == nil {
			kept = kept[:len(kept)-1]
			continue
		}
		if (last == "LT" || last == "L") && prev == "PTE" { // "PTE LT", "PTE L"
			kept = kept[:len(kept)-2]
			continue
		}
		break
	}
	for len(kept) > 1 && isPlace(kept[len(kept)-1]) {
		d.Branch = joinNonEmpty(kept[len(kept)-1], d.Branch)
		kept = kept[:len(kept)-1]
	}
	d.Name = strings.Trim(strings.Join(kept, " "), " -_*.,")
	if d.Name == "" && d.Prefix != "" {
		d.Name = d.Prefix
	}
	d.Key = key(d)
	return d
}

// key is the descriptor's identity for rules and caches. Squashed, so OCR
// spacing ("SMILEDENTALSTUDIO" vs "SMILE DENTAL STUDIO") doesn't matter.
func key(d Descriptor) string {
	k := Squash(d.Name)
	if d.Prefix != "" {
		if b, ok := prefixBrand[d.Prefix]; ok {
			k = b
			if d.Name != d.Prefix && d.Prefix != "FP" && d.Prefix != "GRAB" {
				k += "|" + Squash(d.Name)
			}
		} else {
			k = Squash(d.Prefix) + "|" + k
		}
	}
	return strings.ToLower(k)
}

func isWord(s string) bool {
	for _, r := range s {
		if !unicode.IsLetter(r) {
			return false
		}
	}
	return len(s) >= 3
}

func joinNonEmpty(a, b string) string {
	switch {
	case a == "":
		return b
	case b == "":
		return a
	}
	return a + " " + b
}

func squashSpaces(s string) string { return reSpace.ReplaceAllString(strings.TrimSpace(s), " ") }

// Squash keeps letters and digits only (upper case): "Daily Fusion-Cuisi" →
// "DAILYFUSIONCUISI". Used for prefix matching of truncated names.
func Squash(s string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(s) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// Title turns "SMP DAILY FUSION CUISI" into "Daily Fusion Cuisi" for display
// when nothing better is known.
func Title(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		// Short words without vowels are acronyms: DCC, KFC, MRT, BPSP.
		if len(w) <= 4 && !strings.ContainsAny(w, "aeiou") || w == "&" {
			words[i] = strings.ToUpper(w)
			continue
		}
		r := []rune(w)
		r[0] = unicode.ToUpper(r[0])
		words[i] = string(r)
	}
	return strings.Join(words, " ")
}
