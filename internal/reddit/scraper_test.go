package reddit

import (
	"encoding/json"
	"os"
	"testing"
)

func TestParseSample(t *testing.T) {
	b, err := os.ReadFile("sample.json")
	if err != nil {
		t.Fatalf("failed to read sample.json: %v", err)
	}

	var feed Feed
	if err := json.Unmarshal(b, &feed); err != nil {
		t.Fatalf("failed to parse sample.json: %v", err)
	}

	if len(feed.Data.Children) == 0 {
		t.Errorf("expected to parse posts, got 0")
	}

	for i, child := range feed.Data.Children {
		post := child.Data
		t.Logf("Post %d: ID=%s Title=%q Score=%d SelfTextLen=%d", i, post.ID, post.Title, post.Score, len(post.SelfText))

		// Ensure that even if score is 0 or selftext is empty, we still have the core of the post (ID and Title)
		if post.ID == "" {
			t.Errorf("Post %d has no ID", i)
		}
		if post.Title == "" {
			t.Errorf("Post %d has no Title", i)
		}
	}
}
