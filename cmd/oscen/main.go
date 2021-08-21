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
	"oscen/repositories/listens"
	"oscen/repositories/users"
	"strconv"

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

	jaegerURL := os.Getenv("JAEGER_URL")
	if jaegerURL != "" {
		tp, err := tracerProvider(jaegerURL)
		if err != nil {
			log.Fatal(err)
		}
		otel.SetTracerProvider(tp)
	} else {
		logger.Warn("JAEGER_URL not set, so traces will not be sent")
	}

	discord, err := setupDiscord(logger)
	if err != nil {
		logger.Fatal("failed to setup discord", zap.Error(err))
	}

	db, err := connectToDatabase(ctx)
	if err != nil {
		logger.Fatal("failed to setup db connection", zap.Error(err))
	}
	defer db.Close()

	listensRepo := listens.NewPostgresRepository(db)
	usersRepo := users.NewPostgresRepository(db)

	auth := setupSpotifyAuth()

	publicKey := os.Getenv("DISCORD_PUBLIC_KEY")
	if publicKey == "" {
		logger.Fatal("DISCORD_PUBLIC_KEY must be set")
	}

	decodedKey, err := hex.DecodeString(publicKey)
	if err != nil {
		logger.Fatal("failed to decode public key", zap.Error(err))
	}

	router := interactions.NewRouter(
		logger.Named("router"),
		decodedKey,
		discord,
	)

	err = router.Register(
		interactions.NewNowPlayingInteraction(usersRepo, auth, listensRepo),
		interactions.NewRegisterInteraction(auth),
	)
	if err != nil {
		logger.Fatal("failed to register routes", zap.Error(err))
	}

	var testGuild *objects.Snowflake
	if guildId := os.Getenv("TEST_GUILD_ID"); guildId != "" {
		val, err := strconv.Atoi("TEST_GUILD_ID")
		if err != nil {
			logger.Fatal(
				"failed casting value of TEST_GUILD_ID",
				zap.Error(err),
			)
		}
		snowflake := objects.Snowflake(val)
		testGuild = &snowflake
	}

	err = router.SyncInteractions(testGuild)
	if err != nil {
		logger.Fatal("failed to sync interactions", zap.Error(err))
	}

	http.HandleFunc("/", func(writer http.ResponseWriter, request *http.Request) {
		headers := writer.Header()
		headers.Add("X-Clacks-Overhead", "GNU Corey Kendall")
		headers.Add("X-Clacks-Overhead", "GNU Terry Pratchett")
		writer.WriteHeader(418)

		msg := `Not much to see here. Checkout <a href="https://oscen.io""> the site</a>`
		_, _ = writer.Write([]byte(msg))
	})

	http.Handle("/v1/discord/interactions",
		otelhttp.NewHandler(router, "http.discord_interaction"),
	)

	http.Handle("/v1/spotify/auth/callback",
		otelhttp.NewHandler(
			SpotifyCallback(logger, usersRepo, auth),
			"http.spotify_callback",
		),
	)

	go func() {
		logger.Info("listening for http at 9000")
		err := http.ListenAndServe(":9000", nil)
		if err != nil {
			logger.Fatal("error listening on 9000", zap.Error(err))
		}
	}()

	hl := historyscraper.HistoryScraper{
		Log:         logger.Named("scraper"),
		Auth:        auth,
		ListensRepo: listensRepo,
		UsersRepo:   usersRepo,
	}
	go hl.Run(ctx)

	logger.Info("setup finished")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
