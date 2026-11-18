package main

import (
	"os"
	"strings"
	"unicode"

	"github.com/ebitengine/purego"
)

// langSetting is the language chosen in the menu: "" follows the system, or "ru" / "en".
var langSetting = ""

// uiLang is the language of the menu, CLI messages and the phrases Mansur says on his own.
func uiLang() string {
	if langSetting != "" {
		return langSetting
	}
	return systemLang()
}

var sysLang string

// systemLang reads the first preferred language of macOS ("ru-RU" → ru, anything else → en).
func systemLang() string {
	if sysLang != "" {
		return sysLang
	}
	sysLang = "en"
	pref := os.Getenv("LANG")
	if _, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_GLOBAL|purego.RTLD_NOW); err == nil {
		if l := goStr(cls("NSLocale").Send(sel("preferredLanguages")).Send(sel("firstObject"))); l != "" {
			pref = l
		}
	}
	if strings.HasPrefix(pref, "ru") {
		sysLang = "ru"
	}
	return sysLang
}

// msgLang guesses the language of a message by its letters; without letters it falls back to the UI language.
func msgLang(s string) string {
	cyr, lat := 0, 0
	for _, r := range s {
		switch {
		case unicode.Is(unicode.Cyrillic, r):
			cyr++
		case r < 128 && unicode.IsLetter(r):
			lat++
		}
	}
	switch {
	case cyr > lat:
		return "ru"
	case lat > cyr:
		return "en"
	}
	return uiLang()
}

// texts holds every user-facing string as {Russian, English}.
var texts = map[string][2]string{
	"usage": {`Мансур — пиксельный кот в углу экрана.

  mansur            поселить Мансура (уходит в фон, терминал свободен)
  mansur stop       отпустить Мансура
  mansur status     живёт ли сейчас
  mansur autostart on|off   запускать при входе в систему

Флаги:
  -size N   размер пикселя, по умолчанию 4 (окно 32·N точек)
  -fg       не уходить в фон

С Мансуром: води курсором — гладишь, клик — прыгает,
тащи мышкой — переносишь, правый клик — меню.
`, `Mansur is a pixel cat in the corner of your screen.

  mansur            move Mansur in (runs in the background, the terminal stays free)
  mansur stop       let Mansur go
  mansur status     is he home?
  mansur autostart on|off   start at login

Flags:
  -size N   pixel size, 4 by default (the window is 32·N points)
  -fg       stay in the foreground

With Mansur: move the cursor over him to pet, click to make him jump,
drag to carry him, right click for the menu.
`},
	"alive":        {"Мансур живёт (pid %d)\n", "Mansur is home (pid %d)\n"},
	"absent":       {"Мансура нет", "Mansur is not here"},
	"already":      {"Мансур уже тут (pid %d). mansur stop — отпустить.\n", "Mansur is already here (pid %d). mansur stop to let him go.\n"},
	"moved_in":     {"Мансур поселился в углу экрана. mansur stop — отпустить.", "Mansur moved into the corner of your screen. mansur stop to let him go."},
	"not_here":     {"Мансура и так нет", "Mansur is not here anyway"},
	"bye":          {"Мансур ушёл спать к себе домой. Пока!", "Mansur went home to sleep. Bye!"},
	"on":           {"вкл", "on"},
	"off":          {"выкл", "off"},
	"autostart":    {"автозапуск: ", "autostart: "},
	"full":         {"сытость ", "full "},
	"age":          {"возраст: %d дн.", "age: %d d."},
	"talk":         {"поговорить", "talk"},
	"feed":         {"покормить ягодкой", "feed a berry"},
	"sleep":        {"уложить спать", "put to sleep"},
	"wake":         {"разбудить", "wake up"},
	"breaks":       {"перерывы: ", "breaks: "},
	"language":     {"язык: ", "language: "},
	"auto":         {"авто", "auto"},
	"quit":         {"выход", "quit"},
	"friend":       {"друг: ", "friend: "},
	"trait":        {"характер: ", "nature: "},
	"cuddly":       {"ласковый", "cuddly"},
	"greedy":       {"обжора", "greedy"},
	"independent":  {"сам по себе", "independent"},
	"chatty":       {"болтун", "chatty"},
	"updates":      {"обновления: ", "updates: "},
	"update_to":    {"обновить до %s", "update to %s"},
	"update_new":   {"вышла новая версия %s! она в меню", "version %s is out! see the menu"},
	"update_offer": {"я могу раз в день проверять обновления. включи в меню, если хочешь", "i can check for updates once a day. turn it on in the menu if you like"},
	"write":        {"напиши мансуру:", "write to mansur:"},
	"hint":         {"enter - ок   esc - закрыть", "enter - ok   esc - close"},
}

// tr returns a user-facing string in the UI language.
func tr(key string) string { return trIn(uiLang(), key) }

func trIn(lang, key string) string {
	t := texts[key]
	if lang == "ru" {
		return t[0]
	}
	return t[1]
}
