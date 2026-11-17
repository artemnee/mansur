package brain

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
)

// TestParity checks that weights exported by training/train.py give the same next-token
// probabilities in Go as in PyTorch. Run after training:
//
//	MANSUR_PARITY=../training/out go test ./brain -run TestParity -v
func TestParity(t *testing.T) {
	dir := os.Getenv("MANSUR_PARITY")
	if dir == "" {
		t.Skip("set MANSUR_PARITY to the training output directory")
	}
	data, err := os.ReadFile(filepath.Join(dir, "weights.bin"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := Load(data)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "parity.json"))
	if err != nil {
		t.Fatal(err)
	}
	var cases []struct {
		Toks  []int     `json:"toks"`
		Probs []float64 `json:"probs"`
	}
	if err := json.Unmarshal(raw, &cases); err != nil {
		t.Fatal(err)
	}
	V := m.V()
	worst := 0.0
	for i, c := range cases {
		if len(c.Probs) != V {
			t.Fatalf("case %d: vocabulary %d in PyTorch, %d in Go", i, len(c.Probs), V)
		}
		a := m.newActs()
		m.forward(c.Toks, a)
		got := a.probs[(len(c.Toks)-1)*V : len(c.Toks)*V]
		for v := range V {
			worst = math.Max(worst, math.Abs(float64(got[v])-c.Probs[v]))
		}
	}
	t.Logf("%d sequences, worst probability difference %.2e", len(cases), worst)
	if worst > 1e-3 {
		t.Fatalf("Go and PyTorch disagree: %.2e", worst)
	}
}
