package interactions

import (
	"context"
	"fmt"
	"oscen/playlistcreator"
	"oscen/repositories/users"

	spotifyauth "github.com/zmb3/spotify/v2/auth"

	"github.com/Postcord/objects"
)

func Generate(userRepo *users.PostgresRepository, auth *spotifyauth.Authenticator, playlistCreator *playlistcreator.PlaylistCreator) *Interaction {
	h := func(
		ctx context.Context,
		interaction *objects.Interaction,
		interactionData *objects.ApplicationCommandInteractionData,
	) (*objects.InteractionResponse, error) {
		client, err := ensureSpotifyClient(ctx, interaction, userRepo, auth)
		if err != nil {
			if err == users.ErrUserNotRegistered {
				return &objects.InteractionResponse{
					Type: objects.ResponseChannelMessageWithSource,
					Data: &objects.InteractionApplicationCommandCallbackData{
						Content: "You need to use /register before you can use other commands",
					},
				}, nil
			}

			return nil, err
		}

		url, err := playlistCreator.Create(ctx, interaction, client)
		if err != nil {
			return nil, err
		}

		return &objects.InteractionResponse{
			Type: objects.ResponseChannelMessageWithSource,
			Data: &objects.InteractionApplicationCommandCallbackData{
				Content: fmt.Sprintf("You can find your new playlist here: %s", *url),
			},
		}, nil
	}

	return &Interaction{
		ApplicationCommand: &objects.ApplicationCommand{
			Name:              "generate",
			Description:       "Generates a playlist for your current guild",
			DefaultPermission: true,
		},
		handler: h,
	}
}
