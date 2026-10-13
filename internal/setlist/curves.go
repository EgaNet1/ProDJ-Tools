package setlist

import "math"

// Curve is the intended shape of a set. Energy says how intense the set
// should feel at each moment, from 0 (calm) to 1 (peak); Tempo says where in
// the folder's tempo range it should be. x runs from 0 (first track) to 1
// (last). Tempo shapes are monotonic: energy may breathe, but DJs rarely let
// the tempo go back and forth.
type Curve struct {
	ID          string
	Name        string
	Description string
	energy      func(x float64) float64
	tempo       func(x float64) float64
}

// Curves are the shapes offered in the interface.
var Curves = []Curve{
	{
		ID: "rise", Name: "Разгон",
		Description: "Спокойное начало, рост к пику ближе к концу и короткий выдох в финале.",
		energy: func(x float64) float64 {
			if x < 0.85 {
				return 0.2 + 0.8*smoothstep(x/0.85)
			}
			return 1 - 0.15*(x-0.85)/0.15
		},
		tempo: func(x float64) float64 { return x },
	},
	{
		ID: "arc", Name: "Арка",
		Description: "Пик в середине, мягкие вход и выход. Для сета в середине вечера.",
		energy:      func(x float64) float64 { return 0.25 + 0.75*math.Sin(math.Pi*x) },
		tempo:       func(x float64) float64 { return x / 0.6 },
	},
	{
		ID: "waves", Name: "Волны",
		Description: "Подъёмы и передышки на фоне общего роста — танцпол успевает выдохнуть.",
		energy:      func(x float64) float64 { return 0.3 + 0.45*x + 0.2*math.Sin(2*math.Pi*2.5*x-math.Pi/2) },
		tempo:       func(x float64) float64 { return x },
	},
	{
		ID: "plateau", Name: "Плато",
		Description: "Быстрый выход на высокую энергию и ровный драйв до конца.",
		energy:      func(x float64) float64 { return 0.9 - 0.6*math.Exp(-6*x) },
		tempo:       func(x float64) float64 { return x / 0.3 },
	},
	{
		ID: "cooldown", Name: "Остывание",
		Description: "От пика к спокойному финалу — чтобы закрыть вечер.",
		energy:      func(x float64) float64 { return 1 - 0.75*x },
		tempo:       func(x float64) float64 { return 1 - x },
	},
}

// CurveByID finds a curve, falling back to the first one.
func CurveByID(id string) Curve {
	for _, c := range Curves {
		if c.ID == id {
			return c
		}
	}
	return Curves[0]
}

// Targets samples the energy shape at n evenly spaced positions.
func (c Curve) Targets(n int) []float64 { return sample(c.energy, n) }

// TempoTargets samples the tempo shape at n evenly spaced positions.
func (c Curve) TempoTargets(n int) []float64 { return sample(c.tempo, n) }

func sample(shape func(float64) float64, n int) []float64 {
	out := make([]float64, n)
	for i := range out {
		x := 0.5
		if n > 1 {
			x = float64(i) / float64(n-1)
		}
		out[i] = math.Round(clamp01(shape(x))*100) / 100
	}
	return out
}

func smoothstep(t float64) float64 { return t * t * (3 - 2*t) }
