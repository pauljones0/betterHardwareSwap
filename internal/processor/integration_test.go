package processor

import (
	"context"
	"errors"
	"testing"

	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"github.com/stretchr/testify/mock"
)

func TestRunPipeline_Integration_Success(t *testing.T) {
	ctx := context.Background()

	mockDB := new(MockStore)
	mockAI := new(MockAI)
	mockScraper := new(MockScraper)
	mockDiscord := new(MockDiscord)

	// 1. Setup Data
	var post reddit.Post
	_ = loadFixture("reddit_post.json", &post)
	post.ID = "pipe_1"
	post.Title = "[H] RTX 3080 [W] $500"

	alerts := []store.AlertRule{
		{
			ServerID: "guild_int",
			UserID:   "user_int",
			MustHave: []string{"3080"},
		},
	}

	cleaned := &ai.CleanedPost{
		Title: "RTX 3080",
	}

	serverConfig := &store.ServerConfig{
		FeedChannelID: "feed_int",
		PingChannelID: "ping_int",
	}

	// 2. Setup Mock Expectations for the full flow
	mockScraper.On("FetchNewestPosts", ctx).Return([]reddit.Post{post}, nil)
	mockDB.On("GetAllAlerts", ctx).Return(alerts, nil)
	mockDB.On("GetPostRecord", mock.Anything, "pipe_1").Return(nil, nil) // New post

	// processNewPost flow
	mockAI.On("CleanRedditPost", mock.Anything, post.Title, post.SelfText).Return(cleaned, nil)
	mockDB.On("GetServerConfig", mock.Anything, "guild_int").Return(serverConfig, nil)
	mockDiscord.On("SendEmbedWithComponents", "feed_int", "", mock.Anything, mock.Anything).Return("discord_msg_1", nil)
	mockDiscord.On("AddReaction", "feed_int", "discord_msg_1", mock.Anything).Return(nil).Times(2)
	mockDiscord.On("SendMessage", "ping_int", mock.Anything).Return(nil)
	mockDB.On("SavePostRecords", mock.Anything, "pipe_1", cleaned.Title, map[string]string{"guild_int": "discord_msg_1"}).Return(nil)

	// Cleanup flow
	mockDB.On("TrimOldPosts", mock.Anything).Return(nil)

	// 3. Run
	err := RunPipeline(ctx, mockDB, mockAI, mockScraper, mockDiscord)

	// 4. Assertions
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	mockScraper.AssertExpectations(t)
	mockDB.AssertExpectations(t)
	mockAI.AssertExpectations(t)
	mockDiscord.AssertExpectations(t)
}

func TestRunPipeline_Integration_RedditFailure(t *testing.T) {
	ctx := context.Background()

	mockDB := new(MockStore)
	mockAI := new(MockAI)
	mockScraper := new(MockScraper)
	mockDiscord := new(MockDiscord)

	mockScraper.On("FetchNewestPosts", ctx).Return([]reddit.Post(nil), errors.New("reddit down"))

	err := RunPipeline(ctx, mockDB, mockAI, mockScraper, mockDiscord)

	if err == nil {
		t.Error("expected error when reddit is down, got nil")
	}
}

func TestRunPipeline_Integration_NoPosts(t *testing.T) {
	ctx := context.Background()

	mockDB := new(MockStore)
	mockAI := new(MockAI)
	mockScraper := new(MockScraper)
	mockDiscord := new(MockDiscord)

	mockScraper.On("FetchNewestPosts", ctx).Return([]reddit.Post{}, nil)
	mockDB.On("GetAllAlerts", ctx).Return([]store.AlertRule{}, nil)
	mockDB.On("TrimOldPosts", mock.Anything).Return(nil)

	err := RunPipeline(ctx, mockDB, mockAI, mockScraper, mockDiscord)

	if err != nil {
		t.Errorf("expected no error for empty posts, got %v", err)
	}
}

func TestRunPipeline_PartialFailure(t *testing.T) {
	ctx := context.Background()

	mockDB := new(MockStore)
	mockAI := new(MockAI)
	mockScraper := new(MockScraper)
	mockDiscord := new(MockDiscord)

	p1 := reddit.Post{ID: "p1", Title: "Post 1 (Fail)"}
	p2 := reddit.Post{ID: "p2", Title: "Post 2 (Success)"}

	alerts := []store.AlertRule{{ServerID: "g1", MustHave: []string{"Success"}}}
	serverConfig := &store.ServerConfig{FeedChannelID: "f1"}

	// 1. Scraper returns two posts
	mockScraper.On("FetchNewestPosts", ctx).Return([]reddit.Post{p1, p2}, nil)
	mockDB.On("GetAllAlerts", ctx).Return(alerts, nil)

	// 2. Post 1 fails AI cleaning
	mockDB.On("GetPostRecord", mock.Anything, "p1").Return(nil, nil)
	mockAI.On("CleanRedditPost", mock.Anything, p1.Title, p1.SelfText).Return(nil, errors.New("ai error"))

	// 3. Post 2 succeeds
	mockDB.On("GetPostRecord", mock.Anything, "p2").Return(nil, nil)
	mockAI.On("CleanRedditPost", mock.Anything, p2.Title, p2.SelfText).Return(&ai.CleanedPost{Title: "Success"}, nil)
	mockDB.On("GetServerConfig", mock.Anything, "g1").Return(serverConfig, nil)
	mockDiscord.On("SendEmbedWithComponents", "f1", "", mock.Anything, mock.Anything).Return("m2", nil)
	mockDiscord.On("AddReaction", "f1", "m2", mock.Anything).Return(nil).Times(2)
	mockDiscord.On("SendMessage", mock.Anything, mock.Anything).Return(nil)
	mockDB.On("SavePostRecords", mock.Anything, "p2", "Success", mock.Anything).Return(nil)

	// 4. Cleanup
	mockDB.On("TrimOldPosts", mock.Anything).Return(nil)

	err := RunPipeline(ctx, mockDB, mockAI, mockScraper, mockDiscord)

	// We expect NO error from RunPipeline even if a sub-task (processNewPost) failed its AI call,
	// because processNewPost handles its own errors and logs them (void function).
	if err != nil {
		t.Errorf("expected pipeline to absorb sub-errors, got %v", err)
	}
	mockAI.AssertExpectations(t)
	mockDiscord.AssertCalled(t, "SendEmbedWithComponents", "f1", "", mock.Anything, mock.Anything)
}
