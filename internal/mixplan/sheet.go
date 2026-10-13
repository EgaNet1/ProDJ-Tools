package mixplan

import (
	"fmt"
	"math"
	"strings"

	"prodjtools/internal/analysis"
	"prodjtools/internal/i18n"
)

// Entry is a track as it appears on the cue sheet.
type Entry struct {
	Artist, Title string
	Result        analysis.Result // in the DJ's meter, see analysis.InMeter
}

// Clock formats seconds as m:ss.mmm, the way Rekordbox shows positions.
func Clock(seconds float64) string {
	ms := int(math.Round(seconds * 1000))
	return fmt.Sprintf("%d:%02d.%03d", ms/60000, ms/1000%60, ms%1000)
}

// Roles lists the cue roles in button order, for legends.
var Roles = []string{"mixIn", "drop", "earlyOut", "break", "drop2", "mixOut"}

// Sheet renders the cue sheet of a set: what the cues mean, the cues of
// every track, then every transition step by step, in both ways when the
// track allows an early one.
func Sheet(name string, set []Entry) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	text := func(key string, args ...any) { line("%s", i18n.T(key, args...)) }
	width := len(fmt.Sprint(len(set)))
	num := func(i int) string { return fmt.Sprintf("%0*d", max(2, width), i+1) }
	bar := func(n int) string { return fmt.Sprintf("%-9s", i18n.T("sheet.bar", n)) }

	text("sheet.title", name)
	line("")
	text("sheet.time_note")
	text("sheet.quantize")
	line("")
	for i, role := range Roles {
		slot := string(rune('A' + i))
		line("  %s  %-10s %s", slot, cueNames[role], i18n.T("cue.about."+role))
	}
	line("")
	text("sheet.cues")
	for i, e := range set {
		g := e.Result.Grid
		line("")
		line("%s. %s — %s", num(i), e.Artist, e.Title)
		if !g.Valid() {
			line("    %s", i18n.T("sheet.no_grid"))
			continue
		}
		grid := i18n.T("sheet.grid", fmt.Sprintf("%.2f", g.BPM), Clock(g.FirstBeat))
		if g.Jitter > 0 {
			grid += " · " + i18n.T("sheet.precision", fmt.Sprintf("%.1f", g.Jitter))
		}
		line("    %s", grid)
		for _, c := range Cues(e.Result) {
			line("    %s  %-10s %10s   %s", c.Slot, c.Name, Clock(c.Time), i18n.T("sheet.bar", c.Bar))
		}
	}

	line("")
	text("sheet.transitions")
	for i := 0; i+1 < len(set); i++ {
		a, b := set[i], set[i+1]
		line("")
		line("%s → %s   %s → %s", num(i), num(i+1), a.Title, b.Title)
		var plans []Transition
		if t, ok := Plan(a.Result, b.Result); ok {
			plans = append(plans, t)
		}
		if t, ok := EarlyPlan(a.Result, b.Result); ok {
			plans = append(plans, t)
		}
		if len(plans) == 0 {
			line("    %s", i18n.T("sheet.no_grid_pair"))
			continue
		}
		for _, t := range plans {
			line("  [%s]", i18n.T("sheet.variant."+t.Kind))
			line("    %s", i18n.T("sheet.blend", t.Bars, b.Title, fmt.Sprintf("%+.1f", t.Pitch), fmt.Sprintf("%.2f", a.Result.Grid.BPM)))
			from := i18n.T("sheet.from_bar", Clock(t.CueB.Time), t.CueB.Bar)
			if t.FromB != "" {
				from = i18n.T("sheet.from_cue", t.FromB, Clock(t.CueB.Time))
			}
			line("    1. %10s  %s %s", Clock(t.StartA.Time), bar(t.StartA.Bar), i18n.T("sheet.step1", a.Title, b.Title, from))
			line("    2. %10s  %s %s", Clock(t.SwapA.Time), bar(t.SwapA.Bar), i18n.T("sheet.step2", b.Title, Clock(t.SwapB.Time), t.SwapB.Bar))
			line("    3. %10s  %s %s", Clock(t.EndA.Time), bar(t.EndA.Bar), i18n.T("sheet.step3", a.Title))
			if t.Note != "" {
				line("    ! %s", t.Note)
			}
		}
	}
	return b.String()
}

// cueNames are the names of the cues by role, as Rekordbox shows them.
var cueNames = map[string]string{
	"mixIn": "MIX IN", "drop": "DROP", "earlyOut": "EARLY OUT",
	"break": "BREAK", "drop2": "DROP 2", "mixOut": "MIX OUT",
}
