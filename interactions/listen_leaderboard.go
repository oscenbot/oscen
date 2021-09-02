package interactions

import (
	"context"
	"fmt"
	"oscen/repositories/listens"
	"sort"

	"github.com/Postcord/rest"

	"github.com/Postcord/objects"
)

// TODO: Introduce caching here :)

func NewListenLeaderboardInteraction(listensRepo *listens.PostgresRepository, dc *rest.Client) *Interaction {
	h := func(
		ctx context.Context,
		interaction *objects.Interaction,
		interactionData *objects.ApplicationCommandInteractionData,
	) (*objects.InteractionResponse, error) {
		members, err := dc.ListGuildMembers(interaction.GuildID, &rest.ListGuildMembersParams{
			Limit: 1000,
		})
		// TODO: Pagination
		if err != nil {
			return nil, err
		}

		type result struct {
			member  *objects.GuildMember
			listens int
		}

		results := []result{}
		for _, member := range members {
			discordId := fmt.Sprintf("%d", member.User.ID)
			count, err := listensRepo.GetUserListenCount(ctx, discordId)
			if err != nil {
				return nil, err
			}

			if count != 0 {
				results = append(results, result{listens: count, member: member})
			}
		}

		// Sort them in descending order.
		sort.Slice(results, func(i, j int) bool {
			return results[i].listens > results[j].listens
		})

		msg := "The champion is %s with %d scrobbles!"

		return &objects.InteractionResponse{
			Type: objects.ResponseChannelMessageWithSource,
			Data: &objects.InteractionApplicationCommandCallbackData{
				Content: fmt.Sprintf(msg, results[0].member.Nick, results[0].listens),
			},
		}, nil
	}

	return &Interaction{
		ApplicationCommand: &objects.ApplicationCommand{
			Name:              "np",
			Description:       "Shows your currently playing track",
			DefaultPermission: true,
		},
		handler: h,
	}
}
