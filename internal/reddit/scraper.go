package reddit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
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
	httpClient *http.Client
}

// NewScraper returns an initialized Scraper.
func NewScraper() *Scraper {
	return &Scraper{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// FetchNewestPosts hits the /new.json endpoint of r/CanadianHardwareSwap.
func (s *Scraper) FetchNewestPosts() ([]Post, error) {
	req, err := http.NewRequest("GET", "https://www.reddit.com/r/CanadianHardwareSwap/new.json?limit=100", nil)
	if err != nil {
		return nil, err
	}

	// Reddit explicitly requires a custom User-Agent to avoid IP bans.
	req.Header.Set("User-Agent", "script:canadianhardwareswapbot:v1.0.1 (by u/pauljones0)")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("reddit returned %d: %s", resp.StatusCode, string(body))
	}

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
