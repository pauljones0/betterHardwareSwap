package testutils

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"github.com/stretchr/testify/mock"
)

// LoadFixture loads a JSON file from the test/fixtures directory relative to the project root.
func LoadFixture(filename string, v interface{}) error {
	_, b, _, _ := runtime.Caller(0)
	// runtime.Caller(0) will give the path to this file: internal/testutils/testutils.go
	// So we go up 2 levels to reach the root.
	basepath := filepath.Dir(filepath.Dir(filepath.Dir(b)))
	path := filepath.Join(basepath, "test", "fixtures", filename)
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// MockStore implements store interfaces using testify/mock
type MockStore struct {
	mock.Mock
}

func (m *MockStore) AddAlert(ctx context.Context, rule store.AlertRule) error {
	args := m.Called(ctx, rule)
	return args.Error(0)
}

func (m *MockStore) GetUserAlerts(ctx context.Context, serverID, userID string) ([]store.AlertRule, error) {
	args := m.Called(ctx, serverID, userID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]store.AlertRule), args.Error(1)
}

func (m *MockStore) DeleteAlert(ctx context.Context, docID string) error {
	args := m.Called(ctx, docID)
	return args.Error(0)
}

func (m *MockStore) DeleteAllUserAlerts(ctx context.Context, serverID, userID string) error {
	args := m.Called(ctx, serverID, userID)
	return args.Error(0)
}

func (m *MockStore) GetAllAlerts(ctx context.Context) ([]store.AlertRule, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]store.AlertRule), args.Error(1)
}

func (m *MockStore) GetPostRecord(ctx context.Context, redditID string) (*store.PostRecord, error) {
	args := m.Called(ctx, redditID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*store.PostRecord), args.Error(1)
}

func (m *MockStore) SavePostRecord(ctx context.Context, redditID, cleanedTitle, serverID, discordMsgID string) error {
	args := m.Called(ctx, redditID, cleanedTitle, serverID, discordMsgID)
	return args.Error(0)
}

func (m *MockStore) SavePostRecords(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error {
	args := m.Called(ctx, redditID, cleanedTitle, serverMsgs)
	return args.Error(0)
}

func (m *MockStore) TrimOldPosts(ctx context.Context) error {
	return m.Called(ctx).Error(0)
}

func (m *MockStore) GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error) {
	args := m.Called(ctx, serverID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*store.ServerConfig), args.Error(1)
}

func (m *MockStore) SaveServerConfig(ctx context.Context, serverID string, cfg store.ServerConfig) error {
	args := m.Called(ctx, serverID, cfg)
	return args.Error(0)
}

func (m *MockStore) SaveAnalytics(ctx context.Context, record store.AnalyticsRecord) error {
	args := m.Called(ctx, record)
	return args.Error(0)
}

func (m *MockStore) GetUnprocessedAnalyticsByFlow(ctx context.Context, flowType string, limit int) ([]store.AnalyticsRecord, error) {
	args := m.Called(ctx, flowType, limit)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]store.AnalyticsRecord), args.Error(1)
}

func (m *MockStore) DeleteAnalyticsChunk(ctx context.Context, ids []string) error {
	args := m.Called(ctx, ids)
	return args.Error(0)
}

func (m *MockStore) GetSystemPrompt(ctx context.Context, key string) (string, error) {
	args := m.Called(ctx, key)
	return args.String(0), args.Error(1)
}

func (m *MockStore) SetSystemPrompt(ctx context.Context, key, promptText string) error {
	args := m.Called(ctx, key, promptText)
	return args.Error(0)
}

func (m *MockStore) Close() error {
	return m.Called().Error(0)
}

// MockAI implements AI interface using testify/mock
type MockAI struct {
	mock.Mock
}

func (m *MockAI) CleanRedditPost(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error) {
	args := m.Called(ctx, rawTitle, rawBody)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.CleanedPost), args.Error(1)
}

func (m *MockAI) RunKeywordWizard(ctx context.Context, userRequest, promptOverride string) (*ai.KeywordWizardResponse, error) {
	args := m.Called(ctx, userRequest, promptOverride)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.KeywordWizardResponse), args.Error(1)
}

func (m *MockAI) ValidateManualQuery(ctx context.Context, userQuery, promptOverride string) (*ai.KeywordWizardResponse, error) {
	args := m.Called(ctx, userQuery, promptOverride)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*ai.KeywordWizardResponse), args.Error(1)
}

func (m *MockAI) Close() {
	m.Called()
}

// MockDiscord implements Discord interface using testify/mock
type MockDiscord struct {
	mock.Mock
}

func (m *MockDiscord) SendMessage(channelID, content string) error {
	return m.Called(channelID, content).Error(0)
}

func (m *MockDiscord) SendEmbed(channelID string, content string, embed *discordgo.MessageEmbed) (string, error) {
	args := m.Called(channelID, content, embed)
	return args.String(0), args.Error(1)
}

func (m *MockDiscord) SendEmbedWithComponents(channelID string, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (string, error) {
	args := m.Called(channelID, content, embed, components)
	return args.String(0), args.Error(1)
}

func (m *MockDiscord) EditEmbed(channelID, messageID, content string, embed *discordgo.MessageEmbed) error {
	return m.Called(channelID, messageID, content, embed).Error(0)
}

func (m *MockDiscord) AddReaction(channelID, messageID, emoji string) error {
	return m.Called(channelID, messageID, emoji).Error(0)
}

func (m *MockDiscord) SendFollowupMessage(i *discordgo.Interaction, content string) error {
	return m.Called(i, content).Error(0)
}

func (m *MockDiscord) SendFollowupEmbedWithComponents(i *discordgo.Interaction, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	return m.Called(i, embed, components).Error(0)
}

func (m *MockDiscord) CreateDM(userID string) (string, error) {
	args := m.Called(userID)
	return args.String(0), args.Error(1)
}

func (m *MockDiscord) SendAdminApprovalDM(adminID, newPrompt, flowType string) error {
	return m.Called(adminID, newPrompt, flowType).Error(0)
}

func (m *MockDiscord) SendFallbackAdminApproval(channelID, adminID, newPrompt, flowType string) error {
	return m.Called(channelID, adminID, newPrompt, flowType).Error(0)
}

// MockScraper implements reddit interface using testify/mock
type MockScraper struct {
	mock.Mock
}

func (m *MockScraper) FetchNewestPosts(ctx context.Context) ([]reddit.Post, error) {
	args := m.Called(ctx)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]reddit.Post), args.Error(1)
}
