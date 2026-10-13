// Package i18n translates the program. The messages of every language live
// in web/i18n/<code>.json and are shared: the interface fetches them, and the
// program uses them for what it writes itself — errors, the export log and
// the cue sheet. A message missing from a language falls back to English.
//
// Messages may hold numbered placeholders, {0}, {1}…, filled with arguments
// in the same way in Go (T) and in the interface (t in app.js).
package i18n

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync/atomic"

	"prodjtools/web"
)

// Fallback is the language of missing messages and of unknown locales.
const Fallback = "en"

// Language is one of the translations.
type Language struct {
	Code string `json:"code"`
	Name string `json:"name"` // in the language itself
	RTL  bool   `json:"rtl,omitempty"`
}

// codes lists the translations in the order the language menu shows them.
// To add a language, add web/i18n/<code>.json and its code here.
var codes = []string{"en", "es", "pt", "fr", "de", "it", "pl", "ru", "tr", "ar", "hi", "id", "zh", "ja", "ko"}

var rightToLeft = map[string]bool{"ar": true}

var (
	catalogs = map[string]map[string]string{}
	current  atomic.Value // string
)

func init() {
	for _, code := range codes {
		data, err := web.Files.ReadFile("i18n/" + code + ".json")
		if err != nil {
			panic(err)
		}
		var messages map[string]string
		if err := json.Unmarshal(data, &messages); err != nil {
			panic(fmt.Sprintf("i18n/%s.json: %v", code, err))
		}
		catalogs[code] = messages
	}
	current.Store(Fallback)
}

// Languages lists the translations.
func Languages() []Language {
	out := make([]Language, len(codes))
	for i, code := range codes {
		out[i] = Language{Code: code, Name: catalogs[code]["language.name"], RTL: rightToLeft[code]}
	}
	return out
}

// Supported reports whether code is one of the translations.
func Supported(code string) bool {
	_, ok := catalogs[code]
	return ok
}

// RTL reports whether the language is written right to left.
func RTL(code string) bool { return rightToLeft[code] }

// Set makes code the language of T. Unknown codes are ignored.
func Set(code string) {
	if Supported(code) {
		current.Store(code)
	}
}

// Current is the language T translates into.
func Current() string { return current.Load().(string) }

// Match picks the translation that best fits the user's preferred
// languages, given most preferred first as in "ru-RU", "en-US", "en".
func Match(preferred []string) string {
	for _, p := range preferred {
		base, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(p)), "-")
		if Supported(base) {
			return base
		}
	}
	return Fallback
}

// Messages returns every message of a language, with English filling the
// gaps, for the interface.
func Messages(code string) map[string]string {
	out := make(map[string]string, len(catalogs[Fallback]))
	for k, v := range catalogs[Fallback] {
		out[k] = v
	}
	for k, v := range catalogs[code] {
		out[k] = v
	}
	return out
}

// T translates a message into the current language and fills in its
// placeholders. An unknown key comes back as is, which makes it easy to spot.
func T(key string, args ...any) string {
	msg, ok := catalogs[Current()][key]
	if !ok {
		if msg, ok = catalogs[Fallback][key]; !ok {
			return key
		}
	}
	if len(args) == 0 {
		return msg
	}
	pairs := make([]string, 0, 2*len(args))
	for i, arg := range args {
		pairs = append(pairs, fmt.Sprintf("{%d}", i), fmt.Sprint(arg))
	}
	return strings.NewReplacer(pairs...).Replace(msg)
}

// Error is an error whose text is a translated message.
func Error(key string, args ...any) error { return errorString(T(key, args...)) }

type errorString string

func (e errorString) Error() string { return string(e) }
