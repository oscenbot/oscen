package main

import (
	"context"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"oscen/historyscraper"
	"oscen/interactions"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"

	"github.com/Postcord/objects"
	"github.com/Postcord/rest"
	"github.com/jackc/pgx/v4/pgxpool"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.uber.org/zap"
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
		spotifyauth.WithRedirectURL(os.Getenv("CALLBACK_HOST")+"/v1/spotify/auth/callback"),
		spotifyauth.WithScopes(
			spotifyauth.ScopeUserReadPrivate,
			spotifyauth.ScopeUserReadPlaybackState,
			spotifyauth.ScopeUserReadCurrentlyPlaying,
			spotifyauth.ScopeUserReadRecentlyPlayed,
		),
	)

	return auth
}

func setupDiscord(log *zap.Logger) (*rest.Client, error) {
	log.Info("setting up discord client")
	token := "Bot " + os.Getenv("DISCORD_BOT_TOKEN")
	discord := rest.New(&rest.Config{
		Token:     token,
		UserAgent: "oscen",
	})

	usr, err := discord.GetCurrentUser()
	if err != nil {
		return nil, err
	}
	log.Info("connected to discord",
		zap.String("username", usr.Username),
	)

	return discord, nil
}

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	// Create the Jaeger exporter
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}
	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exp),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("oscen"),
			attribute.String("environment", "dev"),
		)),
	)
	return tp, nil
}

func main() {
	logger, _ := zap.NewDevelopment()
	ctx := context.Background()

	tp, err := tracerProvider("http://localhost:14268/api/traces")
	if err != nil {
		log.Fatal(err)
	}
	otel.SetTracerProvider(tp)

	discord, err := setupDiscord(logger)
	if err != nil {
		log.Fatal(err)
	}

	db, err := connectToDatabase(ctx)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	auth := setupSpotifyAuth()

	publicKey := os.Getenv("DISCORD_PUBLIC_KEY")
	if publicKey == "" {
		log.Fatal("DISCORD_PUBLIC_KEY must be set")
	}

	decodedKey, err := hex.DecodeString(publicKey)
	if err != nil {
		log.Fatal(err)
	}

	router := interactions.NewRouter(
		logger.Named("router"),
		decodedKey,
		discord,
	)

	err = router.Register(
		interactions.NewNowPlayingInteraction(db, auth),
		interactions.NewRegisterInteraction(auth),
	)
	if err != nil {
		log.Fatal(err)
	}

	testGuild := objects.Snowflake(669541384327528461)
	err = router.SyncInteractions(&testGuild)
	if err != nil {
		log.Fatal(err)
	}

	http.Handle("/v1/discord/interactions",
		otelhttp.NewHandler(router, "http.discord_interaction"),
	)
	http.Handle("/v1/spotify/auth/callback", otelhttp.NewHandler(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
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

		},
	), "http.spotify_callback"))
	go func() {
		logger.Info("listening for http at 9000")
		err := http.ListenAndServe(":9000", nil)
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
