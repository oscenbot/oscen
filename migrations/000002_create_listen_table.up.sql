CREATE TABLE IF NOT EXISTS listens(
      discord_id TEXT,
      song_id TEXT,
      time TIMESTAMP,
      PRIMARY KEY (discord_id, song_id, time)
);