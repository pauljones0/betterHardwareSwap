package reddit

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
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

func TestFetchWithRetries(t *testing.T) {
	ctx := context.Background()
	callCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		if callCount < 3 {
			w.WriteHeader(http.StatusTooManyRequests)
			return
		}
		// Return valid empty feed on 3rd call
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(Feed{})
	}))
	defer server.Close()

	s := NewScraper()
	s.BaseURL = server.URL
	s.RetryBackoff = 1 * time.Millisecond // Fast retries for testing

	_, err := s.FetchNewestPosts(ctx)
	if err != nil {
		t.Errorf("expected success after retries, got error: %v", err)
	}

	if callCount != 3 {
		t.Errorf("expected 3 calls, got %d", callCount)
	}
}
