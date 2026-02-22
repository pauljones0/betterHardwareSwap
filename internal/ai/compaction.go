package ai

import (
	"context"
	"fmt"

	"github.com/google/generative-ai-go/genai"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// CompactionResult holds the new prompt suggestion and the IDs of the records it analyzed.
type CompactionResult struct {
	NewPrompt    string
	AnalyticsIDs []string
}

// RunCompaction takes a batch of analytics records, analyzes their outcomes against the current prompt,
// and uses a meta-prompt to generate an improved version of the system prompt.
func (c *AIClient) RunCompaction(ctx context.Context, records []store.AnalyticsRecord, currentPrompt, flowType string) (*CompactionResult, error) {
	if len(records) == 0 {
		return nil, nil // no-op
	}

	recordDetails := ""
	var ids []string
	for i, r := range records {
		ids = append(ids, r.ID)
		recordDetails += fmt.Sprintf("Record %d:\n- Original Prompt: %s\n- Final Stored Query: %s\n- Outcome: %s\n\n",
			i+1, r.OriginalUserPrompt, r.FinalSavedQuery, r.Outcome)
	}

	roleDesc := "a query-building bot"
	if flowType == "manual" {
		roleDesc = "a manual boolean syntax validator bot"
	}

	metaPrompt := fmt.Sprintf(`You are a senior AI prompt engineer improving %s.
The bot uses a system prompt to convert natural language or validate manually typed Boolean queries.

Currently, the bot is using this system prompt:
"""
%s
"""

Here are %d recent interaction analytics from users:
%s

Your task:
Analyze these successes and failures to see if the system prompt needs a slight improvement to handle edge cases better based on what users are actually typing.
Produce an updated version of the system prompt that better aligns with the failures seen above.
If no changes are necessary, return the exact same prompt.

CRITICAL RULES:
1. YOU MUST MAINTAIN THE STRICT JSON SCHEMA REQUIREMENT. The new prompt MUST STILL end with instructions to respond only in JSON.
2. DO NOT change the core structure or purpose of the prompt, only add examples or tweak keywords to dodge failures.
3. ONLY output the raw, plaintext updated prompt. Do NOT include markdown blocks like `+"```...```"+`.

New Prompt:`, roleDesc, currentPrompt, len(records), recordDetails)

	resp, err := c.model.GenerateContent(ctx, genai.Text(metaPrompt))
	if err != nil {
		return nil, fmt.Errorf("gemini generation failed: %w", err)
	}

	if len(resp.Candidates) == 0 || len(resp.Candidates[0].Content.Parts) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}

	part := resp.Candidates[0].Content.Parts[0]
	text, ok := part.(genai.Text)
	if !ok {
		return nil, fmt.Errorf("expected text part")
	}

	return &CompactionResult{
		NewPrompt:    string(text),
		AnalyticsIDs: ids,
	}, nil
}
