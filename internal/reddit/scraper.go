package reddit

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/pauljones0/betterHardwareSwap/internal/logger"
)

// Reddit struct maps the nested structure of Reddit's .json feed.
type Feed struct {
	Data struct {
		Children []struct {
			Data Post `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

// Post is the raw, messy payload from Reddit.
type Post struct {
	ID                  string  `json:"id"`
	Title               string  `json:"title"`
	SelfText            string  `json:"selftext"`
	URL                 string  `json:"url"` // Used for image thumbnails if present
	Permalink           string  `json:"permalink"`
	Subreddit           string  `json:"subreddit"`
	CreatedUtc          float64 `json:"created_utc"`
	Author              string  `json:"author"`
	Score               int     `json:"score"`
	NumComments         int     `json:"num_comments"`
	LinkFlairText       string  `json:"link_flair_text"`     // "Closed", "Selling", etc
	RemovedByByCategory string  `json:"removed_by_category"` // "moderator", "deleted"
	Thumbnail           string  `json:"thumbnail"`
}

// Scraper handles talking to Reddit.
type Scraper struct {
	httpClient   *http.Client
	BaseURL      string
	RetryBackoff time.Duration
}

// NewScraper returns an initialized Scraper.
func NewScraper() *Scraper {
	return &Scraper{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		BaseURL:      "https://www.reddit.com",
		RetryBackoff: 2 * time.Second,
	}
}

// FetchNewestPosts hits the .json endpoint of r/CanadianHardwareSwap.
func (s *Scraper) FetchNewestPosts(ctx context.Context) ([]Post, error) {
	maxRetries := 8
	backoff := s.RetryBackoff
	var lastErr error
	var respStatusCode int
	var body []byte

	for i := 0; i < maxRetries; i++ {
		req, err := http.NewRequestWithContext(ctx, "GET", s.BaseURL+"/r/CanadianHardwareSwap/.json?sort=new&limit=100", nil)
		if err != nil {
			return nil, err
		}

		// Reddit explicitly requires a custom User-Agent to avoid IP bans.
		req.Header.Set("User-Agent", "script:canadianhardwareswapbot:v2.0 (by u/pauljones0)")

		resp, err := s.httpClient.Do(req)
		if err != nil {
			return nil, err
		}

		respStatusCode = resp.StatusCode

		if resp.StatusCode == http.StatusOK {
			defer resp.Body.Close()
			var feed Feed
			if err := json.NewDecoder(resp.Body).Decode(&feed); err != nil {
				return nil, fmt.Errorf("failed to decode reddit json: %w", err)
			}

			var posts []Post
			for _, child := range feed.Data.Children {
				// Only track actual posts, not stickies/announcements
				if child.Data.Author != "AutoModerator" {
					posts = append(posts, child.Data)
				}
			}

			return posts, nil
		}

		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusForbidden || resp.StatusCode >= 500 {
			resp.Body.Close()
			logger.Warn(ctx, "Reddit request failed, retrying", "status", resp.StatusCode, "retry", i+1, "backoff", backoff)

			select {
			case <-time.After(backoff):
				backoff *= 2
				continue
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}

		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		lastErr = fmt.Errorf("reddit returned %d: %s", respStatusCode, string(body))
		break // Not a 429, don't retry
	}

	if lastErr != nil {
		return nil, lastErr
	}
	return nil, fmt.Errorf("max retries exceeded, last status: %d", respStatusCode)
}
