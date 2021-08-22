package listens

import (
	"context"
	"oscen/tracer"
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

func (rp *PostgresRepository) GetUsersLastListenTime(
	ctx context.Context,
	discordID string,
) (*time.Time, error) {
	ctx, childSpan := tracer.Start(ctx, "repositories.listens.get_users_last_listen_time")
	defer childSpan.End()

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

func (rp *PostgresRepository) GetSongListenCount(
	ctx context.Context,
	discordID string,
	songID string,
) (int, error) {
	ctx, childSpan := tracer.Start(ctx, "repositories.listens.get_song_listen_count")
	defer childSpan.End()

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

func (rp *PostgresRepository) GetUserListenCount(
	ctx context.Context,
	discordID string,
) (int, error) {
	ctx, childSpan := tracer.Start(ctx, "repositories.listens.get_user_listen_count")
	defer childSpan.End()

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

type BatchWriteListenEntry struct {
	TrackID  string
	PlayedAt time.Time
}

func (rp *PostgresRepository) BatchWriteListens(
	ctx context.Context,
	discordID string,
	entries []BatchWriteListenEntry,
) error {
	ctx, childSpan := tracer.Start(ctx, "repositories.listens.batch_write_listens")
	defer childSpan.End()

	// TODO: Use an actual batch rather than write loads as a transaction
	tx, err := rp.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	//language=SQL
	sql := `INSERT INTO listens(discord_id, song_id, time) VALUES($1, $2, $3) ON CONFLICT DO NOTHING;`

	for _, entry := range entries {
		_, err = tx.Exec(ctx,
			sql,
			discordID,
			entry.TrackID,
			entry.PlayedAt,
		)
		if err != nil {
			return err
		}
	}

	err = tx.Commit(ctx)
	if err != nil {
		return err
	}

	return nil
}
