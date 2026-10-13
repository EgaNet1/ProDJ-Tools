package camelot

// Relation says how the key changes when one track is mixed into another.
type Relation int

const (
	Unknown  Relation = iota // one of the keys was not detected
	Same                     // identical key
	Adjacent                 // ±1 on the same ring: a fifth up or down
	Relative                 // same number, other ring: relative major/minor
	Diagonal                 // ±1 and the other ring
	Boost                    // +2 or +7 on the same ring: a deliberate lift
	Clash                    // anything else
)

// Relate classifies the move from one key to another.
func Relate(from, to Key) Relation {
	if !from.Valid() || !to.Valid() {
		return Unknown
	}
	step := (to.Number - from.Number + 12) % 12 // clockwise distance
	if from.Minor == to.Minor {
		switch step {
		case 0:
			return Same
		case 1, 11:
			return Adjacent
		case 2, 7: // two steps up, or one semitone up
			return Boost
		}
		return Clash
	}
	switch step {
	case 0:
		return Relative
	case 1, 11:
		return Diagonal
	}
	return Clash
}

// Cost is how hard the move is on the ear: 0 is seamless, 5 is a clash.
func (r Relation) Cost() float64 {
	switch r {
	case Same:
		return 0
	case Adjacent:
		return 0.8
	case Relative:
		return 1
	case Unknown:
		return 1.5
	case Diagonal:
		return 2
	case Boost:
		return 2.2
	}
	return 5
}

func (r Relation) String() string {
	switch r {
	case Same:
		return "тот же ключ"
	case Adjacent:
		return "соседний ключ"
	case Relative:
		return "параллельный лад"
	case Diagonal:
		return "по диагонали"
	case Boost:
		return "подъём ключа"
	case Clash:
		return "конфликт ключей"
	}
	return "ключ не определён"
}
