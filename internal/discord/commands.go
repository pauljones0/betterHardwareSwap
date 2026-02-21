package discord

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

func routeSlashCommand(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	data := i.ApplicationCommandData()
	switch data.Name {
	case "setup":
		handleSetup(ctx, w, i)
	case "help":
		handleHelp(ctx, w, i)
	case "alert":
		handleAlertGroup(ctx, w, i)
	default:
		respondError(w, "Unknown command")
	}
}

func handleSetup(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	// Only allow admins to run this (Discord permissions can enforce this, but double check)
	var feedChannelID, pingChannelID string
	options := i.ApplicationCommandData().Options
	for _, opt := range options {
		if opt.Name == "feed_channel" {
			feedChannelID = opt.Value.(string)
		} else if opt.Name == "ping_channel" {
			pingChannelID = opt.Value.(string)
		}
	}

	if feedChannelID == "" || pingChannelID == "" {
		respondError(w, "Both feed_channel and ping_channel are required.")
		return
	}

	projectID := os.Getenv("GCP_PROJECT_ID")
	db, err := store.NewStore(ctx, projectID)
	if err != nil {
		respondError(w, "Database connection failed.")
		return
	}
	defer db.Close()

	cfg := store.ServerConfig{
		FeedChannelID: feedChannelID,
		PingChannelID: pingChannelID,
	}

	if err := db.SaveServerConfig(ctx, i.GuildID, cfg); err != nil {
		log.Printf("Failed to save config: %v", err)
		respondError(w, "Failed to completely save configuration.")
		return
	}

	// Say hello! Keep it simple and visible only to the person running the setup.
	// We'll let the client internally handle sending a "public" welcome message later if needed.
	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Content: fmt.Sprintf("✅ **Setup Complete!**\n\nDeals will be posted to <#%s>.\nUser Alerts will ping in <#%s>.\n\nUsers can now run `/alert add` to get started!", feedChannelID, pingChannelID),
			Flags:   discordgo.MessageFlagsEphemeral,
		},
	})

	// Send public welcome message via REST Client
	go func() {
		client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))
		client.SendMessage(pingChannelID, "👋 **Hello! Hardware Swap Bot is now online!**\nRun `/help` to see how to set up alerts for specific gear.")
	}()
}

func handleHelp(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	embed := &discordgo.MessageEmbed{
		Title:       "🛠️ Canadian Hardware Swap Bot",
		Description: "I monitor `r/CanadianHardwareSwap` every minute and ping you when your dream gear is posted!",
		Color:       0x00FF00, // Green
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:  "🔔 `/alert add`",
				Value: "Opens a form where you describe what you're looking for in plain English. My AI brain 🧠 will create an optimized search rule for you.",
			},
			{
				Name:  "📋 `/alert list`",
				Value: "Shows all your active keyword alerts and lets you delete them.",
			},
			{
				Name:  "🎯 How it works",
				Value: "1. A user posts a deal on Reddit.\n2. I clean up the post and put it in the Feed channel.\n3. If it matches your alert, I ping you in the Ping channel.",
			},
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Clean, fast, and serverless.",
		},
	}

	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds: []*discordgo.MessageEmbed{embed},
			Flags:  discordgo.MessageFlagsEphemeral, // Only the user who asked sees it
		},
	})
}

// handleAlertGroup routes the subcommands of `/alert`
func handleAlertGroup(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	options := i.ApplicationCommandData().Options
	if len(options) == 0 {
		return
	}

	subCommand := options[0].Name
	switch subCommand {
	case "add":
		handleAlertAddStart(ctx, w, i)
	case "list":
		handleAlertList(ctx, w, i)
	default:
		respondError(w, "Unknown subcommand")
	}
}

// handleAlertAddStart opens the Modal asking the user what they want
func handleAlertAddStart(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseModal,
		Data: &discordgo.InteractionResponseData{
			CustomID: "modal_alert_wizard",
			Title:    "Setup a Hardware Alert",
			Components: []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.TextInput{
							CustomID:    "text_query",
							Label:       "What are you looking for?",
							Style:       discordgo.TextInputParagraph,
							Placeholder: "e.g. A used 3080 series GPU in Toronto under $500",
							Required:    true,
							MaxLength:   300,
						},
					},
				},
			},
		},
	})
}
