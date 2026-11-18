package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

// version is set by release builds (-ldflags "-X main.version=v1.2.3").
var version = ""

// Update checks are off until the human turns them on in the menu.
var (
	updates       *bool // nil: Mansur has not mentioned them yet
	updateChecked time.Time
)

const releasesURL = "https://github.com/artemnee/mansur/releases/latest"

// appVersion is the release this binary was built from, or "" for a local build.
// go install …@vX.Y.Z stamps the module version, so that works without ldflags too.
func appVersion() string {
	if version != "" {
		return version
	}
	if bi, ok := debug.ReadBuildInfo(); ok && semver(bi.Main.Version) != nil {
		return bi.Main.Version
	}
	return ""
}

// semver parses "v1.2.3" into its numbers. Pre-release and pseudo-versions ("v0.0.0-2026…") give nil.
func semver(v string) []int {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if !strings.HasPrefix(v, "v") || len(parts) != 3 {
		return nil
	}
	var n []int
	for _, p := range parts {
		x, err := strconv.Atoi(p)
		if err != nil || x < 0 {
			return nil
		}
		n = append(n, x)
	}
	return n
}

// newer reports whether release a is newer than b.
func newer(a, b string) bool {
	x, y := semver(a), semver(b)
	if x == nil || y == nil {
		return false
	}
	for i := range x {
		if x[i] != y[i] {
			return x[i] > y[i]
		}
	}
	return false
}

// latestRelease asks GitHub for the tag of the latest release. This is the only network
// request Mansur ever makes, and only when the human turned update checks on.
func latestRelease() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "GET", "https://api.github.com/repos/artemnee/mansur/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "mansur")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: %s", resp.Status)
	}
	var r struct {
		Tag string `json:"tag_name"`
	}
	err = json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&r)
	return r.Tag, err
}

func openReleases() { _ = exec.Command("open", releasesURL).Start() }
