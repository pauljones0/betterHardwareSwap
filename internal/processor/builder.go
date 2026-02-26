package processor

import (
	"fmt"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/pauljones0/betterHardwareSwap/internal/ai"
	"github.com/pauljones0/betterHardwareSwap/internal/reddit"
)

// DealBuilder centralizes the logic for creating Discord embeds and UI components from Reddit deals.
type DealBuilder struct{}

// NewDealBuilder returns a new instance of DealBuilder.
func NewDealBuilder() *DealBuilder {
	return &DealBuilder{}
}

// BuildDealEmbed crafts a rich Discord embed for a Reddit post and its AI-cleaned metadata.
func (b *DealBuilder) BuildDealEmbed(post reddit.Post, cleaned *ai.CleanedPost) *discordgo.MessageEmbed {
	embed := &discordgo.MessageEmbed{
		Title:       "📦 " + cleaned.Title,
		URL:         post.URL,
		Description: cleaned.Description,
		Color:       b.getColor(post.Score, post.NumComments),
		Fields:      []*discordgo.MessageEmbedField{},
		Footer: &discordgo.MessageEmbedFooter{
			Text: fmt.Sprintf("r/CanadianHardwareSwap • 👍 %d | 💬 %d", post.Score, post.NumComments),
		},
		Timestamp: time.Unix(int64(post.CreatedUtc), 0).Format(time.RFC3339),
	}

	if cleaned.Price != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "💰 Price",
			Value:  cleaned.Price,
			Inline: true,
		})
	}
	if cleaned.Condition != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "✨ Condition",
			Value:  cleaned.Condition,
			Inline: true,
		})
	}
	if cleaned.Location != "" {
		embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
			Name:   "📍 Location",
			Value:  cleaned.Location,
			Inline: true,
		})
	}

	if post.Thumbnail != "" && post.Thumbnail != "self" && post.Thumbnail != "default" {
		embed.Thumbnail = &discordgo.MessageEmbedThumbnail{URL: post.Thumbnail}
	}

	return embed
}

// BuildDealButtons creates the action buttons (e.g., Open in Reddit, Mute) for a deal message.
func (b *DealBuilder) BuildDealButtons(url string) []discordgo.MessageComponent {
	return []discordgo.MessageComponent{
		discordgo.ActionsRow{
			Components: []discordgo.MessageComponent{
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "🌐",
					},
					Label: "Open in Reddit",
					Style: discordgo.LinkButton,
					URL:   url,
				},
				discordgo.Button{
					Emoji: &discordgo.ComponentEmoji{
						Name: "🔇",
					},
					Label:    "Mute Item",
					Style:    discordgo.SecondaryButton,
					CustomID: "mute_item",
				},
			},
		},
	}
}

// BuildClosedEmbed creates a greyed-out version of an embed for sold/closed listings.
func (b *DealBuilder) BuildClosedEmbed(originalTitle, url, status string) *discordgo.MessageEmbed {
	return &discordgo.MessageEmbed{
		Title:       "~~" + originalTitle + "~~",
		URL:         url,
		Description: fmt.Sprintf("This deal has been marked as **%s** on Reddit.", status),
		Color:       0x2C2F33, // Discord Darker Grey
		Footer: &discordgo.MessageEmbedFooter{
			Text: "Deal Closed",
		},
	}
}

// getColor returns a Discord hex color based on engagement heuristics.
func (b *DealBuilder) getColor(score, comments int) int {
	interactions := score + comments
	switch {
	case interactions >= 16:
		return 0xFF0000 // Lava Red
	case interactions >= 6:
		return 0xFFA500 // Orange
	case interactions >= 3:
		return 0xFFFF00 // Yellow
	default:
		return 0x808080 // Grey
	}
}
