package brain

import (
	"math"
	"math/rand/v2"
	"testing"
)

func TestGradients(t *testing.T) {
	m := newModel(Config{C: 8, L: 2, NH: 2, T: 10}, []string{"a", "b"}, []rune("абвгд"))
	rng := rand.New(rand.NewPCG(3, 4))
	for _, p := range m.params() {
		for i := range p {
			p[i] = float32(rng.NormFloat64() * 0.3)
		}
	}
	toks := []int{0, 4, 5, 2, 6, 7, 8, 3}
	from := 3
	a, g := m.newActs(), m.newGrads()
	m.backprop(toks, from, a, g)
	lossAt := func() float64 {
		l, _ := m.backprop(toks, from, m.newActs(), m.newGrads())
		return l
	}
	gp := g.m.params()
	worst := 0.0
	for pi, p := range m.params() {
		for k := 0; k < 3; k++ {
			i := rng.IntN(len(p))
			old := p[i]
			const h = 1e-2
			p[i] = old + h
			lp := lossAt()
			p[i] = old - h
			lm := lossAt()
			p[i] = old
			num := (lp - lm) / (2 * h)
			an := float64(gp[pi][i])
			rel := math.Abs(num-an) / math.Max(1e-3, math.Abs(num)+math.Abs(an))
			worst = math.Max(worst, rel)
			if rel > 0.05 {
				t.Errorf("tensor %d[%d]: numeric %.5f, analytic %.5f", pi, i, num, an)
			}
		}
	}
	t.Logf("worst relative error: %.4f", worst)
}
