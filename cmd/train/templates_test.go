package main

import (
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

func TestTrainingQuestionsExcludeBaseline(t *testing.T) {
	normalize := func(q string) string {
		q = strings.TrimPrefix(strings.ToLower(q), "мансур, ")
		return strings.Trim(q, "!?., ")
	}
	data, err := os.ReadFile("../../brain/testdata/dialog-baseline.tsv")
	if err != nil {
		t.Fatal(err)
	}
	reserved := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if q, _, _ := strings.Cut(line, "\t"); q != "" {
			reserved[normalize(q)] = true
		}
	}
	rng := rand.New(rand.NewPCG(7, 7))
	blocks := parse("../../brain/dialogs.txt")
	expandQuestions(blocks)
	reservedQ := map[string]bool{}
	for q := range reserved {
		reservedQ[normalizeQ(q)] = true
	}
	samples := dropReserved(build(blocks, rng, ruStyle), reservedQ)
	dialogs := dropReserved(calendarDialogs(blocks, rng), reservedQ)
	t.Logf("%d samples including %d targeted exchanges", len(samples)+len(dialogs), len(dialogs))
	for _, s := range append(samples, dialogs...) {
		if reserved[normalize(s.Q)] {
			t.Errorf("held-out question in training: %q", s.Q)
		}
		for _, h := range s.History {
			if reserved[normalize(h.Q)] {
				t.Errorf("held-out history question: %q", h.Q)
			}
		}
	}
}

// The whole bilingual training set must not contain the held-out test questions.
func TestBilingualSamplesExcludeBaseline(t *testing.T) {
	reserved := reservedQuestions("../../brain/testdata/dialog-baseline.tsv")
	rng := rand.New(rand.NewPCG(7, 7))
	samples := bilingualSamples(parse("../../brain/dialogs.txt"), parse("../../brain/dialogs_en.txt"), rng, reserved)
	langs := map[string]int{}
	for _, s := range samples {
		langs[s.Mood[:2]]++
		if reserved[normalizeQ(s.Q)] {
			t.Errorf("held-out question in training: %q", s.Q)
		}
		for _, h := range s.History {
			if reserved[normalizeQ(h.Q)] {
				t.Errorf("held-out history question: %q", h.Q)
			}
		}
	}
	t.Logf("%d samples: %v", len(samples), langs)
}
