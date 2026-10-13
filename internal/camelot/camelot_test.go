package camelot

import "testing"

func TestFromPitch(t *testing.T) {
	cases := []struct {
		pitch int
		minor bool
		code  string
		name  string
	}{
		{0, false, "8B", "C"},
		{9, true, "8A", "Am"},
		{7, false, "9B", "G"},
		{4, true, "9A", "Em"},
		{1, true, "12A", "C#m"},
		{6, false, "2B", "F#"},
	}
	for _, c := range cases {
		k := FromPitch(c.pitch, c.minor)
		if k.String() != c.code || k.Name() != c.name {
			t.Errorf("FromPitch(%d, %v) = %s %s, want %s %s", c.pitch, c.minor, k, k.Name(), c.code, c.name)
		}
	}
}

func TestRoundTrip(t *testing.T) {
	for pitch := range 12 {
		for _, minor := range []bool{false, true} {
			k := FromPitch(pitch, minor)
			if k.tonic() != pitch {
				t.Errorf("%s: tonic %d, want %d", k, k.tonic(), pitch)
			}
			parsed, err := Parse(k.String())
			if err != nil || parsed != k {
				t.Errorf("Parse(%q) = %v, %v", k.String(), parsed, err)
			}
		}
	}
}

func TestRelate(t *testing.T) {
	cases := []struct {
		from, to string
		want     Relation
	}{
		{"8A", "8A", Same},
		{"8A", "9A", Adjacent},
		{"1A", "12A", Adjacent},
		{"8A", "8B", Relative},
		{"8A", "9B", Diagonal},
		{"8A", "10A", Boost},
		{"8A", "3A", Boost},
		{"8A", "2B", Clash},
		{"", "8A", Unknown},
	}
	for _, c := range cases {
		from, _ := Parse(c.from)
		to, _ := Parse(c.to)
		if got := Relate(from, to); got != c.want {
			t.Errorf("Relate(%s, %s) = %v, want %v", c.from, c.to, got, c.want)
		}
	}
}
