package listens

import (
	"context"
	"time"

	"github.com/jackc/pgx/v4"
	"github.com/jackc/pgx/v4/pgxpool"
)

type PostgresRepository struct {
	db *pgxpool.Pool
}

func NewPostgresRepository(db *pgxpool.Pool) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (rp *PostgresRepository) GetUsersLastListenTime(ctx context.Context, discordID string) (*time.Time, error) {
	var lastListenTime *time.Time
	//language=SQL
	sql := "SELECT time FROM listens WHERE discord_id = $1 ORDER BY time DESC LIMIT 1;"
	row := rp.db.QueryRow(ctx, sql, discordID)
	err := row.Scan(&lastListenTime)
	if err != nil && err != pgx.ErrNoRows {
		return nil, err
	}

	return lastListenTime, nil
}

func (rp *PostgresRepository) GetSongListenCount(ctx context.Context, discordID string, songID string) (int, error) {
	listenCount := 0

	//language=SQL
	sql := "SELECT COUNT(1) AS count FROM listens WHERE discord_id = $1 AND song_id = $2;"
	row := rp.db.QueryRow(ctx, sql, discordID, songID)
	err := row.Scan(&listenCount)
	if err != nil && err != pgx.ErrNoRows {
		return listenCount, err
	}

	return listenCount, nil
}

func (rp *PostgresRepository) GetUserListenCount(ctx context.Context, discordID string) (int, error) {
	listenCount := 0

	//language=SQL
	sql := "SELECT COUNT(1) AS count FROM listens WHERE discord_id = $1;"
	row := rp.db.QueryRow(ctx, sql, discordID)
	err := row.Scan(&listenCount)
	if err != nil && err != pgx.ErrNoRows {
		return listenCount, err
	}

	return listenCount, nil
}
