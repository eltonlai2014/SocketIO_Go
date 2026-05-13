package main

import (
	"errors"
	"log"
	"os"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/zishang520/socket.io/v2/socket"
)

// Default dev secret — DO NOT use in production. Override with JWT_SECRET env.
const defaultSecret = "dev-secret-change-me"

type Claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func jwtSecret() []byte {
	if s := os.Getenv("JWT_SECRET"); s != "" {
		return []byte(s)
	}
	return []byte(defaultSecret)
}

// verifyJWT parses and validates an HS256 token, returning the claims on success.
func verifyJWT(tokenStr string) (*Claims, error) {
	if tokenStr == "" {
		return nil, errors.New("missing token")
	}
	tok, err := jwt.ParseWithClaims(tokenStr, &Claims{}, func(t *jwt.Token) (any, error) {
		if t.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected signing method")
		}
		return jwtSecret(), nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("invalid token")
	}
	return claims, nil
}

// extractToken pulls a token from auth payload, query string, or Authorization header.
func extractToken(auth any, query map[string][]string, headers map[string][]string) string {
	if m, ok := auth.(map[string]any); ok {
		if t, ok := m["token"].(string); ok && t != "" {
			return t
		}
	}
	if v := query["token"]; len(v) > 0 && v[0] != "" {
		return v[0]
	}
	if v := headers["Authorization"]; len(v) > 0 {
		return strings.TrimPrefix(v[0], "Bearer ")
	}
	return ""
}

// jwtMiddleware builds a namespace middleware that verifies the JWT token sent
// by the client and stashes the claims on the socket via SetData.
func jwtMiddleware(nsp string) func(*socket.Socket, func(*socket.ExtendedError)) {
	return func(client *socket.Socket, next func(*socket.ExtendedError)) {
		hs := client.Handshake()
		token := extractToken(hs.Auth, hs.Query, hs.Headers)
		claims, err := verifyJWT(token)
		if err != nil {
			log.Printf("[%s] auth reject sid=%s: %v", nsp, client.Id(), err)
			next(socket.NewExtendedError("unauthorized", map[string]any{
				"code":   "AUTH_FAILED",
				"reason": err.Error(),
			}))
			return
		}
		client.SetData(claims)
		next(nil)
	}
}

// requireRole returns a middleware that rejects unless the socket's claims have
// the given role. Must run AFTER jwtMiddleware on the same namespace.
func requireRole(nsp, role string) func(*socket.Socket, func(*socket.ExtendedError)) {
	return func(client *socket.Socket, next func(*socket.ExtendedError)) {
		claims, _ := client.Data().(*Claims)
		if claims == nil || claims.Role != role {
			log.Printf("[%s] role reject sid=%s want=%s got=%v", nsp, client.Id(), role, claims)
			next(socket.NewExtendedError("forbidden", map[string]any{
				"code":     "ROLE_REQUIRED",
				"required": role,
			}))
			return
		}
		next(nil)
	}
}

// signTokenForDev is used by the make-token CLI; placed here so the secret
// resolution stays in one file.
func signTokenForDev(userID, role string, ttl time.Duration) (string, error) {
	claims := &Claims{
		UserID: userID,
		Role:   role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   userID,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
		},
	}
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(jwtSecret())
}
