package interactionsrouter

import "github.com/bwmarrin/discordgo"
const testGuild = "669541384327528461"

type Handler func(s *discordgo.Session, i *discordgo.InteractionCreate) error
type router struct {
	routes map[string]Handler
	s *discordgo.Session
}

func New(s *discordgo.Session) *router {
	return &router{
		s: s,
		routes: map[string]Handler{},
	}
}

func (r *router) Register(cmd *discordgo.ApplicationCommand, h Handler) error {
	_, err := r.s.ApplicationCommandCreate(r.s.State.User.ID, testGuild, cmd)
	if err != nil {
		return err
	}

	r.routes[cmd.Name] = h

	return nil
}

func (r *router) Handle(s *discordgo.Session, i *discordgo.InteractionCreate) {
	handler, ok := r.routes[i.ApplicationCommandData().Name]
	if !ok {
		panic("handle this")
	}

	err := handler(s, i)
	if err != nil {
		panic("something went wrong, we should report this.")
	}
}