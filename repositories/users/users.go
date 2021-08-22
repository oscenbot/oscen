package users

import (
	"context"
	"fmt"
	"oscen/tracer"

	"github.com/jackc/pgx/v4"

	"golang.org/x/oauth2"

	"github.com/jackc/pgx/v4/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

type User struct {
	DiscordID    string
	SpotifyToken *oauth2.Token
}

func (rp *PostgresRepository) GetUsers(
	ctx context.Context,
) ([]User, error) {
	ctx, childSpan := tracer.Start(ctx, "repositories.users.get_users")
	defer childSpan.End()

	//language=SQL
	sql := "SELECT discord_id, access_token, refresh_token, expiry FROM spotify_discord_links;"
	r, err := rp.db.Query(ctx, sql)
	if err != nil {
		return nil, err
	}
	defer r.Close()

	usrs := []User{}
	for r.Next() {
		data := User{
			SpotifyToken: &oauth2.Token{
				TokenType: "Bearer",
			},
		}
		err = r.Scan(
			&data.DiscordID,
			&data.SpotifyToken.AccessToken,
			&data.SpotifyToken.RefreshToken,
			&data.SpotifyToken.Expiry,
		)
		if err != nil {
			return nil, err
		}
		usrs = append(usrs, data)
	}

	if r.Err() != nil {
		return nil, err
	}

	return usrs, nil
}

var ErrUserNotRegistered = fmt.Errorf("user is not registered")

func (rp *PostgresRepository) GetUserByDiscordID(
	ctx context.Context,
	discordID string,
) (*User, error) {
	ctx, childSpan := tracer.Start(ctx, "repositories.users.get_user_by_discord_id")
	defer childSpan.End()

	//language=SQL
	sql := "SELECT discord_id, access_token, refresh_token, expiry FROM spotify_discord_links WHERE discord_id=$1 LIMIT 1;"
	row := rp.db.QueryRow(ctx, sql, discordID)

	data := User{
		SpotifyToken: &oauth2.Token{
			TokenType: "Bearer",
		},
	}

	err := row.Scan(
		&data.DiscordID,
		&data.SpotifyToken.AccessToken,
		&data.SpotifyToken.RefreshToken,
		&data.SpotifyToken.Expiry,
	)
	if err != nil {
		if err == pgx.ErrNoRows {
			return nil, ErrUserNotRegistered
		}
		return nil, err
	}

	return &data, nil
}

type UpsertUser struct {
	DiscordID    string
	SpotifyToken *oauth2.Token
}

func (rp *PostgresRepository) UpsertUser(
	ctx context.Context,
	usr UpsertUser,
) error {
	ctx, childSpan := tracer.Start(ctx, "repositories.users.upsert_user")
	defer childSpan.End()

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

	_, err := rp.db.Exec(
		ctx,
		sql,
		usr.DiscordID,
		usr.SpotifyToken.AccessToken,
		usr.SpotifyToken.RefreshToken,
		usr.SpotifyToken.Expiry,
	)

	return err
}
