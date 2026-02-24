package discord

import (
	"context"
	"net/http"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// routeComponentInteraction handles Button Clicks and select menu interactions (Confirm/Cancel AI rules, Delete Alerts).
func routeComponentInteraction(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	data := i.MessageComponentData()
	parts := strings.Split(data.CustomID, "|")
	action := parts[0]

	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err != nil {
		respondError(w, "Database connection failed")
		return
	}
	defer db.Close()

	switch action {
	case "wizard_ai":
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_alert_wizard_ai",
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

	case "wizard_manual":
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_alert_wizard_manual",
				Title:    "Manual Alert Entry",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:  "text_title",
								Label:     "Name your alert (e.g., Cheap 4090)",
								Style:     discordgo.TextInputShort,
								Required:  true,
								MaxLength: 50,
							},
						},
					},
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "text_query",
								Label:       "Query Syntax",
								Style:       discordgo.TextInputParagraph,
								Placeholder: "(rtx AND 4090) NOT (broken)",
								Required:    true,
								MaxLength:   150,
							},
						},
					},
				},
			},
		})

	case "confirm_alert":
		flow := "wizard"
		if len(parts) > 2 {
			if parts[2] == "Manual" {
				flow = "manual"
			}
		}
		_ = db.SaveAnalytics(ctx, store.AnalyticsRecord{
			FlowType:  flow,
			Outcome:   "Accepted_" + flow,
			EditCount: 0,
		})
		go triggerCompaction(i.GuildID)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "✨ **Alert Saved Successfully!**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "mute_item":
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "🔇 **Feature coming soon!** Soon you'll be able to mute specific items directly from the feed.",
				Flags:   discordgo.MessageFlagsEphemeral,
			},
		})

	case "cancel_alert":
		if len(parts) > 1 {
			db.DeleteAlert(ctx, parts[1])
		}
		flow := "wizard"
		if len(parts) > 2 {
			if parts[2] == "Manual" {
				flow = "manual"
			}
		}
		_ = db.SaveAnalytics(ctx, store.AnalyticsRecord{
			FlowType:  flow,
			Outcome:   "Cancelled_" + flow,
			EditCount: 0,
		})
		go triggerCompaction(i.GuildID)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🚫 **Alert Cancelled.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "cancel_alert_creation":
		_ = db.SaveAnalytics(ctx, store.AnalyticsRecord{
			FlowType:  "manual",
			Outcome:   "Cancelled_Manual_Syntax_Error",
			EditCount: 0,
		})
		go triggerCompaction(i.GuildID)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🚫 **Alert Creation Cancelled.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "approve_prompt":
		flowType := "wizard"
		if len(parts) > 1 {
			flowType = parts[1]
		}
		embedDesc := i.Message.Embeds[0].Description
		promptParts := strings.Split(embedDesc, "```text\n")
		if len(promptParts) > 1 {
			newPrompt := strings.TrimSuffix(promptParts[1], "\n```")
			_ = db.SetSystemPrompt(ctx, flowType+"_prompt", newPrompt)
		}
		records, _ := db.GetUnprocessedAnalyticsByFlow(ctx, flowType, 20)
		var ids []string
		for _, r := range records {
			ids = append(ids, r.ID)
		}
		_ = db.DeleteAnalyticsChunk(ctx, ids)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "✅ **Prompt Approved & Updated! Analytics cleared.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "reject_prompt":
		flowType := "wizard"
		if len(parts) > 1 {
			flowType = parts[1]
		}
		records, _ := db.GetUnprocessedAnalyticsByFlow(ctx, flowType, 20)
		var ids []string
		for _, r := range records {
			ids = append(ids, r.ID)
		}
		_ = db.DeleteAnalyticsChunk(ctx, ids)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "❌ **Prompt Rejected. Analytics cleared.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "edit_alert":
		editCount := "1"
		if len(parts) > 2 {
			editCount = parts[2]
		}
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseModal,
			Data: &discordgo.InteractionResponseData{
				CustomID: "modal_alert_wizard_manual|" + editCount,
				Title:    "Manual Alert Entry",
				Components: []discordgo.MessageComponent{
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:  "text_title",
								Label:     "Name your alert (e.g., Cheap 4090)",
								Style:     discordgo.TextInputShort,
								Required:  true,
								MaxLength: 50,
							},
						},
					},
					discordgo.ActionsRow{
						Components: []discordgo.MessageComponent{
							discordgo.TextInput{
								CustomID:    "text_query",
								Label:       "Query Syntax",
								Style:       discordgo.TextInputParagraph,
								Placeholder: "(rtx AND 4090) NOT (broken)",
								Required:    true,
								MaxLength:   150,
							},
						},
					},
				},
			},
		})

	case "delete_alert":
		if len(parts) > 1 {
			db.DeleteAlert(ctx, parts[1])
		}
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🗑️ Alert removed.",
				Embeds:     i.Message.Embeds,
				Components: []discordgo.MessageComponent{},
			},
		})

	case "delete_all_alerts":
		db.DeleteAllUserAlerts(ctx, i.GuildID, i.Member.User.ID)
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🚨 **All your alerts on this server have been deleted.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
			},
		})

	default:
		respondError(w, "Unknown component action")
	}
}
