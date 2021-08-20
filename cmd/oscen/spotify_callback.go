package main

import (
	"net/http"

	"github.com/jackc/pgx/v4/pgxpool"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"go.uber.org/zap"
)

// TODO: Extract this to a package someday

func SpotifyCallback(logger *zap.Logger, db *pgxpool.Pool, auth *spotifyauth.Authenticator) http.HandlerFunc {
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
			r.Context(),
			sql,
			state, tok.AccessToken, tok.RefreshToken, tok.Expiry,
		)
		if err != nil {
			logger.Error("failed to create record", zap.Error(err))
			return
		}
		w.WriteHeader(200)
		_, _ = w.Write([]byte("You can return to discord now :)"))
	}
}
