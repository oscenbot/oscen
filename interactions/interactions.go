package interactions

import (
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"go.opentelemetry.io/otel/attribute"

	"github.com/Postcord/rest"

	"github.com/Postcord/objects"

	"go.uber.org/zap"
)

type handler = func(
	ctx context.Context,
	interaction *objects.Interaction,
	interactionData *objects.ApplicationCommandInteractionData,
) (*objects.InteractionResponse, error)

type Interaction struct {
	*objects.ApplicationCommand
	handler handler
}

type router struct {
	routes       map[string]handler
	interactions []*Interaction
	rest         *rest.Client
	log          *zap.Logger
	publicKey    []byte
}

func NewRouter(log *zap.Logger, publicKey ed25519.PublicKey, rest *rest.Client) *router {
	return &router{
		rest:         rest,
		routes:       map[string]handler{},
		interactions: []*Interaction{},
		log:          log,
		publicKey:    publicKey,
	}
}

func (r *router) Register(interactions ...*Interaction) error {
	for _, i := range interactions {
		r.log.Info("registering command with router", zap.String("name", i.Name))

		r.routes[i.Name] = i.handler
		r.interactions = append(r.interactions, i)
	}

	return nil
}

func (r *router) SyncInteractions(guildId *objects.Snowflake) error {
	usr, err := r.rest.GetCurrentUser()
	if err != nil {
		return err
	}

	for _, i := range r.interactions {
		if guildId != nil {
			_, err = r.rest.AddGuildCommand(usr.ID, *guildId, i.ApplicationCommand)
			if err != nil {
				return err
			}
		} else {
			_, err = r.rest.AddCommand(usr.ID, i.ApplicationCommand)
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (r *router) verifySignature(req *http.Request, body []byte) error {
	signatureHeader := req.Header.Get("X-Signature-ED25519")
	timestamp := []byte(req.Header.Get("X-Signature-Timestamp"))

	signature, err := hex.DecodeString(signatureHeader)
	if err != nil {
		return fmt.Errorf("could not decode signature: %w", err)
	}

	if ed25519.Verify(r.publicKey, append(timestamp, body...), signature) == false {
		return fmt.Errorf("invalid signature")
	}

	return nil
}

func (r *router) handleCommand(ctx context.Context, interaction *objects.Interaction) (*objects.InteractionResponse, error) {
	ctx, childSpan := tracer.Start(ctx, "interactions.handle_command")
	defer childSpan.End()

	r.log.Info("interaction", zap.Any("data", interaction))

	commandData := &objects.ApplicationCommandInteractionData{}
	err := json.Unmarshal(interaction.Data, commandData)
	if err != nil {
		return nil, err
	}

	childSpan.SetAttributes(attribute.String("io.oscen.command_name", commandData.Name))

	handler, ok := r.routes[commandData.Name]
	if !ok {
		return nil, fmt.Errorf(
			"cannot find handler for interaction: %s", commandData.Name,
		)
	}

	return handler(ctx, interaction, commandData)
}

type httpStatusErr struct {
	Code  int   `json:"code"`
	Cause error `json:"error"`
}

func wrapErrorForHTTP(code int, cause error) *httpStatusErr {
	return &httpStatusErr{
		Code:  code,
		Cause: cause,
	}
}

func (r *router) handleRequest(req *http.Request) (interface{}, *httpStatusErr) {
	body, err := ioutil.ReadAll(req.Body)
	if err != nil {
		return nil, wrapErrorForHTTP(401, err)
	}

	r.log.Debug("received request", zap.String("body", string(body)))

	err = r.verifySignature(req, body)
	if err != nil {
		return nil, wrapErrorForHTTP(401, err)
	}

	interaction := &objects.Interaction{}
	err = json.Unmarshal(body, &interaction)
	if err != nil {
		return nil, wrapErrorForHTTP(401, err)
	}

	switch interaction.Type {
	case objects.InteractionRequestPing:
		return objects.InteractionResponse{Type: objects.ResponsePong}, nil
	case objects.InteractionApplicationCommand:
		response, err := r.handleCommand(req.Context(), interaction)
		if err != nil {
			return nil, wrapErrorForHTTP(500, err)
		}
		return response, nil
	}

	return nil, wrapErrorForHTTP(404, fmt.Errorf("could not handle request"))
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	data, err := r.handleRequest(req)
	if err != nil {
		r.log.Error("failed to handle request", zap.Error(err.Cause))

		body, marshalErr := json.Marshal(err)
		if marshalErr != nil {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("fatal error serialising error"))
			return
		}

		w.WriteHeader(err.Code)
		_, _ = w.Write(body)
		return
	}

	if data != nil {
		body, marshalErr := json.Marshal(data)
		if marshalErr != nil {
			w.WriteHeader(500)
			_, _ = w.Write([]byte("fatal error serialising response"))
			return
		}
		r.log.Debug("writing response", zap.Any("data", data))

		headers := w.Header()
		headers.Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(body)
		return
	}

	w.WriteHeader(204)
}
