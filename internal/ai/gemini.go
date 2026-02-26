package ai

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// GenerativeModel defines the subset of genai.GenerativeModel methods we use.
type GenerativeModel interface {
	GenerateContent(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error)
	SetSystemInstruction(parts ...genai.Part)
}

// ModelWrapper wraps the real genai.GenerativeModel to satisfy our interface.
type ModelWrapper struct {
	model *genai.GenerativeModel
}

func (m *ModelWrapper) GenerateContent(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	return m.model.GenerateContent(ctx, parts...)
}

func (m *ModelWrapper) SetSystemInstruction(parts ...genai.Part) {
	m.model.SystemInstruction = &genai.Content{Parts: parts}
}

// AIClient wraps the Gemini API.
type AIClient struct {
	client *genai.Client
	model  GenerativeModel
}

// CleanedPost is the structured response we want from Gemini when parsing a Reddit Deal.
type CleanedPost struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Price       string `json:"price,omitempty"`
	Location    string `json:"location,omitempty"`
	Condition   string `json:"condition,omitempty"`
}

// KeywordWizardResponse is the structured response for compiling a Boolean query.
type KeywordWizardResponse struct {
	MustHave         []string `json:"must_have"`                   // AND
	AnyOf            []string `json:"any_of"`                      // OR
	MustNot          []string `json:"must_not"`                    // NOT
	TooBroad         bool     `json:"too_broad"`                   // Warns if this matches > 10% of deals (e.g., just "GPU")
	BroadReason      string   `json:"broad_reason,omitempty"`      // Why is it too broad?
	BroadSuggestions []string `json:"broad_suggestions,omitempty"` // Specific ways to narrow it down
	IsValid          bool     `json:"is_valid"`                    // Indicates if a manually typed query is valid syntax
	ErrorMessage     string   `json:"error_message,omitempty"`     // Explanation of why the syntax is invalid
}

// NewAIClient initializes the Gemini client.
func NewAIClient(ctx context.Context, apiKey string) (*AIClient, error) {
	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return nil, fmt.Errorf("failed to create genai client: %v", err)
	}

	model := client.GenerativeModel("gemini-2.5-flash-lite")
	model.ResponseMIMEType = "application/json" // Force structured JSON output

	schema := &genai.Schema{
		Type: genai.TypeObject,
	}
	model.ResponseSchema = schema

	return &AIClient{
		client: client,
		model:  &ModelWrapper{model: model},
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
	c.model.SetSystemInstruction(genai.Text(CleanPostSystemInstruction))
	prompt := fmt.Sprintf(CleanPostUserPromptTemplate, rawTitle, rawBody)

	var cleaned CleanedPost
	err := c.callWithRetry(ctx, prompt, &cleaned)
	if err != nil {
		return nil, err
	}
	return &cleaned, nil
}

// RunKeywordWizard converts a user's natural language request into a strict Boolean alert query.
func (c *AIClient) RunKeywordWizard(ctx context.Context, userRequest, promptOverride string) (*KeywordWizardResponse, error) {
	basePrompt := promptOverride
	if basePrompt == "" {
		basePrompt = DefaultWizardPrompt
	}
	c.model.SetSystemInstruction(genai.Text(basePrompt))
	prompt := fmt.Sprintf(WizardUserPromptTemplate, userRequest)

	var wizard KeywordWizardResponse
	err := c.callWithRetry(ctx, prompt, &wizard)
	if err != nil {
		return nil, err
	}
	return &wizard, nil
}

// ValidateManualQuery securely validates a user's manually typed Boolean-like query.
func (c *AIClient) ValidateManualQuery(ctx context.Context, userQuery, promptOverride string) (*KeywordWizardResponse, error) {
	basePrompt := promptOverride
	if basePrompt == "" {
		basePrompt = DefaultManualPrompt
	}
	c.model.SetSystemInstruction(genai.Text(basePrompt))
	prompt := fmt.Sprintf(ManualUserPromptTemplate, userQuery)

	var wizard KeywordWizardResponse
	err := c.callWithRetry(ctx, prompt, &wizard)
	if err != nil {
		return nil, err
	}
	return &wizard, nil
}

// callWithRetry handles the actual AI generation with exponential backoff on transient errors.
func (c *AIClient) callWithRetry(ctx context.Context, prompt string, v interface{}) error {
	var lastErr error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		resp, err := c.model.GenerateContent(ctx, genai.Text(prompt))
		if err == nil {
			if parseErr := parseJSONResponse(resp, v); parseErr == nil {
				return nil
			} else {
				// JSON parse error is usually NOT transient, but we retry once just in case of AI flakiness.
				lastErr = parseErr
				if i > 0 {
					break
				}
			}
		} else {
			lastErr = err
		}

		backoff := time.Duration(i+1) * time.Second
		select {
		case <-time.After(backoff):
			continue
		case <-ctx.Done():
			return ctx.Err()
		}
	}

	return fmt.Errorf("gemini call failed after %d attempts: %w", maxRetries, lastErr)
}

// parseJSONResponse is a helper that strips any potential markdown formatting (```json) returned by the model and unmarshals it.
func parseJSONResponse(resp *genai.GenerateContentResponse, v interface{}) error {
	if resp == nil || len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil || len(resp.Candidates[0].Content.Parts) == 0 {
		return fmt.Errorf("empty or malformed response from model")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return fmt.Errorf("expected text part, got %T", part)
	}

	str := string(text)
	if err := json.Unmarshal([]byte(str), v); err != nil {
		return fmt.Errorf("JSON parse error: %w (content: %s)", err, str)
	}

	return nil
}
