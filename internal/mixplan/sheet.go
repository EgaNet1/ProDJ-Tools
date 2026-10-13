package mixplan

import (
	"fmt"
	"math"
	"strings"

	"prodjtools/internal/analysis"
)

// Entry is a track as it appears on the cue sheet.
type Entry struct {
	Artist, Title string
	Result        analysis.Result
}

// Clock formats seconds as m:ss.mmm, the way Rekordbox shows positions.
func Clock(seconds float64) string {
	ms := int(math.Round(seconds * 1000))
	return fmt.Sprintf("%d:%02d.%03d", ms/60000, ms/1000%60, ms%1000)
}

// Sheet renders the cue sheet of a set: cue points of every track, then
// every transition step by step.
func Sheet(name string, set []Entry) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	width := len(fmt.Sprint(len(set)))
	num := func(i int) string { return fmt.Sprintf("%0*d", max(2, width), i+1) }

	line("ProDJ Tools — метки сведения: %s", name)
	line("")
	line("Время — от начала файла (мин:сек.мс). Такт 1 — первая сильная доля в файле.")
	line("В Rekordbox включите Quantize: метка встанет точно на долю сетки.")
	line("")
	line("МЕТКИ ТРЕКОВ")
	for i, e := range set {
		g := e.Result.Grid
		line("")
		line("%s. %s — %s", num(i), e.Artist, e.Title)
		if !g.Valid() {
			line("    сетка не найдена")
			continue
		}
		grid := fmt.Sprintf("%.2f BPM · первая доля %s", g.BPM, Clock(g.FirstBeat))
		if g.Jitter > 0 {
			grid += fmt.Sprintf(" · точность сетки ±%.1f мс", g.Jitter)
		}
		line("    %s", grid)
		for _, c := range Cues(e.Result) {
			line("    %s  %-8s %10s   такт %d", c.Slot, c.Name, Clock(c.Time), c.Bar)
		}
	}

	line("")
	line("ПЕРЕХОДЫ")
	for i := 0; i+1 < len(set); i++ {
		a, b := set[i], set[i+1]
		line("")
		t, ok := Plan(a.Result, b.Result)
		line("%s → %s   %s → %s", num(i), num(i+1), a.Title, b.Title)
		if !ok {
			line("    нет сетки у одного из треков — сводите на слух")
			continue
		}
		line("    сведение %d тактов · темп «%s» %+.1f%% → %.2f BPM", t.Bars, b.Title, t.Pitch, a.Result.Grid.BPM)
		line("    1. %10s  такт %-4d в «%s» запустите «%s» с метки A · MIX IN (%s)",
			Clock(t.StartA.Time), t.StartA.Bar, a.Title, b.Title, Clock(t.CueB.Time))
		line("    2. %10s  такт %-4d смена баса — в «%s» это %s, такт %d",
			Clock(t.SwapA.Time), t.SwapA.Bar, b.Title, Clock(t.SwapB.Time), t.SwapB.Bar)
		line("    3. %10s  такт %-4d «%s» полностью выведен",
			Clock(t.EndA.Time), t.EndA.Bar, a.Title)
		if t.Note != "" {
			line("    ! %s", t.Note)
		}
	}
	return b.String()
}
