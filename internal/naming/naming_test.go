package naming

import "testing"

func TestNumbers(t *testing.T) {
	if got := Strip("07 - ISOxo - FUN.flac"); got != "ISOxo - FUN.flac" {
		t.Errorf("Strip = %q", got)
	}
	if got := Strip("Bishu - 2MINUTES.flac"); got != "Bishu - 2MINUTES.flac" {
		t.Errorf("Strip touched an unnumbered name: %q", got)
	}
	if got := Number("12 - A - B.mp3"); got != 12 {
		t.Errorf("Number = %d", got)
	}
	if got := File(3, Width(31), "17 - A - B.flac"); got != "03 - A - B.flac" {
		t.Errorf("File = %q", got)
	}
	if got := File(7, Width(120), "A - B.flac"); got != "007 - A - B.flac" {
		t.Errorf("File (3 digits) = %q", got)
	}
}

func TestCleanTitle(t *testing.T) {
	cases := []struct {
		title string
		n     int
		want  string
	}{
		{"05 WHIPLASH", 5, "WHIPLASH"},
		{"99 Problems", 0, "99 Problems"},
		{"99 Problems", 4, "99 Problems"},
		{"2MINUTES", 8, "2MINUTES"},
	}
	for _, c := range cases {
		if got := CleanTitle(c.title, c.n); got != c.want {
			t.Errorf("CleanTitle(%q, %d) = %q, want %q", c.title, c.n, got, c.want)
		}
	}
}
