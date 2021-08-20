package interactions

import (
	"crypto/ed25519"
	"testing"

	"github.com/Postcord/rest"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

func TestNew(t *testing.T) {
	s := &rest.Client{}
	log := zaptest.NewLogger(t)
	key := ed25519.PublicKey{12}
	rtr := NewRouter(log, key, s)

	assert.Equal(t, s, rtr.rest)
	assert.Equal(t, log, rtr.log)
	assert.Equal(t, key, rtr.publicKey)
	assert.NotNil(t, rtr.routes)
}
