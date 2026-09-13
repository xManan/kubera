package duplicate

import (
	"testing"
)

func TestNormalizeText(t *testing.T) {
	cases := map[string]string{
		"  EATCLUB   BRANDS  ": "eatclub brands",
		"Grocery\tstore\nX":    "grocery store x",
		"":                     "",
		"  a  ":                "a",
	}
	for in, want := range cases {
		if got := NormalizeText(in); got != want {
			t.Errorf("NormalizeText(%q) = %q, want %q", in, got, want)
		}
	}
	// Case folding makes different casings compare equal.
	if NormalizeText("ABC") != NormalizeText("abc") {
		t.Error("case folding should make casings compare equal")
	}
}

func TestNormalizeReference(t *testing.T) {
	if got := NormalizeReference("  ref-001  "); got != "ref-001" {
		t.Errorf("NormalizeReference = %q, want %q", got, "ref-001")
	}
}
