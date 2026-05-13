// make-token: dev helper that prints a HS256 JWT signed with the same secret
// the server uses ($JWT_SECRET, default "dev-secret-change-me").
//
//   go run ./cmd/make-token -uid alice -role admin -ttl 1h
//   go run ./cmd/make-token -uid bob -ttl -1m            # already expired
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const defaultSecret = "dev-secret-change-me"

type claims struct {
	UserID string `json:"uid"`
	Role   string `json:"role,omitempty"`
	jwt.RegisteredClaims
}

func main() {
	uid := flag.String("uid", "alice", "user id (sub) claim")
	role := flag.String("role", "user", "role claim")
	ttl := flag.Duration("ttl", time.Hour, "token lifetime; use negative for expired")
	flag.Parse()

	secret := os.Getenv("JWT_SECRET")
	if secret == "" {
		secret = defaultSecret
	}

	c := &claims{
		UserID: *uid,
		Role:   *role,
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   *uid,
			IssuedAt:  jwt.NewNumericDate(time.Now()),
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(*ttl)),
		},
	}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString([]byte(secret))
	if err != nil {
		fmt.Fprintln(os.Stderr, "sign error:", err)
		os.Exit(1)
	}
	fmt.Println(tok)
}
