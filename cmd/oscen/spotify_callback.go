package main

import (
	"net/http"
	"oscen/repositories/users"

	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

// TODO: Extract this to a package someday

func SpotifyCallback(logger *zap.Logger, userRepo *users.PostgresRepository, auth *spotifyauth.Authenticator) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		values := r.URL.Query()

		if e := values.Get("error"); e != "" {
			logger.Error("spotify auth failed", zap.String("error", e))
			return
		}
		code := values.Get("code")
		if code == "" {
			logger.Error("no access code")
			return
		}
		state := values.Get("state")
		if state == "" {
			logger.Error("no state")
			return
		}

		tok, err := auth.Exchange(r.Context(), code)
		if err != nil {
			logger.Error("failed to exchange token", zap.Error(err))
		}

		err = userRepo.UpsertUser(
			r.Context(),
			users.UpsertUser{DiscordID: state, SpotifyToken: tok},
		)
		if err != nil {
			logger.Error("failed to create record", zap.Error(err))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("You can return to discord now :)"))
	}
}
