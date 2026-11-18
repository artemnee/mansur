package main

import (
	"encoding/json"
	"encoding/xml"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Persisted state: ~/Library/Application Support/mansur/state.json
type saved struct {
	Hunger float64 `json:"hunger"`
	Breaks bool    `json:"breaks"`
	Lang   string  `json:"lang,omitempty"` // "" follows the system language
	Bond   *bond   `json:"bond,omitempty"`

	Updates       *bool     `json:"updates,omitempty"` // nil: not offered yet
	UpdateChecked time.Time `json:"update_checked,omitempty"`
	Born          time.Time `json:"born"`
	At            time.Time `json:"at"`
}

func statePath() string {
	dir, _ := os.UserConfigDir()
	return filepath.Join(dir, "mansur", "state.json")
}

func loadState(p *Pet) time.Time {
	var s saved
	b, err := os.ReadFile(statePath())
	if err != nil || json.Unmarshal(b, &s) != nil {
		return time.Now()
	}
	// Mansur got hungry while he was away.
	away := time.Since(s.At).Minutes()
	p.hunger = min(100, s.Hunger+away*100/90)
	p.breaks = s.Breaks
	langSetting = s.Lang
	if s.Bond != nil {
		p.bond = s.Bond
	}
	updates, updateChecked = s.Updates, s.UpdateChecked
	return s.Born
}

// loadLang reads only the language setting, for CLI messages before the app starts.
func loadLang() {
	var s saved
	if b, err := os.ReadFile(statePath()); err == nil && json.Unmarshal(b, &s) == nil {
		langSetting = s.Lang
	}
}

func saveState(p *Pet, born time.Time) {
	b, _ := json.MarshalIndent(saved{Hunger: p.hunger, Breaks: p.breaks, Lang: langSetting, Bond: p.bond,
		Updates: updates, UpdateChecked: updateChecked, Born: born, At: time.Now()}, "", "  ")
	_ = os.MkdirAll(filepath.Dir(statePath()), 0o700)
	_ = os.WriteFile(statePath(), b, 0o600)
}

// Autostart: a LaunchAgent in ~/Library/LaunchAgents.
const agentLabel = "com.artemnee.mansur"

func agentPath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "LaunchAgents", agentLabel+".plist")
}

func autostartOn() bool {
	_, err := os.Stat(agentPath())
	return err == nil
}

func setAutostart(on bool) error {
	if !on {
		err := os.Remove(agentPath())
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if exe, err = filepath.EvalSymlinks(exe); err != nil {
		return err
	}
	var esc strings.Builder
	_ = xml.EscapeText(&esc, []byte(exe)) // the path may contain & or <
	plist := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>%s</string>
	<key>ProgramArguments</key>
	<array>
		<string>%s</string>
		<string>-fg</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`, agentLabel, esc.String())
	if err := os.MkdirAll(filepath.Dir(agentPath()), 0o755); err != nil {
		return err
	}
	return os.WriteFile(agentPath(), []byte(plist), 0o600)
}
