package auth

import "testing"

func TestNormalizePhone(t *testing.T) {
	good := map[string]string{
		"":                  "",
		"   ":               "",
		"08012345678":       "08012345678",
		"+234 801 234 5678": "+2348012345678",
		"(0801) 234-5678":   "08012345678",
		"0801.234.5678":     "08012345678",
	}
	for in, want := range good {
		got, err := normalizePhone(in)
		if err != nil || got != want {
			t.Errorf("normalizePhone(%q) = %q, %v; want %q", in, got, err, want)
		}
	}

	for _, in := range []string{"12345", "1234567890123456", "call me", "0801x234567", "080+1234567"} {
		if got, err := normalizePhone(in); err == nil {
			t.Errorf("normalizePhone(%q) = %q; want an error", in, got)
		}
	}
}
