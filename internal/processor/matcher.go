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
		// Use a more flexible boundary check than \b to handle special characters (like $)
		// \b only works if one character is word and the other is non-word.
		// For $500, \b before $ fails if preceded by a space.

		// We use a custom boundary that considers start/end of string or any whitespace/punctuation.
		// However, Go's regexp doesn't support lookaround.
		// A common trick is to use \b if the word starts/ends with a word character,
		// and something else if it doesn't.

		isWordStart := regexp.MustCompile(`^[a-zA-Z0-9]`).MatchString(word)
		isWordEnd := regexp.MustCompile(`[a-zA-Z0-9]$`).MatchString(word)

		pattern := regexp.QuoteMeta(word)
		if isWordStart {
			pattern = `\b` + pattern
		} else {
			// If it starts with a special character, we want it preceded by start of string or whitespace/non-word
			pattern = `(?:^|[^\w])` + pattern
		}

		if isWordEnd {
			pattern = pattern + `\b`
		} else {
			pattern = pattern + `(?:$|[^\w])`
		}

		re = regexp.MustCompile(`(?i)` + pattern)
		m.patterns[word] = re
	}

	return re.MatchString(corpus)
}
