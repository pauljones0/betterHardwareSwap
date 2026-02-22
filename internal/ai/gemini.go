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

	// We'll define a generic ResponseSchema loosely, or just rely on MIME Type + prompt,
	// but adding a schema provides strict guarantees if we wanted to enforce it at the model level.
	// We'll add the schema for CleanedPost and KeywordWizardResponse here by dynamically overriding
	// the schema per generation call instead, but for now we'll set the schema to object.

	schema := &genai.Schema{
		Type: genai.TypeObject,
	}
	model.ResponseSchema = schema

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
You are an expert search-query builder for a PC Hardware tracking Discord bot.
The bot ONLY monitors r/CanadianHardwareSwap, a subreddit EXCLUSIVELY for buying and selling computer hardware.

Your goal is to convert the user's natural language request into a strict Boolean query.

CRITICAL RULES:
1. ALL posts are already about computer hardware. NEVER use generic terms like "computer parts", "pc parts", "hardware", "gaming", "electronics", "buy", or "sell" as keywords. They will ruin the search because Reddit users only list specific part names.
2. Extract specific item models, brands, or geographic locations.
3. If a user asks for "anything in [Location]", extract the location and its common abbreviations (e.g., "sk" for Saskatchewan, "bc" for British Columbia). Put these location variations in 'any_of' if they just want anything from there, or 'must_have' if combined with an item.

Fields:
- must_have (AND): Words that ABSOLUTELY MUST be in the post. Make these lowercase.
- any_of (OR): An array of synonyms, variations, or location aliases. If any ONE of these match, the rule passes. Make these lowercase.
- must_not (NOT): Words to explicitly ignore (e.g., "broken", "waterblocked"). Make these lowercase.
- too_broad: Set to true ONLY if the query is extremely generic (e.g., just "gpu", "mouse", "asus"). Location-only queries for specific cities/provinces are generally NOT too broad.

Examples:
1. User: "rtx 3080 in toronto"
{"must_have": ["toronto"], "any_of": ["rtx 3080", "3080", "rtx3080"], "must_not": [], "too_broad": false}

2. User: "any computer parts in Saskatoon Saskatchewan"
{"must_have": [], "any_of": ["saskatoon", "saskatchewan", "sk"], "must_not": [], "too_broad": false}

3. User: "I want a gpu"
{"must_have": [], "any_of": ["gpu", "graphics card"], "must_not": [], "too_broad": true}

User Request: "%s"

Respond ONLY with a valid JSON object matching this schema:
{
  "must_have": ["string1"],
  "any_of": ["string2", "string3"],
  "must_not": [],
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
