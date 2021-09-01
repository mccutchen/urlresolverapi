package middleware

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/honeycombio/beeline-go"
)

// AuthMap maps from opaque token value to client ID.
type AuthMap map[string]string

func authHandler(next http.Handler, authMap AuthMap) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		clientID, err := authenticate(r, authMap)
		if err != nil {
			beeline.AddField(ctx, "client_authenticated", false)
			beeline.AddField(ctx, "error", err)
			sendAuthError(w)
			return
		}

		beeline.AddField(ctx, "client_authenticated", clientID != "")
		beeline.AddField(ctx, "client_id", clientID)

		r = r.WithContext(contextWithClientID(r.Context(), clientID))
		next.ServeHTTP(w, r)
	})
}

func authenticate(r *http.Request, authMap AuthMap) (string, error) {
	tok, err := authTokenFromRequest(r)
	if err != nil {
		beeline.AddField(r.Context(), "auth_result", "error")
		return "", err
	}

	authProvided := tok != ""
	clientID, authValid := authMap[tok]

	switch {
	case authValid:
		return clientID, nil
	case !authProvided:
		return "", nil
	default:
		return "", errInvalidAuthToken
	}
}

type clientIDKeyType int

const clientIDKey = clientIDKeyType(1)

func clientIDFromContext(ctx context.Context) string {
	v := ctx.Value(clientIDKey)
	if clientID, ok := v.(string); ok {
		return clientID
	}
	return ""
}

func contextWithClientID(ctx context.Context, clientID string) context.Context {
	return context.WithValue(ctx, clientIDKey, clientID)
}

func authTokenFromRequest(r *http.Request) (string, error) {
	val := r.Header.Get("Authorization")
	if val == "" {
		return "", nil
	}

	if !strings.HasPrefix(strings.ToLower(val), "token ") {
		return "", errInvalidAuthHeaderFormat
	}

	parts := strings.Fields(val)
	if len(parts) != 2 {
		return "", errInvalidAuthTokenFormat
	}

	return parts[1], nil
}

func sendAuthError(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusForbidden)
	_, _ = w.Write([]byte(`{"error": "unauthorized"}`))
}

// ParseAuthMap takes a slice of token strings in "client-id:token-value"
// form and returns a mapping from token value to client ID.
func ParseAuthMap(tokenConfig string) (AuthMap, error) {
	if len(strings.TrimSpace(tokenConfig)) == 0 {
		return nil, nil
	}

	tokenDefs := strings.Split(tokenConfig, ",")
	authMap := make(map[string]string, len(tokenDefs)) // mapping from token value to client ID
	for _, tokenDef := range tokenDefs {
		parts := strings.SplitN(tokenDef, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf(`invalid token format %q, token must be in "client-id:token-value" format`, tokenDef)
		}
		clientID := strings.TrimSpace(parts[0])
		token := strings.TrimSpace(parts[1])
		if clientID == "" {
			return nil, fmt.Errorf("auth token %q has empty client ID", tokenDef)
		}
		if token == "" || strings.Contains(token, " ") {
			return nil, fmt.Errorf("auth token value in %q cannot be empty or contain spaces", tokenDef)
		}
		if _, found := authMap[token]; found {
			return nil, fmt.Errorf("duplicate auth token value %q", token)
		}

		authMap[token] = clientID
	}

	return authMap, nil
}
