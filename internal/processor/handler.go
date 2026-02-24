package processor

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/discord"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// HandleCronScrape is the HTTP handler invoked by Cloud Scheduler.
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
