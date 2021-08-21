package historyscraper

import (
	"context"
	"oscen/repositories/listens"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/zmb3/spotify/v2"

	"golang.org/x/oauth2"

	"github.com/jackc/pgx/v4/pgxpool"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

type HistoryScraper struct {
	Log         *zap.Logger
	DB          *pgxpool.Pool
	Auth        *spotifyauth.Authenticator
	ListensRepo *listens.PostgresRepository
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
	hs.Log.Debug("starting scrape")
	start := time.Now()
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

	hs.Log.Info("finished scrape",
		zap.Duration("duration", time.Since(start)),
		zap.Int("user_count", len(usrs)),
	)

	return nil
}

func (hs *HistoryScraper) ScrapeUser(ctx context.Context, discordID string, tok *oauth2.Token) error {
	hs.Log.Debug("scraping user", zap.String("discord_id", discordID))
	start := time.Now()

	lastPolled, err := hs.ListensRepo.GetUsersLastListenTime(ctx, discordID)

	client := spotify.New(hs.Auth.Client(ctx, tok))
	var afterEpochMs int64 = 0
	if lastPolled != nil {
		afterEpochMs = (lastPolled.Add(time.Second).Unix()) * 1000
	}
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
		hs.Log.Debug("song played", zap.String("song_name", rpi.Track.Name), zap.String("discord_id", discordID))

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

	hs.Log.Info("scraped user",
		zap.Duration("duration", time.Since(start)),
		zap.String("discord_id", discordID),
		zap.Int("song_count", len(rp)),
	)

	return nil
}
