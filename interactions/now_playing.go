package interactions

import (
	"context"
	"fmt"
	"oscen/repositories/listens"
	"oscen/repositories/users"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"go.opentelemetry.io/otel"

	"github.com/Postcord/objects"

	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
)

var tracer = otel.Tracer("github.com/oscen/interactions")

func ensureSpotifyClient(
	ctx context.Context,
	i *objects.Interaction,
	userRepo *users.PostgresRepository,
	auth *spotifyauth.Authenticator,
) (*spotify.Client, error) {
	ctx, childSpan := tracer.Start(ctx, "interactions.helper.ensure_spotify_client")
	defer childSpan.End()

	userId := fmt.Sprintf("%d", i.Member.User.ID)
	usr, err := userRepo.GetUserByDiscordID(ctx, userId)
	if err != nil {
		return nil, err
	}

	http := auth.Client(ctx, usr.SpotifyToken)
	http.Transport = otelhttp.NewTransport(http.Transport)
	client := spotify.New(http)

	return client, nil
}

func NewNowPlayingInteraction(userRepo *users.PostgresRepository, auth *spotifyauth.Authenticator, listensRepo *listens.PostgresRepository) *Interaction {
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

		np, err := client.PlayerCurrentlyPlaying(ctx)
		if err != nil {
			return nil, err
		}

		userID := fmt.Sprintf("%d", interaction.Member.User.ID)
		songListens, err := listensRepo.GetSongListenCount(ctx, userID, string(np.Item.ID))
		if err != nil {
			return nil, err
		}

		totalListens, err := listensRepo.GetUserListenCount(ctx, userID)
		if err != nil {
			return nil, err
		}

		artistName := "unknown"
		// TODO: Support multiple artists.
		if len(np.Item.Artists) > 0 {
			artistName = np.Item.Artists[0].Name
		}

		msg := "You are listening to %s - %s. You've listened to this track %d times before, and %d tracks in total."

		return &objects.InteractionResponse{
			Type: objects.ResponseChannelMessageWithSource,
			Data: &objects.InteractionApplicationCommandCallbackData{
				Content: fmt.Sprintf(msg, np.Item.Name, artistName, songListens, totalListens),
			},
		}, nil
	}

	return &Interaction{
		ApplicationCommand: &objects.ApplicationCommand{
			Name:              "np",
			Description:       "Shows your currently playing track",
			DefaultPermission: true,
		},
		handler: h,
	}
}
