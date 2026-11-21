// Package catalog holds the fixed spending categories, their groups (which
// carry the chart colours), and transaction kinds.
package catalog

// Group is a set of categories shown as one colour in stacked charts. The
// order is the colour slot order (validated categorical palette).
type Group struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Slot int    `json:"slot"` // 1..6 categorical slot; 0 = neutral (other)
}

// Category is a spending category.
type Category struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Group string `json:"group"`
	// Essential marks needs (groceries, transport, bills, insurance…) as
	// opposed to wants; used by insights and goal suggestions.
	Essential bool `json:"essential"`
}

var Groups = []Group{
	{"food", "Food", 1},
	{"transport", "Transport", 2},
	{"shopping", "Shopping & home", 3},
	{"bills", "Bills & subscriptions", 4},
	{"health", "Health & insurance", 5},
	{"lifestyle", "Lifestyle", 6},
	{"other", "Fees & other", 0},
}

var Categories = []Category{
	{"dining", "Dining out", "food", false},
	{"delivery", "Food delivery", "food", false},
	{"groceries", "Groceries", "food", true},
	{"transport", "Public transport", "transport", true},
	{"rides", "Taxi & ride-hailing", "transport", false},
	{"car", "Car & fuel", "transport", true},
	{"shopping", "Shopping", "shopping", false},
	{"home", "Home", "shopping", false},
	{"gifts", "Gifts & donations", "shopping", false},
	{"bills", "Bills & telco", "bills", true},
	{"subscriptions", "Subscriptions", "bills", false},
	{"health", "Health & dental", "health", true},
	{"insurance", "Insurance", "health", true},
	{"personal", "Personal care", "health", false},
	{"entertainment", "Games & entertainment", "lifestyle", false},
	{"leisure", "Leisure & fitness", "lifestyle", false},
	{"travel", "Travel", "lifestyle", false},
	{"education", "Education", "lifestyle", false},
	{"fees", "Fees & interest", "other", false},
	{"other", "Other", "other", false},
}

var byID = func() map[string]Category {
	m := make(map[string]Category, len(Categories))
	for _, c := range Categories {
		m[c.ID] = c
	}
	return m
}()

// Get returns a category by id.
func Get(id string) (Category, bool) {
	c, ok := byID[id]
	return c, ok
}

// Valid reports whether id is a known category.
func Valid(id string) bool { _, ok := byID[id]; return ok }

// Kinds of transaction. Only purchase, refund and fee count as spending.
const (
	Purchase = "purchase" // money out to a merchant
	Refund   = "refund"   // a merchant credit (reduces spend in its category)
	Fee      = "fee"      // bank fee / interest / DCC
	Payment  = "payment"  // paying the card bill — never spending
	Income   = "income"   // salary, rebates paid out, etc.
	Transfer = "transfer" // money moved between own accounts
)

// Kinds lists every kind, in display order.
var Kinds = []string{Purchase, Refund, Fee, Payment, Income, Transfer}

// ValidKind reports whether k is a known kind.
func ValidKind(k string) bool {
	for _, x := range Kinds {
		if x == k {
			return true
		}
	}
	return false
}

// Spending reports whether a kind counts towards spending.
func Spending(kind string) bool { return kind == Purchase || kind == Refund || kind == Fee }
