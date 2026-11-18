// Command train builds Mansur's bilingual training set from brain/dialogs.txt (Russian) and
// brain/dialogs_en.txt (English) and trains the mini-GPT.
//
//	go run ./cmd/train -dump training/data   # export tokenized samples for training/train.py (fast, GPU)
//	go run ./cmd/train -check                # print sample replies of the embedded brain/weights.bin
//	go run ./cmd/train                       # train on the CPU in pure Go (slow) and write brain/weights.bin
package main

import (
	"bufio"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"math/rand/v2"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/artemnee/mansur/brain"
)

var chatMoods = []string{"ok", "hungry", "night", "sulk"}

// Characters the human may type even if they never appear in the dialogs.
const extraChars = "абвгдежзийклмнопрстуфхцчшщъыьэюяabcdefghijklmnopqrstuvwxyz0123456789 .,!?-:()'\"♥+*/="

// block is a topic from a dialogs file. follow is the topic after which this question makes sense
// ("and tomorrow?" after "what day is it"): this is how the network learns to use memory.
type block struct {
	topic, mood, follow string
	qs, as              []string
}

func main() {
	epochs := flag.Int("epochs", 6, "")
	width := flag.Int("width", 96, "model width (96 ≈ 470K parameters with 4 layers)")
	layers := flag.Int("layers", 4, "")
	dump := flag.String("dump", "", "write tokenized samples and the vocabulary to this directory and exit")
	checkOnly := flag.Bool("check", false, "print sample replies of brain/weights.bin and exit")
	flag.Parse()
	rng := rand.New(rand.NewPCG(7, 7))

	if *checkOnly {
		data, err := os.ReadFile("brain/weights.bin")
		if err != nil {
			fail(err)
		}
		m, err := brain.Load(data)
		if err != nil {
			fail(err)
		}
		fmt.Printf("weights.bin: %d KB, %d parameters\n", len(data)/1024, m.NumParams())
		check(m, rng)
		return
	}

	samples := bilingualSamples(parse("brain/dialogs.txt"), parse("brain/dialogs_en.txt"), rng, reservedQuestions("brain/testdata/dialog-baseline.tsv"))
	cfg := brain.Config{C: *width, L: *layers, NH: 4, T: 128}
	if *dump != "" {
		if err := dumpSamples(*dump, samples, cfg); err != nil {
			fail(err)
		}
		return
	}
	fmt.Printf("training %d bilingual samples, width %d, epochs %d\n", len(samples), *width, *epochs)

	start := time.Now()
	m := brain.Train(samples, brain.TrainConfig{
		Config: cfg, Epochs: *epochs, Batch: 32, LR: 3e-3,
		Log: func(ep int, loss float64, m *brain.Model) {
			r, _ := m.Reply("ru/ok", nil, "как дела", 0.7, rng)
			fmt.Printf("epoch %3d  loss %.3f  %6s  %q\n", ep, loss, time.Since(start).Round(time.Second), r)
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
	check(q, rng)
}

// dumpSamples writes meta.json (config and vocabulary) and samples.txt, one sequence per line:
// the index the loss starts at, then the tokens.
func dumpSamples(dir string, samples []brain.Sample, cfg brain.Config) error {
	m, seqs := brain.Prepare(samples, cfg, extraChars)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	meta, err := json.MarshalIndent(map[string]any{
		"C": cfg.C, "L": cfg.L, "NH": cfg.NH, "T": cfg.T,
		"moods": m.Moods, "chars": string(m.Chars), "vocab": m.V(),
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		return err
	}
	f, err := os.Create(filepath.Join(dir, "samples.txt"))
	if err != nil {
		return err
	}
	defer f.Close()
	w := bufio.NewWriter(f)
	for _, sq := range seqs {
		w.WriteString(strconv.Itoa(sq.From))
		for _, t := range sq.Toks {
			w.WriteByte(' ')
			w.WriteString(strconv.Itoa(t))
		}
		w.WriteByte('\n')
	}
	if err := w.Flush(); err != nil {
		return err
	}
	fmt.Printf("%d samples, vocabulary %d, %d moods → %s\n", len(seqs), m.V(), len(m.Moods), dir)
	return nil
}

// check prints replies to familiar and unseen prompts in both languages, with confidence.
func check(m *brain.Model, rng *rand.Rand) {
	day := []brain.Turn{{Q: "какой сегодня день", A: "сегодня {день}"}}
	dayEn := []brain.Turn{{Q: "what day is it", A: "today is {day_en}"}}
	for _, t := range []struct {
		mood string
		h    []brain.Turn
		q    string
	}{
		{"ru/ok", nil, "привет"}, {"ru/ok", nil, "как твои дела"}, {"ru/hungry", nil, "как ты"}, {"ru/night", nil, "ты как"},
		{"ru/ok", nil, "как тебя звать"}, {"ru/ok", nil, "мне сегодня грустно"}, {"ru/ok", nil, "че делаешь"},
		{"ru/ok", nil, "какой час"}, {"ru/ok", nil, "что такое луна"}, {"ru/ok", day, "а завтра?"}, {"ru/ok", day, "а вчера"},
		{"ru/ok", nil, "кто президент сша"}, {"ru/ok", nil, "ываыва ывпп"},
		{"en/ok", nil, "hello"}, {"en/ok", nil, "how are you doing today"}, {"en/hungry", nil, "how are you"},
		{"en/ok", nil, "what is your name"}, {"en/ok", nil, "i feel sad today"}, {"en/ok", nil, "what are you doing now"},
		{"en/ok", nil, "what time is it now"}, {"en/ok", nil, "tell me about the moon"}, {"en/ok", dayEn, "and tomorrow?"},
		{"en/ok", nil, "what is 3 plus 4"}, {"en/ok", nil, "who is the president"}, {"en/ok", nil, "asdkj qwpe"},
		{"en/ok", day, "what time is it"}, {"ru/ok", dayEn, "который час"},
		{"ru/hello", nil, ""}, {"en/hello", nil, ""}, {"ru/hungry", nil, ""}, {"en/hungry", nil, ""}, {"en/idle", nil, ""},
	} {
		r1, c1 := m.Reply(t.mood, t.h, t.q, 0.5, rng)
		r2, c2 := m.Reply(t.mood, t.h, t.q, 0.5, rng)
		ctx := ""
		if t.h != nil {
			ctx = t.h[0].Q + " → "
		}
		fmt.Printf("  [%-9s] %-36q → %q (%.2f) | %q (%.2f)\n", t.mood, ctx+t.q, r1, c1, r2, c2)
	}
}

// reservedQuestions reads the held-out test questions, which must never appear in training.
func reservedQuestions(path string) map[string]bool {
	data, err := os.ReadFile(path)
	if err != nil {
		fail(err)
	}
	out := map[string]bool{}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if q, _, _ := strings.Cut(line, "\t"); q != "" { // empty: a phrase Mansur says on his own
			out[normalizeQ(q)] = true
		}
	}
	return out
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
func build(blocks []*block, rng *rand.Rand, st style) []brain.Sample {
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
			conf.qs = append(conf.qs, st.gibberish(rng))
		}
	}
	turn := func(b *block) brain.Turn {
		return brain.Turn{Q: st.noise(b.qs[rng.IntN(len(b.qs))], rng), A: b.as[rng.IntN(len(b.as))]}
	}

	var out []brain.Sample
	for _, b := range blocks {
		if len(b.qs) == 0 {
			for _, a := range b.as {
				for range 8 {
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
				s := brain.Sample{Mood: moods[rng.IntN(len(moods))], Q: st.noise(q, rng), A: b.as[rng.IntN(len(b.as))]}
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

// style holds what makes typos, filler words and gibberish look like a given language.
type style struct {
	letters, prefix string
	before, after   []string // filler words people add around a question
}

var (
	ruStyle = style{"абвгдежзийклмнопрстуфхцчшщыьэюя", "мансур, ",
		[]string{"ну ", "слушай, ", "а ", "так ", "эй, ", "скажи, "}, []string{" вообще", " сегодня", " сейчас", " там", " а", " вот", " ну"}}
	enStyle = style{"abcdefghijklmnopqrstuvwxyz", "mansur, ",
		[]string{"so ", "well, ", "hey, ", "um ", "ok so ", "tell me, "}, []string{" today", " now", " right now", " really", " though", " then", " lol"}}
)

func (st style) gibberish(rng *rand.Rand) string {
	ls := []rune(st.letters)
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

// noise adds human-like typos: a missing, doubled, extra or swapped letter.
func (st style) noise(q string, rng *rand.Rand) string {
	r := []rune(q)
	ls := []rune(st.letters)
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
	if rng.IntN(10) < 3 { // filler words must not change the meaning
		if rng.IntN(2) == 0 {
			s = st.before[rng.IntN(len(st.before))] + s
		} else {
			s += st.after[rng.IntN(len(st.after))]
		}
	}
	switch rng.IntN(4) {
	case 0:
		s += "?"
	case 1:
		s += "!"
	case 2:
		s = st.prefix + s
	}
	return s
}

// noisy adds Russian typos.
func noisy(q string, rng *rand.Rand) string { return ruStyle.noise(q, rng) }
