package historyscraper

import (
	"context"
	"oscen/repositories/listens"
	"oscen/repositories/users"
	"oscen/tracer"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/zmb3/spotify/v2"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

type HistoryScraper struct {
	Log         *zap.Logger
	Auth        *spotifyauth.Authenticator
	ListensRepo *listens.PostgresRepository
	UsersRepo   *users.PostgresRepository
	Interval    time.Duration
}

func (hs *HistoryScraper) Run(ctx context.Context) {
	for {
		if err := ctx.Err(); err != nil {
			return
		}

		if err := hs.RunOnce(ctx); err != nil {
			hs.Log.Error("failed to run history logger", zap.Error(err))
		}

		time.Sleep(hs.Interval)
	}
}

func (hs *HistoryScraper) RunOnce(ctx context.Context) error {
	ctx, childSpan := tracer.Start(ctx, "historyscraper.run_once")
	defer childSpan.End()

	hs.Log.Debug("starting scrape")
	start := time.Now()

	usrs, err := hs.UsersRepo.GetUsers(ctx)
	if err != nil {
		return err
	}

	for _, usr := range usrs {
		if err := hs.ScrapeUser(ctx, &usr); err != nil {
			return err
		}
	}

	hs.Log.Info("finished scrape",
		zap.Duration("duration", time.Since(start)),
		zap.Int("user_count", len(usrs)),
	)

	return nil
}

func (hs *HistoryScraper) ScrapeUser(ctx context.Context, user *users.User) error {
	ctx, childSpan := tracer.Start(
		ctx,
		"historyscraper.scrape_user",
		trace.WithAttributes(attribute.String("io.oscen.discord_user", user.DiscordID)),
	)
	defer childSpan.End()

	hs.Log.Debug("scraping user", zap.String("discord_user", user.DiscordID))
	start := time.Now()

	lastPolled, err := hs.ListensRepo.GetUsersLastListenTime(ctx, user.DiscordID)
	if err != nil {
		return err
	}

	client := user.SpotifyClient(ctx, hs.Auth)
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

	// TODO: if we iterate from the end to the start (oldest to newest)
	// we can get rid of the transactions/batching. Problem for another day :)
	batchWrite := make([]listens.BatchWriteListenEntry, 0, len(rp))
	for _, rpi := range rp {
		hs.Log.Debug("song played",
			zap.String("song_name", rpi.Track.Name),
			zap.String("discord_user", user.DiscordID),
		)

		batchWrite = append(batchWrite, listens.BatchWriteListenEntry{
			TrackID:  string(rpi.Track.ID),
			PlayedAt: rpi.PlayedAt,
		})
	}

	err = hs.ListensRepo.BatchWriteListens(ctx, user.DiscordID, batchWrite)
	if err != nil {
		return err
	}

	hs.Log.Info("scraped user",
		zap.Duration("duration", time.Since(start)),
		zap.String("discord_id", user.DiscordID),
		zap.Int("song_count", len(rp)),
	)

	return nil
}
