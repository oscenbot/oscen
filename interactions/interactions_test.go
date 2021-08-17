package interactionsrouter

import (
	"testing"

	"github.com/bwmarrin/discordgo"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestNew(t *testing.T) {
	s := &discordgo.Session{}
	log := zaptest.NewLogger(t)
	rtr := New(s, log)

	assert.Equal(t, s, rtr.s)
	assert.Equal(t, log, rtr.log)
	assert.NotNil(t, rtr.routes)
}
