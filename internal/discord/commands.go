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
		Title:       "🛡️ Better Hardware Swap Help",
		Description: "I'm your agentic companion for tracking PC gear in Canada. I scan `r/CanadianHardwareSwap` in real-time.",
		Color:       0x00FF00, // Green
		Fields: []*discordgo.MessageEmbedField{
			{
				Name:   "✨ AI-Powered Alerts",
				Value:  "Run `/alert add` and select **'Help Me Write It'**. Just describe your goal (e.g., *\"A 30-series GPU in Vancouver under $400\"*) and my AI handles the logic.",
				Inline: false,
			},
			{
				Name:   "🔧 Technical Querying",
				Value:  "Select **'I'll Type It Myself'** to use Boolean logic like `(rtx AND 4090) NOT broken`.",
				Inline: false,
			},
			{
				Name:  "📋 Management",
				Value: "Use `/alert list` to view or delete your current subscriptions.",
			},
		},
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: "https://em-content.zobj.net/source/microsoft-teams/363/shield_1f6e1-fe0f.png",
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Agentic • Serverless • Open Source",
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

// handleAlertAddStart gives the user the choice between AI assistance and manual entry.
func handleAlertAddStart(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	embed := &discordgo.MessageEmbed{
		Title:       "🛠️ Create a New Alert",
		Description: "How would you like to set up your alert?\n\n✨ **Help Me Write It**: Just tell me what you're looking for in plain English, and I'll generate the perfect match query.\n\n⌨️ **I'll Type It Myself**: If you know exactly what keywords you want (e.g., `rtx AND 4090`), you can type the query manually.",
		Color:       0x00B0F4,
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "✨ Help Me Write It",
					Style:    discordgo.PrimaryButton,
					CustomID: "wizard_ai",
				},
				discordgo.Button{
					Label:    "⌨️ I'll Type It Myself",
					Style:    discordgo.SecondaryButton,
					CustomID: "wizard_manual",
				},
			},
		},
	}

	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: components,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}
