package historyscraper

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/zmb3/spotify/v2"

	"golang.org/x/oauth2"

	"github.com/jackc/pgx/v4/pgxpool"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

type HistoryScraper struct {
	Log  *zap.Logger
	DB   *pgxpool.Pool
	Auth *spotifyauth.Authenticator
}

func (hs *HistoryScraper) Run(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		if err := hs.RunOnce(ctx); err != nil {
			hs.Log.Error("failed to run history logger", zap.Error(err))
		}

		time.Sleep(time.Second * 15)
	}
}

func (hs *HistoryScraper) RunOnce(ctx context.Context) error {
	//language=SQL
	r, err := hs.DB.Query(context.Background(), "SELECT discord_id, access_token, refresh_token, expiry FROM spotify_discord_links;")
	if err != nil {
		return err
	}
	defer r.Close()

	type userData struct {
		discordId string
		tok       *oauth2.Token
	}
	usrs := []userData{}
	{
		for r.Next() {
			data := userData{
				tok: &oauth2.Token{
					TokenType: "Bearer",
				},
			}
			err = r.Scan(
				&data.discordId,
				&data.tok.AccessToken,
				&data.tok.RefreshToken,
				&data.tok.Expiry,
			)
			if err != nil {
				return err
			}
			usrs = append(usrs, data)
		}

		if r.Err() != nil {
			return err
		}
	}

	for _, usr := range usrs {
		if err := hs.ScrapeUser(ctx, usr.discordId, usr.tok); err != nil {
			return err
		}
	}

	return nil
}

func (hs *HistoryScraper) ScrapeUser(ctx context.Context, discordID string, tok *oauth2.Token) error {
	var lastPolled *time.Time
	//language=SQL
	row := hs.DB.QueryRow(ctx, "SELECT time FROM listens WHERE discord_id = $1 ORDER BY time DESC LIMIT 1;", discordID)
	err := row.Scan(&lastPolled)
	if err != nil && err != pgx.ErrNoRows {
		return err
	}

	client := spotify.New(hs.Auth.Client(ctx, tok))
	var afterEpochMs int64 = 0
	if lastPolled != nil {
		afterEpochMs = (lastPolled.Add(time.Second).Unix()) * 1000
	}
	hs.Log.Info("epoch", zap.Int64("epoch", afterEpochMs))
	// Spotify only makes available your last 50 played tracks.
	// If this changes, we will need to add pagination :)
	rp, err := client.PlayerRecentlyPlayedOpt(ctx, &spotify.RecentlyPlayedOptions{
		Limit:        50,
		AfterEpochMs: afterEpochMs,
	})
	if err != nil {
		return err
	}

	tx, err := hs.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}

	for _, rpi := range rp {
		hs.Log.Info("song played", zap.String("name", rpi.Track.Name), zap.String("user", discordID))

		//language=SQL
		_, err = tx.Exec(context.Background(),
			`INSERT INTO listens(discord_id, song_id, time) VALUES($1, $2, $3) ON CONFLICT DO NOTHING;`,
			discordID,
			rpi.Track.ID,
			rpi.PlayedAt,
		)
		if err != nil {
			return err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}
