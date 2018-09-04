package web

import (
	"log"
	"net/http"

	"code.cloudfoundry.org/loggregator/rlp-gateway/internal/auth"
	"github.com/golang/protobuf/jsonpb"
	"github.com/gorilla/mux"
)

// CFAuthMiddlewareProvider defines middleware for authenticating CF clients
type CFAuthMiddlewareProvider struct {
	oauth2Reader  Oauth2ClientReader
	logAuthorizer LogAuthorizer
	marshaller    jsonpb.Marshaler
}

// Oauth2ClientReader defines the interface for retrieving an OAuth2 client
type Oauth2ClientReader interface {
	Read(token string) (auth.Oauth2Client, error)
}

// LogAuthorizer defines the interface for validating and providing access to
// logs
type LogAuthorizer interface {
	IsAuthorized(sourceID, token string) bool
	AvailableSourceIDs(token string) []string
}

// NewCFAuthMiddlewareProvider creates a CFAuthMiddlewareProvider
func NewCFAuthMiddlewareProvider(
	oauth2Reader Oauth2ClientReader,
	logAuthorizer LogAuthorizer,
) CFAuthMiddlewareProvider {
	return CFAuthMiddlewareProvider{
		oauth2Reader:  oauth2Reader,
		logAuthorizer: logAuthorizer,
	}
}

// Middleware provides an http.Handler for checking access to to logs
func (m CFAuthMiddlewareProvider) Middleware(h http.Handler) http.Handler {
	router := mux.NewRouter()

	router.HandleFunc("/v2/read", func(w http.ResponseWriter, r *http.Request) {
		authToken := r.Header.Get("Authorization")
		if authToken == "" {
			http.Error(w, errNotFound.Error(), http.StatusNotFound)
			return
		}

		c, err := m.oauth2Reader.Read(authToken)
		if err != nil {
			log.Printf("failed to read from Oauth2 server: %s", err)
			http.Error(w, errNotFound.Error(), http.StatusNotFound)
			return
		}

		sourceIDs := r.URL.Query()["source_id"]

		if !c.IsAdmin && len(sourceIDs) == 0 {
			http.Error(w, errNotFound.Error(), http.StatusNotFound)
			return
		}

		for _, sourceID := range sourceIDs {
			if !c.IsAdmin {
				if !m.logAuthorizer.IsAuthorized(sourceID, authToken) {
					http.Error(w, errNotFound.Error(), http.StatusNotFound)
					return
				}
			}
		}

		h.ServeHTTP(w, r)
	})

	return router
}
