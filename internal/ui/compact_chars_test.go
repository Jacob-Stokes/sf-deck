package ui

import "testing"

// TestCompactChars covers the Apex SIZE formatter. The source figure is
// ApexClass.LengthWithoutComments — a CHARACTER count, not a line count;
// the old "LINES" label made classes look 30-50x too big.
func TestCompactChars(t *testing.T) {
	cases := map[int]string{
		0:       "0",
		847:     "847",
		999:     "999",
		1000:    "1.0K",
		3200:    "3.2K",
		992501:  "992.5K", // the real MetadataService figure — ~990k CHARS, not lines
		1400000: "1.4M",
	}
	for in, want := range cases {
		if got := compactChars(in); got != want {
			t.Errorf("compactChars(%d) = %q, want %q", in, got, want)
		}
	}
}
