package interactions

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"go.uber.org/zap"
)

const testGuild = "669541384327528461"

type handler = func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) error
type Interaction struct {
	*discordgo.ApplicationCommand
	handler handler
}

type router struct {
	routes map[string]handler
	s      *discordgo.Session
	log    *zap.Logger
}

func NewRouter(s *discordgo.Session, log *zap.Logger) *router {
	return &router{
		s:      s,
		routes: map[string]handler{},
		log:    log,
	}
}

func (r *router) RegisterRoute(interactions ...*Interaction) error {
	for _, i := range interactions {
		r.log.Info("registering command with discord", zap.String("name", i.Name))
		_, err := r.s.ApplicationCommandCreate(r.s.State.User.ID, testGuild, i.ApplicationCommand)
		if err != nil {
			return err
		}

		r.routes[i.Name] = i.handler
	}

	return nil
}

func (r *router) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	ctx := context.Background()

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

	err := handler(ctx, s, i)
	if err != nil {
		r.log.Error("error handling interaction", zap.Error(err))
	}
}
