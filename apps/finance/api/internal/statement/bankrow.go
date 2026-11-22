package statement

import (
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

// classify reads a bank-account row's description lines and says what kind
// of money movement it is, who the other side is, and a note worth keeping.
// kind is "" when the merchant matcher should decide (a purchase or refund).
//
//	OCBC: PAYMENT/TRANSFER | via PayNow-DBSS | from A PERSON | OTHR Photoshoot
//	OCBC: NETS QR | 88968901 | NETS QR PURCHASE | AXS PTE LTD
//	OCBC: IBG GIRO | SALA | EMPLOYER PTE LTD
//	DBS:  FAST Payment / Receipt | PAYNOW TRANSFER 8911289 | TO: YX GUANGXXXX | PAYNOW TRANSFER NOODLES | OTHER
//	DBS:  Debit Card Transaction | GRAB* AB27… SI SGP 28SEP | 4628-…
//
// Money between your own accounts is a transfer; salary, interest and
// government payouts are income; AXS is a credit-card bill (a payment, never
// spending); paying a business (UEN, QR, NETS, a card, a name the bank prints
// in full) is a purchase. Paying a person (a name the bank masks, PayNow to a
// phone, an account number) is spending up to PersonSpendLimit — food stalls
// take PayNow — and a transfer above it. Money in from people is a transfer.
func classify(lines []string, credit bool, holder string, amount int64) (kind, party, note string, check bool) {
	if len(lines) == 0 {
		return "", "", "", false
	}
	typ := strings.ToUpper(strings.TrimSpace(lines[0]))
	all := strings.ToUpper(strings.Join(lines, " "))
	var channel string
	partyAt := -1
	for i, l := range lines {
		t := strings.TrimSpace(l)
		up := strings.ToUpper(t)
		switch {
		case partyAt < 0 && (strings.HasPrefix(up, "TO:") || strings.HasPrefix(up, "TO ") || strings.HasPrefix(up, "FROM:") || strings.HasPrefix(up, "FROM ")):
			cut := strings.IndexAny(t, ": ")
			party, partyAt = trimCut(strings.TrimSpace(strings.TrimLeft(t[cut:], ": "))), i
		case strings.HasPrefix(up, "VIA "):
			channel = up[4:]
		case up == "SALA" && i+1 < len(lines):
			return "income", strings.TrimSpace(lines[i+1]), "Salary", false
		}
	}
	memo := memoAfter(lines, partyAt)

	switch {
	case strings.HasPrefix(typ, "INTEREST"):
		if credit {
			return "income", "Interest", "", false
		}
	case credit && strings.Contains(typ, "GIRO") && containsWord(all, "GOV"):
		return "income", "Government", "Government payout", false
	case strings.Contains(all, "INVESTMENT & SECURITIES") || isInvestment(party) || partyAt < 0 && len(lines) > 1 && isInvestment(lines[1]):
		name := party
		if name == "" && len(lines) > 1 && isInvestment(lines[1]) {
			name = strings.TrimSpace(lines[1])
		}
		if name == "" || strings.EqualFold(name, "INVESTMENT") {
			name = "Investments"
		}
		dir := "To "
		if credit {
			dir = "From "
		}
		return "transfer", title(name), dir + "your investments", false
	case strings.HasPrefix(typ, "FAST COLLECTION"):
		// A direct debit someone collected (an investment top-up, a bill…).
		return "transfer", "Direct debit", "Collected by direct debit: mark it as a purchase if it paid for something.", true
	}
	if party == "" {
		switch {
		case strings.Contains(typ, "DEBIT CARD") && len(lines) > 1:
			party = strings.TrimSpace(lines[1]) // "GRAB* AB27… SI SGP 28SEP"
		case strings.Contains(all, "NETS QR"):
			// OCBC prints the merchant last: "NETS QR | 88968901 | NETS QR PURCHASE | AXS PTE LTD".
			for i := len(lines) - 1; i > 0; i-- {
				l := strings.TrimSpace(lines[i])
				if letters(l) >= 3 && !strings.HasPrefix(strings.ToUpper(l), "NETS") {
					party = l
					break
				}
			}
		case len(lines) == 1:
			party = strings.TrimSpace(lines[0]) // card-style row (Trust): the name is the whole line
		}
	}
	if party != "" && holder != "" && sameName(party, holder) {
		n := "Between your own accounts"
		if memo != "" {
			n += ": " + memo
		}
		return "transfer", "Own account", n, false
	}
	if !credit && strings.Contains(strings.ToUpper(party), "AXS") {
		return "payment", "AXS", "Credit-card bill paid at AXS (not spending: the card's purchases are)", false
	}

	business := strings.Contains(channel, "UEN") || strings.Contains(channel, "QR") || strings.Contains(typ, "NETS") ||
		strings.Contains(typ, "POINT-OF-SALE") || strings.Contains(typ, "DEBIT CARD") || strings.Contains(all, "NETS QR") || reCompany.MatchString(strings.ToUpper(party))
	person := !business && (strings.Contains(channel, "MOBILE") || strings.Contains(channel, "NRIC") || masked(party) ||
		len(lines) > 1 && reAcctLine.MatchString(strings.Join(lines, " ")) && !strings.Contains(typ, "GIRO"))

	if credit {
		switch {
		case len(lines) == 1 && !person:
			return "", party, memo, false // a card-style refund or credit: the matcher decides
		case party != "":
			n := "From " + title(party)
			if memo != "" {
				n += ": " + memo
			}
			return "transfer", title(party), n, false
		}
		// Money in that says little ("IBG GIRO … HSBCn_071026", "SUPPLIER
		// PAYMENT"): a transfer until you say otherwise, never a refund.
		name := "Money in"
		if strings.Contains(typ, "GIRO") {
			name = "GIRO transfer"
		}
		// The reference often carries the sending bank ("…OCBCSGSGBRT…").
		for code, bank := range bankCodes {
			if strings.Contains(all, code) {
				name = "Money in from " + bank
				break
			}
		}
		return "transfer", name, "Money in: " + strings.Join(lines, " "), true
	}
	switch {
	case person && amount <= PersonSpendLimit:
		n := "PayNow to a person, counted as spending (up to " + moneyText(PersonSpendLimit) + ")"
		if memo != "" {
			n = memo + " · " + n
		}
		return "", title(party), n, true
	case person:
		n := "To " + title(party)
		if memo != "" {
			n += ": " + memo
		}
		return "transfer", title(party), n, false
	case party != "":
		return "", party, memo, false
	case len(lines) > 1:
		return "transfer", "Money out", "Money out: " + strings.Join(lines, " "), true
	}
	return "", "", memo, false
}

// PersonSpendLimit: a payment to a person up to this many cents counts as
// spending (hawkers and stalls take PayNow); above it, it's a transfer.
const PersonSpendLimit = 30_00

func moneyText(c int64) string { return "S$" + strconv.FormatInt(c/100, 10) }

// memoAfter finds the payer's message: the first line after the payee that
// isn't boilerplate or a reference code ("OTHR Photoshoot" → "Photoshoot",
// "PAYNOW TRANSFER NOODLES" → "Noodles").
func memoAfter(lines []string, partyAt int) string {
	from := partyAt + 1
	if partyAt < 0 {
		from = 1
	}
	for _, l := range lines[min(from, len(lines)):] {
		t := strings.TrimSpace(l)
		up := strings.ToUpper(t)
		for _, p := range []string{"OTHR", "PAYNOW TRANSFER"} {
			if strings.HasPrefix(up, p) {
				t = strings.TrimSpace(t[len(p):])
				up = strings.ToUpper(t)
			}
		}
		if m := cleanMemo(t); m != "" {
			if m == strings.ToUpper(m) {
				m = title(m)
			}
			return m
		}
	}
	return ""
}

var (
	// Business names: company suffixes.
	reCompany = regexp.MustCompile(`\b(?:PTE|LTD|LIMITED|PRIVATE|LLP|INC|CORP|CORPORATION|COMPANY|GROUP|ENTERPRISES?|TRADING|SERVICES|RESTAURANT|RESTAU)\b`)
	// Investment platforms whose money moves are transfers, not spending.
	investPlatforms = []string{"SYFE", "ENDOWUS", "STASHAWAY", "MOOMOO", "TIGER BROKERS", "INTERACTIVE BROKERS", "IBKR", "WEBULL", "FSMONE", "POEMS", "INVESTMENT"}
)

// bankCodes: SWIFT codes in transfer references → bank.
var bankCodes = map[string]string{"OCBCSGSG": "OCBC", "DBSSSGSG": "DBS", "UOVBSGSG": "UOB", "TRBUSGSG": "Trust", "HSBCSGSG": "HSBC", "SCBLSG": "Standard Chartered", "CITISGSG": "Citibank", "MBBESGS": "Maybank", "GXSPSGSG": "GXS"}

func isInvestment(s string) bool {
	up := strings.ToUpper(strings.TrimSpace(s))
	for _, p := range investPlatforms {
		if up == p || strings.HasPrefix(up, p+" ") {
			return true
		}
	}
	return false
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
	case "", "othr", "other", "othr - othr", "paynow transfer", "transfer - mobile", "-":
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
