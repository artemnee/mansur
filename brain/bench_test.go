package brain

import (
	"math/rand/v2"
	"testing"
)

func BenchmarkReply(b *testing.B) {
	m, err := Default()
	if err != nil {
		b.Fatal(err)
	}
	rng := rand.New(rand.NewPCG(1, 1))
	h := []Turn{{Q: "какой сегодня день", A: "сегодня {день}"}}
	for b.Loop() {
		m.Reply("ru/ok", h, "как дела", 0.7, rng)
	}
}
