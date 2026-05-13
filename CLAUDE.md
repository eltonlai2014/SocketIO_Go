# SocketIO_Go

測試專案：**Socket.IO 4.x 伺服器 ↔ Go 語言客戶端** 的互通。

參考：<https://socket.io/>

---

## 專案結構

```
SocketIO_Go/
├── server/                 # Node.js Socket.IO 4.x 伺服器（參考實作，含 JWT）
│   ├── package.json
│   └── index.js
├── server-go/              # Go + Gin + Socket.IO 4.x 伺服器（含 JWT + 測試）
│   ├── go.mod
│   ├── main.go             # 只負責 main() + signal handling
│   ├── app.go              # 可測試的 App wiring（main 與 tests 共用）
│   ├── auth.go             # JWT verify + namespace middleware
│   ├── cmd/make-token/     # dev 用 token 產生器
│   ├── setup_test.go       # TestMain + dial/mustDial/waitFor helpers
│   ├── connection_test.go  # 連線 + 事件正確性
│   ├── stress_test.go      # 並發連線 / broadcast fanout
│   └── payload_test.go     # 大資料 / burst / 並發大 payload
├── client/                 # Go 客戶端（從 JWT_TOKEN env 讀 token）
│   ├── go.mod
│   └── main.go
└── CLAUDE.md
```

`server/` 與 `server-go/` 完全等價（相同事件、相同 payload、相同 namespace），同一個 `client/` 切換 URL 即可對接任一邊。

---

## 技術選型

| 角色 | 選型 | 版本 | 備註 |
|---|---|---|---|
| 伺服器（Node） | `socket.io` | `^4.7.5` | Engine.IO v4 協定 |
| 伺服器（Go） | [`github.com/zishang520/socket.io/v2`](https://github.com/zishang520/socket.io) + [`gin-gonic/gin`](https://github.com/gin-gonic/gin) | `v2.5.0` + `v1.10.1` | 用 `gin.WrapH(io.ServeHandler(nil))` 掛載到 `/socket.io/*any` |
| Go 客戶端 | [`github.com/zishang520/socket.io-client-go`](https://github.com/zishang520/socket.io-client-go) | `v1.1.0` | 對 Socket.IO v4 協定支援最完整 |
| Go toolchain | Go | `>= 1.24.1` | `engine.io-client-go` 強制要求 |
| 傳輸 | WebSocket（預設） | — | 客戶端會先 polling 再 upgrade 到 WS |

> 注意：另一個常見 Go 套件 `googollee/go-socket.io` 主要是 server-side，且對 v4 protocol 的 client 支援不完整，故本專案不使用。

---

## 執行方式

### 1. 啟動伺服器（terminal A）

```powershell
cd server
npm install
npm start
```

預期輸出：
```
[server] Socket.IO 4.x listening on http://localhost:3000
```

### 2. 啟動 Go 客戶端（terminal B）

```powershell
cd client
go mod download    # 第一次依鎖定版本拉取依賴；不會修改 go.mod / go.sum
go run .
```

預期看到 Go 端送出 `chat`、`ping`（含 ack）、`join` + `roomMsg`，並收到 server 的 `welcome` / `chat` / `system` / `roomMsg`。

---

## 伺服器事件介面

| 事件 | 方向 | Payload | 說明 |
|---|---|---|---|
| `welcome` | server → client | `{ message, id, ts }` | 連線成功後 server 主動送 |
| `chat` | 雙向 | `string`（送出）/ `{ from, msg, ts }`（收到） | broadcast 到所有人 |
| `ping` | client → server | `any` + ack callback | server 回 `{ pong, echo, ts }` 給 ack |
| `join` | client → server | `room: string` | 加入房間 |
| `roomMsg` | 雙向 | `{ room, msg }` | 只送到該 room |
| `system` | server → client | `string` | 房間內系統訊息 |

### Namespace `/admin`

| 事件 | 方向 | Payload |
|---|---|---|
| `welcome` | server → client | `{ ns, id }` |
| `op` | client → server | `any` + ack → `{ ok: true }` |

Go 端切換 namespace：把 [client/main.go:31](client/main.go#L31) 的 `socket.Connect(serverURL, opts)` 改成 `socket.Connect(serverURL+"admin", opts)`（`serverURL` 已含尾斜線）。

---

## 常見問題

### Q1：依賴下載失敗 / 找不到套件
注意 import 路徑沒有 `/v3` 字尾（v1.x 上仍以 `github.com/zishang520/socket.io-client-go/socket` 為準），且需要 Go ≥ 1.24.1。

**正確修法**：先確認 `go.sum` 與 `go.mod` 已有鎖定版本（這個 repo 提交時就是齊全的），然後跑：
```powershell
go mod download
```
這只會依 `go.sum` 把鎖定版本拉到 module cache，**不會**改 `go.mod` / `go.sum`。

**禁忌**（任一條都會破壞版本鎖、引發守則第 7 條的編譯衝突）：
- ❌ `go mod tidy`（會自動升級可升的 indirect deps）
- ❌ `go get <package>@latest`（會把 `quic-go` / `qpack` / `gin` 升過上限）
- ❌ `go get -u`（升級所有依賴）

### Q2：客戶端連不上、卡在 polling
Socket.IO 4.x 預設 `transports: ['polling', 'websocket']`，先 long-polling 再 upgrade 到 WS。確認：
- 伺服器有起來、port 3000 沒被佔用（`netstat -ano | findstr :3000`）
- 防火牆沒擋 localhost:3000
- 客戶端 `serverURL` 用 `http://`（不是 `ws://`，client 函式庫會自己升級）

### Q3：ack 沒收到
伺服器端必須在 handler 簽名收下最後一個參數（`(payload, ack) => ack(...)`），且只能呼叫一次。Go 端在 `Emit` 的最後一個參數放 callback 即代表是 ack。

---

---

## JWT 驗證設計

### 設計決策
- **演算法**：HS256（共享 secret）— dev 階段最簡單；要換 RS256 改 `auth.go` 的 `verifyJWT` keyfunc 即可。
- **Token 位置**：`socket.handshake.auth.token`（推薦）→ `query.token` → `Authorization: Bearer ...` 三來源都接受。client 用 `opts.SetAuth(...)` 走第一個。
- **驗證點**：Namespace `Use()` middleware，**不是** Gin middleware。原因：Gin 看不到 Socket.IO CONNECT packet 內的 auth payload，且只看得到 polling 階段的 HTTP 握手。
- **Secret 管理**：`JWT_SECRET` 環境變數；不設則用 `dev-secret-change-me`（Node、Go server、make-token 三邊一致）。
- **Claims**：自訂結構 `Claims{UserID, Role, RegisteredClaims}`，存在 `socket.Data()`（Go）/ `socket.data.claims`（Node）。
- **錯誤回傳**：`socket.NewExtendedError("unauthorized", {code:"AUTH_FAILED", reason:"..."})`，client 在 `connect_error` 事件收到。

### 兩端等價檢查（任何時候修改 auth 都要保證）
| 項目 | Node | Go |
|---|---|---|
| 三來源 token 抽取 | `extractToken(socket)` | `extractToken(auth, query, headers)` |
| `/` 與 `/admin` 都走 middleware | `io.use(...)` + `admin.use(...)` | `io.Use(...)` + 各 namespace `.Use(...)` |
| Claims 存取 | `socket.data.claims` | `client.Data().(*Claims)` |
| 錯誤 payload | `err.data = {code, reason}` | `socket.NewExtendedError(msg, {code, reason})` |

### 不在這個專案做（但要記得）
- Token 撤銷（logout 主動踢人）— 需要 sid↔user 表 + 廣播指令
- Token refresh（連線中換新 token）— 需要 `auth:refresh` 事件 + 重新呼叫 `verifyJWT`
- Rate limit — 在 Gin middleware 對 `/socket.io/*any` 加 IP 級限制
- 換 RS256 / JWKS — 把 keyfunc 改成從 JWKS endpoint 取 public key

---

## 開發守則（給 Claude）

1. **協定優先**：所有除錯先確認 Engine.IO v4 packet 是否正確（`0` open / `4` message / `42` event / `43` ack）。瀏覽器 devtools 的 WS frame 是真實來源。
2. **不要改用 `googollee/go-socket.io`**：它對 v4 protocol 不完整，會產生難以追蹤的握手錯誤。Server 用 `zishang520/socket.io/v2`，client 用 `zishang520/socket.io-client-go`，兩邊共用同一條 protocol stack 是相容性保證。
3. **Node server 為純 JS**：不引入 TypeScript / 打包工具，這是測試用最小 server。
4. **Go server 必須跟 Node server 同步**：新增事件要兩邊都改、payload schema 一致，client 才能對任一 server 都打得通。
5. **事件命名**：使用小寫短字串（`chat`、`ping`、`roomMsg`），與 Socket.IO 慣例一致；不要用點號或路徑式命名。
6. **Gin 整合採模式 A**：`r.Any("/socket.io/*any", gin.WrapH(io.ServeHandler(nil)))`（GET + POST 都要掛）。不要把 socket.io 路徑放進需要 response writer 包裝的 middleware group（gzip、自訂 writer 會破壞 WebSocket upgrade）。
7. **Go server 依賴鎖版**：`quic-go` 必須鎖 `v0.53.0`、`qpack` 鎖 `v0.5.1`、`gin` 鎖 `v1.10.1` 以下，否則 `webtransport-go@v0.9.1` 會編不過（webtransport 是 engine.io 強制相依，即使我們沒用 WebTransport transport）。升 gin 至 v1.11+ 會拉進 http3 → quic-go v0.59+ 衝突。
8. **新功能流程**：先在 Node server 加事件 → 同步到 Go server → 同步 README 事件表格 → 在 Go client 對應 `On/Emit` → 兩個 server 各跑一次驗證。
9. **JWT secret 不要 commit**：`JWT_SECRET` 只能用環境變數注入；dev 預設值 `dev-secret-change-me` 已在程式碼中明示「change me」，production 必須覆寫。
10. **改 JWT 邏輯時兩邊同步**：Node 的 `jwtMiddleware` 與 Go 的 `auth.go` 必須行為等價（token 抽取順序、錯誤碼、claims schema）。檢查表見上方「兩端等價檢查」。
11. **改 server 行為前先跑 `go test ./...`**：12 個測試覆蓋 auth、事件正確性、並發、大 payload。新增事件時請順手在 `connection_test.go` 補一個對應的測試。
12. **註冊 listener 要趕在 connect 之前**：Engine.IO 的事件（welcome、connect 之後 server 主動 emit 的訊息）可能在 client 端 `connect` 回呼觸發前就送達。測試 helper `dial()` 不等 connect、`mustDial()` 等 connect——大部分情境用 `dial()` 後立刻 `On(...)` 才不會錯過第一個 packet。
13. **`MaxHttpBufferSize` 已從預設 1 MiB 拉到 8 MiB**（[app.go](server-go/app.go)），這是大 payload 測試的前提；若要再放大需同步調整測試上限與守則此條。
