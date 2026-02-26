package processor

import (
	"testing"

	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
)

func TestBuildDealEmbed(t *testing.T) {
	tests := []struct {
		name      string
		post      reddit.Post
		cleaned   *ai.CleanedPost
		wantTitle string
		checkNil  bool
	}{
		{
			name: "Full metadata",
			post: reddit.Post{
				Title: "[H] RTX 3080 [W] $500",
				URL:   "https://reddit.com/post1",
			},
			cleaned: &ai.CleanedPost{
				Title:       "RTX 3080",
				Description: "Great card",
				Price:       "$500",
				Location:    "Toronto",
				Condition:   "Used",
			},
			wantTitle: "📦 RTX 3080",
		},
		{
			name: "Missing optional metadata",
			post: reddit.Post{
				Title: "Looking for a mouse",
			},
			cleaned: &ai.CleanedPost{
				Title:       "Mouse",
				Description: "Any mouse will do",
			},
			wantTitle: "📦 Mouse",
		},
	}

	builder := NewDealBuilder()
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := builder.BuildDealEmbed(tt.post, tt.cleaned)
			if got.Title != tt.wantTitle {
				t.Errorf("expected title %q, got %q", tt.wantTitle, got.Title)
			}
			if got.URL != tt.post.URL {
				t.Errorf("expected URL %q, got %q", tt.post.URL, got.URL)
			}
			// Check if fields were added correctly
			expectedFields := 0
			if tt.cleaned.Price != "" {
				expectedFields++
			}
			if tt.cleaned.Location != "" {
				expectedFields++
			}
			if tt.cleaned.Condition != "" {
				expectedFields++
			}
			if len(got.Fields) != expectedFields {
				t.Errorf("expected %d fields, got %d", expectedFields, len(got.Fields))
			}
		})
	}
}
