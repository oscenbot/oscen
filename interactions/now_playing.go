package interactions

import (
	"context"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

func ensureSpotifyClient(
	ctx context.Context,
	s *discordgo.Session,
	i *discordgo.InteractionCreate,
	userDb *pgxpool.Pool,
	auth *spotifyauth.Authenticator,
) (*spotify.Client, error) {
	userId := i.Member.User.ID

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
			err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
				Type: discordgo.InteractionResponseChannelMessageWithSource,
				Data: &discordgo.InteractionResponseData{
					Content: "You need to use /register before you can use other commands",
				},
			})
			if err != nil {
				return nil, err
			}
		}
		return nil, err
	}

	client := spotify.New(auth.Client(ctx, tok))
	return client, nil
}

func NewNowPlayingInteraction(db *pgxpool.Pool, auth *spotifyauth.Authenticator) *Interaction {
	h := func(ctx context.Context, s *discordgo.Session, i *discordgo.InteractionCreate) error {
		client, err := ensureSpotifyClient(ctx, s, i, db, auth)
		if err != nil {
			return err
		}

		np, err := client.PlayerCurrentlyPlaying(ctx)
		if err != nil {
			return err
		}
		err = s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "You are listening to: " + np.Item.Name,
			},
		})
		if err != nil {
			return err
		}

		return nil
	}

	return &Interaction{
		ApplicationCommand: &discordgo.ApplicationCommand{
			Name:        "np",
			Description: "Shows your currently playing track",
		},
		handler: h,
	}
}
