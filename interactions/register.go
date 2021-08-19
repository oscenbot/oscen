package interactions

import (
	"context"
	"fmt"

	"github.com/Postcord/objects"

	"golang.org/x/oauth2"
)

type authURLProvider interface {
	AuthURL(string, ...oauth2.AuthCodeOption) string
}

func NewRegisterInteraction(auth authURLProvider) *Interaction {
	h := func(
		ctx context.Context,
		interaction *objects.Interaction,
		interactionData *objects.ApplicationCommandInteractionData,
	) (*objects.InteractionResponse, error) {
		url := auth.AuthURL(fmt.Sprintf("%d", interaction.Member.User.ID))
		return &objects.InteractionResponse{
			Type: objects.ResponseChannelMessageWithSource,
			Data: &objects.InteractionApplicationCommandCallbackData{
				Content: fmt.Sprintf("Howdy! Visit: %s", url),
			}}, nil
	}

	return &Interaction{
		ApplicationCommand: &objects.ApplicationCommand{
			Name:        "register",
			Description: "Links your spotify account to your discord account",
		},
		handler: h,
	}
}
