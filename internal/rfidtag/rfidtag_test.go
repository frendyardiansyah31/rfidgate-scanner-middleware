package rfidtag

import "testing"

func TestNormalize(t *testing.T) {
	tests := []struct{ in, want string }{
		{"SN0038029C1301943003F6F46A", "0038029C1301943003F6F46A"}, // prefixed
		{"0038029C1301943003F6F46A", "0038029C1301943003F6F46A"},   // already plain
		{"993388221", "993388221"},                                 // barcode-like, no SN
		{"SNSN123", "123"},                                         // every SN removed, like replace()
		{"AB12SNCD", "AB12CD"},                                     // SN in the middle, same as legacy SQL
		{"SN", "SN"},                                               // would be empty: keep original
		{"", ""},
	}
	for _, tc := range tests {
		if got := Normalize(tc.in); got != tc.want {
			t.Errorf("Normalize(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

// The property the whole design relies on: a tag with and without the prefix
// land on the same key.
func TestNormalize_PrefixedAndPlainMatch(t *testing.T) {
	plain := "0038029C1301943003F6F46A"
	if Normalize("SN"+plain) != Normalize(plain) {
		t.Fatal("prefixed and plain tag must normalise to the same key")
	}
}
