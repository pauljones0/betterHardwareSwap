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

// routeModalSubmit handles the response when a user submits the wizard forms.
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

	tempRule := store.AlertRule{
		UserID:   i.Member.User.ID,
		ServerID: i.GuildID,
		MustHave: wizard.MustHave,
		AnyOf:    wizard.AnyOf,
		MustNot:  wizard.MustNot,
		RawQuery: query,
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
		RawQuery: title,
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
