package words

import "testing"

// DM-06.4: listed words are refused, ordinary names and labels are not.
func TestBlocked(t *testing.T) {
	for _, s := range []string{rot13("i1nten"), "Big " + rot13("CBEA") + " fan", rot13("p4fvab") + "-bonus", rot13("a v t t r e") + " lover", rot13("snttbgf") + "!"} {
		if !Blocked(s) {
			t.Errorf("Blocked(%q) = false", s)
		}
	}
	for _, s := range []string{"Sunny Otter", "Priya Raman", "Scunthorpe", "specialist", "grape", "spice", "class", "first-snow-sale", "k8s", "tech-debt", "Escorted tours"} {
		if Blocked(s) {
			t.Errorf("Blocked(%q) = true", s)
		}
	}
	if len(whole) < 20 || len(inside) < 3 {
		t.Errorf("the list has %d entries, %d inside words", len(whole), len(inside))
	}
}
