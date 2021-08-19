package interactions

import (
	"context"
	"fmt"

	"github.com/Postcord/objects"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

var ErrNotRegistered = fmt.Errorf("user not registered")

func ensureSpotifyClient(
	ctx context.Context,
	i *objects.Interaction,
	userDb *pgxpool.Pool,
	auth *spotifyauth.Authenticator,
) (*spotify.Client, error) {
	userId := fmt.Sprintf("%d", i.Member.User.ID)

	tok := &oauth2.Token{
		TokenType: "Bearer",
	}
	//language=SQL
	sql := `
		SELECT
			access_token, refresh_token, expiry
		FROM spotify_discord_links
		WHERE discord_id=$1
		LIMIT 1;
	`
	r := userDb.QueryRow(ctx, sql, userId)
	err := r.Scan(
		&tok.AccessToken,
		&tok.RefreshToken,
		&tok.Expiry,
	)

	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrNotRegistered
		}
		return nil, err
	}

	client := spotify.New(auth.Client(ctx, tok))
	return client, nil
}

func NewNowPlayingInteraction(db *pgxpool.Pool, auth *spotifyauth.Authenticator) *Interaction {
	h := func(
		ctx context.Context,
		interaction *objects.Interaction,
		interactionData *objects.ApplicationCommandInteractionData,
	) (*objects.InteractionResponse, error) {
		client, err := ensureSpotifyClient(ctx, interaction, db, auth)
		if err != nil {
			if err == ErrNotRegistered {
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

		return &objects.InteractionResponse{
			Type: objects.ResponseChannelMessageWithSource,
			Data: &objects.InteractionApplicationCommandCallbackData{
				Content: "You are listening to: " + np.Item.Name,
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
