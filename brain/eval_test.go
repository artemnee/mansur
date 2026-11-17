// Run with go test ./brain -run TestDialogBaseline -v -count=1.
package brain

import (
	"math/rand/v2"
	"os"
	"strings"
	"testing"
)

// Report a reproducible score for the embedded weights without imposing a pass threshold.
func TestDialogBaseline(t *testing.T) {
	m, err := Default()
	if err != nil {
		t.Fatal(err)
	}
	normalize := func(s string) string { return strings.Trim(strings.ToLower(strings.TrimSpace(s)), "!?., ") }
	known := map[string]bool{}
	for _, file := range []string{"dialogs.txt", "dialogs_en.txt"} {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		for line := range strings.SplitSeq(string(data), "\n") {
			if q, ok := strings.CutPrefix(line, "> "); ok {
				known[normalize(q)] = true
			}
		}
	}
	histories := map[string][]Turn{
		"nil":    nil,
		"day":    {{Q: "какой сегодня день", A: "сегодня {день}"}},
		"day_en": {{Q: "what day is it", A: "today is {day_en}"}},
	}
	type example struct {
		q, lang, mood string
		want          []string // any of these markers counts as a hit
		history       []Turn
	}
	var cases []example
	fixture, err := os.ReadFile("testdata/dialog-baseline.tsv")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) < 3 || len(fields) > 5 {
			t.Fatalf("invalid fixture: %q", line)
		}
		c := example{q: fields[0], want: strings.Split(fields[1], "|"), lang: "ru", mood: "ok"}
		if len(fields) >= 4 {
			c.lang = fields[3]
		}
		if len(fields) == 5 {
			c.mood = fields[4]
		}
		h, ok := histories[fields[2]]
		if !ok {
			t.Fatalf("unknown history: %s", fields[2])
		}
		c.history = h
		cases = append(cases, c)
	}
	points, sameLang := 0, 0
	byLang := map[string][2]int{}
	const seeds = 3
	for _, c := range cases {
		if c.q != "" && known[normalize(c.q)] {
			t.Fatalf("training overlap: %q", c.q)
		}
		hits := 0
		for seed := range uint64(seeds) {
			r, _ := m.Reply(c.lang+"/"+c.mood, c.history, c.q, 0.7, rand.New(rand.NewPCG(seed, 1)))
			for _, w := range c.want {
				if strings.Contains(strings.ToLower(r), w) {
					hits++
					break
				}
			}
			if replyLang(r) == c.lang {
				sameLang++
			}
		}
		points += hits
		b := byLang[c.lang]
		byLang[c.lang] = [2]int{b[0] + hits, b[1] + seeds}
		t.Logf("[%s/%s] %q history=%d: %d/%d", c.lang, c.mood, c.q, len(c.history), hits, seeds)
	}
	total := len(cases) * seeds
	for lang, b := range byLang {
		t.Logf("%s: expected marker present %d/%d (%.1f%%)", lang, b[0], b[1], 100*float64(b[0])/float64(b[1]))
	}
	t.Logf("All: expected marker present %d/%d (%.1f%%)", points, total, 100*float64(points)/float64(total))
	t.Logf("Replied in the language of the question: %d/%d (%.1f%%)", sameLang, total, 100*float64(sameLang)/float64(total))
}

// replyLang tells whether a reply is mostly Cyrillic (ru) or Latin (en); template names do not count.
func replyLang(s string) string {
	for _, tpl := range []string{"{время}", "{дата}", "{день}", "{вчера}", "{завтра}", "{послезавтра}", "{сытость}", "{возраст}"} {
		s = strings.ReplaceAll(s, tpl, "ру")
	}
	cyr, lat := 0, 0
	for _, r := range s {
		switch {
		case r >= 'а' && r <= 'я' || r == 'ё':
			cyr++
		case r >= 'a' && r <= 'z':
			lat++
		}
	}
	if lat > cyr {
		return "en"
	}
	return "ru"
}
