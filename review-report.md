# SocketIO_Go Code Review — 2026-05-13

> Reviewer: Codex  
> Commit: 517ad9c613b798e1ba29433ae80803e782866ec0  
> Verdict: ✅ Ship

## Phase 0 — Bootstrap
Per `AGENTS.md` §3.0 / §3.0.1, Codex did **not** execute any Go toolchain command. Bootstrap result below is copied from the owner-provided local verification block.

```text
最後驗證日期：2026-05-13（Round B+C 測試補完後）
驗證者：主人（本機 Windows + Go 1.24.1）

cd server-go && go build ./...      → ✅ exit 0
cd server-go && go vet ./...        → ✅ exit 0
cd server-go && go test ./... -count=1 -timeout 180s
                                    → ✅ ok  socketio-go-server  1.484s
                                       ?  socketio-go-server/cmd/make-token  [no test files]
                                    → 26 tests pass:
                                       - 4 TestAdmin_*   (admin namespace, JWT-only)
                                       - 8 TestAuth_*    (token sources, Bearer parser, wrong alg)
                                       - 3 TestConnect_* (auth gate)
                                       - 4 TestEvent_*   (welcome/ping/chat/room)
                                       - 5 TestPayload_* (incl. 8 MiB boundary)
                                       - 2 TestStress_*  (concurrent + broadcast fanout)
cd client && go build ./...         → ✅ exit 0
cd server && node -c index.js       → ✅ exit 0

go.sum integrity:
  server-go/go.sum: 136 lines, all required h1 + /go.mod h1 pairs verified
  client/go.sum:    62 lines, all required h1 + /go.mod h1 pairs verified
```

## Phase 1 — 對等性
`AGENTS.md` §2.5 intentional divergences are not findings: `MaxHttpBufferSize`, `welcome.message` literal, CORS preflight, `dial()` not waiting for connect, and dev-secret fallback.

| 項目 | Node | Go | 狀態 |
|---|---|---|---|
| `welcome` payload schema | `server/index.js:52` emits `{message,id,ts}` | `server-go/main.go:41` emits `{message,id,ts}` | ✅ Schema equivalent; message literal intentionally differs |
| `chat` 廣播語意 | `server/index.js:67` uses `io.emit` including sender | `server-go/main.go:67` uses `io.Emit` including sender | ✅ Equivalent |
| `ping` ack `{pong, echo, ts}` | `server/index.js:59` acks `{pong,echo,ts}` | `server-go/main.go:47` acks `{pong,echo,ts}` | ✅ Equivalent |
| `join` + `roomMsg` + `system` 順序 | `server/index.js:73` joins then emits room `system`; `server/index.js:79` emits `roomMsg` to room | `server-go/main.go:77` joins then emits room `system`; `server-go/main.go:84` emits `roomMsg` to room | ✅ Equivalent |
| `/admin` `welcome` / `op` | `server/index.js:89` namespace emits `{ns,id}` and acks `{ok:true}` | `server-go/main.go:103` namespace emits `{ns,id}` and acks `{ok:true}` | ✅ Equivalent |
| JWT middleware scope | `server/index.js:46` root and `server/index.js:90` `/admin` | `server-go/main.go:33` root and `server-go/main.go:105` `/admin` | ✅ Equivalent JWT-only scope |
| Token source priority | `server/index.js:16` auth → `server/index.js:18` query → `server/index.js:20` header | `server-go/auth.go:57` auth → `server-go/auth.go:62` query → `server-go/auth.go:65` header | ✅ Same priority |
| Bearer parser | `server/index.js:20` uses `/^Bearer\s+/i` | `server-go/auth.go:16` uses `(?i)^Bearer\s+` and `server-go/auth.go:66` applies it | ✅ Round B fix effective; case-insensitive + multi-whitespace now aligned |
| Claims schema (`uid`, `role`) | `server/index.js:34` verifies HS256 claims and `server/index.js:35` stores on `socket.data.claims` | `server-go/auth.go:21` defines `uid`, `role`; `server-go/auth.go:86` stores via `SetData` | ✅ Equivalent intended schema |
| Error payload `{code, reason}` | `server/index.js:29` / `server/index.js:39` send `unauthorized` with `{code,reason}` | `server-go/auth.go:80` sends `NewExtendedError("unauthorized", {code,reason})` | ✅ Equivalent JWT errors |
| `MaxHttpBufferSize` | Node default | `server-go/app.go:25` sets 8 MiB | ✅ Intentional divergence |
| CORS 設定 | `server/index.js:8` allows `origin: "*"` | `server-go/app.go:30` has no CORS setup | ✅ Intentional divergence |

### Round B+C Verification
- ✅ `server-go/auth.go` no longer defines `requireRole`; `/admin` is consistently JWT-only in Go and Node.
- ✅ Bearer parsing mismatch is fixed by `server-go/auth.go:16` / `server-go/auth.go:66`.
- ✅ No remaining Phase 1 differences outside the documented intentional divergences.

## Phase 2 — Security
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[Low]** `server-go/app.go:64` — `App.Shutdown()` correctly closes Socket.IO before HTTP, but still uses `httpServer.Close()` at `server-go/app.go:67`; acceptable for tests/dev, but use `Shutdown(ctx)` if production needs graceful drain of in-flight HTTP/polling requests.

### Security Checks
- ✅ HS256 is locked in both implementations: Node `jwt.verify(..., { algorithms: ["HS256"] })` at `server/index.js:34`; Go rejects non-HS256 in `server-go/auth.go:39`.
- ✅ `alg=none` / `RS256` confusion coverage exists in `server-go/auth_test.go:84` and `server-go/auth_test.go:102`.
- ✅ Token extraction order is aligned: auth payload → query → Authorization header.
- ✅ `/admin` policy is now explicitly JWT-only: Node mounts only JWT middleware at `server/index.js:90`; Go mounts only JWT middleware at `server-go/main.go:105`; `CLAUDE.md:135` documents that `role` is not an authorization gate.
- ✅ Raw token / secret logging was not found; Node logs parser messages at `server/index.js:38`, Go logs parser messages at `server-go/auth.go:79`, and the client logs only token length at `client/main.go:26`.
- ✅ Gin Socket.IO route is mounted outside gzip/writer-wrapping middleware; only `gin.Recovery()` and optional `gin.Logger()` are present at `server-go/app.go:31` / `server-go/app.go:33`.
- ℹ️ The 8 MiB buffer remains a known local-test tradeoff; rate limiting is still listed as out-of-scope in `CLAUDE.md:151`.

## Phase 3 — Concurrency
- Codex did not run `go test ./... -race -count=3` because `AGENTS.md:13` / `AGENTS.md:257` prohibit Go toolchain commands in this environment.
- ✅ `App.Shutdown()` ordering is correct: `server-go/app.go:65` closes Socket.IO before `server-go/app.go:67` closes HTTP.
- ✅ `setup_test.go` preserves the listener-before-connect pattern: `dial()` returns without waiting at `server-go/setup_test.go:49`, while `waitFor()` registers per-event listeners at `server-go/setup_test.go:231`.
- ✅ Broadcast fanout assertion is strict: `server-go/stress_test.go:77` waits for all `n` clients and `server-go/stress_test.go:103` fails if fewer than `n` receive the marker.
- ℹ️ Demo client ack handling at `client/main.go:43` has no explicit application-level timeout; acceptable for demo use, but add one before using it as automation or production tooling.

## Phase 4 — Tests
**Coverage status**:
- ✅ `/admin` coverage added: no token rejected, valid JWT with any role accepted, welcome payload, and `op` ack are covered in `server-go/admin_test.go:11`, `server-go/admin_test.go:22`, `server-go/admin_test.go:31`, and `server-go/admin_test.go:51`.
- ✅ Token-source coverage added: query token, Authorization header, case-insensitive/multi-space Bearer parsing, auth-over-query, and query-over-header are covered in `server-go/auth_test.go:17`, `server-go/auth_test.go:25`, `server-go/auth_test.go:33`, `server-go/auth_test.go:56`, and `server-go/auth_test.go:70`.
- ✅ Wrong-algorithm coverage added: `alg=none` and `RS256` rejection tests are present at `server-go/auth_test.go:84` and `server-go/auth_test.go:102`, with a parser sanity check at `server-go/auth_test.go:124`.
- ✅ Payload boundary coverage added: near-8 MiB success at `server-go/payload_test.go:192` and clear-over-8 MiB rejection/non-success at `server-go/payload_test.go:241`.
- ✅ Existing event tests cover welcome, ping ack, chat broadcast, and room scoping in `server-go/connection_test.go:43`, `server-go/connection_test.go:63`, `server-go/connection_test.go:113`, and `server-go/connection_test.go:147`.

**Missing coverage**:
- None blocking. If strict byte-boundary semantics become important, add exact 8 MiB and 8 MiB + 1 byte cases alongside the current near-max / 9 MiB tests.

## Phase 5 — Docs
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[Low]** `CLAUDE.md:168` / `README.md:143` / `AGENTS.md:32` — prose still says the Go server has 12 tests, while `AGENTS.md:96` records 26 passing tests; update the stale test-count text so docs match Round B+C.

### Docs Checks
- ✅ `CLAUDE.md:133` now documents the aligned Bearer parser.
- ✅ `CLAUDE.md:135` now documents `/admin` as JWT-only and explicitly states `role` does not gate authorization.
- ✅ The CLAUDE event table remains consistent with `server/index.js` and `server-go/main.go` for `welcome`, `chat`, `ping`, `join`, `roomMsg`, `system`, and `/admin op`.
- ✅ README exists; no recommendation to create one is needed.

## 建議下一步
1. Update stale test-count docs at `CLAUDE.md:168`, `README.md:143`, and `AGENTS.md:32` from 12 to 26.
2. Optionally replace `server-go/app.go:67` with graceful `Shutdown(ctx)` if this server is promoted beyond local/demo testing.
3. Optional hardening only: add application-level ack timeout in `client/main.go:43` if the demo client becomes automation tooling.
