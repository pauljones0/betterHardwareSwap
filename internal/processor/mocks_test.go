package processor

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// MockStore implements Storer
type MockStore struct {
	GetAllAlertsFn    func(ctx context.Context) ([]store.AlertRule, error)
	GetPostRecordFn   func(ctx context.Context, redditID string) (*store.PostRecord, error)
	SavePostRecordFn  func(ctx context.Context, redditID, cleanedTitle, serverID, discordMsgID string) error
	SavePostRecordsFn func(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error
	TrimOldPostsFn    func(ctx context.Context) error
	GetServerConfigFn func(ctx context.Context, serverID string) (*store.ServerConfig, error)
	CloseFn           func() error
}

func (m *MockStore) GetAllAlerts(ctx context.Context) ([]store.AlertRule, error) {
	return m.GetAllAlertsFn(ctx)
}
func (m *MockStore) GetPostRecord(ctx context.Context, redditID string) (*store.PostRecord, error) {
	return m.GetPostRecordFn(ctx, redditID)
}
func (m *MockStore) SavePostRecord(ctx context.Context, redditID, cleanedTitle, serverID, discordMsgID string) error {
	return m.SavePostRecordFn(ctx, redditID, cleanedTitle, serverID, discordMsgID)
}
func (m *MockStore) SavePostRecords(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error {
	return m.SavePostRecordsFn(ctx, redditID, cleanedTitle, serverMsgs)
}
func (m *MockStore) TrimOldPosts(ctx context.Context) error { return m.TrimOldPostsFn(ctx) }
func (m *MockStore) GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error) {
	return m.GetServerConfigFn(ctx, serverID)
}
func (m *MockStore) Close() error { return m.CloseFn() }

// MockAI implements AIService
type MockAI struct {
	CleanRedditPostFn func(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error)
	CloseFn           func()
}

func (m *MockAI) CleanRedditPost(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error) {
	return m.CleanRedditPostFn(ctx, rawTitle, rawBody)
}
func (m *MockAI) Close() { m.CloseFn() }

// MockDiscord implements DiscordMessenger
type MockDiscord struct {
	SendEmbedWithComponentsFn func(channelID string, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (string, error)
	AddReactionFn             func(channelID, messageID, emoji string) error
	SendMessageFn             func(channelID, content string) error
	EditEmbedFn               func(channelID, messageID, content string, embed *discordgo.MessageEmbed) error
}

func (m *MockDiscord) SendEmbedWithComponents(channelID string, content string, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) (string, error) {
	return m.SendEmbedWithComponentsFn(channelID, content, embed, components)
}
func (m *MockDiscord) AddReaction(channelID, messageID, emoji string) error {
	return m.AddReactionFn(channelID, messageID, emoji)
}
func (m *MockDiscord) SendMessage(channelID, content string) error {
	return m.SendMessageFn(channelID, content)
}
func (m *MockDiscord) EditEmbed(channelID, messageID, content string, embed *discordgo.MessageEmbed) error {
	return m.EditEmbedFn(channelID, messageID, content, embed)
}

// MockScraper implements Scraper
type MockScraper struct {
	FetchNewestPostsFn func(ctx context.Context) ([]reddit.Post, error)
}

func (m *MockScraper) FetchNewestPosts(ctx context.Context) ([]reddit.Post, error) {
	return m.FetchNewestPostsFn(ctx)
}

// MockCache implements ServerConfigGetter
type MockCache struct {
	GetServerConfigFn func(ctx context.Context, serverID string) (*store.ServerConfig, error)
}

func (m *MockCache) GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error) {
	return m.GetServerConfigFn(ctx, serverID)
}
