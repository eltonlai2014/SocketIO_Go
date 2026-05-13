# SocketIO_Go Code Review — 2026-05-13

> Reviewer: Codex  
> Commit: 73394c2d6484ec2c6c5dec0550fc13ede49adf44  
> Verdict: ⚠️ Ship with notes

## Phase 0 — Bootstrap
Per AGENTS.md §3.0, Codex did not execute any Go toolchain commands. Bootstrap result below is copied from AGENTS.md §3.0.1.

```text
最後驗證日期：2026-05-13
驗證者：主人（本機 Windows + Go 1.24.1）

cd server-go && go build ./...      → ✅ exit 0
cd server-go && go vet ./...        → ✅ exit 0
cd server-go && go test ./... -count=1 -timeout 120s
                                    → ✅ ok  socketio-go-server  1.994s
                                       ?  socketio-go-server/cmd/make-token  [no test files]
cd client && go build ./...         → ✅ exit 0
cd server && node -c index.js       → ✅ exit 0

go.sum integrity:
  server-go/go.sum: 136 lines, all required h1 + /go.mod h1 pairs verified
  client/go.sum:    62 lines, all required h1 + /go.mod h1 pairs verified
```

## Phase 1 — 對等性
AGENTS.md §2.5 intentional divergences are not findings: `MaxHttpBufferSize`, `welcome.message` literal, CORS preflight, `dial()` not waiting for connect, and dev-secret fallback.

| 項目 | Node | Go | 狀態 |
|---|---|---|---|
| `welcome` payload schema | `server/index.js:52` emits `{message,id,ts}` | `server-go/main.go:41` emits `{message,id,ts}` | ⚪ Schema equivalent; message literal intentionally differs |
| `chat` 廣播語意 | `server/index.js:67` uses `io.emit` including sender | `server-go/main.go:67` uses `io.Emit` including sender | ✅ Equivalent |
| `ping` ack `{pong, echo, ts}` | `server/index.js:59` acks `{pong,echo,ts}` | `server-go/main.go:47` acks `{pong,echo,ts}` | ✅ Equivalent |
| `join` + `roomMsg` + `system` | `server/index.js:73` joins then emits room `system`; `server/index.js:79` emits `roomMsg` to room | `server-go/main.go:77` joins then emits room `system`; `server-go/main.go:84` emits `roomMsg` to room | ✅ Equivalent |
| `/admin` `welcome` / `op` | `server/index.js:89` namespace emits `{ns,id}` and acks `{ok:true}` | `server-go/main.go:103` namespace emits `{ns,id}` and acks `{ok:true}` | ✅ Event-equivalent |
| JWT middleware scope | `server/index.js:46` root and `server/index.js:90` `/admin` | `server-go/main.go:33` root and `server-go/main.go:105` `/admin` | ✅ Equivalent JWT scope |
| Token source priority | `server/index.js:16` auth → `server/index.js:18` query → `server/index.js:20` header | `server-go/auth.go:53` auth → `server-go/auth.go:58` query → `server-go/auth.go:61` header | ✅ Same priority |
| Claims schema (`uid`, `role`) | `server/index.js:35` stores verified claims on `socket.data.claims` | `server-go/auth.go:17` defines `uid`, `role`, registered claims; `server-go/auth.go:82` stores claims | ✅ Equivalent intended schema |
| Error payload `{code, reason}` | `server/index.js:29` / `server/index.js:39` send `unauthorized` with `{code,reason}` | `server-go/auth.go:76` sends `NewExtendedError("unauthorized", {code,reason})` | ✅ Equivalent JWT errors |
| `MaxHttpBufferSize` | Node default | `server-go/app.go:25` sets 8 MiB | ⚪ Intentional divergence |
| CORS 設定 | `server/index.js:8` allows `origin: "*"` | `server-go/app.go:30` has no CORS setup | ⚪ Intentional divergence |

### Findings
- **[Medium]** `server-go/auth.go:62` — Go only accepts exact `Authorization: Bearer `, while Node accepts case-insensitive `Bearer\s+` at `server/index.js:20`; header-token extraction is not equivalent — choose one rule and align both implementations plus tests.

## Phase 2 — Security
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[High]** `server-go/main.go:105` — `/admin` mounts only `jwtMiddleware("/admin")`; `requireRole` is defined at `server-go/auth.go:89` but never mounted, despite the security checklist requiring order verification — decide whether `/admin` is JWT-only or admin-role-only, then either mount `requireRole("/admin", "admin")` after JWT and mirror Node/docs/tests, or remove/document the unused middleware.
- **[Medium]** `server-go/app.go:25` — the intentional 8 MiB buffer increases memory/DoS exposure, and no rate limit or per-IP cap exists on `/socket.io/*any`; `CLAUDE.md:149` lists rate limiting as out-of-scope — add bounded connection/message limits before exposing beyond local testing.
- **[Medium]** `server-go/auth.go:35` — HS256 is enforced by the method check at `server-go/auth.go:36`, but `IssuedAt` from `server-go/auth.go:112` has no explicit verification policy/test; if future-issued tokens must be rejected, add parser options and tests.
- **[Low]** `server-go/auth.go:75` — auth rejection logs include parser error strings but not raw tokens; acceptable as-is, but keep raw tokens/secrets out of future diagnostics.
- **[Low]** `client/main.go:26` — the client logs JWT length only, not token contents; no raw token/secret logging found.

## Phase 3 — Concurrency
- Race tests were not run by Codex because AGENTS.md forbids Go toolchain commands; Phase 0 is owner-provided.
- `App.Shutdown()` closes Socket.IO first at `server-go/app.go:65`, which matches the required ordering.
- `dial()` intentionally does not wait for connect per AGENTS.md §2.5 and CLAUDE.md rule 12, so it is not a finding.
- `server-go/stress_test.go:65` uses strict N/N broadcast fanout (`wg.Add(n)` and timeout failure at `server-go/stress_test.go:102`), so assertion strength is good.

### Findings
- **[Low]** `server-go/app.go:67` — shutdown uses `httpServer.Close()` rather than bounded graceful `Shutdown(ctx)` after `IO.Close(nil)`; this can force-close in-flight polling requests and make leak/debug output noisy — prefer graceful shutdown with timeout.
- **[Low]** `client/main.go:43` — demo client emits `ping` with ack but has no explicit user-visible ack timeout; a lost ack leaves the callback silent — wrap ack waits where CLI behavior matters.
- **[Low]** `server-go/stress_test.go:36` — stress goroutines call helpers that can fail the test from child goroutines; failures may be harder to attribute and can race with cleanup — return errors through channels instead.

## Phase 4 — Tests
**Coverage observed**:
- `server-go/connection_test.go:12` covers no-token rejection, `server-go/connection_test.go:22` covers expired-token rejection, and `server-go/connection_test.go:33` covers valid-token acceptance.
- `server-go/connection_test.go:43` covers root `welcome`, `server-go/connection_test.go:63` covers `ping` ack, `server-go/connection_test.go:102` covers `chat`, and `server-go/connection_test.go:147` covers room scoping.
- `server-go/stress_test.go:20` checks concurrent connects and `server-go/stress_test.go:65` checks strict N/N broadcast fanout.
- `server-go/payload_test.go:15` checks 1 MiB round-trip, `server-go/payload_test.go:68` checks 500 burst acks, and `server-go/payload_test.go:120` checks 5 concurrent 256 KiB payloads.

**Missing coverage**:
- **[Medium]** `server-go/main.go:103` — `/admin` namespace `welcome` and `op` ack are implemented but no test connects to `/admin` or verifies `{ok:true}` — add admin namespace tests, including the chosen role policy.
- **[Medium]** `server-go/auth.go:53` — tests use only `handshake.auth.token`; there is no coverage for `query.token`, `Authorization`, or precedence conflicts — add tests for all three token sources and priority order.
- **[Medium]** `server-go/auth.go:36` — HS256 enforcement has no negative test for `alg=none` or RS256 confusion — add explicit wrong-alg JWT rejection tests.
- **[Medium]** `server-go/payload_test.go:19` — large-payload coverage stops at 1 MiB even though Go intentionally sets 8 MiB at `server-go/app.go:25`; there is no just-under/at/over boundary test for the configured Go limit — add boundary tests aligned with the 8 MiB contract.

## Phase 5 — Docs
- README exists and uses `go mod download` at `README.md:52` and `README.md:92`; no forbidden `go mod tidy` / `go get @latest` setup instruction found in README.
- `CLAUDE.md:106` also uses `go mod download`, and `CLAUDE.md:111`–`CLAUDE.md:113` explicitly forbid `go mod tidy`, `go get <package>@latest`, and `go get -u`.
- `CLAUDE.md:79` event table matches implemented event names/directions at schema level.

### Findings
- **[Low]** `CLAUDE.md:95` — Go namespace switching text references `manager.Socket("/", nil)`, but current `client/main.go:31` uses `socket.Connect(serverURL, opts)` directly and README documents `socket.Connect(serverURL+"admin", opts)` at `README.md:220` — update CLAUDE.md to match the current client API.

## 建議下一步
1. Decide and implement the `/admin` authorization policy: JWT-only or admin-role-only.
2. Align Authorization `Bearer` parsing between Node and Go, then add token-source precedence tests.
3. Add missing tests for `/admin`, wrong JWT algorithms, and 8 MiB payload boundaries.
4. Consider graceful HTTP shutdown and ack timeout improvements as reliability hardening.
5. Update CLAUDE.md namespace-switching instructions to match `socket.Connect` usage.
