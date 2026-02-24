package processor

import (
	"regexp"
	"strings"
)

// Matcher provides robust keyword matching with word boundary awareness.
type Matcher struct {
	patterns map[string]*regexp.Regexp
}

func NewMatcher() *Matcher {
	return &Matcher{
		patterns: make(map[string]*regexp.Regexp),
	}
}

// Matches returns true if the corpus matches the criteria defined by mustHave, anyOf, and mustNot.
func (m *Matcher) Matches(corpus string, mustHave, anyOf, mustNot []string) bool {
	corpus = strings.ToLower(corpus)

	// 1. MustNot check (Fails if any are present)
	for _, word := range mustNot {
		if m.containsWord(corpus, word) {
			return false
		}
	}

	// 2. MustHave check (Fails if any are missing)
	for _, word := range mustHave {
		if !m.containsWord(corpus, word) {
			return false
		}
	}

	// 3. AnyOf check (Fails if none are present, but only if AnyOf is not empty)
	if len(anyOf) > 0 {
		matchedAny := false
		for _, word := range anyOf {
			if m.containsWord(corpus, word) {
				matchedAny = true
				break
			}
		}
		if !matchedAny {
			return false
		}
	}

	return true
}

// containsWord checks if a word exists in the corpus with word boundary awareness.
func (m *Matcher) containsWord(corpus, word string) bool {
	word = strings.ToLower(strings.TrimSpace(word))
	if word == "" {
		return false
	}

	// Cache the regex for performance
	re, ok := m.patterns[word]
	if !ok {
		// Use word boundaries \b to ensure "3080" doesn't match "3080ti"
		// We escape the word to handle special characters like '+' in 'C++' safely,
		// though in hardware swap it's mostly alphanumeric.
		pattern := `\b` + regexp.QuoteMeta(word) + `\b`
		re = regexp.MustCompile(pattern)
		m.patterns[word] = re
	}

	return re.MatchString(corpus)
}
