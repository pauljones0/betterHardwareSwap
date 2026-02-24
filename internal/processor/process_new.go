package processor

import (
	"context"
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

var globalMatcher = NewMatcher()

// processNewPost handles sending the post to Gemini, matching against alerts, and dispatching.
func processNewPost(ctx context.Context, db Storer, cache ServerConfigGetter, aiSvc AIService, client DiscordMessenger, post reddit.Post, alerts []store.AlertRule) {
	logger.Info(ctx, "Processing NEW post",
		"reddit_id", post.ID,
		"title", post.Title,
		"author", post.Author,
		"subreddit", post.Subreddit,
	)

	// 1. Give Gemini the messy post to clean up
	cleaned, err := aiSvc.CleanRedditPost(ctx, post.Title, post.SelfText)
	if err != nil {
		logger.Error(ctx, "Gemini failed to clean post", "reddit_id", post.ID, "error", err)
		return
	}

	// 2. Build the searchable corpus.
	corpus := cleaned.Title + " " + cleaned.Description + " " + cleaned.Location

	// 3. Match against alerts mapping ServerID -> matched users
	matches := findMatches(ctx, alerts, corpus)

	// 4. Create the beautiful Dispatch Embed
	embed := buildDealEmbed(post, cleaned)

	// 5. Dispatch!
	serverMsgs := dispatchToServers(ctx, cache, client, post, embed, matches)

	// 6. Batch save all server message IDs
	if len(serverMsgs) > 0 {
		if err := db.SavePostRecords(ctx, post.ID, cleaned.Title, serverMsgs); err != nil {
			logger.Error(ctx, "Failed to batch save post records", "reddit_id", post.ID, "error", err)
		}
	}
}

func findMatches(ctx context.Context, alerts []store.AlertRule, corpus string) map[string][]string {
	matches := make(map[string][]string) // ServerID -> array of UserIDs
	for _, alert := range alerts {
		if globalMatcher.Matches(corpus, alert.MustHave, alert.AnyOf, alert.MustNot) {
			matches[alert.ServerID] = append(matches[alert.ServerID], alert.UserID)
		}
	}

	if len(matches) > 0 {
		logger.Debug(ctx, "Alert matches found", "server_count", len(matches))
	}

	return matches
}

func buildDealEmbed(post reddit.Post, cleaned *ai.CleanedPost) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "📦 " + cleaned.Title,
		URL:         post.URL,
		Description: cleaned.Description,
		Color:       getColor(post.Score, post.NumComments),
		Fields:      []*discordgo.MessageEmbedField{},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("r/CanadianHardwareSwap • 👍 %d | 💬 %d", post.Score, post.NumComments),
		},
		Timestamp: time.Unix(int64(post.CreatedUtc), 0).Format(time.RFC3339),
	}

	if cleaned.Price != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "💰 Price",
			Value:  cleaned.Price,
			Inline: true,
		})
	}
	if cleaned.Condition != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "✨ Condition",
			Value:  cleaned.Condition,
			Inline: true,
		})
	}
	if cleaned.Location != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "📍 Location",
			Value:  cleaned.Location,
			Inline: true,
		})
	}

	if post.Thumbnail != "" && post.Thumbnail != "self" && post.Thumbnail != "default" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: post.Thumbnail}
	}

	return embed
}

func dispatchToServers(ctx context.Context, cache ServerConfigGetter, client DiscordMessenger, post reddit.Post, embed *discordgo.MessageEmbed, matches map[string][]string) map[string]string {
	serverMsgs := make(map[string]string)

	for serverID, userIDs := range matches {
		cfg, err := cache.GetServerConfig(ctx, serverID)
		if err != nil {
			logger.Error(ctx, "Could not get config for server", "server_id", serverID, "error", err)
			continue
		}

		// Send to Feed Channel
		msgID, err := client.SendEmbedWithComponents(cfg.FeedChannelID, "", embed, buildDealButtons(post.URL))
		if err == nil {
			_ = client.AddReaction(cfg.FeedChannelID, msgID, "%F0%9F%91%8D") // Thumbs up
			_ = client.AddReaction(cfg.FeedChannelID, msgID, "%F0%9F%91%8E") // Thumbs down
			serverMsgs[serverID] = msgID
		} else {
			logger.Error(ctx, "Failed to post feed to server", "server_id", serverID, "error", err)
			continue
		}

		// Send deduped Ping to Ping Channel
		if len(userIDs) > 0 {
			pingContent := ""
			for _, uid := range userIDs {
				pingContent += fmt.Sprintf("<@%s> ", uid)
			}
			pingContent += fmt.Sprintf("- **Match Found in the Deal Feed!** <https://discord.com/channels/%s/%s/%s>", serverID, cfg.FeedChannelID, msgID)

			_ = client.SendMessage(cfg.PingChannelID, pingContent)
		}
	}
	return serverMsgs
}

func buildDealButtons(url string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "🌐",
					},
					Label: "Open in Reddit",
					Style: discordgo.LinkButton,
					URL:   url,
				},
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "🔇",
					},
					Label:    "Mute Item",
					Style:    discordgo.SecondaryButton,
					CustomID: "mute_item",
				},
			},
		},
	}
}

// getColor returns a Discord hex color based on engagement heuristics.
func getColor(score, comments int) int {
	interactions := score + comments
	switch {
	case interactions >= 16:
		return 0xFF0000 // Lava Red
	case interactions >= 6:
		return 0xFFA500 // Orange
	case interactions >= 3:
		return 0xFFFF00 // Yellow
	default:
		return 0x808080 // Grey
	}
}

func safeContains(corpus, substring string) bool {
	return globalMatcher.containsWord(corpus, substring)
}
