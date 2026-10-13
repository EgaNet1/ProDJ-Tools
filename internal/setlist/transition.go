package setlist

import (
	"fmt"
	"math"

	"prodjtools/internal/camelot"
)

// Grade is how a transition will sound.
type Grade string

const (
	Smooth     Grade = "good"
	Noticeable Grade = "ok"
	Rough      Grade = "rough"
)

// Transition describes mixing one track into the next.
type Transition struct {
	TempoChange float64 `json:"tempoChange"` // percent; positive means faster
	Key         string  `json:"key"`         // the key change, in words
	Grade       Grade   `json:"grade"`
	Advice      string  `json:"advice,omitempty"`
}

// Judge describes mixing a into b the way a DJ thinks about it: how far the
// tempo has to move and whether the keys sit well together.
func Judge(a, b Track) Transition {
	change := tempoChange(a.BPM, b.BPM)
	relation := camelot.Relate(a.Key, b.Key)
	return Transition{
		TempoChange: math.Round(change*10) / 10,
		Key:         relation.String(),
		Grade:       worse(tempoGrade(change), keyGrade(relation)),
		Advice:      advice(change, relation),
	}
}

func tempoChange(from, to float64) float64 {
	if from <= 0 || to <= 0 {
		return 0
	}
	return (to/from - 1) * 100
}

// Up to about 3% a DJ nudges the pitch fader and nobody notices; up to 6% the
// shift is audible but workable; beyond that the mix needs a trick.
func tempoGrade(change float64) Grade {
	switch d := math.Abs(change); {
	case d <= 3:
		return Smooth
	case d <= 6:
		return Noticeable
	}
	return Rough
}

func keyGrade(r camelot.Relation) Grade {
	switch r {
	case camelot.Same, camelot.Adjacent, camelot.Relative:
		return Smooth
	case camelot.Clash:
		return Rough
	}
	return Noticeable
}

func worse(a, b Grade) Grade {
	rank := map[Grade]int{Smooth: 0, Noticeable: 1, Rough: 2}
	if rank[a] >= rank[b] {
		return a
	}
	return b
}

func advice(change float64, r camelot.Relation) string {
	d := math.Abs(change)
	switch {
	case d > 6 && r == camelot.Clash:
		return "Большой скачок темпа и конфликт ключей — лучше переставить трек или склеить резко, на брейке."
	case d > 6:
		return fmt.Sprintf("Темп меняется на %.0f%% — переходите через брейк, эхо-аут или резкую склейку.", d)
	case r == camelot.Clash:
		return "Ключи конфликтуют — сводите на ударных и не накладывайте мелодии друг на друга."
	case d > 3:
		return "Заметная смена темпа — подтяните его заранее или растяните переход."
	case r == camelot.Boost:
		return "Подъём по ключу даёт слышимый прирост энергии — хорошо перед дропом."
	}
	return ""
}
