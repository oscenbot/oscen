package main

import (
	"fmt"
	"log"
	"os"

	"github.com/bwmarrin/discordgo"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

// TODO: from env
const appId = "876893270934945824"

func main() {
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}

	root := cobra.Command{
		Use: "oscen-cli",
	}

	dc, err := setupDiscordSession(logger)
	if err != nil {
		logger.Fatal("failed to setup discord", zap.Error(err))
	}

	root.AddCommand(ListApplicationCommands(dc, logger))
	root.AddCommand(ResetApplicationCommands(dc, logger))

	if err := root.Execute(); err != nil {
		logger.Fatal("failed to execute command", zap.Error(err))
	}
}

func ListApplicationCommands(session *discordgo.Session, logger *zap.Logger) *cobra.Command {
	return &cobra.Command{
		Use: "list-application-commands <guildId>",
		RunE: func(cmd *cobra.Command, args []string) error {
			guildId := "" // by default global
			if len(args) == 1 {
				guildId = args[0]
			}
			logger.Info("listing application commands", zap.String("guild", guildId))

			appCommands, err := session.ApplicationCommands(appId, guildId)
			if err != nil {
				return err
			}
			logger.Info("found commands", zap.Int("count", len(appCommands)))

			for _, appCmd := range appCommands {
				logger.Info("application command", zap.String("name", appCmd.Name))
			}
			return nil
		},
	}
}

func ResetApplicationCommands(session *discordgo.Session, logger *zap.Logger) *cobra.Command {
	return &cobra.Command{
		Use: "reset-application-commands <guildId>",
		RunE: func(cmd *cobra.Command, args []string) error {
			guildId := "" // by default global
			if len(args) == 1 {
				guildId = args[0]
			}
			logger.Info("resetting application commands", zap.String("guild", guildId))

			appCommands, err := session.ApplicationCommands(appId, guildId)
			if err != nil {
				return err
			}
			logger.Info("found commands", zap.Int("count", len(appCommands)))

			for _, appCmd := range appCommands {
				logger.Info("deleting application command", zap.String("name", appCmd.Name))
				err := session.ApplicationCommandDelete(appId, guildId, appCmd.ID)
				if err != nil {
					return err
				}
			}
			return nil
		},
	}
}

func setupDiscordSession(log *zap.Logger) (*discordgo.Session, error) {
	log.Info("instantiating discord session")
	discordSession, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate discord connection: %w", err)
	}

	return discordSession, nil
}
