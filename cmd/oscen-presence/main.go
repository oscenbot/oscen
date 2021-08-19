package main

import (
	"log"
	"os"
	"os/signal"

	"github.com/bwmarrin/discordgo"
)

func main() {
	log.Println("Starting oscen-presence")

	token := "Bot " + os.Getenv("DISCORD_BOT_TOKEN")
	discord, err := discordgo.New(token)
	if err != nil {
		log.Fatalf("failed to create discordgo: %s", err)
	}

	// Set intents to none so we dont recieve any traffic
	discord.Identify.Intents = discordgo.IntentsNone

	discord.AddHandler(func(s *discordgo.Session, r *discordgo.Ready) {
		log.Println("connected")
	})

	err = discord.Open()
	if err != nil {
		log.Fatalf("failed to create connection to discord: %s", err)
	}
	defer discord.Close()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
