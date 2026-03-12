package cmd

import "testing"

func TestParseOpenAnswer(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want bool
	}{
		{name: "empty defaults yes", in: "", want: true},
		{name: "newline defaults yes", in: "\n", want: true},
		{name: "yes lowercase", in: "y\n", want: true},
		{name: "yes uppercase", in: "Y\n", want: true},
		{name: "yes word", in: "yes\n", want: true},
		{name: "no lowercase", in: "n\n", want: false},
		{name: "no uppercase", in: "N\n", want: false},
		{name: "other input", in: "abc\n", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseOpenAnswer(tt.in)
			if got != tt.want {
				t.Fatalf("parseOpenAnswer(%q)=%v want %v", tt.in, got, tt.want)
			}
		})
	}
}
