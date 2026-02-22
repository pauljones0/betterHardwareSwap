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

// routeModalSubmit handles the response when a user submits the `/alert add` modal form.
func routeModalSubmit(ctx context.Context, w http.ResponseWriter, i *discordgo.Interaction) {
	data := i.ModalSubmitData()
	if data.CustomID != "modal_alert_wizard" {
		respondError(w, "Unknown modal ID")
		return
	}

	rawQuery := data.Components[0].(*discordgo.ActionsRow).Components[0].(*discordgo.TextInput).Value

	// Immediately acknowledge the request so Discord doesn't timeout while Gemini thinks.
	// We use "DeferredChannelMessageWithSource" because it gives us up to 15 minutes to respond via the Followup API.
	writeJSON(w, discordgo.InteractionResponse{
		Type: discordgo.InteractionResponseDeferredChannelMessageWithSource,
		Data: &discordgo.InteractionResponseData{
			Flags: discordgo.MessageFlagsEphemeral,
		},
	})

	// Fire off a background goroutine to process with Gemini and send the followup.
	go processAIWizard(context.Background(), i, rawQuery)
}

func processAIWizard(ctx context.Context, i *discordgo.Interaction, query string) {
	client := NewClient(os.Getenv("DISCORD_BOT_TOKEN"))

	aiSvc, err := ai.NewAIClient(ctx, os.Getenv("GEMINI_API_KEY"))
	if err != nil {
		client.SendFollowupMessage(i, "⚠️ Could not connect to Gemini AI.")
		return
	}
	defer aiSvc.Close()

	wizard, err := aiSvc.RunKeywordWizard(ctx, query)
	if err != nil {
		log.Printf("Gemini Wizard Error: %v", err)
		client.SendFollowupMessage(i, "⚠️ Gemini failed to parse your request. Try wording it differently.")
		return
	}

	// Build the embed preview for the user
	desc := fmt.Sprintf("**Your Request:** *\"%s\"*\n\n**Bot will match posts that have:**\n", query)
	if len(wizard.MustHave) > 0 {
		desc += fmt.Sprintf("- **ALL of:** `%s`\n", strings.Join(wizard.MustHave, "`, `"))
	}
	if len(wizard.AnyOf) > 0 {
		desc += fmt.Sprintf("- **AT LEAST ONE of:** `%s`\n", strings.Join(wizard.AnyOf, "`, `"))
	}
	if len(wizard.MustNot) > 0 {
		desc += fmt.Sprintf("- **NONE of:** `%s`\n", strings.Join(wizard.MustNot, "`, `"))
	}

	if wizard.TooBroad {
		desc += "\n⚠️ **WARNING: This query is very broad and might result in a lot of spam. Are you sure?**"
	}

	embed := &discordgo.MessageEmbed{
		Title:       "🤖 AI Keyword Match Preview",
		Description: desc,
		Color:       0x5865F2, // Blurple
	}

	// We inject the AI's generated arrays into the custom ID of the button so we don't have to keep state in memory!
	// Discord limits custom IDs to 100 characters. For safety/size, a real app might temporarily store this in Redis/Firestore
	// but for simplicity, we'll store a "PendingAlert" in Firestore, pass the ID in the button, and confirm it later.

	db, err := store.NewStore(ctx, os.Getenv("GCP_PROJECT_ID"))
	if err != nil {
		client.SendFollowupMessage(i, "⚠️ Database error.")
		return
	}
	defer db.Close()

	// Temporarily save as unconfirmed (we can cleanup unconfirmed later, or just overwrite)
	tempRule := store.AlertRule{
		UserID:   i.Member.User.ID,
		ServerID: i.GuildID,
		MustHave: wizard.MustHave,
		AnyOf:    wizard.AnyOf,
		MustNot:  wizard.MustNot,
		RawQuery: query,
	}

	// Add directly to DB. If they hit cancel, we physically delete it.
	// This is vastly easier than dealing with 100char custom_id limits on Discord buttons.
	if err := db.AddAlert(ctx, tempRule); err != nil {
		client.SendFollowupMessage(i, "⚠️ Failed to stage alert in database.")
		return
	}

	// We need to fetch it right back to get the generated firestore ID, or we could have just
	// used the lower-level firestore add method. Since store.AddAlert doesn't return the ID,
	// we'll just fetch their most recent alert.
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
	case "confirm_alert": // The alert was already saved to DB in processAIWizard, so confirming just updates the UI.
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "✨ **Alert Saved Successfully!**",
				Embeds:     nil,                            // Clear the embed
				Components: []discordgo.MessageComponent{}, // Clear the buttons
			},
		})

	case "cancel_alert":
		if len(parts) > 1 {
			db.DeleteAlert(ctx, parts[1])
		}
		writeJSON(w, discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseUpdateMessage,
			Data: &discordgo.InteractionResponseData{
				Content:    "🚫 **Alert Cancelled.**",
				Embeds:     nil,
				Components: []discordgo.MessageComponent{},
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
