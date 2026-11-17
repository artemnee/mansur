package main

import (
	"math/rand/v2"
	"strings"

	"github.com/artemnee/mansur/brain"
)

// Add phrasings to the same topics used by the hand-written dialogs.
func expandQuestions(blocks []*block) {
	phrases := map[string][]string{
		"time":    {"сколько сейчас на часах", "который теперь час", "сколько там времени", "сколько щас на часах"},
		"date":    {"какая у нас дата", "какое сейчас число", "какое число на календаре"},
		"weekday": {"какой нынче день недели", "что за день недели сегодня", "какой день недели у нас"},
		"name":    {"как звать тебя", "как тебя зовут вообще", "какое у тебя имя"},
	}
	for _, b := range blocks {
		if b.mood != "" || b.follow != "" {
			continue
		}
		for _, phrase := range phrases[b.topic] {
			for _, frame := range []string{"%", "подскажи %", "% подскажи", "мне интересно %", "% пожалуйста"} {
				b.qs = append(b.qs, strings.Replace(frame, "%", phrase, 1))
			}
		}
	}
}

// Pair explicit topic switches and short follow-ups with the same calendar history.
func calendarDialogs(blocks []*block, rng *rand.Rand) []brain.Sample {
	topics := map[string]*block{}
	for _, b := range blocks {
		if b.mood == "" && b.follow == "" {
			topics[b.topic] = b
		}
	}
	day := topics["weekday"]
	pick := func(options []string) string { return options[rng.IntN(len(options))] }
	var samples []brain.Sample
	for _, intent := range []string{"name", "date", "time", "tomorrow", "yesterday", "after_tomorrow"} {
		target := topics[intent]
		questions := target.qs
		switch intent {
		case "tomorrow":
			questions = []string{"а на следующий день", "а днем позже", "и что будет завтра"}
		case "yesterday":
			questions = []string{"а днем раньше", "а за день до этого", "и что было вчера"}
		case "after_tomorrow":
			questions = []string{"а спустя два дня", "а еще через день после завтра", "и какой будет послезавтра"}
		}
		for _, q := range questions {
			for _, mood := range chatMoods {
				h := brain.Turn{Q: pick(day.qs), A: pick(day.as)}
				samples = append(samples, brain.Sample{Mood: mood, Q: noisy(q, rng), A: pick(target.as), History: []brain.Turn{h}})
			}
		}
	}
	return samples
}
