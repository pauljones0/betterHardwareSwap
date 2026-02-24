package discord

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/store"
)

// routeModalSubmit handles the response when a user submits the `wizard_ai` or `wizard_manual` modal forms.
func routeModalSubmit(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	data := i.ModalSubmitData()

	// Immediately acknowledge the request so Discord doesn't timeout while Gemini thinks.
	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	if data.CustomID == "modal_alert_wizard_ai" {
		rawQuery := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		sanitizedQuery := Sanitize(rawQuery)
		go processAIWizard(context.Background(), i, sanitizedQuery)
	} else if strings.HasPrefix(data.CustomID, "modal_alert_wizard_manual") {
		// e.g. modal_alert_wizard_manual|edit_count
		editCount := 0
		parts := strings.Split(data.CustomID, "|")
		if len(parts) > 1 {
			fmt.Sscanf(parts[1], "%d", &editCount)
		}

		title := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value
		query := data.Components[1].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

		sanitizedTitle := Sanitize(title)
		sanitizedQuery := Sanitize(query)

		go processManualWizard(context.Background(), i, sanitizedTitle, sanitizedQuery, editCount)
	} else {
		client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))
		client.SendFollowupMessage(i, "⚠️ Unknown modal ID")
	}
}

func processAIWizard(ctx context.Context, i *discordgo.Interaction, query string) {
	client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))

	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err != nil {
		client.SendFollowupMessage(i, "⚠️ Database error.")
		return
	}
	defer db.Close()

	sysPrompt, _ := db.GetSystemPrompt(ctx, "wizard_prompt")

	aiSvc, err := ai.NewAIClient(ctx, os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		client.SendFollowupMessage(i, "⚠️ Could not connect to Gemini AI.")
		return
	}
	defer aiSvc.Close()

	wizard, err := aiSvc.RunKeywordWizard(ctx, query, sysPrompt)
	if err != nil {
		log.Printf("Gemini Wizard Error: %v", err)
		client.SendFollowupMessage(i, "⚠️ Gemini failed to parse your request. Try wording it differently.")
		return
	}

	color := 0x5865F2 // Blurple
	var fields []*discordgo.MessageEmbedField

	if len(wizard.MustHave) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "✅ Must Include",
			Value:  fmt.Sprintf("`%s`", strings.Join(wizard.MustHave, "`, `")),
			Inline: false,
		})
	}
	if len(wizard.AnyOf) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "🔍 Match Any Of",
			Value:  fmt.Sprintf("`%s`", strings.Join(wizard.AnyOf, "`, `")),
			Inline: false,
		})
	}
	if len(wizard.MustNot) > 0 {
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "🚫 Exclude",
			Value:  fmt.Sprintf("`%s`", strings.Join(wizard.MustNot, "`, `")),
			Inline: false,
		})
	}

	if wizard.TooBroad {
		color = 0xFEE75C // Yellow
		suggestions := ""
		if len(wizard.BroadSuggestions) > 0 {
			for _, s := range wizard.BroadSuggestions {
				suggestions += fmt.Sprintf("• %s\n", s)
			}
		}
		fields = append(fields, &discordgo.MessageEmbedField{
			Name:   "⚠️ Search is Too Broad",
			Value:  fmt.Sprintf("> %s\n\n**Suggestions:**\n%s", wizard.BroadReason, suggestions),
			Inline: false,
		})
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🎯 Match Rule Created",
		Description: fmt.Sprintf("I've converted your request into a precise search rule.\n\n**Intent:** *\"%s\"*", query),
		Color:       color,
		Fields:      fields,
		Thumbnail: &discordgo.MessageEmbedThumbnail{
			URL: "https://em-content.zobj.net/source/microsoft-teams/363/robot_1f916.png",
		},
		Footer: &discordgo.MessageEmbedFooter{
			Text: "You can refine this rule anytime using /alert list",
		},
	}

	// We inject the AI's generated arrays into the custom ID of the button so we don't have to keep state in memory!
	// Discord limits custom IDs to 100 characters. For safety/size, a real app might temporarily store this in Redis/Firestore
	// but for simplicity, we'll store a "PendingAlert" in Firestore, pass the ID in the button, and confirm it later.

	// Temporarily save as unconfirmed (we can cleanup unconfirmed later, or just overwrite)
	tempRule := store.AlertRule{
		UserID:   i.Member.User.ID,
		ServerID: i.GuildID,
		MustHave: wizard.MustHave,
		AnyOf:    wizard.AnyOf,
		MustNot:  wizard.MustNot,
		RawQuery: query, // Use query as title for pure AI flow
	}

	if err := db.AddAlert(ctx, tempRule); err != nil {
		client.SendFollowupMessage(i, "⚠️ Failed to stage alert in database.")
		return
	}

	alerts, _ := db.GetUserAlerts(ctx, i.GuildID, i.Member.User.ID)
	if len(alerts) == 0 {
		client.SendFollowupMessage(i, "⚠️ Failed to retrieve staged alert.")
		return
	}
	stagedAlertID := alerts[0].ID

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "✅ Looks Good! - Save",
					Style:    discordgo.SuccessButton,
					CustomID: "confirm_alert|" + stagedAlertID,
				},
				discordgo.Button{
					Label:    "❌ Cancel",
					Style:    discordgo.DangerButton,
					CustomID: "cancel_alert|" + stagedAlertID,
				},
			},
		},
	}

	client.SendFollowupEmbedWithComponents(i, embed, components)
}

func processManualWizard(ctx context.Context, i *discordgo.Interaction, title, query string, editCount int) {
	client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))

	if editCount >= 3 {
		client.SendFollowupMessage(i, "⚠️ **Alert creation cancelled due to multiple invalid query attempts.** Please start over.")
		return
	}

	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err == nil {
		defer db.Close()
	}

	sysPrompt := ""
	if db != nil {
		sysPrompt, _ = db.GetSystemPrompt(ctx, "manual_prompt")
	}

	aiSvc, err := ai.NewAIClient(ctx, os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		client.SendFollowupMessage(i, "⚠️ Could not connect to Gemini AI.")
		return
	}
	defer aiSvc.Close()

	wizard, err := aiSvc.ValidateManualQuery(ctx, query, sysPrompt)
	if err != nil {
		log.Printf("Gemini Validation Error: %v", err)
		client.SendFollowupMessage(i, "⚠️ Gemini failed to validate your request. Please try again later.")
		return
	}

	if !wizard.IsValid {
		// Log analytics for failed attempt
		if db != nil {
			_ = db.SaveAnalytics(ctx, store.AnalyticsRecord{
				OriginalUserPrompt: query,
				Outcome:            "Rejected_Syntax_Error",
				EditCount:          editCount,
			})
		}

		desc := fmt.Sprintf("**Query Syntax Error:**\n`%s`\n\n**Reason:** %s", query, wizard.ErrorMessage)
		embed := &discordgo.MessageEmbed{
			Title:       "❌ Invalid Query Syntax",
			Description: desc,
			Color:       0xFF0000,
		}

		components := []discordgo.MessageComponent{
			discordgo.ActionsRow{
				Components: []discordgo.MessageComponent{
					discordgo.Button{
						Label:    "✏️ Edit Query",
						Style:    discordgo.PrimaryButton,
						CustomID: fmt.Sprintf("edit_alert||%d", editCount+1),
					},
					discordgo.Button{
						Label:    "🗑️ Cancel Alert Creation",
						Style:    discordgo.DangerButton,
						CustomID: "cancel_alert_creation|",
					},
				},
			},
		}
		client.SendFollowupEmbedWithComponents(i, embed, components)
		return
	}

	// Valid query!
	desc := fmt.Sprintf("**Title:** *%s*\n**Raw Query:** `%s`\n\n**Parsed As:**\n", title, query)
	if len(wizard.MustHave) > 0 {
		desc += fmt.Sprintf("- **ALL of:** `%s`\n", strings.Join(wizard.MustHave, "`, `"))
	}
	if len(wizard.AnyOf) > 0 {
		desc += fmt.Sprintf("- **AT LEAST ONE of:** `%s`\n", strings.Join(wizard.AnyOf, "`, `"))
	}
	if len(wizard.MustNot) > 0 {
		desc += fmt.Sprintf("- **NONE of:** `%s`\n", strings.Join(wizard.MustNot, "`, `"))
	}

	embed := &discordgo.MessageEmbed{
		Title:       "✅ Check Your Manual Query",
		Description: desc,
		Color:       0x00FF00,
	}

	tempRule := store.AlertRule{
		UserID:   i.Member.User.ID,
		ServerID: i.GuildID,
		MustHave: wizard.MustHave,
		AnyOf:    wizard.AnyOf,
		MustNot:  wizard.MustNot,
		RawQuery: title, // We store the title in RawQuery to show in lists
	}

	if db != nil {
		if err := db.AddAlert(ctx, tempRule); err != nil {
			client.SendFollowupMessage(i, "⚠️ Failed to stage alert in database.")
			return
		}
		alerts, _ := db.GetUserAlerts(ctx, i.GuildID, i.Member.User.ID)
		if len(alerts) > 0 {
			stagedAlertID := alerts[0].ID
			components := []discordgo.MessageComponent{
				discordgo.ActionsRow{
					Components: []discordgo.MessageComponent{
						discordgo.Button{
							Label:    "💾 Save Alert",
							Style:    discordgo.SuccessButton,
							CustomID: "confirm_alert|" + stagedAlertID + "|Manual",
						},
						discordgo.Button{
							Label:    "❌ Cancel",
							Style:    discordgo.DangerButton,
							CustomID: "cancel_alert|" + stagedAlertID + "|Manual",
						},
					},
				},
			}
			client.SendFollowupEmbedWithComponents(i, embed, components)
			return
		}
	}
	client.SendFollowupMessage(i, "⚠️ System error while saving alert.")
}

// routeComponentInteraction handles Button Clicks (Confirm/Cancel AI rules, Delete Alerts).
func routeComponentInteraction(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	data := i.MessageComponentData()
	parts := strings.Split(data.CustomID, "|")
	action := parts[0]
	// payload := parts[1] (sometimes missing based on action)

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
		// Pop the manual entry modal
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

	case "confirm_alert": // The alert was already saved to DB in processAIWizard, so confirming just updates the UI.
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
				Embeds:     nil,                            // Clear the embed
				Components: []discordgo.MessageComponent{}, // Clear the buttons
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
		// CustomID format: approve_prompt|flowType
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
		// When they delete an alert from the list, update the message to basically just say "Deleted."
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🗑️ Alert removed.",
				Embeds:     i.Message.Embeds, // Keep the old list visible if they want? Or clear it. We'll clear components.
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

	// We can only put 5 buttons per ActionRow, and max 5 ActionRows per message.
	// So we can show max 5 alerts easily in one message. (We'll cap at 4 so we have room for Delete All).
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

	// Add Delete All button at the end
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
