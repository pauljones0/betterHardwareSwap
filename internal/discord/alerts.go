package discord

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// handleAlertList fetches a user's alerts and displays them with inline delete buttons.
func handleAlertList(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err != nil {
		respondError(w, "Database connection error.")
		return
	}
	defer db.Close()

	userID := i.Member.User.ID
	if userID == "" {
		respondError(w, "Could not identify user.")
		return
	}

	alerts, err := db.GetUserAlerts(ctx, i.GuildID, userID)
	if err != nil {
		log.Printf("Error fetching user alerts for user %s: %v", userID, err)
		respondError(w, "Failed to load alerts.")
		return
	}

	if len(alerts) == 0 {
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You don't have any active alerts setup for this server.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})
		return
	}

	var rows []discordgo.MessageComponent
	desc := ""
	for idx, a := range alerts {
		if idx >= 4 {
			desc += "\n*...and more.*"
			break
		}
		desc += fmt.Sprintf("**Alert #%d:** \"%s\"\n", idx+1, a.RawQuery)
		btnRow := discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    fmt.Sprintf("🗑️ Delete #%d", idx+1),
					Style:    discordgo.SecondaryButton,
					CustomID: "delete_alert|" + a.ID,
				},
			},
		}
		rows = append(rows, btnRow)
	}

	rows = append(rows, discordgo.ActionsRow{
		Components: []discordgo.MessageComponent{
			discordgo.Button{
				Label:    "🚨 Delete All",
				Style:    discordgo.DangerButton,
				CustomID: "delete_all_alerts|",
			},
		},
	})

	embed := &discordgo.MessageEmbed{
		Title:       "📋 Your Active Alerts",
		Description: desc,
		Color:       0x00B0F4,
	}

	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Embeds:     []*discordgo.MessageEmbed{embed},
			Components: rows,
			Flags:      discordgo.MessageFlagsEphemeral,
		},
	})
}

func triggerCompaction(serverID string) {
	ctx := context.Background()
	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err != nil {
		return
	}
	defer db.Close()

	aiSvc, err := ai.NewAIClient(ctx, os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		return
	}
	defer aiSvc.Close()

	client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))
	adminID := os.Getenv("ADMIN_USER_ID")

	flows := []string{"wizard", "manual"}
	for _, flowType := range flows {
		records, err := db.GetUnprocessedAnalyticsByFlow(ctx, flowType, 20)
		if err != nil || len(records) < 20 {
			continue
		}

		sysPrompt, _ := db.GetSystemPrompt(ctx, flowType+"_prompt")
		if sysPrompt == "" {
			if flowType == "wizard" {
				sysPrompt = ai.DefaultWizardPrompt
			} else {
				sysPrompt = ai.DefaultManualPrompt
			}
		}

		result, err := aiSvc.RunCompaction(ctx, records, sysPrompt, flowType)
		if err != nil || result == nil {
			log.Printf("Compaction failed for %s: %v", flowType, err)
			continue
		}

		if adminID == "" {
			continue
		}

		err = client.SendAdminApprovalDM(adminID, result.NewPrompt, flowType)
		if err != nil && serverID != "" {
			cfg, _ := db.GetServerConfig(ctx, serverID)
			if cfg != nil && cfg.PingChannelID != "" {
				_ = client.SendFallbackAdminApproval(cfg.PingChannelID, adminID, result.NewPrompt, flowType)
			}
		}
	}
}
