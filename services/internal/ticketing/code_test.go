package ticketing

import (
	"strings"
	"testing"
)

func TestNewCodeFormat(t *testing.T) {
	for range 1000 {
		code := NewCode()
		if !ValidCode(code) {
			t.Fatalf("invalid code %q", code)
		}
		if strings.ContainsAny(code[4:], "01ILO") {
			t.Fatalf("code %q contains an ambiguous character", code)
		}
	}
}

func TestNewCodeIsRandom(t *testing.T) {
	seen := make(map[string]bool)
	for range 10000 {
		code := NewCode()
		if seen[code] {
			t.Fatalf("duplicate code %q after %d codes", code, len(seen))
		}
		seen[code] = true
	}
}

func TestValidCode(t *testing.T) {
	for _, c := range []string{"OPS-7KQ2-M9XA", "OPS-2222-ZZZZ"} {
		if !ValidCode(c) {
			t.Errorf("ValidCode(%q) = false", c)
		}
	}
	for _, c := range []string{"", "OPS-7KQ2M9XA", "ops-7kq2-m9xa", "OPS-0KQ2-M9XA", "XYZ-7KQ2-M9XA"} {
		if ValidCode(c) {
			t.Errorf("ValidCode(%q) = true", c)
		}
	}
}
