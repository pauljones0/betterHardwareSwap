package discord

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/logger"
)

// Global discord session for handling Webhook interaction payloads types.
// We don't actually use this session to connect a websocket, just to utilize their struct definitions.
var (
	session       *discordgo.Session
	globalLimiter = NewRateLimiter()
)

func init() {
	var err error
	session, err = discordgo.New("")
	if err != nil {
		log.Fatalf("Error creating discord session for types: %v", err)
	}
}

// HandleInteraction is the main HTTP endpoint hit by Discord for every slash command, button click, and modal submit.
// It verifies the cryptographic signature to ensure the request is actually from Discord.
func HandleInteraction(w http.ResponseWriter, r *http.Request) {
	pubKey := os.Getenv("DISCORD_PUBLIC_KEY")
	if pubKey == "" {
		log.Println("DISCORD_PUBLIC_KEY is not set")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	// The discord API expects the public key as an ed25519.PublicKey object.
	decodedKey, err := hex.DecodeString(pubKey)
	if err != nil {
		log.Printf("Failed to decode public key: %v", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	if len(decodedKey) != ed25519.PublicKeySize {
		log.Printf("Invalid public key length")
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	edKey := ed25519.PublicKey(decodedKey)

	// 1. Verify the signature
	verified := discordgo.VerifyInteraction(r, edKey)
	if !verified {
		log.Println("Interaction verification failed")
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// 2. Read the body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		log.Printf("Error reading body: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// 3. Parse the Interaction
	var interaction discordgo.Interaction
	if err := json.Unmarshal(body, &interaction); err != nil {
		log.Printf("Error unmarshaling interaction: %v", err)
		http.Error(w, "Bad Request", http.StatusBadRequest)
		return
	}

	// 4. Handle PING (Discord requires this during initial app setup)
	if interaction.Type == discordgo.InteractionPing {
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponsePong,
		})
		return
	}

	ctx := logger.WithRequestID(r.Context(), interaction.ID)

	// Rate limiting check
	userID := ""
	if interaction.Member != nil && interaction.Member.User != nil {
		userID = interaction.Member.User.ID
	} else if interaction.User != nil {
		userID = interaction.User.ID
	}

	if userID != "" && !globalLimiter.Allow(userID) {
		logger.Warn(ctx, "Rate limit exceeded for user", "user_id", userID)
		respondError(w, "You are doing that too fast! Please wait a few seconds.")
		return
	}

	logger.Info(ctx, "Handling Discord interaction", "type", interaction.Type, "user", userID)

	// 5. Route to appropriate handler
	handleInteractionEvent(ctx, w, &interaction)
}

func handleInteractionEvent(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	switch i.Type {
	case discordgo.InteractionApplicationCommand:
		routeSlashCommand(ctx, w, i)
	case discordgo.InteractionMessageComponent:
		routeComponentInteraction(ctx, w, i)
	case discordgo.InteractionModalSubmit:
		routeModalSubmit(ctx, w, i)
	default:
		log.Printf("Unknown interaction type: %v", i.Type)
		http.Error(w, "Unknown Type", http.StatusBadRequest)
	}
}

// Helper to write a JSON response quickly
func writeJSON(w http.ResponseWriter, resp discordgo.InteractionResponse) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		log.Printf("Error encoding response: %v", err)
	}
}

// Helper to respond with an ephemeral error message
func respondError(w http.ResponseWriter, msg string) {
	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("⚠️ Error: %s", msg),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})
}
