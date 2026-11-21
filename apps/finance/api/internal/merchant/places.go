package merchant

import "strings"

// Singapore places as they appear at the end of descriptors, with what to
// look for in a directory entry's address or branch name.
var placeAbbr = map[string][]string{
	"AMK": {"ANG MO KIO"}, "AMKH": {"AMK HUB", "ANG MO KIO"}, "TPY": {"TOA PAYOH"},
	"CCK": {"CHOA CHU KANG"}, "JE": {"JURONG EAST"}, "JW": {"JURONG WEST"}, "JP": {"JURONG POINT"},
	"SK": {"SENGKANG"}, "PGL": {"PUNGGOL"}, "NOV": {"NOVENA"}, "ORC": {"ORCHARD"},
	"VIVO": {"VIVOCITY"}, "PLQ": {"PAYA LEBAR QUARTER", "PAYA LEBAR"}, "CWP": {"CAUSEWAY POINT", "WOODLANDS"},
	"NP": {"NORTHPOINT", "YISHUN"}, "TM": {"TAMPINES MALL", "TAMPINES"}, "MBS": {"MARINA BAY SANDS"},
	"TPS": {"TOA PAYOH"}, "BPP": {"BUKIT PANJANG PLAZA", "BUKIT PANJANG"}, "HBF": {"HARBOURFRONT"},
	"CENTR": {"CENTRE"}, "CTR": {"CENTRE"}, "SQ": {"SQUARE"}, "PT": {"POINT"}, "STN": {"STATION"},
}

var placeWords = set(`ANG MO KIO BEDOK BISHAN BOON LAY BUKIT BATOK MERAH PANJANG TIMAH CHOA CHU KANG
	CLEMENTI GEYLANG HOUGANG JURONG KALLANG MARINE PARADE NOVENA ORCHARD PASIR RIS PUNGGOL QUEENSTOWN
	SEMBAWANG SENGKANG SERANGOON TAMPINES TOA PAYOH WOODLANDS YISHUN TIONG BAHRU HARBOURFRONT BUGIS
	SOMERSET LAVENDER CHINATOWN HOLLAND KATONG SIGLAP THOMSON KHATIB CANBERRA ADMIRALTY MARSILING
	KRANJI SIMEI CHANGI JEWEL ALJUNIED EUNOS KEMBANGAN PIONEER TUAS HILLVIEW NEWTON CALDECOTT LENTOR
	SPRINGLEAF POTONG BALESTIER MACPHERSON UBI BARTLEY SUNTEC VIVOCITY NEX ION JEM IMM POIZ
	NORTHPOINT RAFFLES MARINA FUNAN PLAZA MALL HUB CITY CBD`)

// isPlace reports whether a trailing token is a location rather than part of
// the merchant name.
func isPlace(tok string) bool {
	if _, ok := placeAbbr[tok]; ok {
		return true
	}
	return placeWords[tok] && tok != "PLAZA" && tok != "MALL" && tok != "HUB" && tok != "CITY"
}

// placeTerms expands a branch hint into terms to look for in addresses:
// "VELOCITY@NOV" → VELOCITY, NOVENA; "AMKH" → AMK HUB, ANG MO KIO.
func placeTerms(branch string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(strings.ToUpper(branch), func(r rune) bool {
		return r == ' ' || r == '@' || r == '-' || r == '/' || r == ','
	}) {
		if exp, ok := placeAbbr[f]; ok {
			out = append(out, exp...)
			continue
		}
		if len(f) >= 3 && f != "SINGAPORE" && f != "STORE" {
			out = append(out, f)
		}
	}
	return out
}
