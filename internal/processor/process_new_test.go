package processor

import (
	"context"
	"testing"

	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"github.com/stretchr/testify/mock"
)

func TestProcessNewPost(t *testing.T) {
	ctx := context.Background()

	// 1. Setup Mocks
	mockDB := new(MockStore)
	mockCache := new(MockCache)
	mockAI := new(MockAI)
	mockDiscord := new(MockDiscord)

	var post reddit.Post
	if err := loadFixture("reddit_post.json", &post); err != nil {
		t.Fatalf("failed to load fixture: %v", err)
	}
	// Override some fields for specific test control, but using the fixture as base
	post.ID = "t3_abc"
	post.Title = "[H] RTX 3080 [W] $500"

	cleaned := &ai.CleanedPost{
		Title:       "RTX 3080",
		Description: "Clean Desc",
		Price:       "$500",
		Location:    "Toronto",
	}

	alerts := []store.AlertRule{
		{
			ServerID: "guild1",
			UserID:   "user1",
			MustHave: []string{"3080"},
		},
	}

	serverConfig := &store.ServerConfig{
		FeedChannelID: "feed1",
		PingChannelID: "ping1",
	}

	// Expectation settings
	mockAI.On("CleanRedditPost", ctx, post.Title, post.SelfText).Return(cleaned, nil)
	mockCache.On("GetServerConfig", ctx, "guild1").Return(serverConfig, nil)
	mockDiscord.On("SendEmbedWithComponents", "feed1", "", mock.Anything, mock.Anything).Return("msg123", nil)
	mockDiscord.On("AddReaction", "feed1", "msg123", mock.Anything).Return(nil).Times(2)
	mockDiscord.On("SendMessage", "ping1", mock.Anything).Return(nil)
	mockDB.On("SavePostRecords", ctx, post.ID, cleaned.Title, map[string]string{"guild1": "msg123"}).Return(nil)

	// 2. Run
	processNewPost(ctx, mockDB, mockCache, mockAI, mockDiscord, post, alerts)

	// 3. Assertions
	mockAI.AssertExpectations(t)
	mockCache.AssertExpectations(t)
	mockDiscord.AssertExpectations(t)
	mockDB.AssertExpectations(t)
}

func TestProcessNewPost_NoMatch(t *testing.T) {
	ctx := context.Background()

	mockDB := new(MockStore)
	mockCache := new(MockCache)
	mockAI := new(MockAI)
	mockDiscord := new(MockDiscord)

	post := reddit.Post{ID: "t3_abc", Title: "Something else"}
	cleaned := &ai.CleanedPost{Title: "Clean Title"}
	alerts := []store.AlertRule{
		{
			ServerID: "guild1",
			UserID:   "user1",
			MustHave: []string{"3080"},
		},
	}

	mockAI.On("CleanRedditPost", ctx, post.Title, post.SelfText).Return(cleaned, nil)
	// We expect NO calls to Discord or Cache or DB.SavePostRecords if no match occurs.

	// 2. Run
	processNewPost(ctx, mockDB, mockCache, mockAI, mockDiscord, post, alerts)

	// 3. Assertions
	mockAI.AssertExpectations(t)
	mockDiscord.AssertNotCalled(t, "SendEmbedWithComponents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	mockDB.AssertNotCalled(t, "SavePostRecords", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}
