package processor

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/discord"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
	"golang.org/x/sync/errgroup"
)

type ServerConfigGetter interface {
	GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error)
}

// Storer defines the database operations needed by the processor.
type Storer interface {
	GetAllAlerts(ctx context.Context) ([]store.AlertRule, error)
	GetPostRecord(ctx context.Context, redditID string) (*store.PostRecord, error)
	SavePostRecord(ctx context.Context, redditID, cleanedTitle, serverID, discordMsgID string) error
	SavePostRecords(ctx context.Context, redditID, cleanedTitle string, serverMsgs map[string]string) error
	TrimOldPosts(ctx context.Context) error
	GetServerConfig(ctx context.Context, serverID string) (*store.ServerConfig, error)
	Close() error
}

// AIService defines the AI operations needed by the processor.
type AIService interface {
	CleanRedditPost(ctx context.Context, rawTitle, rawBody string) (*ai.CleanedPost, error)
	Close()
}

func HandleCronScrape(w http.ResponseWriter, r *http.Request) {
	// Generate a simple request ID for the cron run
	requestID := fmt.Sprintf("cron-%d", time.Now().UnixNano())
	ctx := logger.WithRequestID(r.Context(), requestID)

	logger.Info(ctx, "Starting cron scrape pipeline")

	projectID := os.Getenv("GCP_PROJECT_ID")
	db, err := store.NewStore(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "Failed to init db", "error", err)
		http.Error(w, "Failed to init db", http.StatusInternalServerError)
		return
	}
	defer db.Close()

	aiSvc, err := ai.NewAIClient(ctx, os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		logger.Error(ctx, "Failed to init ai", "error", err)
		http.Error(w, "Failed to init ai", http.StatusInternalServerError)
		return
	}
	defer aiSvc.Close()

	scraper := reddit.NewScraper()
	discordClient := discord.NewClient(os.Getenv("DISCORD_BOT_TOKEN"))

	if err := RunPipeline(ctx, db, aiSvc, scraper, discordClient); err != nil {
		logger.Error(ctx, "Pipeline failed", "error", err)
		http.Error(w, "Pipeline failed", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusOK)
	w.Write([]byte("✅ Pipeline complete."))
}

// RunPipeline sweeps Reddit, parses via AI, checks user alerts, and dispatches to Discord.
func RunPipeline(ctx context.Context, db Storer, aiSvc AIService, scraper *reddit.Scraper, discordClient *discord.Client) error {

	posts, err := scraper.FetchNewestPosts(ctx)
	if err != nil {
		// If Reddit is down, we could DM the admin here. For simplicity in V1, we just return the error.
		return fmt.Errorf("failed to fetch reddit: %w", err)
	}

	// 1. Fetch all user keywords in one shot
	alerts, err := db.GetAllAlerts(ctx)
	if err != nil {
		return fmt.Errorf("failed to load alerts: %w", err)
	}

	// 2. Fetch server routing configs (using a TTL cache)
	cache := NewConfigCache(db, 5*time.Minute)

	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(10) // Process max 10 posts concurrently to stay within API quotas

	for _, p := range posts {
		post := p // closure capture
		g.Go(func() error {
			// Check if we've seen this post
			record, err := db.GetPostRecord(ctx, post.ID)

			isNew := (record == nil || err != nil)

			// If it's closed/sold or deleted, handle updates.
			if !isNew {
				err = handleExistingPostStatus(ctx, cache, discordClient, post, record)
				if err != nil {
					logger.Warn(ctx, "Failed to update status", "reddit_id", post.ID, "error", err)
				}
				return nil
			}

			// Only process NEW posts that are not deleted/removed instantly
			if isNew && post.RemovedByByCategory == "" && !strings.EqualFold(post.LinkFlairText, "Sold") && !strings.EqualFold(post.LinkFlairText, "Closed") {
				processNewPost(ctx, db, cache, aiSvc, discordClient, post, alerts)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return fmt.Errorf("parallel processing error: %w", err)
	}

	// 3. Trim DB to prevent unlimited growth
	if err := db.TrimOldPosts(ctx); err != nil {
		logger.Warn(ctx, "Non-fatal: failed to trim old posts", "error", err)
	}

	logger.Info(ctx, "Pipeline finished successfully")
	return nil
}

func handleExistingPostStatus(ctx context.Context, cache ServerConfigGetter, client *discord.Client, post reddit.Post, record *store.PostRecord) error {
	// If the post was sold or closed
	if strings.EqualFold(post.LinkFlairText, "Sold") || strings.EqualFold(post.LinkFlairText, "Closed") {
		logger.Info(ctx, "Detected SOLD/CLOSED post, updating messages", "reddit_id", post.ID, "count", len(record.ServerMsgs))

		for serverID, msgID := range record.ServerMsgs {
			cfg, err := cache.GetServerConfig(ctx, serverID)
			if err != nil {
				logger.Warn(ctx, "Could not get config for server during update", "server_id", serverID, "error", err)
				continue
			}

			// Construct a greyed out, struck-through version of the original deal
			embed := &discordgo.MessageEmbed{
				Title:       "~~" + record.CleanedTitle + "~~",
				URL:         post.URL,
				Description: fmt.Sprintf("This deal has been marked as **%s** on Reddit.", post.LinkFlairText),
				Color:       0x2C2F33, // Discord Darker Grey
				Footer: &discordgo.MessageEmbedFooter{
					Text: "Deal Closed",
				},
			}

			err = client.EditEmbed(cfg.FeedChannelID, msgID, "", embed)
			if err != nil {
				logger.Error(ctx, "Failed to edit message", "server_id", serverID, "msg_id", msgID, "error", err)
			}
		}
	}

	// If the post was deleted by user/mods
	if post.RemovedByByCategory != "" {
		logger.Info(ctx, "Detected DELETED post", "reddit_id", post.ID, "category", post.RemovedByByCategory)
	}

	return nil
}
