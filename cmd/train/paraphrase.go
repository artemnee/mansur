package main

import (
	"math/rand/v2"
	"strings"

	"github.com/artemnee/mansur/brain"
)

// intent is a "knowledge" question in one language: full questions, noun phrases that fit into
// frames like "tell me %", and the answers.
type intent struct {
	name             string
	questions, nouns []string
	answers          []string
}

var knowledge = map[string][]intent{
	"ru": {
		{"time", []string{"который час", "сколько времени", "сколько время", "который сейчас час", "сколько на часах", "что на часах", "сколько щас времени"},
			[]string{"время", "текущее время", "который час", "сколько времени"}, []string{"сейчас {время}", "на часах {время}", "{время}!"}},
		{"date", []string{"какое число", "какое сегодня число", "какая дата", "какая сегодня дата", "какое число на календаре"},
			[]string{"дату", "число", "сегодняшнюю дату", "какое число"}, []string{"сегодня {дата}", "{дата}!"}},
		{"weekday", []string{"какой день недели", "какой сегодня день", "что за день", "что сегодня за день недели"},
			[]string{"день недели", "какой сегодня день", "какой день недели"}, []string{"сегодня {день}", "{день}!"}},
		{"name", []string{"как тебя зовут", "как тебя звать", "кто ты", "как твое имя"},
			[]string{"свое имя", "как тебя зовут", "твое имя"}, []string{"я мансур!", "меня зовут мансур"}},
	},
	"en": {
		{"time", []string{"what time is it", "what's the time", "got the time", "what's the time now", "what does the clock say", "time please"},
			[]string{"the time", "the current time", "the time now"}, []string{"it's {time_en}", "the time is {time_en}", "{time_en}!"}},
		{"date", []string{"what's the date", "what date is it", "what's today's date", "date please", "which date is it"},
			[]string{"the date", "today's date", "the current date"}, []string{"today is {date_en}", "it's {date_en}"}},
		{"weekday", []string{"what day is it", "what weekday is it", "which day is it", "what day is today", "which weekday is today"},
			[]string{"the weekday", "what day it is", "today's weekday"}, []string{"today is {day_en}", "it's {day_en}!"}},
		{"name", []string{"what's your name", "who are you", "what are you called", "what is your name"},
			[]string{"your name", "who you are"}, []string{"i'm mansur!", "my name is mansur"}},
	},
}

var frames = map[string]struct{ question, noun []string }{
	"ru": {
		[]string{"%", "а %", "слушай, %", "% скажи", "скажи, %", "не подскажешь, %", "%, не знаешь", "эй, %", "%, мансур"},
		[]string{"подскажи %", "скажи %", "назови %", "знаешь %", "мне нужно %", "хочу узнать %"},
	},
	"en": {
		[]string{"%", "so %", "hey, %", "% please", "um, %", "%, mansur", "quick question, %", "%?"},
		[]string{"tell me %", "do you know %", "can you tell me %", "i need %", "i want to know %", "say %"},
	},
}

// Short follow-ups after "what day is it": the network has to look at the previous exchange.
var followUps = map[string][]struct{ qs, answers []string }{
	"ru": {
		{[]string{"а завтра", "а завтра какой", "а на следующий день", "завтра какой", "а завтра что"}, []string{"завтра {завтра}", "а завтра {завтра}"}},
		{[]string{"а вчера", "а вчера какой был", "а днем раньше", "вчера какой был", "а вчера что"}, []string{"вчера был {вчера}", "а вчера {вчера}"}},
		{[]string{"а послезавтра", "а послезавтра какой", "а спустя два дня", "послезавтра какой"}, []string{"послезавтра {послезавтра}"}},
	},
	"en": {
		{[]string{"and tomorrow", "what about tomorrow", "tomorrow then", "and the next day", "and tomorrow?", "what's tomorrow"}, []string{"tomorrow is {tomorrow_en}"}},
		{[]string{"and yesterday", "what about yesterday", "yesterday then", "and the day before", "what was yesterday"}, []string{"yesterday was {yesterday_en}"}},
		{[]string{"and the day after tomorrow", "what about the day after tomorrow", "in two days", "two days from now"}, []string{"the day after tomorrow is {after_en}"}},
	},
}

const perIntent = 300

// knowledgeSamples combines frames and phrasings for the questions the app answers with real data.
// Questions reserved for the held-out test (brain/testdata) are skipped.
func knowledgeSamples(rng *rand.Rand, reserved map[string]bool) []brain.Sample {
	pick := func(s []string) string { return s[rng.IntN(len(s))] }
	var out []brain.Sample
	for lang, intents := range knowledge {
		st := ruStyle
		weekday := []brain.Turn{{Q: "какой сегодня день", A: "сегодня {день}"}}
		if lang == "en" {
			st = enStyle
			weekday = []brain.Turn{{Q: "what day is it", A: "today is {day_en}"}}
		}
		fr := frames[lang]
		add := func(s brain.Sample) {
			if !reserved[normalizeQ(s.Q)] {
				out = append(out, s)
			}
		}
		for _, in := range intents {
			for range perIntent {
				q := strings.Replace(pick(fr.question), "%", pick(in.questions), 1)
				if rng.IntN(2) == 0 {
					q = strings.Replace(pick(fr.noun), "%", pick(in.nouns), 1)
				}
				s := brain.Sample{Mood: lang + "/" + pick(chatMoods), Q: st.noise(q, rng), A: pick(in.answers)}
				if rng.IntN(3) == 0 { // an unrelated previous exchange must not change the answer
					other := intents[rng.IntN(len(intents))]
					s.History = []brain.Turn{{Q: pick(other.questions), A: pick(other.answers)}}
				}
				add(s)
			}
		}
		for _, f := range followUps[lang] {
			for range perIntent / 2 {
				add(brain.Sample{Mood: lang + "/" + pick(chatMoods), Q: st.noise(pick(f.qs), rng), A: pick(f.answers), History: weekday})
			}
		}
	}
	return out
}

func normalizeQ(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	for _, p := range []string{ruStyle.prefix, enStyle.prefix} {
		q = strings.TrimPrefix(q, p)
	}
	return strings.Trim(q, "!?., ")
}

// Names for "my name is …" examples. The network must not copy the name: it answers with the
// {имя} / {name_en} template, and the app remembers the real name itself.
var sampleNames = []string{"петя", "маша", "саша", "дима", "катя", "никита", "оля", "ваня", "аня", "миша", "лена", "гоша",
	"alex", "sam", "kate", "john", "emma", "max", "lily", "tom", "anna", "mike", "nick", "eva"}

var intro = map[string]struct{ frames, answers []string }{
	"ru": {[]string{"меня зовут %", "зови меня %", "мое имя %", "привет, меня зовут %", "кстати, меня зовут %", "а меня зовут %", "можешь звать меня %"},
		[]string{"приятно познакомиться, {имя}!", "очень приятно, {имя} ♥", "запомнил, {имя}!", "привет, {имя}!"}},
	"en": {[]string{"my name is %", "call me %", "hi, my name is %", "my name's %", "btw my name is %", "you can call me %"},
		[]string{"nice to meet you, {name_en}!", "got it, {name_en}!", "hi, {name_en} ♥", "i'll remember, {name_en}!"}},
}

// introSamples teaches "меня зовут …" / "my name is …" with many different names.
func introSamples(rng *rand.Rand) []brain.Sample {
	pick := func(s []string) string { return s[rng.IntN(len(s))] }
	var out []brain.Sample
	for _, lang := range []string{"ru", "en"} { // fixed order keeps the fixed-seed rng reproducible
		in := intro[lang]
		st := ruStyle
		if lang == "en" {
			st = enStyle
		}
		for range 600 {
			q := strings.Replace(pick(in.frames), "%", pick(sampleNames), 1)
			out = append(out, brain.Sample{Mood: lang + "/" + pick(chatMoods), Q: st.noise(q, rng), A: pick(in.answers)})
		}
	}
	return out
}
