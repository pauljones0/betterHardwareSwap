package ai

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/google/generative-ai-go/genai"
)

// MockModel satisfies the GenerativeModel interface for testing.
type MockModel struct {
	GenerateContentFn      func(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error)
	SetSystemInstructionFn func(parts ...genai.Part)
}

func (m *MockModel) GenerateContent(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
	return m.GenerateContentFn(ctx, parts...)
}

func (m *MockModel) SetSystemInstruction(parts ...genai.Part) {
	if m.SetSystemInstructionFn != nil {
		m.SetSystemInstructionFn(parts...)
	}
}

func TestCleanRedditPost(t *testing.T) {
	ctx := context.Background()

	t.Run("Success", func(t *testing.T) {
		expected := &CleanedPost{
			Title:       "[WTS] RTX 3080",
			Description: "Great condition",
			Price:       "$500",
			Location:    "Toronto",
		}
		respJSON, _ := json.Marshal(expected)

		mock := &MockModel{
			GenerateContentFn: func(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []genai.Part{genai.Text(respJSON)},
							},
						},
					},
				}, nil
			},
		}

		client := &AIClient{model: mock}
		got, err := client.CleanRedditPost(ctx, "Selling 3080", "Used but works well")

		if err != nil {
			t.Fatalf("CleanRedditPost failed: %v", err)
		}
		if got.Title != expected.Title {
			t.Errorf("got title %q, want %q", got.Title, expected.Title)
		}
	})

	t.Run("Retry on failure", func(t *testing.T) {
		calls := 0
		mock := &MockModel{
			GenerateContentFn: func(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
				calls++
				if calls < 2 {
					return nil, errors.New("transient error")
				}
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []genai.Part{genai.Text(`{"title":"Success"}`)},
							},
						},
					},
				}, nil
			},
		}

		client := &AIClient{model: mock}
		_, err := client.CleanRedditPost(ctx, "title", "body")

		if err != nil {
			t.Errorf("expected success after retry, got error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
	})

	t.Run("JSON Parse Error", func(t *testing.T) {
		mock := &MockModel{
			GenerateContentFn: func(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []genai.Part{genai.Text(`invalid json`)},
							},
						},
					},
				}, nil
			},
		}

		client := &AIClient{model: mock}
		_, err := client.CleanRedditPost(ctx, "title", "body")

		if err == nil {
			t.Error("expected error for invalid JSON, got nil")
		}
	})
}

func TestRunKeywordWizard(t *testing.T) {
	ctx := context.Background()

	t.Run("Normal Query", func(t *testing.T) {
		resp := KeywordWizardResponse{
			MustHave: []string{"3080"},
			AnyOf:    []string{"rtx"},
			IsValid:  true,
		}
		respJSON, _ := json.Marshal(resp)

		mock := &MockModel{
			GenerateContentFn: func(ctx context.Context, parts ...genai.Part) (*genai.GenerateContentResponse, error) {
				return &genai.GenerateContentResponse{
					Candidates: []*genai.Candidate{
						{
							Content: &genai.Content{
								Parts: []genai.Part{genai.Text(respJSON)},
							},
						},
					},
				}, nil
			},
		}

		client := &AIClient{model: mock}
		got, err := client.RunKeywordWizard(ctx, "I want a 3080", "")

		if err != nil {
			t.Fatalf("RunKeywordWizard failed: %v", err)
		}
		if len(got.MustHave) != 1 || got.MustHave[0] != "3080" {
			t.Errorf("unexpected must_have: %v", got.MustHave)
		}
	})
}
