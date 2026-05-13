package main

import (
	"encoding/base64"
	"strings"
	"testing"
	"time"

	jwtlib "github.com/golang-jwt/jwt/v5"
)

// --- Token source precedence ----------------------------------------------
// Priority: handshake.auth.token > query.token > Authorization: Bearer ...
// Each source alone must work (covered by Accepted_*); precedence test
// (Auth_Beats_Query) confirms the highest source wins when more than one is set.

func TestAuth_QueryToken_Accepted(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := dialWithQuery(t, tok)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("query token should be accepted, got: %v", err)
	}
}

func TestAuth_AuthorizationHeader_Accepted(t *testing.T) {
	tok := mustToken(t, "alice", "user", time.Hour)
	sock := dialWithHeader(t, "Bearer "+tok)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("Authorization Bearer header should be accepted, got: %v", err)
	}
}

func TestAuth_AuthorizationHeader_CaseInsensitiveAndMultiSpace(t *testing.T) {
	// Node uses `replace(/^Bearer\s+/i, "")`; Go must match.
	tok := mustToken(t, "alice", "user", time.Hour)
	cases := []string{
		"Bearer " + tok,    // canonical
		"bearer " + tok,    // lowercase
		"BEARER " + tok,    // uppercase
		"Bearer  " + tok,   // two spaces
		"Bearer\t" + tok,   // tab
		"Bearer \t " + tok, // mixed whitespace
	}
	for _, header := range cases {
		t.Run(strings.ReplaceAll(header[:min(12, len(header))], "\t", "TAB"), func(t *testing.T) {
			sock := dialWithHeader(t, header)
			if err := waitForConnect(t, sock, 3*time.Second); err != nil {
				t.Fatalf("header %q should accept token, got: %v", header, err)
			}
		})
	}
}

// TestAuth_AuthBeatsQuery: when both auth payload and query are set with
// different tokens, auth payload wins (it's checked first in extractToken).
func TestAuth_AuthBeatsQuery(t *testing.T) {
	validTok := mustToken(t, "alice", "user", time.Hour)
	expiredTok := mustToken(t, "mallory", "user", -1*time.Minute)

	// auth.token = valid, query.token = expired => should connect (auth wins)
	opts := makeDualSourceOpts(t, validTok, expiredTok, "")
	sock := connectAndCleanup(t, testURL, opts)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("auth payload should win over query, got: %v", err)
	}
}

// TestAuth_QueryBeatsHeader: when query and header are both set with
// different tokens, query wins (auth payload absent; query checked before header).
func TestAuth_QueryBeatsHeader(t *testing.T) {
	validTok := mustToken(t, "alice", "user", time.Hour)
	expiredTok := mustToken(t, "mallory", "user", -1*time.Minute)

	// auth = "", query = valid, header = expired => should connect (query wins)
	opts := makeDualSourceOpts(t, "", validTok, "Bearer "+expiredTok)
	sock := connectAndCleanup(t, testURL, opts)
	if err := waitForConnect(t, sock, 3*time.Second); err != nil {
		t.Fatalf("query should win over header, got: %v", err)
	}
}

// --- Wrong-algorithm rejection --------------------------------------------

func TestAuth_AlgNone_Rejected(t *testing.T) {
	// jwt/v5 explicitly disallows alg=none by default unless you opt in.
	// We build the token manually so the server actually receives an alg=none
	// header and proves verifyJWT rejects it on the alg check, not the lib.
	claims := `{"sub":"mallory","uid":"mallory","role":"user","exp":9999999999}`
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"none","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	tok := header + "." + payload + "." // empty signature

	_, err := dialAndWait(t, tok, 3*time.Second)
	if err == nil {
		t.Fatal("expected connect_error for alg=none token, got success")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized for alg=none, got: %v", err)
	}
}

func TestAuth_AlgRS256_Rejected(t *testing.T) {
	// Classic RS256 confusion attempt: present a token claiming RS256 algorithm.
	// Server's keyfunc checks alg before signature verification, so we don't
	// even need a real RSA-signed token — an arbitrary signature is enough.
	claims := `{"sub":"mallory","uid":"mallory","role":"admin","exp":9999999999}`
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	sig := base64.RawURLEncoding.EncodeToString([]byte("not-a-real-signature"))
	tok := header + "." + payload + "." + sig

	_, err := dialAndWait(t, tok, 3*time.Second)
	if err == nil {
		t.Fatal("expected connect_error for RS256-claimed token, got success")
	}
	if !strings.Contains(err.Error(), "unauthorized") {
		t.Fatalf("expected unauthorized for RS256 claim, got: %v", err)
	}
}

// Sanity check: verify our manually-built token format actually parses through
// jwt/v5 — this guards against base64 / segment issues making the test
// accidentally pass because the token was malformed.
func TestAuth_ManualToken_ParsesAsClaimedAlg(t *testing.T) {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	payload := base64.RawURLEncoding.EncodeToString([]byte(`{"sub":"x"}`))
	sig := base64.RawURLEncoding.EncodeToString([]byte("x"))
	tok := header + "." + payload + "." + sig

	parsed, _, err := jwtlib.NewParser().ParseUnverified(tok, jwtlib.MapClaims{})
	if err != nil {
		t.Fatalf("manual token did not parse: %v", err)
	}
	if got := parsed.Method.Alg(); got != "RS256" {
		t.Fatalf("manual token alg: got %q, want RS256", got)
	}
}

