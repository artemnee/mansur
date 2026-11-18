package main

import (
	"testing"
	"time"
)

func TestParseName(t *testing.T) {
	for in, want := range map[string]string{
		"меня зовут фёдор":          "Фёдор",
		"Привет! Меня зовут Маша.":  "Маша",
		"зови меня саша пожалуйста": "Саша",
		"моё имя Дима":              "Дима",
		"my name is alex":           "Alex",
		"hi, call me Kate!":         "Kate",
		"как меня зовут":            "",
		"what's my name":            "",
		"я устал":                   "",
		"зови меня просто Саша":     "Саша",
		"меня зовут не так":         "",
		"call me back later":        "",
		"i recall me saying that":   "",
		"you can call me just Sam":  "Sam",
	} {
		if got := parseName(in); got != want {
			t.Errorf("parseName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestGreeting(t *testing.T) {
	now := time.Date(2026, 11, 18, 7, 0, 0, 0, time.Local)
	b := newBond()
	if g := b.greeting(now); g != "hello" {
		t.Errorf("first launch: %s", g)
	}
	b.LastSeen = now.Add(-72 * time.Hour)
	if g := b.greeting(now); g != "missed" {
		t.Errorf("after three days: %s", g)
	}
	b.LastSeen = now.Add(-10 * time.Hour)
	b.Hours[10] = 1 // usually shows up at ten
	if g := b.greeting(now); g != "early" {
		t.Errorf("at seven, usually at ten: %s", g)
	}
	b.Hours[10], b.Affection = 0, 0.1
	if g := b.greeting(now); g != "sulk" {
		t.Errorf("ignored: %s", g)
	}
}

func TestAffection(t *testing.T) {
	b := newBond()
	for range 4 * 60 { // four hours of being ignored
		b.minute(time.Now(), false, true)
	}
	if !b.sulky() {
		t.Errorf("affection %.2f after four ignored hours, want sulky", b.Affection)
	}
	for range 30 {
		b.petted()
	}
	if b.sulky() {
		t.Errorf("affection %.2f after petting, want forgiven", b.Affection)
	}
}
