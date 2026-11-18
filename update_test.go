package main

import "testing"

func TestNewer(t *testing.T) {
	for _, c := range []struct {
		a, b string
		want bool
	}{
		{"v0.2.0", "v0.1.9", true},
		{"v0.10.0", "v0.9.0", true},
		{"v1.0.0", "v0.99.99", true},
		{"v0.2.0", "v0.2.0", false},
		{"v0.1.0", "v0.2.0", false},
		{"v0.3.0", "", false},
		{"v0.3.0", "v0.0.0-20261118-abcdef", false},
		{"0.3.0", "v0.2.0", false},
		{"v0.3.0-rc1", "v0.2.0", false},
	} {
		if got := newer(c.a, c.b); got != c.want {
			t.Errorf("newer(%q, %q) = %v, want %v", c.a, c.b, got, c.want)
		}
	}
}
