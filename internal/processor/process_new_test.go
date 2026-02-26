package processor

import (
	"context"
	"testing"

	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"github.com/pauljones0/betterHardwareSwap/internal/testutils"
	"github.com/stretchr/testify/mock"
)

func TestProcessNewPost_TableDriven(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name         string
		post         reddit.Post
		cleaned      *ai.CleanedPost
		alerts       []store.AlertRule
		serverConfig *store.ServerConfig
		expectMatch  bool
		setupMocks   func(mDB *testutils.MockStore, mAI *testutils.MockAI, mD *testutils.MockDiscord)
	}{
		{
			name: "Successful Match",
			post: reddit.Post{ID: "t3_match", Title: "[H] RTX 3080 [W] $500", SelfText: "Desc"},
			cleaned: &ai.CleanedPost{
				Title: "RTX 3080",
			},
			alerts: []store.AlertRule{
				{ServerID: "guild1", UserID: "user1", MustHave: []string{"3080"}},
			},
			serverConfig: &store.ServerConfig{FeedChannelID: "feed1", PingChannelID: "ping1"},
			expectMatch:  true,
			setupMocks: func(mDB *testutils.MockStore, mAI *testutils.MockAI, mD *testutils.MockDiscord) {
				mAI.On("CleanRedditPost", mock.Anything, "[H] RTX 3080 [W] $500", "Desc").Return(&ai.CleanedPost{Title: "RTX 3080"}, nil)
				mDB.On("GetServerConfig", mock.Anything, "guild1").Return(&store.ServerConfig{FeedChannelID: "feed1", PingChannelID: "ping1"}, nil)
				mD.On("SendEmbedWithComponents", "feed1", "", mock.Anything, mock.Anything).Return("msg123", nil)
				mD.On("AddReaction", "feed1", "msg123", mock.Anything).Return(nil).Times(2)
				mD.On("SendMessage", "ping1", mock.Anything).Return(nil)
				mDB.On("SavePostRecords", mock.Anything, "t3_match", "RTX 3080", map[string]string{"guild1": "msg123"}).Return(nil)
			},
		},
		{
			name: "No Match",
			post: reddit.Post{ID: "t3_nomatch", Title: "Something else", SelfText: "Desc"},
			cleaned: &ai.CleanedPost{
				Title: "Something else",
			},
			alerts: []store.AlertRule{
				{ServerID: "guild1", UserID: "user1", MustHave: []string{"3080"}},
			},
			expectMatch: false,
			setupMocks: func(mDB *testutils.MockStore, mAI *testutils.MockAI, mD *testutils.MockDiscord) {
				mAI.On("CleanRedditPost", mock.Anything, "Something else", "Desc").Return(&ai.CleanedPost{Title: "Something else"}, nil)
				// AssertNotCalled expectations are handled at the end
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockDB := new(testutils.MockStore)
			mockAI := new(testutils.MockAI)
			mockDiscord := new(testutils.MockDiscord)

			if tt.setupMocks != nil {
				tt.setupMocks(mockDB, mockAI, mockDiscord)
			}

			processNewPost(ctx, mockDB, mockDB, mockAI, mockDiscord, tt.post, tt.alerts)

			mockAI.AssertExpectations(t)
			mockDB.AssertExpectations(t)
			mockDiscord.AssertExpectations(t)

			if !tt.expectMatch {
				mockDiscord.AssertNotCalled(t, "SendEmbedWithComponents", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
				mockDB.AssertNotCalled(t, "SavePostRecords", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			}
		})
	}
}
