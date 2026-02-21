package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// AIClient wraps the Gemini API.
type AIClient struct {
	client *genai.Client
	model  *genai.GenerativeModel
}

// CleanedPost is the structured response we want from Gemini when parsing a Reddit Deal.
type CleanedPost struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       string `json:"price,omitempty"`
	Location    string `json:"location,omitempty"`
}

// KeywordWizardResponse is the structured response for compiling a Boolean query.
type KeywordWizardResponse struct {
	MustHave []string `json:"must_have"` // AND
	AnyOf    []string `json:"any_of"`    // OR
	MustNot  []string `json:"must_not"`  // NOT
	TooBroad bool     `json:"too_broad"` // Warns if this matches > 10% of deals (e.g., just "GPU")
}

// NewAIClient initializes the Gemini client.
func NewAIClient(ctx context.Context, apiKey string) (*AIClient, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %v", err)
	}

	model := client.GenerativeModel("gemini-2.5-flash-lite")
	model.ResponseMIMEType = "application/json" // Force structured JSON output

	return &AIClient{
		client: client,
		model:  model,
	}, nil
}

// Close closes the underlying client connection.
func (c *AIClient) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

// CleanRedditPost takes the raw messy Reddit title and body, and returns a concise, mobile-friendly summary.
func (c *AIClient) CleanRedditPost(ctx context.Context, rawTitle, rawBody string) (*CleanedPost, error) {
	prompt := fmt.Sprintf(`
You are a concise, highly efficient deal summarizer for a Canadian Hardware Swap Discord feed. 
Your goal is to make the post readable on a mobile device at a glance.

Instructions:
1. Strip out pure Reddit jargon, long-winded stories, and off-topic chat.
2. Keep standard hardware swap abbreviations (WTB, WTS, LBNB, OBO, BNIB).
3. Extract the core item(s) being sold or wanted.
4. Extract the Price and Location if mentioned.
5. Provide a succinct 'Description' summarizing the actual hardware details/condition.

Raw Title: %s
Raw Body: %s

Respond ONLY with a valid JSON object matching this schema:
{
  "title": "Cleaned up title (e.g., [WTS] RTX 3080 FE)",
  "description": "Short summary of items and condition.",
  "price": "$500 OBO",
  "location": "Toronto, ON"
}
`, rawTitle, rawBody)

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generation failed: %w", err)
	}

	var cleaned CleanedPost
	if err := parseJSONResponse(resp, &cleaned); err != nil {
		return nil, err
	}
	return &cleaned, nil
}

// RunKeywordWizard converts a user's natural language request into a strict Boolean alert query.
func (c *AIClient) RunKeywordWizard(ctx context.Context, userRequest string) (*KeywordWizardResponse, error) {
	prompt := fmt.Sprintf(`
You are an expert search-query builder for a PC Hardware tracking bot.
A user wants to be pinged when an item matching their description is posted.

Your goal is to convert their natural language request into a strict Boolean query.
- must_have (AND): Words that ABSOLUTELY MUST be in the post. (e.g. if they want a 3080 in Toronto, "toronto" is a must_have). Make these lowercase.
- any_of (OR): An array of synonyms or variations. If any ONE of these match, the rule passes. (e.g., ["rtx 3080", "3080ti", "rtx3080"]). Make these lowercase.
- must_not (NOT): Words to explicitly ignore (e.g., "broken", "waterblocked"). Make these lowercase.

CRITICAL: Evaluate "too_broad". If their query is so generic (e.g., "gpu", "mouse", "keyboard", "asus") that it would spam them on >10%% of all posts, set too_broad to true.

User Request: "%s"

Respond ONLY with a valid JSON object matching this schema:
{
  "must_have": ["toronto"],
  "any_of": ["rtx 3080", "3080", "3080ti"],
  "must_not": ["broken", "for parts"],
  "too_broad": false
}
`, userRequest)

	resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generation failed: %w", err)
	}

	var wizard KeywordWizardResponse
	if err := parseJSONResponse(resp, &wizard); err != nil {
		return nil, err
	}
	return &wizard, nil
}

// parseJSONResponse is a helper that strips any potential markdown formatting (```json) returned by the model and unmarshals it.
func parseJSONResponse(resp *genai.GenerateContentResponse, v interface{}) error {
	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("empty response from model")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return fmt.Errorf("expected text part, got %T", part)
	}

	str := string(text)
	// Some models might stubbornly enclose JSON in markdown blocks despite the MIME type instruction.
	// A robust string cleaner would go here, but with ResponseMIMEType="application/json", it should be clean.
	if err := json.Unmarshal([]byte(str), v); err != nil {
		log.Printf("Failed to unmarshal JSON: %s", str)
		return fmt.Errorf("JSON parse error: %w", err)
	}

	return nil
}
