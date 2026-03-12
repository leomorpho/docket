package cmd

import "testing"

func TestIsVersionNewer(t *testing.T) {
	tests := []struct {
		latest  string
		current string
		want    bool
	}{
		{latest: "v0.2.0", current: "0.1.0", want: true},
		{latest: "0.1.1", current: "v0.1.0", want: true},
		{latest: "v0.1.0", current: "0.1.0", want: false},
		{latest: "v0.1.0", current: "0.2.0", want: false},
	}

	for _, tt := range tests {
		got := isVersionNewer(tt.latest, tt.current)
		if got != tt.want {
			t.Fatalf("isVersionNewer(%q,%q)=%v want %v", tt.latest, tt.current, got, tt.want)
		}
	}
}

func TestNormalizeVersion(t *testing.T) {
	if got := normalizeVersion("0.1.0"); got != "v0.1.0" {
		t.Fatalf("normalizeVersion(0.1.0)=%q", got)
	}
	if got := normalizeVersion("v0.1.0"); got != "v0.1.0" {
		t.Fatalf("normalizeVersion(v0.1.0)=%q", got)
	}
}
