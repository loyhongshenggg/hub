package merchant

import "testing"

func TestClean(t *testing.T) {
	cases := []struct {
		raw, name, branch, prefix, special, key string
		foreign                                 bool
	}{
		{raw: "BUS/MRT 900000001 SINGAPORE SG", name: "BUS/MRT", special: "transit", key: "transit"},
		{raw: "PYMT @ AXS -THANK YOU", special: "payment", name: "CARD PAYMENT", key: "payment"},
		{raw: "DCC FEE", special: "fee", name: "DCC FEE", key: "fee|dccfee"},
		{raw: "SMP*DAILY FUSION CUISI Singapore SG", name: "DAILY FUSION CUISI", prefix: "SMP", key: "smp|dailyfusioncuisi"},
		{raw: "fp*Food Panda Singapore SG", name: "FOOD PANDA", prefix: "FP", key: "foodpanda"},
		{raw: "Grab* 1a2b3c4d5e6f7a8b Singapore", name: "GRAB", prefix: "GRAB", key: "grab"},
		{raw: "Grab* A-7QXKLMN2PRS4TUV Singapore SG", name: "GRAB", prefix: "GRAB", key: "grab"},
		{raw: "KOPITIAM@JEM Singapore SG", name: "KOPITIAM", branch: "JEM", key: "kopitiam"},
		{raw: "SONG FA - VELOCITY@NOV SINGAPORE 307 SG", name: "SONG FA", branch: "VELOCITY@NOV", key: "songfa"},
		{raw: "TS/Steak 99 TPY Singapore SG", name: "STEAK 99", branch: "TPY", prefix: "TS", key: "ts|steak99"},
		{raw: "SMP_GAO JI FOOD (S) PT Singapore SG", name: "GAO JI FOOD", prefix: "SMP", key: "smp|gaojifood"},
		{raw: "MCDONALD'S (TPS) SINGAPORE SG", name: "MCDONALD'S", branch: "TPS", key: "mcdonalds"},
		{raw: "WL *STEAM PURCHASE 425-889-9642 WA", name: "STEAM PURCHASE", prefix: "WL", key: "wl|steampurchase", foreign: true},
		{raw: "Fotonica- MRK S.P.A. Milano IT", name: "FOTONICA", branch: "MRK S.P.A.", key: "fotonica", foreign: true},
		{raw: "7-ELEVEN TPY SINGAPORE SG", name: "7-ELEVEN", branch: "TPY", key: "7eleven"},
		{raw: "SMILEDENTALSTUDIO _ Singapore SG", name: "SMILEDENTALSTUDIO", key: "smiledentalstudio"},
	}
	for _, c := range cases {
		d := Clean(c.raw)
		if d.Name != c.name || d.Branch != c.branch || d.Prefix != c.prefix || d.Special != c.special || d.Key != c.key || d.Foreign != c.foreign {
			t.Errorf("Clean(%q) = name %q branch %q prefix %q special %q key %q foreign %v", c.raw, d.Name, d.Branch, d.Prefix, d.Special, d.Key, d.Foreign)
		}
	}
	// OCR spacing doesn't change the key.
	if Clean("SMILE DENTAL STUDIO Singapore SG").Key != Clean("SMILEDENTALSTUDIO Singapore SG").Key {
		t.Error("keys differ by spacing")
	}
}

func TestTitle(t *testing.T) {
	for in, want := range map[string]string{"GAO JI FOOD": "Gao Ji Food", "BPSP": "BPSP", "DCC FEE": "DCC Fee", "SANOOK KITCHEN": "Sanook Kitchen"} {
		if got := Title(in); got != want {
			t.Errorf("Title(%q) = %q, want %q", in, got, want)
		}
	}
}

func resolver(t *testing.T, entries ...Entry) *Resolver {
	t.Helper()
	b, err := LoadBrands()
	if err != nil {
		t.Fatal(err)
	}
	r := &Resolver{Brands: b}
	r.SetDirectory(NewDirectory(entries))
	return r
}

func TestResolveBrandsAndSpecials(t *testing.T) {
	r := resolver(t)
	cases := []struct {
		raw      string
		credit   bool
		name     string
		category string
		kind     string
	}{
		{"BUS/MRT 900000001 SINGAPORE SG", false, "SimplyGo (bus/MRT)", "transport", "purchase"},
		{"PYMT @ AXS -THANK YOU", true, "Card payment", "", "payment"},
		{"DCC FEE", false, "DCC Fee", "fees", "fee"},
		{"fp*Food Panda Singapore SG", false, "foodpanda", "delivery", "purchase"},
		{"SHOPEE SG MP SINGAPORE SG", true, "Shopee", "shopping", "refund"},
		{"AMZNPRIMESG MEMBERSHI SINGAPORE SG", false, "Amazon Prime", "subscriptions", "purchase"},
		{"AMAZON MAR* AMAZON SG SINGAPORE SG", false, "Amazon", "shopping", "purchase"},
		{"NTUC FairPrice App Pay SINGAPORE SG", false, "FairPrice", "groceries", "purchase"},
		{"CS FRESH LENTOR Singapore SG", false, "CS Fresh", "groceries", "purchase"},
		{"WL *STEAM PURCHASE 425-889-9642 WA", false, "Steam", "entertainment", "purchase"},
		{"EIGHT TELECOM SINGAPORE SG", false, "Eight", "bills", "purchase"},
		{"Prudential Singapore SINGAPORE SG", false, "Prudential", "insurance", "purchase"},
		{"SAIZERIYA - POIZ CENTR SINGAPORE SG", false, "Saizeriya", "dining", "purchase"},
		{"SMILE DENTAL STUDIO Singapore SG", false, "Smile Dental Studio", "health", "purchase"}, // keyword
		{"SMP_GAO JI FOOD (S) PT Singapore SG", false, "Gao Ji Food", "dining", "purchase"},      // keyword
	}
	for _, c := range cases {
		_, m, kind := r.Resolve(c.raw, c.credit)
		if m.Name != c.name || m.Category != c.category || kind != c.kind {
			t.Errorf("Resolve(%q) = %q %q %q (%s), want %q %q %q", c.raw, m.Name, m.Category, kind, m.Source, c.name, c.category, c.kind)
		}
	}
	_, m, _ := r.Resolve("Grab* 1a2b3c4d5e6f7a8b Singapore", false)
	if !m.Check {
		t.Error("Grab should be flagged: ride or food?")
	}
	// Steamboat restaurants are not Steam.
	if _, m, _ := r.Resolve("STEAMBOAT PALACE SINGAPORE SG", false); m.Name == "Steam" {
		t.Error("steamboat matched Steam")
	}
}

func TestResolveDirectory(t *testing.T) {
	r := resolver(t,
		Entry{Source: "acra", Ref: "201900001A", Name: "DAILY FUSION CUISINE PTE. LTD.", Category: "dining", Address: "1 TOA PAYOH LORONG 1"},
		Entry{Source: "osm", Ref: "node/1", Name: "Bright Dental Studio", Category: "health"},
		Entry{Source: "osm", Ref: "node/2", Name: "Wen Zhang Lao Mian", Category: "dining", Detail: "restaurant"},
		Entry{Source: "osm", Ref: "node/3", Name: "Singapore Zoo", Category: "entertainment"},
		Entry{Source: "osm", Ref: "node/4", Name: "Steak 99", Branch: "Toa Payoh", Category: "dining", Address: "TOA PAYOH"},
	)
	// Truncated descriptor → full registered name.
	_, m, _ := r.Resolve("SMP*DAILY FUSION CUISI Singapore SG", false)
	if m.Name != "Daily Fusion Cuisine" || m.Category != "dining" || m.Source != "acra" || m.Confidence < AutoConfidence {
		t.Errorf("truncated: %+v", m)
	}
	// Exact OSM name.
	_, m, _ = r.Resolve("SMP*WEN ZHANG LAO MIAN", false)
	if m.Name != "Wen Zhang Lao Mian" || m.Source != "osm" {
		t.Errorf("osm: %+v", m)
	}
	// Similar but different business: keep own name, borrow category.
	_, m, _ = r.Resolve("SMILE DENTAL STUDIO Singapore SG", false)
	if m.Name != "Smile Dental Studio" || m.Category != "health" {
		t.Errorf("similar: %+v", m)
	}
	// Stopwords only never match the directory.
	_, m, _ = r.Resolve("SINGAPORE * SINGAPORE SINGAPORE SG", false)
	if m.Source != "none" {
		t.Errorf("stopwords matched: %+v", m)
	}
	// Your rule beats everything.
	r.Rule = func(key string) (Match, bool) {
		if key == "smp|dailyfusioncuisi" {
			return Match{Name: "Office canteen", Category: "dining"}, true
		}
		return Match{}, false
	}
	_, m, _ = r.Resolve("SMP*DAILY FUSION CUISI Singapore SG", false)
	if m.Name != "Office canteen" || m.Source != "rule" {
		t.Errorf("rule: %+v", m)
	}
}

func TestCoreName(t *testing.T) {
	for in, want := range map[string]string{
		"GAO JI FOOD (S) PTE. LTD.":    "GAO JI FOOD",
		"DAILY FUSION CUISINE PTE LTD": "DAILY FUSION CUISINE",
		"KOUFU PTE LTD":                "KOUFU",
		"ACME SINGAPORE":               "ACME",
	} {
		if got := coreName(in); got != want {
			t.Errorf("coreName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCategoryMaps(t *testing.T) {
	for code, want := range map[string]string{"56111": "dining", "47111": "groceries", "86202": "health", "65120": "insurance", "47910": "shopping", "62011": ""} {
		if got := SSICCategory(code); got != want {
			t.Errorf("SSIC %s = %q, want %q", code, got, want)
		}
	}
	if c, _ := OSMCategory(map[string]string{"amenity": "food_court"}); c != "dining" {
		t.Error(c)
	}
	if c, _ := OSMCategory(map[string]string{"shop": "supermarket"}); c != "groceries" {
		t.Error(c)
	}
}

func TestCitiDescriptors(t *testing.T) {
	r := resolver(t)
	cases := []struct{ raw, name, cat, kind string }{
		{"CCY CONVERSION FEE SGD 10.12", "CCY Conversion Fee", "fees", "fee"},
		{"WWW.YAKUN.COM SINGAPORE SG", "Ya Kun", "dining", "purchase"},
		{"GOOGLE*YOUTUBEPREMIUM SINGAPORE SG", "YouTube Premium", "subscriptions", "purchase"},
		{"CLASSPASS* TOP_UP SINGAPORE SG", "ClassPass", "leisure", "purchase"},
		{"COMFORT/CITYCAB TAXI Singapore SG", "CDG Zig", "rides", "purchase"},
		{"M1 MAXX SINGAPORE SG", "M1", "bills", "purchase"},
		{"FLYSCOOT.0000000MHSW4B SINGAPORE SG", "Scoot", "travel", "purchase"},
		{"GIANT SUPER-AMK 316B Singapore SG", "Giant", "groceries", "purchase"},
		{"PAYMENT - THANK YOU SG", "Card payment", "", "payment"},
	}
	for _, c := range cases {
		_, m, kind := r.Resolve(c.raw, false)
		if m.Name != c.name || m.Category != c.cat || kind != c.kind {
			t.Errorf("%q → %q %q %q", c.raw, m.Name, m.Category, kind)
		}
	}
	if d := Clean("GOLDMOON SUNTEC PTE LT N/A SG"); d.Name != "GOLDMOON" || d.Branch != "SUNTEC" {
		t.Errorf("legal suffix: %+v", d)
	}
	if d := Clean("GlobalE Bandai Spirits SINGAPORE SG"); d.Name != "BANDAI SPIRITS" {
		t.Errorf("globale: %+v", d)
	}
	if d := Clean("aptsg*NINE FRESH PTE L SINGAPORE SG"); d.Name != "NINE FRESH" || d.Prefix != "APTSG" {
		t.Errorf("pte l: %+v", d)
	}
}
