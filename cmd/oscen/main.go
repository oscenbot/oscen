package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"oscen/interactions"
	"time"

	"github.com/bwmarrin/discordgo"
	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
	"golang.org/x/oauth2"
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

		logger.Info("d", zap.Any("d", tok))

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

	go func() {
		for {
			logger.Info("polling")
			//language=SQL
			r, err := db.Query(context.Background(), "SELECT discord_id, last_polled, access_token, refresh_token, expiry FROM spotify_discord_links;")
			if err != nil {
				logger.Error("something went wrong polling", zap.Error(err))
				return
			}
			defer r.Close()

			type userData struct {
				discordId  string
				lastPolled *time.Time
				tok        *oauth2.Token
			}
			usrs := []userData{}

			for r.Next() {
				data := userData{
					tok: &oauth2.Token{
						TokenType: "Bearer",
					},
				}
				err = r.Scan(
					&data.discordId,
					&data.lastPolled,
					&data.tok.AccessToken,
					&data.tok.RefreshToken,
					&data.tok.Expiry,
				)
				if err != nil {
					logger.Error("something went wrong polling", zap.Error(err))
					return
				}
				usrs = append(usrs, data)
			}
			// Any errors encountered by rows.Next or rows.Scan will be returned here
			if r.Err() != nil {
				logger.Error("something went wrong polling", zap.Error(r.Err()))
				return
			}

			for _, usr := range usrs {
				client := spotify.New(auth.Client(context.Background(), usr.tok))
				var afterEpochMs int64 = 0
				if usr.lastPolled != nil {
					afterEpochMs = (usr.lastPolled.Unix()) * 1000
				}
				logger.Info("epoch", zap.Int64("epoch", afterEpochMs))
				// Spotify only makes available your last 50 played tracks.
				// If this changes, we will need to add pagination :)
				rp, err := client.PlayerRecentlyPlayedOpt(context.Background(), &spotify.RecentlyPlayedOptions{
					Limit:        50,
					AfterEpochMs: afterEpochMs,
				})
				// 19:18:52
				// 19:21:52
				if err != nil {
					logger.Error("something went wrong polling", zap.Error(err))
				}

				discordUsr, err := discordSession.User(usr.discordId)
				if err != nil {
					logger.Error("getting user failed", zap.Error(err))
				}

				tx, err := db.BeginTx(ctx, pgx.TxOptions{})
				if err != nil {
					logger.Error("getting tx failed", zap.Error(err))
				}

				for _, rpi := range rp {
					logger.Info("song played", zap.String("name", rpi.Track.Name), zap.String("username", discordUsr.Username))

					_, err := discordSession.ChannelMessageSend(testChannel, fmt.Sprintf("%s: %s (%s)", discordUsr.Username, rpi.Track.Name, rpi.Track.Artists[0].Name))
					if err != nil {
						log.Fatalf("failed to create record: %s", err)
					}

					//language=SQL
					_, err = tx.Exec(context.Background(),
						`INSERT INTO listens(discord_id, song_id, time) VALUES($1, $2, $3);`, usr.discordId, rpi.Track.ID, rpi.PlayedAt)
					if err != nil {
						log.Fatalf("failed to create record: %s", err)
					}
				}
				//language=SQL
				_, err = tx.Exec(context.Background(),
					`UPDATE spotify_discord_links SET last_polled=$1 WHERE discord_id=$2;`, time.Now(), usr.discordId)
				if err != nil {
					log.Fatalf("failed to update record: %s", err)
				}

				err = tx.Commit(ctx)
				if err != nil {
					log.Fatalf("failed to commit tx: %s", err)
				}
			}

			time.Sleep(time.Second * 15)
		}
	}()

	logger.Info("setup finished")
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)
	<-stop
}
