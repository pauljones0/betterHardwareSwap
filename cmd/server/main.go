package main

import (
	"log"
	"net/http"
	"os"

	"github.com/pauljones0/betterHardwareSwap/internal/discord"
	"github.com/pauljones0/betterHardwareSwap/internal/processor"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
		log.Printf("Defaulting to port %s", port)
	}

	// Setup Discord Interactions webhook handler
	http.HandleFunc("/interactions", discord.HandleInteraction)

	// Setup Cloud Scheduler endpoint for scraping
	http.HandleFunc("/cron/scrape", processor.HandleCronScrape)

	log.Printf("Listening on port %s", port)
	if err := http.ListenAndServe(":"+port, nil); err != nil {
		log.Fatalf("Fatal: %v", err)
	}
}
