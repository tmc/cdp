package browser

import "testing"

func TestIsChallengeTitle(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  bool
	}{
		{"cloudflare just a moment", "Just a moment...", true},
		{"cloudflare mixed case", "JUST A MOMENT", true},
		{"attention required", "Attention Required! | Cloudflare", true},
		{"checking browser", "Checking your browser before accessing example.com", true},
		{"verifying human", "Verifying you are human", true},
		{"leading whitespace", "   Just a moment...  ", true},
		{"real article", "The Holy Grail of Crypto AI | by 0xjacobzhao | Medium", false},
		{"empty", "", false},
		{"unrelated moment", "A moment in history", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isChallengeTitle(tt.title); got != tt.want {
				t.Errorf("isChallengeTitle(%q) = %v, want %v", tt.title, got, tt.want)
			}
		})
	}
}
