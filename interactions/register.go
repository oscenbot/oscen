package interactions

import (
	"context"
	"fmt"

	"golang.org/x/oauth2"

	"github.com/bwmarrin/discordgo"
)

type authURLProvider interface {
	AuthURL(string, ...oauth2.AuthCodeOption) string
}

func NewRegisterInteraction(auth authURLProvider) *Interaction {
	h := func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) error {
		url := auth.AuthURL(i.Member.User.ID)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: fmt.Sprintf("Howdy! Visit: %s", url),
			},
		})
		return err
	}

	return &Interaction{
		ApplicationCommand: &discordgo.ApplicationCommand{
			Name:        "register",
			Description: "Links your spotify account to your discord account",
		},
		handler: h,
	}
}
