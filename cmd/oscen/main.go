package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"oscen/historyscraper"
	"oscen/interactions"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4/pgxpool"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

const (
	testChannel = "876943275490168883"
)

func connectToDatabase(ctx context.Context) (*pgxpool.Pool, error) {
	db, err := pgxpool.Connect(ctx, os.Getenv("POSTGRESQL_URL"))
	if err != nil {
		return nil, fmt.Errorf("failed to connect to db: %w", err)

	}

	if err := db.Ping(ctx); err != nil {
		return nil, fmt.Errorf("failed to ping db: %w", err)
	}
	return db, nil
}

func setupSpotifyAuth() *spotifyauth.Authenticator {
	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL("http://localhost:8080/v1/spotify/auth/callback"),
		spotifyauth.WithScopes(
			spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserReadPlaybackState,
			spotifyauth.ScopeUserReadCurrentlyPlaying,
			spotifyauth.ScopeUserReadRecentlyPlayed,
		),
	)

	return auth
}

func setupDiscordSession(log *zap.Logger) (*discordgo.Session, error) {
	log.Info("instantiating discord session")
	discordSession, err := discordgo.New("Bot " + os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		return nil, fmt.Errorf("failed to instantiate discord connection: %w", err)
	}

	log.Info("opening gateway")
	err = discordSession.Open()
	if err != nil {
		return nil, fmt.Errorf("failed to open discord gateway: %w", err)
	}

	return discordSession, nil
}

func main() {
	logger, _ := zap.NewProduction()
	ctx := context.Background()

	discordSession, err := setupDiscordSession(logger)
	if err != nil {
		log.Fatal(err)
	}
	defer discordSession.Close()

	db, err := connectToDatabase(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	auth := setupSpotifyAuth()

	router := interactions.NewRouter(discordSession, logger)
	discordSession.AddHandler(router.Handle)

	err = router.RegisterRoute(
		interactions.NewNowPlayingInteraction(db, auth),
		interactions.NewRegisterInteraction(auth),
	)
	if err != nil {
		log.Fatalf("%s", err)
	}

	http.HandleFunc("/v1/spotify/auth/callback", func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()
		if e := values.Get("error"); e != "" {
			log.Fatal("sptoify auth failed")
		}
		code := values.Get("code")
		if code == "" {
			log.Fatal("no access code")
		}
		state := values.Get("state")

		tok, err := auth.Exchange(context.Background(), code)
		if err != nil {
			log.Fatalf("failed to create command: %s", err)
		}

		//language=SQL
		sql := `
			INSERT INTO spotify_discord_links(
				discord_id,
				access_token,
				refresh_token,
				expiry
			) VALUES($1, $2, $3, $4)
			ON CONFLICT(discord_id) DO UPDATE
				SET access_token=$2, refresh_token=$3, expiry=$4;
		`
		_, err = db.Exec(
			context.Background(),
			sql,
			state, tok.AccessToken, tok.RefreshToken, tok.Expiry,
		)
		if err != nil {
			log.Fatalf("failed to create record: %s", err)
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("You can return to discord now :)"))

	})
	go func() {
		logger.Info("listening for callbacks at 8080")
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	hl := historyscraper.HistoryScraper{
		Log:  logger.Named("scraper"),
		DB:   db,
		Auth: auth,
	}
	go hl.Run(ctx)

	logger.Info("setup finished")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
