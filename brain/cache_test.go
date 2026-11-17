package brain

import (
	"math"
	"testing"
)

// The KV cache must give the same probabilities as the full forward pass.
func TestCacheMatchesForward(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	toks, _ := m.prompt("ru/ok", []Turn{{Q: "какой сегодня день", A: "сегодня {день}"}}, "а завтра?", 40)
	toks = append(toks, m.encode("завтра")...)
	a := m.newActs()
	m.forward(toks, a)
	c := m.newCache()
	V := m.V()
	worst := 0.0
	for i, tok := range toks {
		got := m.push(c, tok)
		want := a.probs[i*V : (i+1)*V]
		for v := range V {
			worst = math.Max(worst, math.Abs(float64(got[v]-want[v])))
		}
	}
	t.Logf("%d positions, worst difference %.2e", len(toks), worst)
	if worst > 1e-5 {
		t.Fatalf("cache disagrees with forward: %.2e", worst)
	}
}
