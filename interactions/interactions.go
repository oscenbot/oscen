package interactionsrouter

import (
	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)
const testGuild = "669541384327528461"

type Handler func(s *discordgo.Session, i *discordgo.InteractionCreate) error
type router struct {
	routes map[string]Handler
	s *discordgo.Session
	log *zap.Logger
}

func New(s *discordgo.Session, log *zap.Logger) *router {
	return &router{
		s: s,
		routes: map[string]Handler{},
		log: log,
	}
}

func (r *router) Register(cmd *discordgo.ApplicationCommand, h Handler) error {
	r.log.Info("registering command with discord", zap.String("name", cmd.Name))
	_, err := r.s.ApplicationCommandCreate(r.s.State.User.ID, testGuild, cmd)
	if err != nil {
		return err
	}

	r.routes[cmd.Name] = h

	return nil
}

func (r *router) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	name := i.ApplicationCommandData().Name
	r.log.Info("received interaction", zap.String("name", name))
	handler, ok := r.routes[i.ApplicationCommandData().Name]
	if !ok {
		r.log.Warn("no command handler registered", zap.String("name", name))
		_ = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "We couldn't handle that command.",
			},
		})
		return
	}

	err := handler(s, i)
	if err != nil {
		r.log.Error("error handling interaction", zap.Error(err))
	}
}