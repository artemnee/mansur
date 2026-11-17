// Command train trains Mansur's mini-GPT on brain/dialogs.txt and saves brain/weights.bin.
//
//	go run ./cmd/train
package main

import (
	"bufio"
	"bytes"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/artemnee/mansur/brain"
)

var chatMoods = []string{"ok", "hungry", "night"}

// Characters the human may type even if they never appear in the dialogs.
const extraChars = "абвгдежзийклмнопрстуфхцчшщъыьэюяabcdefghijklmnopqrstuvwxyz0123456789 .,!?-:()'\"♥+*/="

// block is a topic from dialogs.txt. follow is the topic after which this question makes sense
// ("and tomorrow?" after "what day is it"): this is how the network learns to use memory.
type block struct {
	topic, mood, follow string
	qs, as              []string
}

func main() {
	epochs := flag.Int("epochs", 40, "")
	width := flag.Int("width", 64, "model width (64 ≈ 200K parameters with 4 layers)")
	layers := flag.Int("layers", 4, "")
	flag.Parse()

	blocks := parse("brain/dialogs.txt")
	rng := rand.New(rand.NewPCG(7, 7))
	samples := build(blocks, rng)

	start := time.Now()
	test := []struct{ mood, q string }{{"ok", "привет"}, {"ok", "как дела"}, {"hungry", "как дела"}, {"ok", "кто ты"}, {"ok", "мне грустно"}, {"pet", ""}}
	m := brain.Train(samples, brain.TrainConfig{
		Config: brain.Config{C: *width, L: *layers, NH: 4, T: 128},
		Epochs: *epochs, Batch: 32, LR: 3e-3,
		Log: func(ep int, loss float64, m *brain.Model) {
			if ep == 1 {
				fmt.Printf("samples: %d, parameters: %d\n", len(samples), m.NumParams())
			}
			t := test[rng.IntN(len(test))]
			r, _ := m.Reply(t.mood, nil, t.q, 0.7, rng)
			fmt.Printf("epoch %3d  loss %.3f  %6s  [%s] %q → %q\n", ep, loss, time.Since(start).Round(time.Second), t.mood, t.q, r)
		},
	}, extraChars)

	var buf bytes.Buffer
	if err := m.Save(&buf); err != nil {
		fail(err)
	}
	if err := os.WriteFile("brain/weights.bin", buf.Bytes(), 0o644); err != nil {
		fail(err)
	}
	q, err := brain.Load(buf.Bytes())
	if err != nil {
		fail(err)
	}
	fmt.Printf("\nweights.bin: %d KB. Check (int8), including phrases not seen in training; confidence in parentheses:\n", buf.Len()/1024)
	day := []brain.Turn{{Q: "какой сегодня день", A: "сегодня {день}"}}
	for _, t := range []struct {
		mood string
		h    []brain.Turn
		q    string
	}{
		{"ok", nil, "привет"}, {"ok", nil, "приветствую тебя"}, {"ok", nil, "как твои дела"}, {"hungry", nil, "как ты"},
		{"night", nil, "ты как"}, {"ok", nil, "как тебя звать"}, {"ok", nil, "мне сегодня грустно"}, {"ok", nil, "ты такой милый"},
		{"ok", nil, "что умеешь"}, {"ok", nil, "пока"}, {"ok", nil, "какой час"}, {"ok", nil, "че делаешь"}, {"ok", nil, "че как"}, {"ok", []brain.Turn{{Q: "какой час", A: "ой, я такого не знаю"}}, "что делаешь"}, {"ok", nil, "сколько сейчас времени"}, {"ok", nil, "что такое луна"},
		{"ok", nil, "какой вчера былб день"}, {"ok", nil, "какой завтра день"}, {"ok", day, "а завтра?"}, {"ok", day, "а вчера"},
		{"ok", []brain.Turn{{Q: "что такое солнце", A: "это большая теплая звезда"}}, "а луна?"},
		{"ok", []brain.Turn{{Q: "расскажи шутку", A: "хи-хи"}}, "еще"},
		{"ok", nil, "ываыва ывпп"}, {"ok", nil, "кто президент сша"}, {"ok", nil, "как приготовить борщ"},
		{"hello", nil, ""}, {"idle", nil, ""},
	} {
		r1, c1 := q.Reply(t.mood, t.h, t.q, 0.5, rng)
		r2, c2 := q.Reply(t.mood, t.h, t.q, 0.5, rng)
		ctx := ""
		if t.h != nil {
			ctx = t.h[0].Q + " → "
		}
		fmt.Printf("  [%-6s] %-40q → %q (%.2f) | %q (%.2f)\n", t.mood, ctx+t.q, r1, c1, r2, c2)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, err)
	os.Exit(1)
}

func parse(path string) []*block {
	f, err := os.Open(path)
	if err != nil {
		fail(err)
	}
	defer f.Close()
	var blocks []*block
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		switch {
		case strings.HasPrefix(line, "@"):
			// A repeated block of the same topic adds more phrases to it.
			nb := &block{}
			for i, f := range strings.Fields(line[1:]) {
				switch {
				case i == 0:
					nb.topic = f
				case strings.HasPrefix(f, "~"):
					nb.follow = f[1:]
				default:
					nb.mood = f
				}
			}
			i := slices.IndexFunc(blocks, func(b *block) bool {
				return b.topic == nb.topic && b.mood == nb.mood && b.follow == nb.follow
			})
			if i < 0 {
				blocks = append(blocks, nb)
				i = len(blocks) - 1
			}
			blocks = append(blocks[:i], append(blocks[i+1:], blocks[i])...)
		case strings.HasPrefix(line, "> "):
			blocks[len(blocks)-1].qs = append(blocks[len(blocks)-1].qs, line[2:])
		case strings.HasPrefix(line, "< "):
			blocks[len(blocks)-1].as = append(blocks[len(blocks)-1].as, line[2:])
		}
	}
	return blocks
}

// build multiplies the dialogs: every human phrase with different answers, moods,
// typos and a past exchange in memory.
func build(blocks []*block, rng *rand.Rand) []brain.Sample {
	special := map[string][]string{} // topic → moods that have their own block
	byTopic := map[string]*block{}
	var chat []*block // plain topics with questions, used as the past exchange
	for _, b := range blocks {
		if b.mood != "" && len(b.qs) > 0 {
			special[b.topic] = append(special[b.topic], b.mood)
		}
		if b.mood == "" && b.follow == "" && len(b.qs) > 0 {
			byTopic[b.topic] = b
			chat = append(chat, b)
		}
	}
	// Gibberish teaches too: Mansur honestly answers it with "didn't get it".
	if conf, ok := byTopic["confused"]; ok {
		for range 150 {
			conf.qs = append(conf.qs, gibberish(rng))
		}
	}
	turn := func(b *block) brain.Turn {
		return brain.Turn{Q: noisy(b.qs[rng.IntN(len(b.qs))], rng), A: b.as[rng.IntN(len(b.as))]}
	}

	var out []brain.Sample
	for _, b := range blocks {
		if len(b.qs) == 0 {
			for _, a := range b.as {
				for range 3 {
					out = append(out, brain.Sample{Mood: b.mood, A: a})
				}
			}
			continue
		}
		moods := []string{b.mood}
		if b.mood == "" {
			moods = nil
			for _, md := range chatMoods {
				if !slices.Contains(special[b.topic], md) {
					moods = append(moods, md)
				}
			}
		}
		for _, q := range b.qs {
			for range 10 {
				s := brain.Sample{Mood: moods[rng.IntN(len(moods))], Q: noisy(q, rng), A: b.as[rng.IntN(len(b.as))]}
				switch prev := byTopic[b.follow]; {
				case prev != nil:
					s.History = []brain.Turn{turn(prev)}
				case rng.IntN(2) == 0: // an unrelated past exchange teaches it not to mix that up with the question
					s.History = []brain.Turn{turn(chat[rng.IntN(len(chat))])}
				}
				out = append(out, s)
			}
		}
	}
	return out
}

func gibberish(rng *rand.Rand) string {
	const letters = "абвгдежзийклмнопрстуфхцчшщыьэюяasdfghjkl"
	ls := []rune(letters)
	var b strings.Builder
	for w := range 1 + rng.IntN(3) {
		if w > 0 {
			b.WriteByte(' ')
		}
		for range 2 + rng.IntN(6) {
			b.WriteRune(ls[rng.IntN(len(ls))])
		}
	}
	return b.String()
}

// noisy adds human-like typos: a missing, doubled, extra or swapped letter.
func noisy(q string, rng *rand.Rand) string {
	r := []rune(q)
	const letters = "абвгдежзийклмнопрстуфхцчшщыьэюя"
	ls := []rune(letters)
	for range 2 {
		if rng.IntN(10) >= 3 || len(r) < 4 {
			continue
		}
		i := rng.IntN(len(r))
		switch rng.IntN(4) {
		case 0:
			r = append(r[:i], r[i+1:]...)
		case 1:
			r = slices.Insert(r, i, r[i])
		case 2:
			r = slices.Insert(r, i, ls[rng.IntN(len(ls))])
		case 3:
			if i+1 < len(r) {
				r[i], r[i+1] = r[i+1], r[i]
			}
		}
	}
	s := string(r)
	switch rng.IntN(4) {
	case 0:
		s += "?"
	case 1:
		s += "!"
	case 2:
		s = "мансур, " + s
	}
	return s
}
