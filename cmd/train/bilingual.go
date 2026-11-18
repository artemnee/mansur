package main

import (
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/artemnee/mansur/brain"
)

// Each row represents the same intent or fact in both languages.
// A vertical bar separates interchangeable formulations.
type pair struct{ ruQ, ruA, enQ, enA string }

var core = []pair{
	{"который час|что там на часах|сколько сейчас времени", "сейчас {время}|на часах {время}", "what time is it|got the time|what does the clock say", "it is {time_en}|the time is {time_en}"},
	{"какое число|какая сегодня дата", "сегодня {дата}", "what is today's date|what date is it", "today is {date_en}"},
	{"какой день недели|сегодня какой день", "сегодня {день}", "which day is it|what weekday is it", "today is {day_en}"},
	{"как тебя зовут|назови свое имя|ты кто", "меня зовут мансур|я мансур", "what is your name|who are you|what are you called", "my name is mansur|i am mansur"},
	{"какой день будет завтра|что завтра за день", "завтра {завтра}", "what day is tomorrow|tomorrow is what day", "tomorrow is {tomorrow_en}"},
	{"какой день был вчера|какой день накануне", "вчера {вчера}", "what day was yesterday|what weekday was yesterday", "yesterday was {yesterday_en}"},
	{"какой день послезавтра|что за день через двое суток", "послезавтра {послезавтра}", "what day is after tomorrow|which day in two days", "it will be {after_en}"},
	{"привет|здарова|ку", "привет!|мур!", "hi|hello|hey there", "hi there!|hello friend!"},
	{"че делаешь|чем занят|чо делаешь", "охраняю экран|думаю о ягодках", "what are you doing|whatcha doing|what are you up to", "guarding the screen|thinking about berries"},
	{"спасибо|пасиб", "пожалуйста!", "thanks|thank you", "you're welcome!"},
	{"мне грустно|я расстроен", "я рядом ♥", "i feel sad|i am upset", "i am here for you"},
	{"что такое луна|луна это что", "луна спутник земли", "what is the moon|tell me about the moon", "earth's natural satellite"},
	{"какого цвета трава|цвет травы", "трава зеленая", "what color is grass|grass is what color", "grass is green"},
	{"что такое солнце|расскажи про солнце", "солнце это звезда", "what is the sun|tell me about the sun", "the sun is a star"},
	{"сколько лап у кота|у кошки сколько лап", "четыре лапы", "how many paws does a cat have|count a cat's paws", "four paws"},
	{"кто король квамбрии|где город флубия", "я не знаю", "who rules quambria|where is flubia", "i don't know"},
}

// factsPerLang is how many samples the shared fact table adds per language. It stays small
// next to the dialogs so a handful of facts does not drown out small talk.
const factsPerLang = 3000

// bilingualSamples builds the training set from both dialog files, the shared fact table and
// conversations that switch topic or language. Every mood is prefixed with its language ("ru/ok").
func bilingualSamples(ru, en []*block, rng *rand.Rand, reserved map[string]bool) []brain.Sample {
	expandQuestions(ru)
	tag := func(lang string, ss []brain.Sample) []brain.Sample {
		for i := range ss {
			ss[i].Mood = lang + "/" + ss[i].Mood
		}
		return ss
	}
	ruS := tag("ru", append(build(ru, rng, ruStyle), calendarDialogs(ru, rng)...))
	enS := tag("en", build(en, rng, enStyle))

	rows := append([]pair(nil), core...)
	for a := 1; a <= 10; a++ {
		for b := 1; b <= 10; b++ {
			rows = append(rows, pair{fmt.Sprintf("сколько будет %d плюс %d|сложи %d и %d", a, b, a, b), fmt.Sprint(a + b), fmt.Sprintf("what is %d plus %d|add %d and %d", a, b, a, b), fmt.Sprint(a + b)})
		}
	}
	for _, f := range [][4]string{{"франции", "париж", "france", "paris"}, {"японии", "токио", "japan", "tokyo"}, {"италии", "рим", "italy", "rome"}, {"германии", "берлин", "germany", "berlin"}, {"канады", "оттава", "canada", "ottawa"}, {"испании", "мадрид", "spain", "madrid"}} {
		rows = append(rows, pair{"столица " + f[0] + "|какая столица " + f[0], f[1], "capital of " + f[2] + "|what is the capital of " + f[2], f[3]})
	}
	pick := func(s string) string { parts := strings.Split(s, "|"); return parts[rng.IntN(len(parts))] }
	for _, lang := range []string{"ru", "en"} {
		st := ruStyle
		if lang == "en" {
			st = enStyle
		}
		for range factsPerLang {
			row := rows[rng.IntN(len(rows))]
			q, a := pick(row.ruQ), pick(row.ruA)
			if lang == "en" {
				q, a = pick(row.enQ), pick(row.enA)
			}
			s := brain.Sample{Mood: lang + "/" + chatMoods[rng.IntN(len(chatMoods))], Q: st.noise(q, rng), A: a}
			if lang == "ru" {
				ruS = append(ruS, s)
			} else {
				enS = append(enS, s)
			}
		}
	}

	// Switching languages mid-conversation: the reply follows the language of the new question,
	// whatever the previous exchange was in.
	switchHistory := func(dst, other []brain.Sample) {
		for i := range dst {
			if dst[i].Q == "" || rng.IntN(100) >= 15 {
				continue
			}
			o := other[rng.IntN(len(other))]
			for o.Q == "" {
				o = other[rng.IntN(len(other))]
			}
			dst[i].History = []brain.Turn{{Q: o.Q, A: o.A}}
		}
	}
	switchHistory(ruS, enS)
	switchHistory(enS, ruS)

	out := append(append(ruS, enS...), knowledgeSamples(rng, reserved)...)
	out = dropReserved(append(out, introSamples(rng)...), reserved)
	rng.Shuffle(len(out), func(i, j int) { out[i], out[j] = out[j], out[i] })
	return out
}

// dropReserved removes samples that ask a held-out test question, in the question or in the
// history. Typos and filler words can produce one by chance, and the test must stay unseen.
func dropReserved(samples []brain.Sample, reserved map[string]bool) []brain.Sample {
	out := samples[:0]
	for _, s := range samples {
		ok := !reserved[normalizeQ(s.Q)]
		for _, h := range s.History {
			ok = ok && !reserved[normalizeQ(h.Q)]
		}
		if ok {
			out = append(out, s)
		}
	}
	return out
}
