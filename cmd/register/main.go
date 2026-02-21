package main

import (
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/joho/godotenv"
)

func main() {
	_ = godotenv.Load() // Load .env file if it exists (for local testing)

	token := os.Getenv("DISCORD_BOT_TOKEN")
	appID := os.Getenv("DISCORD_APP_ID")

	if token == "" || appID == "" {
		log.Fatal("DISCORD_BOT_TOKEN and DISCORD_APP_ID must be set")
	}

	// Create a new Discord session using the bot token
	dg, err := discordgo.New("Bot " + token)
	if err != nil {
		log.Fatalf("Error creating Discord session: %v", err)
	}

	// We only need to register commands, we don't need to open a websocket connection
	// because this is an HTTP interactions bot.

	commands := []*discordgo.ApplicationCommand{
		{
			Name:        "setup",
			Description: "Configure the bot for this server (Admin Only)",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "feed_channel",
					Description: "The channel where new deals will be posted",
					Required:    true,
				},
				{
					Type:        discordgo.ApplicationCommandOptionChannel,
					Name:        "ping_channel",
					Description: "The channel where users will be pinged when their alerts match",
					Required:    true,
				},
			},
		},
		{
			Name:        "help",
			Description: "Learn how to use the bot and set up alerts",
		},
		{
			Name:        "alert",
			Description: "Manage your hardware alerts",
			Options: []*discordgo.ApplicationCommandOption{
				{
					Name:        "add",
					Description: "Add a new hardware alert",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
				{
					Name:        "list",
					Description: "List and manage your active alerts",
					Type:        discordgo.ApplicationCommandOptionSubCommand,
				},
			},
		},
	}

	log.Println("Registering commands globally...")
	for _, v := range commands {
		_, err := dg.ApplicationCommandCreate(appID, "", v)
		if err != nil {
			log.Panicf("Cannot create '%v' command: %v", v.Name, err)
		}
		log.Printf("Successfully registered command /%s", v.Name)
	}

	log.Println("All commands registered successfully!")
}
