package discord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/bwmarrin/discordgo"
)

const discordAPI = "https://discord.com/api/v10"

// Client is a wrapper around the Discord REST API to perform actions the Interaction webhook cannot
// (e.g. sending proactive messages to channels, editing messages, adding reactions).
type Client struct {
	token      string
	httpClient *http.Client
}

// NewClient initializes a new Discord REST client.
func NewClient(token string) *Client {
	return &Client{
		token:      token,
		httpClient: &http.Client{},
	}
}

func (c *Client) doRequest(method, endpoint string, body interface{}) ([]byte, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, discordAPI+endpoint, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", "Bot "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "DiscordBot (https://github.com/pauljones0/betterHardwareSwap, 1.0.0)")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("discord API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// SendMessage sends a plain text message to a channel.
func (c *Client) SendMessage(channelID, content string) error {
	payload := map[string]string{"content": content}
	_, err := c.doRequest("POST", "/channels/"+channelID+"/messages", payload)
	return err
}

// SendEmbed sends a message with an Embed to a channel and returns the created Message ID.
func (c *Client) SendEmbed(channelID string, content string, embed *discordgo.MessageEmbed) (string, error) {
	payload := discordgo.MessageSend{
		Content: content,
		Embeds:  []*discordgo.MessageEmbed{embed},
	}

	resp, err := c.doRequest("POST", "/channels/"+channelID+"/messages", payload)
	if err != nil {
		return "", err
	}

	var msg discordgo.Message
	if err := json.Unmarshal(resp, &msg); err != nil {
		return "", err
	}
	return msg.ID, nil
}

// EditEmbed updates an existing message with a new embed.
func (c *Client) EditEmbed(channelID, messageID, content string, embed *discordgo.MessageEmbed) error {
	payload := discordgo.MessageEdit{
		Content: &content,
		Embeds:  &[]*discordgo.MessageEmbed{embed},
	}
	_, err := c.doRequest("PATCH", "/channels/"+channelID+"/messages/"+messageID, payload)
	return err
}

// AddReaction adds a unicode emoji reaction to a message.
func (c *Client) AddReaction(channelID, messageID, emoji string) error {
	// Emoji needs to be URL encoded if it's custom, but standard unicode works directly in the path if properly escaped.
	// For standard thumbs up \U0001F44D, url.PathEscape isn't strictly necessary for the DoRequest helper if we manually encode it,
	// but let's assume `emoji` is already url-safe (e.g. "%F0%9F%91%8D")
	_, err := c.doRequest("PUT", "/channels/"+channelID+"/messages/"+messageID+"/reactions/"+emoji+"/@me", nil)
	return err
}

// SendFollowupMessage sends a followup to a deferred Interaction.
func (c *Client) SendFollowupMessage(i *discordgo.Interaction, content string) error {
	payload := discordgo.WebhookParams{
		Content: content,
		Flags:   discordgo.MessageFlagsEphemeral,
	}
	endpoint := fmt.Sprintf("/webhooks/%s/%s", i.AppID, i.Token)
	_, err := c.doRequest("POST", endpoint, payload)
	return err
}

// SendFollowupEmbedWithComponents sends a followup with embeds and UI components.
func (c *Client) SendFollowupEmbedWithComponents(i *discordgo.Interaction, embed *discordgo.MessageEmbed, components []discordgo.MessageComponent) error {
	payload := map[string]interface{}{
		"embeds":     []*discordgo.MessageEmbed{embed},
		"components": components,
		"flags":      discordgo.MessageFlagsEphemeral,
	}
	endpoint := fmt.Sprintf("/webhooks/%s/%s", i.AppID, i.Token)
	_, err := c.doRequest("POST", endpoint, payload)
	return err
}

// CreateDM opens a DM channel with a specific user.
func (c *Client) CreateDM(userID string) (string, error) {
	payload := map[string]string{"recipient_id": userID}
	resp, err := c.doRequest("POST", "/users/@me/channels", payload)
	if err != nil {
		return "", err
	}
	var ch discordgo.Channel
	if err := json.Unmarshal(resp, &ch); err != nil {
		return "", err
	}
	return ch.ID, nil
}

// SendAdminApprovalDM attempts to DM the admin with the newly compacted prompt.
// If Discord blocks it due to privacy, it returns an error so we can fallback.
func (c *Client) SendAdminApprovalDM(adminID, newPrompt, flowType string) error {
	dmChannelID, err := c.CreateDM(adminID)
	if err != nil {
		return err
	}

	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🧠 AI Self-Improvement Suggestion (%s)", flowType),
		Description: "I analyzed 20 recent interactions and generated an improved system prompt.\n\n**New Prompt:**\n```text\n" + newPrompt + "\n```",
		Color:       0xFFD700, // Gold
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "✅ Approve New Prompt",
					Style:    discordgo.SuccessButton,
					CustomID: "approve_prompt|" + flowType,
				},
				discordgo.Button{
					Label:    "❌ Reject & Clear",
					Style:    discordgo.DangerButton,
					CustomID: "reject_prompt|" + flowType,
				},
			},
		},
	}

	payload := map[string]interface{}{
		"embeds":     []*discordgo.MessageEmbed{embed},
		"components": components,
	}

	_, err = c.doRequest("POST", "/channels/"+dmChannelID+"/messages", payload)
	return err
}

// SendFallbackAdminApproval sends the approval to a fallback channel pinging the admin.
func (c *Client) SendFallbackAdminApproval(channelID, adminID, newPrompt, flowType string) error {
	embed := &discordgo.MessageEmbed{
		Title:       fmt.Sprintf("🧠 AI Self-Improvement Suggestion (%s)", flowType),
		Description: fmt.Sprintf("<@%s> I couldn't DM you! I analyzed 20 recent interactions and generated an improved system prompt.\n\n**New Prompt:**\n```text\n%s\n```", adminID, newPrompt),
		Color:       0xFFD700, // Gold
	}

	components := []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Label:    "✅ Approve New Prompt",
					Style:    discordgo.SuccessButton,
					CustomID: "approve_prompt|" + flowType,
				},
				discordgo.Button{
					Label:    "❌ Reject & Clear",
					Style:    discordgo.DangerButton,
					CustomID: "reject_prompt|" + flowType,
				},
			},
		},
	}

	payload := map[string]interface{}{
		"content":    fmt.Sprintf("<@%s>", adminID), // To trigger an actual ping notification
		"embeds":     []*discordgo.MessageEmbed{embed},
		"components": components,
	}

	_, err := c.doRequest("POST", "/channels/"+channelID+"/messages", payload)
	return err
}
