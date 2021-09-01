package middleware

import (
	"context"
	"crypto/sha1"
	"fmt"
	"net/http"
	"strings"

	"github.com/honeycombio/beeline-go"
)

func authHandler(next http.Handler, authTokens []string) http.Handler {
	authTokenMap := stringSliceToMap(authTokens)

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()
		clientID, err := authenticate(r, authTokenMap)
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

func authenticate(r *http.Request, authTokenMap map[string]struct{}) (string, error) {
	tok, err := authTokenFromRequest(r)
	if err != nil {
		beeline.AddField(r.Context(), "auth_result", "error")
		return "", err
	}

	authProvided := tok != ""
	_, authValid := authTokenMap[tok]

	switch {
	case authValid:
		return authTokenToClientID(tok), nil
	case !authProvided:
		return "", nil
	default:
		return "", errInvalidAuthToken
	}
}

func authTokenToClientID(tok string) string {
	return fmt.Sprintf("%x", sha1.Sum([]byte(tok)))
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

func stringSliceToMap(xs []string) map[string]struct{} {
	m := make(map[string]struct{}, len(xs))
	for _, k := range xs {
		k = strings.TrimSpace(k)
		if k != "" {
			m[k] = struct{}{}
		}
	}
	return m
}
