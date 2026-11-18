package main

import (
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"
)

// bond is what Mansur remembers about the human and how he feels about them. It is saved in
// state.json; the brain only sees it through mood tokens and the {имя} / {name_en} templates.
type bond struct {
	Name     string      `json:"name,omitempty"`
	LastSeen time.Time   `json:"last_seen"`
	Hours    [24]float64 `json:"hours"` // how active the human usually is at each hour of the day

	Affection float64 `json:"affection"` // 0..1: grows with attention, fades when ignored

	// Character traits, 0..1. They grow from how Mansur is treated, not at random.
	Cuddly      float64 `json:"cuddly"`      // petted a lot: comes to the cursor, purrs more
	Greedy      float64 `json:"greedy"`      // fed a lot: asks for berries earlier
	Independent float64 `json:"independent"` // often left alone: walks and climbs on his own
	Chatty      float64 `json:"chatty"`      // talked to a lot: speaks up more often
}

func newBond() *bond { return &bond{Affection: 0.5} }

// fast speeds up the character's clock for testing: MANSUR_FAST=60 mansur -fg makes a minute
// pass every second, so sulking takes four minutes instead of four hours.
var fast = func() int {
	if n, err := strconv.Atoi(os.Getenv("MANSUR_FAST")); err == nil && n > 1 {
		return min(n, tps*60)
	}
	return 1
}()

// grow moves a value towards 1 by a fraction k of the remaining distance.
func grow(v *float64, k float64) { *v += (1 - *v) * k }

func (b *bond) petted() {
	grow(&b.Affection, 0.02)
	grow(&b.Cuddly, 0.004)
}

func (b *bond) fed() {
	grow(&b.Affection, 0.05)
	grow(&b.Greedy, 0.03)
}

func (b *bond) chatted() {
	grow(&b.Affection, 0.02)
	grow(&b.Chatty, 0.01)
}

// minute is called once a minute. active: the human moved the mouse in the last minute.
// ignored: Mansur got no attention for a long while.
func (b *bond) minute(now time.Time, active, ignored bool) {
	if active {
		h := now.Hour()
		for i := range b.Hours {
			b.Hours[i] *= 0.999
		}
		b.Hours[h] += 0.01
		b.LastSeen = now
	}
	if ignored {
		b.Affection *= 0.996 // from 0.5 down to sulky in about four hours of being ignored
		grow(&b.Independent, 0.0005)
	}
	// Traits slowly fade unless they are fed by the same kind of attention.
	for _, t := range []*float64{&b.Cuddly, &b.Greedy, &b.Independent, &b.Chatty} {
		*t *= 0.9998
	}
}

// sulky: ignored long enough that Mansur is offended.
func (b *bond) sulky() bool { return b.Affection < 0.2 }

// usualStart is the hour the human usually shows up (the earliest busy hour), or -1 if unknown.
func (b *bond) usualStart() int {
	busiest := 0.0
	for _, v := range b.Hours {
		busiest = max(busiest, v)
	}
	if busiest < 0.05 {
		return -1
	}
	for h := 4; h < 24; h++ {
		if b.Hours[h] >= busiest*0.3 {
			return h
		}
	}
	return -1
}

// greeting picks how Mansur says hello at startup, from how long the human was away and when
// they usually come.
func (b *bond) greeting(now time.Time) string {
	if b.LastSeen.IsZero() {
		return "hello"
	}
	away := now.Sub(b.LastSeen)
	switch start := b.usualStart(); {
	case away > 36*time.Hour:
		return "missed"
	case away > 4*time.Hour && start > 0 && now.Hour() >= 4 && now.Hour() < start-1:
		return "early"
	case b.sulky():
		return "sulk"
	}
	return "hello"
}

// trait is the strongest character trait, if it stands out at all.
func (b *bond) trait() string {
	best, name := 0.25, ""
	for n, v := range map[string]float64{"cuddly": b.Cuddly, "greedy": b.Greedy, "independent": b.Independent, "chatty": b.Chatty} {
		if v > best {
			best, name = v, n
		}
	}
	return name
}

var namePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?:меня зовут|зови меня|мо[её] имя)\s+([a-zа-яё][a-zа-яё-]{1,19})`),
	regexp.MustCompile(`(?:my name is|my name's|call me)\s+([a-z][a-z-]{1,19})`),
}

// parseName finds "меня зовут Фёдор" / "call me Alex" in a message.
func parseName(text string) string {
	lower := strings.ToLower(text)
	for _, re := range namePatterns {
		if m := re.FindStringSubmatch(lower); m != nil {
			r, size := utf8.DecodeRuneInString(m[1])
			return string(unicode.ToUpper(r)) + m[1][size:]
		}
	}
	return ""
}
