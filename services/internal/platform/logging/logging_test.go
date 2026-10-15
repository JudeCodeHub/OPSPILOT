package logging

import "testing"

func TestMaskEmail(t *testing.T) {
	cases := map[string]string{
		"jude@example.com": "j***@example.com",
		"a@b.co":           "a***@b.co",
		"not-an-email":     "***",
		"@example.com":     "***",
	}
	for in, want := range cases {
		if got := MaskEmail(in); got != want {
			t.Errorf("MaskEmail(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMaskCard(t *testing.T) {
	if got := MaskCard("4242 4242 4242 4242"); got != "************4242" {
		t.Errorf("MaskCard = %q", got)
	}
	if got := Last4("4000-0000-0000-0002"); got != "0002" {
		t.Errorf("Last4 = %q", got)
	}
}
