# AGENTS.md — Codex Code Review Playbook

本檔案是 **Codex 進行本專案 code review 時的唯一指令來源**。Claude（負責實作 / 修補）的指令在同層的 [CLAUDE.md](CLAUDE.md)。兩份檔案在開發守則上必須保持一致——若發現衝突，以 `CLAUDE.md` 為準並把差異列入 review report。

---

## 1. Codex 在這個 repo 的角色

- **角色**：靜態審查者（reviewer），**只讀不寫程式碼**。
- **唯一可寫的檔案**：`review-report.md`（放在 repo root，每次 review 覆寫）。
- **禁止行為**：
  - 不要修改 `server/`、`server-go/`、`client/` 下任何 `.go` / `.js` / `package.json` / `go.mod` / `go.sum`。
  - 不要執行 `go mod tidy`、`npm install`、`npm update`、`go get -u`、`go get <package>`——版本鎖在這個專案是**正確性條件**，見 `CLAUDE.md` 開發守則第 7 條。`go mod download`（純粹下載已鎖定的依賴到 module cache，不會動 go.mod / go.sum）是**允許**的，用來預熱沙箱環境。
  - 不要 `git push --force` / 不要推到 `main`。允許 `git add review-report.md && git commit && git push` 把 review 結果推回當前的 review branch（事實上 Codex sandbox 工作流就是這樣 checkpoint）。**只能** commit `review-report.md`；任何其他檔案的 commit 屬禁止行為。
  - 不要建立任何其他文件檔（除非主人明確指示）。
- **允許的執行動作**：`go build ./...`、`go vet ./...`、`go test ./...`、`go mod download`、`golangci-lint run`（如可用）、`node -c server/index.js`、唯讀的 `git log` / `git diff` / `git grep`。

---

## 2. 專案心智模型（先讀這段再開始）

這是 **Socket.IO 4.x 互通測試專案**。三個元件：

| 元件 | 路徑 | 角色 |
|---|---|---|
| Node Socket.IO server | [server/](server/) | 參考實作（reference impl），含 JWT |
| Go Socket.IO server | [server-go/](server-go/) | 等價的 Go 版，含 JWT + 12 個測試 |
| Go client | [client/](client/) | 同一份 client 可打兩邊 server |

**核心不變條件**（review 必須對著這些檢查）：

1. `server/` 與 `server-go/` 必須**事件等價**：相同事件名、相同 payload schema、相同 namespace、相同 ack 行為、相同 middleware 順序。
2. JWT 走 **HS256**，token 抽取順序固定為 `handshake.auth.token` → `query.token` → `Authorization: Bearer`。錯誤一律回 `socket.NewExtendedError("unauthorized", {code, reason})`。
3. JWT 驗證必須在 **Socket.IO namespace middleware**，不是 Gin / Express middleware。原因見 `CLAUDE.md`「JWT 驗證設計」。
4. Gin 整合採模式 A：`r.Any("/socket.io/*any", gin.WrapH(io.ServeHandler(nil)))`，**GET + POST 都要掛**，且 socket.io 路徑不能進 gzip / response-writer-wrapping middleware。
5. 依賴鎖版（破壞會編不過或行為改變）：
   - `quic-go` = `v0.53.0`
   - `qpack` = `v0.5.1`
   - `gin` ≤ `v1.10.1`
   - Go toolchain ≥ `1.24.1`
6. `MaxHttpBufferSize` 已從 1 MiB 拉到 **8 MiB**（[server-go/app.go](server-go/app.go:25)），這是 payload 測試的前提；若這條被改回去，相關測試會壞。
7. 預設 `JWT_SECRET` 是 `"dev-secret-change-me"`——**dev only**，production 必須從環境變數注入。

---

## 3. Review 流程：5 個 Phase

每個 Phase 結束都把 finding 寫進 `review-report.md` 對應段落。**順序不可跳**，因為後面的 phase 依賴前面已建立的事實。

### Phase 0 — Bootstrap（驗證可建置可測試）

```bash
# 在 repo root
# 先預熱依賴 cache（沙箱環境必要，本機跑通常已 cache 過可省略）
cd server-go && go mod download && cd ..
cd client    && go mod download && cd ..

cd server-go && go build ./... && go vet ./... && go test ./... -count=1 -timeout 120s
cd ../client  && go build ./...
cd ../server  && node -c index.js   # 不需 npm install，只做語法檢查
```

**通過條件**：
- `go test ./...` 12 個測試全綠（如果有名稱不同，以實際輸出為準）。
- `go vet` 無警告。
- `node -c` 無語法錯誤。

**若失敗，先做以下「環境性失敗」排除**（很重要，否則容易誤判）：

1. **錯誤訊息為 `missing go.sum entry for module ...`**：
   先用 `grep '<module>' server-go/go.sum client/go.sum` 確認該 entry 是否真的不在 go.sum。
   - 若 grep **有找到**對應的 `h1:` 與 `/go.mod` 兩行 → 這是 **environment limitation**（沙箱 module cache 為空 + 無網路下載），**不是 finding**。在 report Phase 0 標註「environment-limited bootstrap」並**繼續往 Phase 1**（後續 phase 多為靜態審查，不需執行 build）。
   - 若 grep **真的找不到** → 才是 Critical finding，照常記錄。

2. **錯誤訊息含 `Access is denied` / `permission denied` / 路徑為 `AppData\Local\go-build`**：沙箱對 Go cache 無寫權，屬 environment limitation，處理方式同上。

3. **`dial tcp: ... timeout` / `proxy.golang.org: no such host`**：沙箱無對外網路，屬 environment limitation。

**只有「非環境性」失敗才寫成 finding 並停下**：例如真的 `*.go` 編譯錯誤、`go vet` 報出實際問題、`go test` 跑起來但測試 fail。這些才代表 repo 有問題。

> **Phase 0 寬鬆原則**：本專案 Codex review 的價值在 Phase 1–5（對等性、安全、並發、測試、文件）的**靜態審查**，這些都可以離線只讀檔案完成。Phase 0 只是錦上添花，不該因環境問題擋住整個 review。

---

### Phase 1 — 對等性審查（最重要的一段）

**目標**：證明 Node 與 Go server 行為等價，或精確指出不等價的地方。

逐項對照（產出一張表，左欄 Node、右欄 Go、第三欄「等價 / 差異 / 風險」）：

| 檢查項目 | Node 來源 | Go 來源 |
|---|---|---|
| `welcome` payload schema | `server/index.js` | `server-go/app.go` 或事件註冊處 |
| `chat` 廣播語意 | 同上 | 同上 |
| `ping` ack `{pong, echo, ts}` | 同上 | 同上 |
| `join` + `roomMsg` + `system` 順序 | 同上 | 同上 |
| `/admin` namespace 的 `welcome` / `op` | 同上 | 同上 |
| JWT middleware 套用範圍（`/` 與 `/admin` 都要） | `io.use` / `admin.use` | `io.Use` / 各 namespace `.Use` |
| Token 三來源優先序 | `extractToken(socket)` | [server-go/auth.go:52](server-go/auth.go#L52) |
| Claims schema (`uid`, `role`) | `socket.data.claims` | `Claims` struct |
| 錯誤 payload `{code, reason}` | `err.data = {...}` | `socket.NewExtendedError(...)` |
| `MaxHttpBufferSize` | 預設或顯式 | [server-go/app.go:25](server-go/app.go#L25) = 8 MiB |
| CORS 設定 | `cors: {...}` | gin / socket.io options |

**只要有一格「差異」，主人就需要決定哪邊是 source of truth，並在兩邊改齊。**

---

### Phase 2 — 安全審查

針對 [server-go/auth.go](server-go/auth.go) 與 Node 對應段落：

- [ ] HS256 演算法是否鎖死？`verifyJWT` 是否拒絕 `alg=none` / `alg=RS256` 混淆攻擊？（[server-go/auth.go:36](server-go/auth.go#L36) 已檢查 `t.Method.Alg()`，確認 Node 那邊也有）
- [ ] `extractToken` 三來源的優先序是否與 Node 一致？順序錯了等於放寬攻擊面。
- [ ] `Authorization: Bearer ` 解析是否 case-sensitive？`strings.TrimPrefix` 只吃精確大小寫——確認 Node 端也是大小寫敏感的，否則兩邊不等價。
- [ ] `defaultSecret` 是否只能在 dev 出現？有沒有任何 production 路徑會 fallback 到它而沒警告？
- [ ] JWT 過期 / nbf / iat 驗證——`jwt.ParseWithClaims` 預設行為是否足夠？
- [ ] `requireRole` 中介層是否真的有被掛到 `/admin`？掛載順序對嗎（必須在 `jwtMiddleware` 之後）？
- [ ] `MaxHttpBufferSize=8MB` 是否被用來放大 DoS 風險？有沒有 rate limit / per-IP cap？
- [ ] Gin 上是否有任何 middleware 會吞掉 WebSocket upgrade？（壓縮、自訂 response writer）
- [ ] CORS 是否允許 `*` 而不該允許？
- [ ] Logging 是否會把 raw token / secret 寫到 stdout？

---

### Phase 3 — 並發 / 可靠性審查

針對 [server-go/stress_test.go](server-go/stress_test.go) 與 [server-go/payload_test.go](server-go/payload_test.go)：

- [ ] 測試是否真的有 race？跑一次 `go test ./... -race -count=3` 看看。
- [ ] `App.Shutdown()` 的 ordering 對嗎？（先關 `IO` 再關 `http.Server`，反過來會卡 WS connection）— 看 [server-go/app.go:64](server-go/app.go#L64)。
- [ ] `httpServer.Close()` 用的是非 graceful close，是否會造成測試殘留 goroutine？建議改 `Shutdown(ctx)`？
- [ ] [server-go/setup_test.go](server-go/setup_test.go) 的 `dial` / `mustDial` / `waitFor` 是否正確處理「listener 註冊要趕在 connect 之前」這條 CLAUDE.md 第 12 條守則？
- [ ] Broadcast fanout 在高並發下是否會掉訊息？stress test 的 assertion 強度夠嗎（嚴格收滿 N，還是只看「至少收到一個」）？
- [ ] Client 端 [client/main.go](client/main.go) 有沒有 ack timeout？沒有的話卡住怎麼辦？

---

### Phase 4 — 測試覆蓋審查

審查 4 個 test 檔：

| 檔案 | 預期覆蓋 | 檢查 |
|---|---|---|
| [setup_test.go](server-go/setup_test.go) | TestMain + dial helpers | helper 是否會吞錯誤？timeout 多長？|
| [connection_test.go](server-go/connection_test.go) | 連線 + 每個事件 | 每個事件（welcome/chat/ping/join/roomMsg/system/admin:op）是否都有對應測試？|
| [stress_test.go](server-go/stress_test.go) | 並發連線 + broadcast fanout | 並發數量、assertion 嚴謹度 |
| [payload_test.go](server-go/payload_test.go) | 大資料 / burst / 並發大 payload | 是否測到 `MaxHttpBufferSize` 邊界？8 MiB 剛好/超過/遠超？|

**建議補測清單**（如有缺漏，列在 report）：
- JWT 過期 token 被拒
- 錯誤演算法（`alg=none`）被拒
- `/admin` 沒有 admin role 被拒
- Token 從 query string 進來能通過（驗證三來源等價性）
- `MaxHttpBufferSize` 邊界：剛好 8 MiB、8 MiB + 1 byte（應該被拒）

---

### Phase 5 — 文件審查

- [ ] `CLAUDE.md` 第 1-13 條守則是否每一條都在程式碼中可驗證？任何一條過時的→列出來。
- [ ] `CLAUDE.md` 事件表是否與實際程式碼一致（event name、payload、方向）？
- [ ] `README` 不存在——是否應該建議建立？（**不要自己建**，只在 report 裡建議）

---

## 4. Review Report 格式（寫到 `review-report.md`）

```markdown
# SocketIO_Go Code Review — <YYYY-MM-DD>

> Reviewer: Codex
> Commit: <git rev-parse HEAD>
> Verdict: ✅ Ship / ⚠️ Ship with notes / ❌ Block

## Phase 0 — Bootstrap
- Build: ✅/❌
- Tests: <N>/12 passing
- Vet: clean / <issues>

## Phase 1 — 對等性
| 項目 | Node | Go | 狀態 |
|---|---|---|---|
| ... | ... | ... | ✅/⚠️/❌ |

## Phase 2 — Security
### 🔴 Critical / 🟠 High / 🟡 Medium / 🔵 Low
- **[Severity]** [path:line] — 描述 — 建議

## Phase 3 — Concurrency
...

## Phase 4 — Tests
**Missing coverage**:
- ...

## Phase 5 — Docs
...

## 建議下一步
1. ...（主人 / Claude 該做的事，依優先序排）
```

每個 finding **必須**附 `path:line`，否則 Claude 之後沒辦法定位修補點。

---

## 5. 與 Claude 的協作協議

1. Codex 寫完 `review-report.md` 就停。
2. 主人會把 report 丟給 Claude，Claude 依 finding 在本機修補並跑 `go test ./...`。
3. 修完後，主人會請 Codex 重跑「**只跑 Phase 0 + 對應的修補項所屬 Phase**」做 verification round。
4. Codex 不主動跨輪做事；每一輪都從讀 `review-report.md` 上一輪結尾的「建議下一步」開始，做完寫進新的 report，覆寫舊檔。

---

## 6. 不要做的事（速查）

- ❌ 改任何 `.go` / `.js` / `package.json` / `go.mod` / `go.sum`
- ❌ `go mod tidy` / `npm install` / `go get -u` / `go get <pkg>`（`go mod download` 允許）
- ❌ 建議升級 `gin` / `quic-go` / `qpack`（鎖版有原因，見 `CLAUDE.md` 第 7 條）
- ❌ 建議把 `googollee/go-socket.io` 換成 v3 protocol stack
- ❌ `git push --force` / 推到 `main`（推到 review branch 並 commit `review-report.md` 是 OK 的）
- ❌ commit 任何**非** `review-report.md` 的檔案
- ❌ 建立 README / 任何文件檔（report 以外）
- ❌ 把「環境性失敗」（go.sum 看似缺項但實際 grep 得到、cache 權限被拒、網路 timeout）寫成 Critical finding——這是誤判，見 Phase 0 排除規則
- ❌ 在 production 路徑使用 `dev-secret-change-me`
- ❌ 把 socket.io 路徑放進 gzip middleware

---

## 7. 速查事件表（與 [CLAUDE.md](CLAUDE.md) 同步）

| 事件 | 方向 | Payload | Namespace |
|---|---|---|---|
| `welcome` | server → client | `{ message, id, ts }` | `/` |
| `chat` | 雙向 | `string` → `{ from, msg, ts }` | `/` |
| `ping` | client → server | `any` + ack `{pong, echo, ts}` | `/` |
| `join` | client → server | `room: string` | `/` |
| `roomMsg` | 雙向 | `{ room, msg }` | `/` |
| `system` | server → client | `string` | `/` |
| `welcome` | server → client | `{ ns, id }` | `/admin` |
| `op` | client → server | `any` + ack `{ok:true}` | `/admin` |

任何新事件**必須**先在這張表登記、且 Node 與 Go 兩邊都實作後，才算完成。
