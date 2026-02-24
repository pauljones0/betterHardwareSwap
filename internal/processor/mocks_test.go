package processor

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"github.com/stretchr/testify/mock"
)

// MockStore implements Storer using testify/mock
type MockStore struct {
	mock.Mock
}

func (m *MockStore) GetAllAlerts(ctx context.Context) ([]store.AlertRule, error) {
	args := m.Called(ctx)
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
func (m *MockStore) Close() error {
	return m.Called().Error(0)
}

// MockAI implements AIService using testify/mock
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
func (m *MockAI) Close() {
	m.Called()
}

// MockDiscord implements DiscordMessenger using testify/mock
type MockDiscord struct {
	mock.Mock
}

func (m *MockDiscord) SendEmbedWithComponents(channelID string, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (string, error) {
	args := m.Called(channelID, content, embed, components)
	return args.String(0), args.Error(1)
}
func (m *MockDiscord) AddReaction(channelID, messageID, emoji string) error {
	return m.Called(channelID, messageID, emoji).Error(0)
}
func (m *MockDiscord) SendMessage(channelID, content string) error {
	return m.Called(channelID, content).Error(0)
}
func (m *MockDiscord) EditEmbed(channelID, messageID, content string, embed *discordgo.MessageEmbed) error {
	return m.Called(channelID, messageID, content, embed).Error(0)
}

// MockScraper implements Scraper using testify/mock
type MockScraper struct {
	mock.Mock
}

func (m *MockScraper) FetchNewestPosts(ctx context.Context) ([]reddit.Post, error) {
	args := m.Called(ctx)
	return args.Get(0).([]reddit.Post), args.Error(1)
}

// MockCache implements ServerConfigGetter using testify/mock
type MockCache struct {
	mock.Mock
}

func (m *MockCache) GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error) {
	args := m.Called(ctx, serverID)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*store.ServerConfig), args.Error(1)
}
