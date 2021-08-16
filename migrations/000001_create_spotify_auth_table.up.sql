CREATE TABLE IF NOT EXISTS spotify_discord_links(
    discord_id TEXT PRIMARY KEY,
    access_token TEXT NOT NULL,
    refresh_token TEXT NOT NULL,
    expiry TIMESTAMP NOT NULL,
    last_polled TIMESTAMP
);