# SocketIO_Go Code Review — 2026-05-13

> Reviewer: Codex  
> Commit: 613e0af24e2de627966d18e233813a69b97d4b58  
> Verdict: ❌ Block

## Phase 0 — Bootstrap
- Build: ❌ Go build did not complete; Node syntax check passed.
- Tests: ❌ 0/12 executed; package setup failed before test execution.
- Vet: ❌ did not complete; package setup/cache failed before analysis.
- Environment notes: `go mod download` failed against `proxy.golang.org` via `127.0.0.1:9`, and some runs also hit `AppData\Local\go-build` `Access is denied`; these are environment limitations and are not findings.
- `go.sum` check: missing-entry errors were verified with grep. The listed modules have `/go.mod h1:` lines but no module-content `h1:` lines, so this is not only an environment limitation.
- Node syntax: ✅ `cd server && node -c index.js` passed.
- Race run: ⚠️ `go test ./... -race -count=3` first failed because `-race requires cgo`; with `CGO_ENABLED=1`, it still failed during package setup/cache, so no race result is available.

### 🔴 Critical
- **[Critical]** `server-go/go.sum:11` — `github.com/gin-gonic/gin v1.10.1` has only the `/go.mod` checksum; `go build` reports a missing go.sum entry for the imported module content — restore the intended locked `h1:` entry without changing versions.
- **[Critical]** `server-go/go.sum:16` — `github.com/golang-jwt/jwt/v5 v5.3.1` has only the `/go.mod` checksum; server and `cmd/make-token` fail package setup — restore the locked module-content `h1:` entry.
- **[Critical]** `server-go/go.sum:55` — `github.com/zishang520/socket.io/v2 v2.5.0` has only the `/go.mod` checksum; server build/test cannot load the package — restore the locked module-content `h1:` entry.
- **[Critical]** `client/go.sum:10` — `github.com/zishang520/engine.io-client-go v1.1.0` has only the `/go.mod` checksum; client build fails package setup — restore the locked module-content `h1:` entry.
- **[Critical]** `client/go.sum:12` — `github.com/zishang520/engine.io/v2 v2.5.0` has only the `/go.mod` checksum; client build fails package setup — restore the locked module-content `h1:` entry.
- **[Critical]** `client/go.sum:13` — `github.com/zishang520/socket.io-client-go v1.1.0` has only the `/go.mod` checksum; client build fails package setup — restore the locked module-content `h1:` entry.

## Phase 1 — 對等性
| 項目 | Node | Go | 狀態 |
|---|---|---|---|
| `welcome` payload schema | `server/index.js:52` emits `{message,id,ts}` with message `hello from socket.io 4.x server` | `server-go/main.go:41` emits `{message,id,ts}` with message `hello from go-gin socket.io 4.x server` | ⚠️ Schema equivalent; literal message differs |
| `chat` 廣播語意 | `server/index.js:67` uses `io.emit("chat", {from,msg,ts})`, including sender | `server-go/main.go:67` uses `io.Emit("chat", {from,msg,ts})`, including sender | ✅ Equivalent |
| `ping` ack `{pong, echo, ts}` | `server/index.js:59` calls `ack({pong:true,echo,ts})` | `server-go/main.go:47` calls `ack([]any{map{pong,echo,ts}}, nil)` | ✅ Equivalent shape |
| `join` + `roomMsg` + `system` | `server/index.js:73` joins, emits `system`, then `roomMsg` to room | `server-go/main.go:77` joins, emits `system`, then `roomMsg` to room | ✅ Equivalent order and room scope |
| `/admin` `welcome` / `op` | `server/index.js:89` namespace emits `{ns,id}` and acks `{ok:true}` | `server-go/main.go:103` namespace emits `{ns,id}` and acks `{ok:true}` | ✅ Equivalent events |
| JWT middleware scope | `server/index.js:46` root and `server/index.js:90` `/admin` use JWT middleware | `server-go/main.go:33` root and `server-go/main.go:105` `/admin` use JWT middleware | ✅ Equivalent JWT scope |
| Token source priority | `server/index.js:16` auth → `server/index.js:18` query → `server/index.js:20` header | `server-go/auth.go:53` auth → `server-go/auth.go:58` query → `server-go/auth.go:61` header | ✅ Same priority |
| Authorization parsing | `server/index.js:20` accepts case-insensitive `Bearer\s+` | `server-go/auth.go:62` only trims exact `Bearer ` | ⚠️ Difference |
| Claims schema | `server/index.js:35` stores JWT claims on `socket.data.claims` | `server-go/auth.go:17` defines `uid`, `role`, registered claims and `server-go/auth.go:82` stores `*Claims` | ✅ Equivalent intended schema |
| Error payload | `server/index.js:29` / `server/index.js:39` send `unauthorized` with `{code,reason}` | `server-go/auth.go:76` sends `NewExtendedError("unauthorized", {code,reason})` | ✅ Equivalent for JWT failures |
| `MaxHttpBufferSize` | `server/index.js:7` does not configure it, so Node remains at Socket.IO default | `server-go/app.go:25` sets 8 MiB | ❌ Difference for payloads above default limit |
| CORS | `server/index.js:8` allows `origin: "*"` | `server-go/app.go:30` uses bare Gin and `server-go/app.go:41`/`42` only mount GET/POST, no CORS/OPTIONS | ❌ Difference for browser cross-origin clients |

### Phase 1 Findings
- **[High]** `server-go/app.go:25` — Go accepts up to 8 MiB, while `server/index.js:7` leaves Node at the Socket.IO default; payload behavior is not equivalent despite `README.md:66` claiming the two servers expose the same event interface — decide whether Node should also set 8 MiB or Go should match Node.
- **[High]** `server-go/app.go:41` — Go has no CORS/OPTIONS equivalent to Node's `cors: { origin: "*" }` at `server/index.js:8`; browser clients that work against Node can fail against Go, especially with authorization headers/preflight — add explicit Go-side CORS policy or intentionally narrow both sides.
- **[Medium]** `server-go/auth.go:62` — Go only accepts exact `Authorization: Bearer ` while Node accepts case-insensitive `Bearer\s+` at `server/index.js:20`; token extraction is not equivalent — choose one rule and align both implementations/tests.

## Phase 2 — Security
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[High]** `server-go/main.go:105` — `/admin` mounts only `jwtMiddleware("/admin")`; the `requireRole` middleware defined at `server-go/auth.go:89` is never used, so any valid JWT role can access admin `op` — either mount `requireRole("/admin", "admin")` after JWT and mirror Node, or remove the unused middleware and document that `/admin` is auth-only.
- **[Medium]** `server/index.js:5` — Node silently falls back to `dev-secret-change-me`; `server-go/auth.go:27` and `server-go/cmd/make-token/main.go:33` do the same, so a production process can boot with the dev secret and no warning — fail fast or loudly warn outside explicit dev mode.
- **[Medium]** `server-go/app.go:25` — 8 MiB request buffers increase memory/DoS exposure and there is no rate limit or per-IP cap on `/socket.io/*any`; `CLAUDE.md:143` says rate limiting is not implemented — add a bounded Socket.IO/Gin-side connection/message limit if this is exposed beyond local testing.
- **[Medium]** `server-go/auth.go:35` — `jwt.ParseWithClaims` plus the HS256 method check rejects `alg=none`/RS256, but no parser option requires or validates `iat`; future-issued tokens may be accepted depending on jwt/v5 defaults — add explicit parser options/tests if `iat` must be enforced.
- **[Low]** `server-go/auth.go:75` — auth failure logs include parser error strings but not raw tokens; this is acceptable, but keep it that way when improving diagnostics.
- **[Low]** `client/main.go:26` — the client logs only JWT length, not token contents; no raw secret/token logging found.

## Phase 3 — Concurrency
- Race validation could not run due cgo/package setup/cache limitations described in Phase 0.
- `App.Shutdown()` closes Socket.IO before HTTP, which matches the required ordering.

### Findings
- **[Medium]** `server-go/setup_test.go:55` — `dial()` calls `clientsocket.Connect` before callers register event listeners; tests such as `server-go/connection_test.go:47` then register `welcome` listeners after dialing, which can still miss early server-emitted packets if `Connect` progresses faster than listener setup — provide a helper that registers handlers before connection or prove the client library is lazy.
- **[Low]** `server-go/app.go:67` — shutdown uses `httpServer.Close()` rather than graceful `Shutdown(ctx)` after closing Socket.IO; this can force-close in-flight polling requests and make goroutine-leak/debug output noisy — prefer bounded graceful shutdown after `IO.Close(nil)`.
- **[Low]** `client/main.go:43` — production/demo client emits `ping` with ack but no explicit timeout/cancellation; a missing ack only leaves the callback silent and gives no user-visible timeout — wrap ack waits where CLI behavior matters.
- **[Low]** `server-go/stress_test.go:36` — stress goroutines call helpers that can invoke `t.Fatalf` from child goroutines; failures may be harder to attribute and can race with cleanup — return errors through channels instead.

## Phase 4 — Tests
**Coverage observed**:
- `server-go/connection_test.go:12` covers no-token reject, `server-go/connection_test.go:22` covers expired-token reject, and `server-go/connection_test.go:33` covers valid-token accept.
- `server-go/connection_test.go:43` covers root `welcome`, `server-go/connection_test.go:63` covers `ping` ack, `server-go/connection_test.go:102` covers `chat`, and `server-go/connection_test.go:147` covers room scoping.
- `server-go/stress_test.go:20` checks concurrent connects and `server-go/stress_test.go:65` checks strict N/N broadcast fanout.
- `server-go/payload_test.go:15` checks 1 MiB round-trip, `server-go/payload_test.go:68` checks 500 burst acks, and `server-go/payload_test.go:120` checks 5 concurrent 256 KiB payloads.

**Missing coverage**:
- **[Medium]** `server-go/main.go:103` — `/admin` namespace `welcome` and `op` ack are implemented but no test connects to `/admin` or verifies `{ok:true}` — add admin namespace tests, including unauthorized and role behavior after the role decision.
- **[Medium]** `server-go/auth.go:53` — tests use only `handshake.auth.token`; there is no coverage for `query.token` or `Authorization` fallback priority — add tests for all three token sources and precedence conflicts.
- **[Medium]** `server-go/auth.go:36` — HS256 enforcement has no negative test for `alg=none` or RS256 confusion — add explicit malformed/wrong-alg JWT rejection tests.
- **[Medium]** `server-go/payload_test.go:19` — large-payload coverage stops at 1 MiB despite Go setting 8 MiB at `server-go/app.go:25`; there is no 8 MiB boundary or 8 MiB + 1 rejection test — add boundary tests aligned with `MaxHttpBufferSize`.
- **[Low]** `server-go/connection_test.go:55` — welcome test asserts `message` contains `go-gin`, which bakes in the Go literal rather than the cross-server contract from `CLAUDE.md:81` — assert schema/type and document literal if it is intentional.

## Phase 5 — Docs
- `README.md` now exists, so the old “README 不存在” checklist item is obsolete.
- `CLAUDE.md` event table matches the implemented event names/directions at a schema level.

### Findings
- **[High]** `README.md:52` — README tells users to run `go mod tidy`; `README.md:92` repeats this for the client, but AGENTS/CLAUDE lock dependency versions as a correctness condition — replace with `go mod download` or a non-mutating build instruction.
- **[High]** `CLAUDE.md:104` — the troubleshooting section recommends `go get ...@latest`, which conflicts with `CLAUDE.md:156` dependency lock rules and AGENTS.md's explicit ban on `go get -u` / `go get <package>` — remove latest-upgrade advice and point to locked versions.
- **[Medium]** `README.md:66` — README says the two servers have a completely identical event interface, but Phase 1 found CORS, header parsing, max-buffer, and `welcome.message` literal differences — update docs after the source-of-truth decision.
- **[Medium]** `README.md:101` — expected `welcome` output shows the Node message string, while Go emits `hello from go-gin socket.io 4.x server` at `server-go/main.go:42` — make the README example server-specific or align the implementation strings.
- **[Low]** `CLAUDE.md:95` — Go namespace switching text references `manager.Socket("/admin", nil)`, but current `client/main.go:31` uses `socket.Connect(serverURL, opts)` directly — update the instruction to match the current client API.

## 建議下一步
1. Restore the missing module-content `h1:` checksum entries for the locked Go dependencies, without running `go mod tidy` or upgrading versions.
2. Decide the source of truth for Phase 1 differences: CORS/OPTIONS, max payload size, Authorization `Bearer` parsing, and `welcome.message` literal.
3. Decide whether `/admin` is JWT-only or admin-role-only; then align Node, Go, tests, and docs.
4. Add missing tests for `/admin`, token source precedence, wrong JWT algorithms, and 8 MiB payload boundaries.
5. Fix README/CLAUDE dependency commands so they never instruct `go mod tidy` or `go get ...@latest` for normal setup.
