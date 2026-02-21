package reddit

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
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

// TokenResponse is from Reddit OAuth Basic Authentication
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int    `json:"expires_in"`
	Scope       string `json:"scope"`
}

// Scraper handles talking to Reddit.
type Scraper struct {
	httpClient   *http.Client
	clientID     string
	clientSecret string
	token        TokenResponse
	tokenExpiry  time.Time
}

// NewScraper returns an initialized Scraper.
func NewScraper(clientID, clientSecret string) *Scraper {
	return &Scraper{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		clientID:     clientID,
		clientSecret: clientSecret,
	}
}

func (s *Scraper) getToken() error {
	// If token isn't expired, just use it
	if s.token.AccessToken != "" && time.Now().Before(s.tokenExpiry) {
		return nil
	}

	data := url.Values{}
	data.Set("grant_type", "client_credentials")

	req, err := http.NewRequest("POST", "https://www.reddit.com/api/v1/access_token", strings.NewReader(data.Encode()))
	if err != nil {
		return err
	}

	req.SetBasicAuth(s.clientID, s.clientSecret)
	req.Header.Set("User-Agent", "script:canadianhardwareswapbot:v1.0.1 (by u/pauljones0)")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("auth error: status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResponse TokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&tokenResponse); err != nil {
		return fmt.Errorf("failed to decode token response: %w", err)
	}

	s.token = tokenResponse
	s.tokenExpiry = time.Now().Add(time.Duration(tokenResponse.ExpiresIn-60) * time.Second) // refresh 60s early
	return nil
}

// FetchNewestPosts hits the /new.json endpoint of r/CanadianHardwareSwap using OAuth.
func (s *Scraper) FetchNewestPosts() ([]Post, error) {
	if err := s.getToken(); err != nil {
		return nil, fmt.Errorf("failed to get reddit oauth token: %w", err)
	}

	req, err := http.NewRequest("GET", "https://oauth.reddit.com/r/CanadianHardwareSwap/new.json?limit=100", nil)
	if err != nil {
		return nil, err
	}

	// Reddit explicitly requires a custom User-Agent to avoid IP bans.
	req.Header.Set("User-Agent", "script:canadianhardwareswapbot:v1.0.1 (by u/pauljones0)")
	req.Header.Set("Authorization", "Bearer "+s.token.AccessToken)

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
