package playlistcreator

import (
	"context"
	"fmt"
	"oscen/repositories/users"
	"oscen/tracer"
	"time"

	spotifyauth "github.com/zmb3/spotify/v2/auth"

	"github.com/Postcord/objects"
	"github.com/Postcord/rest"
	"github.com/zmb3/spotify/v2"
	"go.uber.org/zap"
)

type PlaylistCreator struct {
	Logger      *zap.Logger
	Discord     *rest.Client
	UsersRepo   *users.PostgresRepository
	SpotifyAuth *spotifyauth.Authenticator
}

func New(
	auth *spotifyauth.Authenticator,
	discord *rest.Client,
	usersRepo *users.PostgresRepository,
	logger *zap.Logger,
) *PlaylistCreator {
	return &PlaylistCreator{
		SpotifyAuth: auth,
		Discord:     discord,
		UsersRepo:   usersRepo,
		Logger:      logger,
	}
}

func (pc *PlaylistCreator) Create(ctx context.Context, interaction *objects.Interaction, initiatorSpotify *spotify.Client) (*string, error) {
	ctx, childSpan := tracer.Start(ctx, "playlist_creator.create")
	defer childSpan.End()

	const songsPerMember = 5

	members, err := pc.Discord.ListGuildMembers(interaction.GuildID, &rest.ListGuildMembersParams{
		Limit: 1000,
	})
	// TODO: Pagination
	if err != nil {
		return nil, err
	}

	// Filter down to guild members registered on our platform
	registeredGuildMembers := []users.User{}
	for _, member := range members {
		discordId := fmt.Sprintf("%d", member.User.ID)
		member, err := pc.UsersRepo.GetUserByDiscordID(ctx, discordId)
		if err != nil {
			if err == users.ErrUserNotRegistered {
				continue
			}
			return nil, err
		}

		registeredGuildMembers = append(registeredGuildMembers, *member)
	}

	// Get top songs per user
	playlistSongs := []spotify.ID{}
	for _, member := range registeredGuildMembers {
		memberSpotify := member.SpotifyClient(ctx, pc.SpotifyAuth)
		topTracks, err := memberSpotify.CurrentUsersTopTracks(ctx, spotify.Limit(songsPerMember))
		if err != nil {
			pc.Logger.Warn("failed to fetch top tracks for user",
				zap.Error(err),
				zap.String("user_id", member.DiscordID),
			)
			continue
		}

		for _, track := range topTracks.Tracks {
			playlistSongs = append(playlistSongs, track.ID)
		}
	}
	playlistSongs = deduplicateTracks(playlistSongs)

	// TODO: Pagination when more than 100
	if len(playlistSongs) >= 100 {
		return nil, fmt.Errorf("more than 100 songs not supported")
	}

	initiator, err := initiatorSpotify.CurrentUser(ctx)
	if err != nil {
		return nil, err
	}

	guild, err := pc.Discord.GetGuild(interaction.GuildID)
	if err != nil {
		return nil, err
	}

	createdPlaylist, err := initiatorSpotify.CreatePlaylistForUser(
		ctx,
		initiator.ID,
		fmt.Sprintf("Guild Playlist - %s", guild.Name),
		fmt.Sprintf(
			"Guild playlist generated at %s by %s",
			time.Now().String(),
			interaction.Member.User.Username,
		),
		true,
		false,
	)
	if err != nil {
		return nil, err
	}

	_, err = initiatorSpotify.AddTracksToPlaylist(
		ctx,
		createdPlaylist.ID,
		playlistSongs...,
	)
	if err != nil {
		return nil, err
	}
	spotifyURL, ok := createdPlaylist.ExternalURLs["spotify"]
	if !ok {
		return nil, fmt.Errorf("no spotify link")
	}

	return &spotifyURL, nil
}

func deduplicateTracks(tracks []spotify.ID) []spotify.ID {
	keys := make(map[spotify.ID]bool)
	deduplicatedList := []spotify.ID{}

	for _, trackId := range tracks {
		if _, isPresent := keys[trackId]; !isPresent {
			keys[trackId] = true
			deduplicatedList = append(deduplicatedList, trackId)
		}
	}
	return deduplicatedList
}
