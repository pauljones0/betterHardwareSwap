package processor

import (
	"testing"
)

func TestMatcher(t *testing.T) {
	m := NewMatcher()
	corpus := "Selling my RTX 3080ti for $500 in Toronto. BNIB."

	tests := []struct {
		name     string
		mustHave []string
		anyOf    []string
		mustNot  []string
		want     bool
	}{
		{
			name:     "Direct Match",
			mustHave: []string{"3080ti"},
			want:     true,
		},
		{
			name:     "Case Insensitive",
			mustHave: []string{"rtx"},
			want:     true,
		},
		{
			name:     "Word Boundary - Prevent Partial Match",
			mustHave: []string{"3080"},
			want:     false, // "3080" should not match "3080ti"
		},
		{
			name:    "MustNot Match",
			mustNot: []string{"3080ti"},
			want:    false,
		},
		{
			name:  "AnyOf Match",
			anyOf: []string{"toronto", "vancouver"},
			want:  true,
		},
		{
			name:     "MustHave and AnyOf Match",
			mustHave: []string{"bnib"},
			anyOf:    []string{"3080ti"},
			want:     true,
		},
		{
			name:     "Multiple MustHave - All required",
			mustHave: []string{"3080ti", "toronto"},
			want:     true,
		},
		{
			name:     "Multiple MustHave - One missing",
			mustHave: []string{"3080ti", "vancouver"},
			want:     false,
		},
		{
			name:     "Special Characters in Corpus",
			mustHave: []string{"$500"},
			want:     true,
		},
		{
			name:  "Partial word match in AnyOf",
			anyOf: []string{"3080"},
			want:  false, // should not match 3080ti
		},
		{
			name:     "MustNot takes precedence",
			mustHave: []string{"3080ti"},
			mustNot:  []string{"bnib"},
			want:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Matches(corpus, tt.mustHave, tt.anyOf, tt.mustNot); got != tt.want {
				t.Errorf("Matcher.Matches() = %v, want %v", got, tt.want)
			}
		})
	}
}
