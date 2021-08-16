package main

import (
	"context"
	"github.com/bwmarrin/discordgo"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
	"log"
	"net/http"
	"os"
	"os/signal"
	interactionsrouter "oscen/interactions"
)

const (
	testGuild = "669541384327528461"
)

type spotifyDiscordLink map[string]*oauth2.Token

func ensureSpotifyClient(s *discordgo.Session, i *discordgo.InteractionCreate, userDb spotifyDiscordLink, auth *spotifyauth.Authenticator) (*spotify.Client, error) {
	userId := i.Member.User.ID
	tok, ok := userDb[userId]
	if !ok {
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

	client := spotify.New(auth.Client(context.Background(), tok))
	return client, nil
}

func main() {
	userDb := spotifyDiscordLink{}

	discordSession, err := discordgo.New("Bot "+ os.Getenv("DISCORD_BOT_TOKEN"))
	if err != nil {
		log.Fatalf("failed to connect to discord: %s", err)
	}

	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL("http://localhost:8080/v1/spotify/auth/callback"),
		spotifyauth.WithScopes(spotifyauth.ScopeUserReadPrivate, spotifyauth.ScopeUserReadPlaybackState, spotifyauth.ScopeUserReadCurrentlyPlaying),
	)

	err = discordSession.Open()
	if err != nil {
		log.Fatalf("failed to open to discord: %s", err)
	}
	defer discordSession.Close()

	router := interactionsrouter.New(discordSession)
	err = router.Register(&discordgo.ApplicationCommand{
		Name: "np",
		Description: "Shows your currently playing track",
	}, func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
		client, err := ensureSpotifyClient(s, i, userDb, auth)
		if err != nil {
			return err
		}

		np, err := client.PlayerCurrentlyPlaying(context.Background())
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
	})
	if err != nil {
		log.Fatalf("%s", err)
	}

	err = router.Register(&discordgo.ApplicationCommand{
		Name: "register",
		Description: "Links your spotify account to your discord account",
	}, func(s *discordgo.Session, i *discordgo.InteractionCreate) error {
		url := auth.AuthURL(i.Member.User.ID)
		err := s.InteractionRespond(i.Interaction, &discordgo.InteractionResponse{
			Type: discordgo.InteractionResponseChannelMessageWithSource,
			Data: &discordgo.InteractionResponseData{
				Content: "Howdy! Visit: " + url,
			},
		})
		return err
	})
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

		userDb[state] = tok
		w.WriteHeader(200)
		w.Write([]byte("You can return to discord now :)"))

	})
	go func() {
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			log.Fatal(err)
		}
	}()

	discordSession.AddHandler(router.Handle)

	stop := make(chan os.Signal)
	signal.Notify(stop, os.Interrupt)
	<-stop
}