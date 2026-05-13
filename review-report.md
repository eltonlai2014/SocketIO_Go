# SocketIO_Go Code Review — 2026-05-13

> Reviewer: Codex  
> Commit: 73394c2d6484ec2c6c5dec0550fc13ede49adf44  
> Verdict: ❌ Block

## Phase 0 — Bootstrap
- Build: ❌ Go build did not complete; Node syntax check passed.
- Tests: ❌ 0/12 executed; Go package setup failed before tests could run.
- Vet: ❌ did not complete; Go package setup/cache failed before analysis.
- Environment-limited bootstrap notes: `go mod download` failed via `proxy.golang.org` through `127.0.0.1:9`, and Go cache writes under `AppData\Local\go-build` hit `Access is denied`; these are environment limitations and are not findings.
- Node syntax: ✅ `cd server && node -c index.js` passed.
- Race run: ⚠️ `go test ./... -race -count=3` first failed because `-race requires cgo`; with `CGO_ENABLED=1`, package setup/cache still failed, so no race result is available.

### go.sum grep verification
For each `missing go.sum entry` module, I ran the two required independent checks: module-content `h1:` and `/go.mod h1:`. Network/cache misses without `missing go.sum entry` were not marked Critical.

```text
server-go/go.sum :: github.com/golang-jwt/jwt/v5 v5.3.1 :: module-content h1
<no match>
server-go/go.sum :: github.com/golang-jwt/jwt/v5 v5.3.1 :: go.mod h1
server-go/go.sum:16: github.com/golang-jwt/jwt/v5 v5.3.1/go.mod h1:fxCRLWMO43lRc8nhHWY6LGqRcf+1gQWArsqaEUEa5bE=

client/go.sum :: github.com/zishang520/engine.io-client-go v1.1.0 :: module-content h1
<no match>
client/go.sum :: github.com/zishang520/engine.io-client-go v1.1.0 :: go.mod h1
client/go.sum:10: github.com/zishang520/engine.io-client-go v1.1.0/go.mod h1:4rXW69vdgWxm0F3jHV1LHITR77Jif19gmjJozG8DhQ4=

client/go.sum :: github.com/zishang520/engine.io/v2 v2.5.0 :: module-content h1
<no match>
client/go.sum :: github.com/zishang520/engine.io/v2 v2.5.0 :: go.mod h1
client/go.sum:12: github.com/zishang520/engine.io/v2 v2.5.0/go.mod h1:ohfMsnzOCA9NEklEGiQ5Y9j6cWvzLNeVFaB+Bkn1KcQ=

client/go.sum :: github.com/zishang520/socket.io-client-go v1.1.0 :: module-content h1
<no match>
client/go.sum :: github.com/zishang520/socket.io-client-go v1.1.0 :: go.mod h1
client/go.sum:13: github.com/zishang520/socket.io-client-go v1.1.0/go.mod h1:pFkvhEgVIjkUJyNvOvb62k78aFt2qwgJFNh2rfaCOc0=
```

### 🔴 Critical
- **[Critical]** `server-go/go.sum:16` — `github.com/golang-jwt/jwt/v5 v5.3.1` has the `/go.mod` checksum but no module-content `h1:` checksum, and `server-go/cmd/make-token/main.go:14` imports it directly — restore the locked module-content checksum without changing versions.
- **[Critical]** `client/go.sum:10` — `github.com/zishang520/engine.io-client-go v1.1.0` has the `/go.mod` checksum but no module-content `h1:` checksum, and `client/main.go:10` imports it directly — restore the locked module-content checksum.
- **[Critical]** `client/go.sum:12` — `github.com/zishang520/engine.io/v2 v2.5.0` has the `/go.mod` checksum but no module-content `h1:` checksum, and `client/main.go:11` imports it directly — restore the locked module-content checksum.
- **[Critical]** `client/go.sum:13` — `github.com/zishang520/socket.io-client-go v1.1.0` has the `/go.mod` checksum but no module-content `h1:` checksum, and `client/main.go:12` imports it directly — restore the locked module-content checksum.

## Phase 1 — 對等性
Known AGENTS.md 2.5 intentional divergences were skipped as findings: `MaxHttpBufferSize`, `welcome.message` literal, `dial()` not waiting for connect, and dev-secret fallback.

| 項目 | Node | Go | 狀態 |
|---|---|---|---|
| `welcome` payload schema | `server/index.js:52` emits `{message,id,ts}` | `server-go/main.go:41` emits `{message,id,ts}` | ⚪ Equivalent schema; literal difference is intentional |
| `chat` broadcast | `server/index.js:67` uses `io.emit`, including sender | `server-go/main.go:67` uses `io.Emit`, including sender | ✅ Equivalent |
| `ping` ack | `server/index.js:59` acks `{pong,echo,ts}` | `server-go/main.go:47` acks `{pong,echo,ts}` | ✅ Equivalent |
| `join` / `system` / `roomMsg` | `server/index.js:73` joins then emits room `system`; `server/index.js:79` emits room `roomMsg` | `server-go/main.go:77` joins then emits room `system`; `server-go/main.go:84` emits room `roomMsg` | ✅ Equivalent |
| `/admin` events | `server/index.js:89` emits `{ns,id}` and acks `{ok:true}` | `server-go/main.go:103` emits `{ns,id}` and acks `{ok:true}` | ✅ Event-equivalent |
| JWT middleware scope | `server/index.js:46` and `server/index.js:90` | `server-go/main.go:33` and `server-go/main.go:105` | ✅ Equivalent JWT scope |
| Token source priority | `server/index.js:16` auth → `server/index.js:18` query → `server/index.js:20` header | `server-go/auth.go:53` auth → `server-go/auth.go:58` query → `server-go/auth.go:61` header | ✅ Same priority |
| Authorization parser | `server/index.js:20` accepts case-insensitive `Bearer\s+` | `server-go/auth.go:62` trims only exact `Bearer ` | ⚠️ Difference |
| Error payload | `server/index.js:29` / `server/index.js:39` send `unauthorized` with `{code,reason}` | `server-go/auth.go:76` sends `NewExtendedError("unauthorized", {code,reason})` | ✅ Equivalent JWT errors |
| CORS / preflight | `server/index.js:8` allows `origin: "*"` | `server-go/app.go:30` has no CORS/OPTIONS setup; only GET/POST socket routes at `server-go/app.go:41` | ❌ Difference |

### Findings
- **[High]** `server-go/app.go:41` — Go has no CORS/OPTIONS equivalent to Node's `cors: { origin: "*" }` at `server/index.js:8`; browser clients that work against Node can fail against Go, especially if they use headers/preflight — add an explicit Go-side CORS policy or intentionally narrow both sides and document it.
- **[Medium]** `server-go/auth.go:62` — Go only accepts exact `Authorization: Bearer ` while Node accepts case-insensitive `Bearer\s+` at `server/index.js:20`; token extraction behavior is not equivalent — choose one rule and align implementation plus tests.

## Phase 2 — Security
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[High]** `server-go/main.go:105` — `/admin` mounts only `jwtMiddleware("/admin")`; `requireRole` is defined at `server-go/auth.go:89` but never mounted, so any valid JWT role can call admin `op` — either mount `requireRole("/admin", "admin")` after JWT and mirror the policy in Node, or remove/document the unused middleware as intentionally auth-only.
- **[Medium]** `server-go/app.go:25` — the intentional 8 MiB buffer increases memory/DoS exposure, and there is no rate limit or per-IP cap on `/socket.io/*any`; `CLAUDE.md:149` explicitly lists rate limit as out-of-scope — add a bounded connection/message limit before exposing beyond local testing.
- **[Medium]** `server-go/auth.go:35` — HS256 is checked at `server-go/auth.go:36`, but there is no explicit parser option/test for `iat` semantics; `signTokenForDev` sets `IssuedAt` at `server-go/auth.go:112`, yet future-issued tokens are not covered — add an explicit policy and tests if `iat` must be enforced.
- **[Low]** `server-go/auth.go:75` — auth rejection logs include parser error strings but not raw tokens; acceptable as-is, but future diagnostics should keep raw token/secret out of logs.
- **[Low]** `client/main.go:26` — client logs JWT length only, not raw token contents; no raw secret/token logging found.

## Phase 3 — Concurrency
- Race validation could not run because of cgo/package setup/cache limitations from Phase 0.
- `App.Shutdown()` closes Socket.IO first at `server-go/app.go:65`, matching the required shutdown order.

### Findings
- **[Low]** `server-go/app.go:67` — shutdown uses `httpServer.Close()` rather than a bounded graceful `Shutdown(ctx)` after `IO.Close(nil)`; this can force-close in-flight polling requests and make goroutine/debug output noisy — prefer graceful shutdown with timeout.
- **[Low]** `client/main.go:43` — demo client emits `ping` with ack but has no explicit user-visible ack timeout; a lost ack leaves the callback silent — wrap ack waits where CLI behavior matters.
- **[Low]** `server-go/stress_test.go:36` — stress goroutines call helpers that can `t.Fatalf` from child goroutines; failures may be harder to attribute and can race with cleanup — return errors through channels instead.

## Phase 4 — Tests
**Coverage observed**:
- `server-go/connection_test.go:12` covers no-token rejection, `server-go/connection_test.go:22` covers expired-token rejection, and `server-go/connection_test.go:33` covers valid-token acceptance.
- `server-go/connection_test.go:43` covers root `welcome`, `server-go/connection_test.go:63` covers `ping` ack, `server-go/connection_test.go:102` covers `chat`, and `server-go/connection_test.go:147` covers room scoping.
- `server-go/stress_test.go:20` checks concurrent connects and `server-go/stress_test.go:65` checks strict N/N broadcast fanout.
- `server-go/payload_test.go:15` checks 1 MiB round-trip, `server-go/payload_test.go:68` checks 500 burst acks, and `server-go/payload_test.go:120` checks 5 concurrent 256 KiB payloads.

**Missing coverage**:
- **[Medium]** `server-go/main.go:103` — `/admin` namespace `welcome` and `op` ack are implemented but no test connects to `/admin` or verifies `{ok:true}` — add admin namespace tests, including the chosen auth/role policy.
- **[Medium]** `server-go/auth.go:53` — tests use only `handshake.auth.token`; there is no coverage for `query.token`, `Authorization`, or precedence conflicts — add tests for all three token sources and source priority.
- **[Medium]** `server-go/auth.go:36` — HS256 enforcement has no negative test for `alg=none` or RS256 confusion — add explicit wrong-alg JWT rejection tests.
- **[Medium]** `server-go/payload_test.go:19` — large-payload coverage stops at 1 MiB even though Go intentionally sets 8 MiB at `server-go/app.go:25`; there is no just-under/at/over boundary test for the configured Go limit — add boundary tests aligned with the 8 MiB contract.

## Phase 5 — Docs
- `README.md` exists; no need to recommend creating it.
- `README.md:52`, `README.md:92`, and `CLAUDE.md:106` now use `go mod download`, so the prior dependency-command docs issue is resolved.
- `CLAUDE.md` event table at `CLAUDE.md:79` matches implemented event names/directions at a schema level.

### Findings
- **[Medium]** `README.md:66` — README says the two servers have a completely identical event interface, but Phase 1 still has non-exempt CORS/header-parsing differences — update wording to distinguish event schema equivalence from transport/auth edge-case differences, or fix the code differences.
- **[Low]** `CLAUDE.md:95` — Go namespace switching text references `manager.Socket("/admin", nil)`, but current `client/main.go:31` uses `socket.Connect(serverURL, opts)` directly and README now documents `socket.Connect(serverURL+"admin", opts)` at `README.md:220` — update CLAUDE.md to match the current client API.

## 建議下一步
1. Restore missing module-content `h1:` checksums for the four Phase 0 modules without changing pinned versions.
2. Fix or document the non-exempt Node/Go differences: CORS/OPTIONS and Authorization `Bearer` parsing.
3. Decide `/admin` policy: JWT-only versus admin-role-only; then align Node, Go, tests, and docs.
4. Add missing tests for `/admin`, token source precedence, wrong JWT algorithms, and 8 MiB payload boundaries.
5. Consider whether CORS divergence should become a documented intentional divergence in AGENTS.md 2.5 if it is deliberate.
