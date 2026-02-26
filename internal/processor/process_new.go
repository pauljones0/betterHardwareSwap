package processor

import (
	"context"
	"fmt"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

var (
	globalMatcher = NewMatcher()
	globalBuilder = NewDealBuilder()
)

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
	embed := globalBuilder.BuildDealEmbed(post, cleaned)

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

func dispatchToServers(ctx context.Context, cache ServerConfigGetter, client DiscordMessenger, post reddit.Post, embed *discordgo.MessageEmbed, matches map[string][]string) map[string]string {
	serverMsgs := make(map[string]string)

	for serverID, userIDs := range matches {
		cfg, err := cache.GetServerConfig(ctx, serverID)
		if err != nil {
			logger.Error(ctx, "Could not get config for server", "server_id", serverID, "error", err)
			continue
		}

		// Send to Feed Channel
		msgID, err := client.SendEmbedWithComponents(cfg.FeedChannelID, "", embed, globalBuilder.BuildDealButtons(post.URL))
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

func safeContains(corpus, substring string) bool {
	return globalMatcher.containsWord(corpus, substring)
}
