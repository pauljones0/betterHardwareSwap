package processor

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/discord"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// processNewPost handles sending the post to Gemini, matching against alerts, and dispatching.
func processNewPost(ctx context.Context, db Storer, aiSvc AIService, client *discord.Client, post reddit.Post, alerts []store.AlertRule) {
	logger.Debug(ctx, "Processing NEW post", "reddit_id", post.ID, "title", post.Title)

	// 1. Give Gemini the messy post to clean up
	cleaned, err := aiSvc.CleanRedditPost(ctx, post.Title, post.SelfText)
	if err != nil {
		logger.Error(ctx, "Gemini failed to clean post", "reddit_id", post.ID, "error", err)
		return
	}

	// 2. Build the searchable corpus. We'll search against the cleaned title and description.
	corpus := strings.ToLower(cleaned.Title + " " + cleaned.Description + " " + cleaned.Location)

	// 3. Match against alerts mapping ServerID -> matched users
	matches := make(map[string][]string) // ServerID -> array of UserIDs
	for _, alert := range alerts {
		if ruleMatches(alert, corpus) {
			matches[alert.ServerID] = append(matches[alert.ServerID], alert.UserID)
		}
	}

	// If nobody matched we still might want to post it to the deal feed if the admin configured a feed.
	// But let's check what servers want to receive feeds at all.
	// For simplicity in this scaffold, let's just get the one server.

	// Since we only know about servers from the Alerts list (or we can query all servers),
	// here we query all server configs to see where to post.

	// A better way at scale is to keep server configs in memory (with TTL) or fetch only the ones we need.
	// Here, we'll just mock pulling a single server configuration assuming a small bot, or assume
	// we just loop `matches`.

	// Let's assume we post it to the feed ANYWAY (because feeds are feeds)
	// and ping the matched users in the ping channel.

	// A real production app would query the DB for `SELECT * FROM servers`
	// and post to every server's feed_channel.

	// Let's create the beautiful Dispatch Embed
	embed := &discordgo.MessageEmbed{
		Title:       "📦 " + cleaned.Title,
		URL:         post.URL, // Click title to go to reddit
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

	// 4. Dispatch! (Since we don't have a `GetAllServers` func right now, we'll use the matches)
	// If you want it to post even when no matches happen, add `GetAllServers` to store.

	serverMsgs := make(map[string]string)

	for serverID, userIDs := range matches {
		cfg, err := db.GetServerConfig(ctx, serverID)
		if err != nil {
			logger.Error(ctx, "Could not get config for server", "server_id", serverID, "error", err)
			continue
		}

		// Send to Feed Channel
		msgID, err := client.SendEmbed(cfg.FeedChannelID, "", embed)
		if err == nil {
			// Add default reaction voting
			_ = client.AddReaction(cfg.FeedChannelID, msgID, "%F0%9F%91%8D") // Thumbs up
			_ = client.AddReaction(cfg.FeedChannelID, msgID, "%F0%9F%91%8E") // Thumbs down

			// Track mapping for batched save
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

	// 5. Batch save all server message IDs
	if len(serverMsgs) > 0 {
		if err := db.SavePostRecords(ctx, post.ID, cleaned.Title, serverMsgs); err != nil {
			logger.Error(ctx, "Failed to batch save post records", "reddit_id", post.ID, "error", err)
		}
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

// ruleMatches runs the Boolean validation logic against the text corpus.
func ruleMatches(rule store.AlertRule, corpus string) bool {
	// First, if it has any MustNot words, it fails instantly.
	for _, word := range rule.MustNot {
		if safeContains(corpus, word) {
			return false
		}
	}

	// Next, if it has MustHave words, EVERY single one must be present.
	for _, word := range rule.MustHave {
		if !safeContains(corpus, word) {
			return false
		}
	}

	// Lastly, if it has AnyOf words, AT LEAST ONE must be present.
	if len(rule.AnyOf) > 0 {
		matchedAny := false
		for _, word := range rule.AnyOf {
			if safeContains(corpus, word) {
				matchedAny = true
				break
			}
		}
		if !matchedAny {
			return false
		}
	}

	// Passed all checks!
	return true
}

func safeContains(corpus, substring string) bool {
	return strings.Contains(corpus, strings.ToLower(strings.TrimSpace(substring)))
}
