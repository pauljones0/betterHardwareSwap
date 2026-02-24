package processor

import (
	"context"
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

func TestProcessNewPost(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Mocks
	mockDB := &MockStore{
		SavePostRecordsFn: func(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error {
			return nil
		},
	}
	mockCache := &MockCache{
		GetServerConfigFn: func(ctx context.Context, serverID string) (*store.ServerConfig, error) {
			return &store.ServerConfig{
				FeedChannelID: "feed1",
				PingChannelID: "ping1",
			}, nil
		},
	}
	mockAI := &MockAI{
		CleanRedditPostFn: func(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error) {
			return &ai.CleanedPost{
				Title:       "Clean Title",
				Description: "Clean Desc",
				Price:       "$500",
				Location:    "Toronto",
			}, nil
		},
	}
	mockDiscord := &MockDiscord{
		SendEmbedWithComponentsFn: func(channelID string, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (string, error) {
			return "msg123", nil
		},
		AddReactionFn: func(channelID, messageID, emoji string) error {
			return nil
		},
		SendMessageFn: func(channelID, content string) error {
			return nil
		},
	}

	post := reddit.Post{
		ID:    "t3_abc",
		Title: "[H] RTX 3080 [W] $500",
	}

	alerts := []store.AlertRule{
		{
			ServerID: "guild1",
			UserID:   "user1",
			MustHave: []string{"3080"},
		},
	}

	// 2. Run
	processNewPost(ctx, mockDB, mockCache, mockAI, mockDiscord, post, alerts)

	// Since it's a void function, we would typically check if mocks were called.
	// We can add counters or flags to our mocks if we want to be more specific.
}

func TestProcessNewPost_NoMatch(t *testing.T) {
	ctx := context.Background()

	matchCalled := false
	mockDB := &MockStore{
		SavePostRecordsFn: func(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error {
			matchCalled = true
			return nil
		},
	}
	mockCache := &MockCache{}
	mockAI := &MockAI{
		CleanRedditPostFn: func(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error) {
			return &ai.CleanedPost{
				Title: "Clean Title",
			}, nil
		},
	}
	mockDiscord := &MockDiscord{}

	post := reddit.Post{ID: "t3_abc", Title: "Something else"}
	alerts := []store.AlertRule{
		{
			ServerID: "guild1",
			UserID:   "user1",
			MustHave: []string{"3080"},
		},
	}

	processNewPost(ctx, mockDB, mockCache, mockAI, mockDiscord, post, alerts)

	if matchCalled {
		t.Errorf("expected no match, but SavePostRecords was called")
	}
}
