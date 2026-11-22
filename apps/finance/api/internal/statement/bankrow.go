package statement

import (
	"regexp"
	"strings"
	"unicode"
)

// classify reads a bank-account row's description lines and says what kind
// of money movement it is, who the other side is, and a note worth keeping.
// kind is "" when the merchant matcher should decide (a purchase or refund).
//
//	PAYMENT/TRANSFER | via PayNow-DBSS | from A PERSON | OTHR Photoshoot
//	FAST PAYMENT     | via PayNow-UEN  | to SINGAPORE POOLS
//	NETS QR          | 88968901 | NETS QR PURCHASE | AXS PTE LTD
//	IBG GIRO         | SALA | EMPLOYER PTE LTD
//
// Money between your own accounts is a transfer; salary, interest and
// government payouts are income; AXS (usually a card bill) is a payment;
// paying a business (UEN, QR, NETS) is a purchase; paying a person is a
// transfer (it isn't spending you can categorise).
func classify(lines []string, credit bool, holder string) (kind, party, note string, check bool) {
	if len(lines) == 0 {
		return "", "", "", false
	}
	typ := strings.ToUpper(lines[0])
	var channel, memo string
	business, person := false, false
	for i, l := range lines {
		t := strings.TrimSpace(l)
		low := strings.ToLower(t)
		switch {
		case strings.HasPrefix(low, "to ") || strings.HasPrefix(low, "from "):
			party = trimCut(strings.TrimSpace(t[strings.IndexByte(t, ' ')+1:]))
		case strings.HasPrefix(low, "via "):
			channel = strings.ToUpper(t[4:])
		case strings.HasPrefix(strings.ToUpper(t), "OTHR"):
			if m := cleanMemo(t[4:]); m != "" {
				memo = m
			}
		case t == "SALA" && i+1 < len(lines):
			return "income", strings.TrimSpace(lines[i+1]), "Salary", false
		}
	}
	upAll := strings.ToUpper(strings.Join(lines, " "))
	switch {
	case strings.HasPrefix(typ, "INTEREST"), strings.EqualFold(strings.TrimSpace(lines[0]), "interest"):
		if credit {
			return "income", "Interest", "", false
		}
	case credit && strings.Contains(typ, "GIRO") && containsWord(upAll, "GOV"):
		return "income", "Government", "Government payout", false
	}
	if strings.Contains(typ, "NETS") && party == "" {
		// The merchant is the last line with letters that isn't "NETS QR …".
		for i := len(lines) - 1; i > 0; i-- {
			l := strings.TrimSpace(lines[i])
			if letters(l) >= 3 && !strings.HasPrefix(strings.ToUpper(l), "NETS") {
				party = l
				break
			}
		}
		business = true
	}
	if party == "" && len(lines) == 1 {
		party = strings.TrimSpace(lines[0]) // card-style row (Trust): the name is the whole line
	}
	switch {
	case strings.Contains(channel, "UEN"), strings.Contains(channel, "QR"):
		business = true
	case strings.Contains(channel, "MOBILE"), strings.Contains(channel, "NRIC"):
		person = true
	}
	if !business && (masked(party) || reAcctLine.MatchString(strings.Join(lines, " ")) && !strings.Contains(typ, "GIRO")) {
		person = true
	}
	if party != "" && holder != "" && sameName(party, holder) {
		n := "Between your own accounts"
		if memo != "" {
			n += ": " + memo
		}
		return "transfer", "Own account", n, false
	}
	if !credit && strings.Contains(strings.ToUpper(party), "AXS") {
		return "payment", "AXS", "Paid at AXS, usually a card bill. If it was another bill, change it to a purchase.", true
	}
	switch {
	case business && !credit:
		return "", party, memo, false
	case credit && len(lines) > 1 && party == "":
		// Money in that says little ("IBG GIRO … HSBCn_071026"): a transfer
		// until you say otherwise — never a refund that lowers your spending.
		name := "Money in"
		if strings.Contains(typ, "GIRO") {
			name = "GIRO transfer"
		}
		return "transfer", name, "Money in: " + strings.Join(lines, " "), true
	case person || (len(lines) > 1 && party != "" && !business):
		dir := "To "
		if credit {
			dir = "From "
		}
		n := dir + title(party)
		if memo != "" {
			n += ": " + memo
		}
		return "transfer", title(party), n, false
	}
	return "", party, memo, false
}

var (
	// An account number on its own line ("0193491628"): a transfer to a
	// bank account, not a merchant.
	reAcctLine = regexp.MustCompile(`(?:^|\s)\d{9,12}(?:\s|$)`)
	reCodeLike = regexp.MustCompile(`[A-Za-z]*\d[A-Za-z0-9]{6,}`)
)

// trimCut drops a bracket the bank cut off ("YAN JING YI (Y" → "YAN JING YI").
func trimCut(s string) string {
	if i := strings.LastIndexByte(s, '('); i > 0 && !strings.Contains(s[i:], ")") {
		return strings.TrimSpace(s[:i])
	}
	return s
}

// cleanMemo keeps a payment message ("Photoshoot", "Happy Deepavali") and
// drops reference codes and boilerplate.
func cleanMemo(s string) string {
	s = strings.TrimSpace(strings.TrimLeft(strings.TrimSpace(s), "-: "))
	switch strings.ToLower(s) {
	case "", "othr", "other", "othr - othr", "paynow transfer", "transfer - mobile":
		return ""
	}
	if reCodeLike.MatchString(s) || letters(s) < 3 {
		return ""
	}
	return s
}

func letters(s string) int {
	n := 0
	for _, r := range s {
		if unicode.IsLetter(r) {
			n++
		}
	}
	return n
}

func containsWord(s, w string) bool {
	for _, f := range strings.Fields(s) {
		if f == w {
			return true
		}
	}
	return false
}

// masked: a name with letters hidden by the bank ("LOX HOXX SHEXX", "ISABEXXX").
func masked(s string) bool {
	if s == "" {
		return false
	}
	for _, f := range strings.Fields(s) {
		if strings.Contains(strings.ToUpper(f), "XX") && letters(f) == len(f) {
			return true
		}
	}
	return false
}

// sameName: is name (possibly masked with X, any word order, possibly a
// subset like "hong sheng") the account holder?
func sameName(name, holder string) bool {
	n := strings.Fields(strings.ToUpper(name))
	h := strings.Fields(strings.ToUpper(holder))
	if len(n) < 2 || len(h) < 2 || len(n) > len(h) {
		return false
	}
	used := make([]bool, len(h))
	for _, w := range n {
		found := false
		for i, hw := range h {
			if !used[i] && maskEqual(w, hw) {
				used[i], found = true, true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// maskEqual compares a word with a holder word, X matching any letter.
func maskEqual(w, hw string) bool {
	if len(w) != len(hw) {
		return false
	}
	for i := range len(w) {
		if w[i] != hw[i] && w[i] != 'X' {
			return false
		}
	}
	return true
}

// title: "LOK XIN YI" → "Lok Xin Yi"; keeps mixed-case names as they are.
func title(s string) string {
	if s != strings.ToUpper(s) {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
