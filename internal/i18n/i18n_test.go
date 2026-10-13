package i18n

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var placeholder = regexp.MustCompile(`\{\d+\}`)

func placeholders(msg string) []string {
	found := placeholder.FindAllString(msg, -1)
	slices.Sort(found)
	return slices.Compact(found)
}

// Every translation has exactly the English messages, with the same
// placeholders, so no text silently falls back to English.
func TestTranslationsComplete(t *testing.T) {
	english := catalogs[Fallback]
	for _, code := range codes {
		if code == Fallback {
			continue
		}
		messages := catalogs[code]
		for key, msg := range english {
			got, ok := messages[key]
			switch {
			case !ok:
				t.Errorf("%s: missing %q", code, key)
			case strings.TrimSpace(got) == "":
				t.Errorf("%s: empty %q", code, key)
			case !slices.Equal(placeholders(got), placeholders(msg)):
				t.Errorf("%s: %q has placeholders %v, English has %v", code, key, placeholders(got), placeholders(msg))
			}
		}
		for key := range messages {
			if _, ok := english[key]; !ok {
				t.Errorf("%s: %q is not an English message", code, key)
			}
		}
	}
}

// Every message the code asks for exists.
func TestKeysInCodeExist(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := map[string]*regexp.Regexp{
		"web/index.html": regexp.MustCompile(`data-i18n(?:-html|-title|-label|-placeholder)?="([^"]+)"`),
		"web/app.js":     regexp.MustCompile(`'([a-z]+(?:\.[a-z0-9_]+)+)'`),
		"*.go":           regexp.MustCompile(`i18n\.(?:T|Error)\("([^"]+)"`),
	}
	var keys []string
	for pattern, re := range patterns {
		var files []string
		if pattern == "*.go" {
			filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
				if d.IsDir() && strings.HasPrefix(d.Name(), ".") && path != root {
					return filepath.SkipDir
				}
				if strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
					files = append(files, path)
				}
				return nil
			})
		} else {
			files = []string{filepath.Join(root, pattern)}
		}
		for _, file := range files {
			data, err := os.ReadFile(file)
			if err != nil {
				t.Fatal(err)
			}
			for _, m := range re.FindAllStringSubmatch(string(data), -1) {
				keys = append(keys, m[1])
			}
		}
	}
	// Curve names are looked up by ID.
	curves, err := os.ReadFile(filepath.Join(root, "internal", "setlist", "curves.go"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range regexp.MustCompile(`ID: "(\w+)"`).FindAllStringSubmatch(string(curves), -1) {
		keys = append(keys, "curve."+m[1]+".name", "curve."+m[1]+".description")
	}

	if len(keys) < 100 {
		t.Fatalf("found only %d keys; did the patterns stop matching?", len(keys))
	}
	for _, key := range keys {
		if strings.HasSuffix(key, ".") {
			continue // a prefix completed at run time, like "curve."+id
		}
		if _, ok := catalogs[Fallback][key]; !ok {
			t.Errorf("%q is used but not in en.json", key)
		}
	}
}

func TestMatch(t *testing.T) {
	cases := []struct {
		preferred []string
		want      string
	}{
		{[]string{"ru-RU", "en"}, "ru"},
		{[]string{"zh-CN"}, "zh"},
		{[]string{"nl-NL", "de-DE"}, "de"},
		{[]string{"nl"}, Fallback},
		{[]string{""}, Fallback},
	}
	for _, c := range cases {
		if got := Match(c.preferred); got != c.want {
			t.Errorf("Match(%v) = %s, want %s", c.preferred, got, c.want)
		}
	}
}

func TestT(t *testing.T) {
	Set("en")
	if got := T("scan.count", 3, 10); got != "3 of 10" {
		t.Errorf("T = %q", got)
	}
	if got := T("no.such.key"); got != "no.such.key" {
		t.Errorf("unknown key = %q", got)
	}
	Set("ru")
	defer Set(Fallback)
	if got := T("tab.set"); got != "Сет" {
		t.Errorf("ru tab.set = %q", got)
	}
}
