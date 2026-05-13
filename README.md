# SocketIO_Go

Socket.IO 4.x 伺服器 ↔ Go 客戶端 互通測試專案。

- 伺服器（兩種等價實作，擇一啟動）：
  - Node.js + `socket.io` `^4.7.5` — [`server/`](server/)
  - Go + Gin + `zishang520/socket.io/v2` — [`server-go/`](server-go/)
- 客戶端：Go + [`github.com/zishang520/socket.io-client-go`](https://github.com/zishang520/socket.io-client-go) `v1.1.0` — [`client/`](client/)
- 連線埠：`3000`

詳細設計與開發守則見 [CLAUDE.md](CLAUDE.md)。

---

## 環境需求

| 工具 | 版本 |
|---|---|
| Node.js | ≥ 18（建議 LTS） |
| npm | 隨 Node.js |
| Go | ≥ 1.24.1（`engine.io-client-go` 強制要求） |

確認版本：
```powershell
node -v
go version
```

---

## 測試方式

### Step 1 — 啟動伺服器（Terminal A，選一）

#### 選項 A：Node.js 伺服器

```powershell
cd server
npm install
npm start
```

預期輸出：
```
[server] Socket.IO 4.x listening on http://localhost:3000
```

#### 選項 B：Go + Gin 伺服器

```powershell
cd server-go
go mod download    # 第一次依鎖定版本拉依賴；不會動 go.mod / go.sum
go run .
```

預期輸出：
```
[GIN-debug] GET    /health                   --> main.main.func1 (3 handlers)
[GIN-debug] GET    /socket.io/*any           --> main.main.WrapH.func3 (3 handlers)
[GIN-debug] POST   /socket.io/*any           --> main.main.WrapH.func4 (3 handlers)
[server-go] Gin + Socket.IO listening on http://localhost:3000
```

額外有 REST 端點 `GET /health` 可用 `curl http://localhost:3000/health` 驗活。

> 兩個伺服器**事件介面完全相同**，client 不需要改任何程式碼就能切換。
> 不要同時啟動兩邊（port 3000 衝突）。

伺服器啟動後保持執行，Ctrl+C 可關閉。

### Step 2 — 產生 JWT token（兩個 server 都會驗 token）

```powershell
cd server-go
go run ./cmd/make-token -uid alice -role user -ttl 1h
```

把印出的 token 複製起來。`make-token` 與 server 共用同一個 `JWT_SECRET`，預設 `dev-secret-change-me`（兩邊都不設環境變數時可直接用）。其他可用旗標：

| 旗標 | 預設 | 說明 |
|---|---|---|
| `-uid` | `alice` | 寫入 `sub` 與 `uid` claim |
| `-role` | `user` | 寫入 `role` claim |
| `-ttl` | `1h` | 過期時間；用負值（如 `-1m`）可產生已過期 token 做驗證測試 |

### Step 3 — 啟動 Go 客戶端（Terminal B）

把 token 設到 `JWT_TOKEN` 環境變數後再執行：

```powershell
cd client
go mod download                # 第一次依鎖定版本下載依賴；不會動 go.mod / go.sum
$env:JWT_TOKEN = "<貼上 step 2 的 token>"
go run .
```

預期客戶端輸出（時間戳省略）：
```
connecting to http://localhost:3000/ ...
connected, sid=XXXXXXXXXXXXXXXX
welcome <- [map[id:... message:hello from socket.io 4.x server ts:...]]
chat    <- [map[from:... msg:hello from go client ts:...]]
system  <- [... joined go-room]
ack ping <- [map[echo:map[hello:world] pong:true ts:...]]
roomMsg <- [map[from:... msg:hello room from go room:go-room]]
```

預期伺服器端輸出：
```
[/] connected: XXXXXXXXXXXXXXXX
[/] chat from XXXXXXXXXXXXXXXX: hello from go client
[/] ping from XXXXXXXXXXXXXXXX: { hello: 'world' }
[/] XXXXXXXXXXXXXXXX joined room "go-room"
```

按 Ctrl+C 關閉客戶端，client 端會送 `disconnect`，server 也會印出對應的離線記錄。

---

## 驗證項目對照表

| 驗證點 | 期望結果 |
|---|---|
| JWT 驗證 | server log 顯示 `connected: <sid> (uid=alice role=user)`；client 印出 `auth: sending JWT (len=...)` |
| 連線建立 | client 印出 `connected, sid=...`；server 印出 `[/] connected: <id>` |
| Server → Client 主動推送 | client 收到 `welcome` 事件 |
| Client → Server (broadcast) | server 印出 `[/] chat from ...`；client 自己也會收到 `chat`（因為 server 用 `io.emit` broadcast 給所有人） |
| Ack（帶 callback） | client 收到 `ack ping <- [{echo, pong, ts}]` |
| Room | client 收到 `system` 訊息與 `roomMsg` 回波 |
| 中斷 | client Ctrl+C 後 server 印出 `[/] disconnected ... transport close` |

---

## 自動化測試

所有測試在 [server-go/](server-go/) 套件中，**不需要先手動啟動 server**——`TestMain` 會在隨機 port 起一個 in-process server，跑完自動關閉。

```powershell
cd server-go
go test ./... -v
```

預期 ~1.2 秒跑完 12 個測試全綠：

| 類別 | 檔案 | 測試 |
|---|---|---|
| **連線與正確性** | [connection_test.go](server-go/connection_test.go) | `TestConnect_NoToken_Rejected` · `TestConnect_ExpiredToken_Rejected` · `TestConnect_ValidToken_Accepted` · `TestEvent_Welcome_PayloadShape` · `TestEvent_PingAck_RoundTrip` · `TestEvent_ChatBroadcast_BetweenTwoClients` · `TestEvent_RoomScoped_OnlyMembersReceive` |
| **多連線壓力** | [stress_test.go](server-go/stress_test.go) | `TestStress_ManyConcurrentConnects` (預設 50 並發) · `TestStress_BroadcastFanout` (20 client 同時收 broadcast) |
| **大量資料** | [payload_test.go](server-go/payload_test.go) | `TestPayload_LargeAckRoundTrip` (1 MiB 單次 + SHA-256 驗證) · `TestPayload_BurstManyMessages` (500 連續 ack) · `TestPayload_ConcurrentLargePayloads` (5 client × 256 KiB) |

### 加壓跑

```powershell
# 並發連線數提高到 500（預設 50）
go test -run TestStress -stress 500 -v -timeout 5m

# 只跑單一測試
go test -run TestPayload_LargeAckRoundTrip -v
```

### 觀察吞吐量

`-v` 模式下測試會印出實測數字，例如：
- `round-tripped 1048576 bytes in 25.3ms (79.0 MB/s)`
- `500 round-trips in 18ms (27000 msg/s)`
- `connected 500 clients in 21.5s (23.2 conns/sec)`

這些不是 assertion，只是觀察值；可用來追蹤回歸（例如改 server 設定後是否變慢）。

### 注意事項

- 測試 server 把 Engine.IO 的 `MaxHttpBufferSize` 拉到 8 MiB（main server 也一樣，見 [app.go](server-go/app.go)），預設 1 MiB 會擋住大 payload 測試。
- 並發數受 Windows 開檔/socket 上限影響，loopback 上 500 並發在 i7 約 22 秒；想跑 1000+ 請考慮 `ulimit -n`（Linux）或開到 Linux/容器測試。
- 測試使用相同的 `JWT_SECRET` 邏輯（`signTokenForDev` 共用 `auth.go`），不需要設環境變數。

---

## JWT 驗證測試

驗證 server 真的有把關 token 三種情境：

```powershell
# A. 沒帶 token — 預期 client 印 connect_error: [unauthorized]
Remove-Item Env:JWT_TOKEN -ErrorAction SilentlyContinue
go run .

# B. 帶過期 token — 預期 client 印 connect_error: [unauthorized]，server 印 token is expired
cd ..\server-go
$env:JWT_TOKEN = (go run ./cmd/make-token -uid bob -ttl -1m)
cd ..\client
go run .

# C. 帶合法 token — 預期所有事件正常流動，server 印 uid=alice role=user
cd ..\server-go
$env:JWT_TOKEN = (go run ./cmd/make-token -uid alice -role user -ttl 1h)
cd ..\client
go run .
```

> **製作 token 與 server 必須用相同 `JWT_SECRET`**。不設就用 dev 預設值；要換成自己的 secret 時，請在啟動 server 與 make-token 兩個 terminal 各 `$env:JWT_SECRET = "..."` 一次。

---

## 多客戶端測試

開第三個 terminal 再跑一次 `go run .`，會看到兩個 client 互相收到對方的 `chat` 訊息（broadcast），但只有 join 過 `go-room` 的會收到 `roomMsg`。

---

## 進階測試

### 切換 namespace `/admin`

修改 [client/main.go](client/main.go)，把：
```go
sock, err := socket.Connect(serverURL, opts)
```
改成連到 `/admin`：
```go
sock, err := socket.Connect(serverURL+"admin", opts)
```
再執行 `go run .`，會收到 `/admin` namespace 的 `welcome`（含 `ns: "/admin"`）。

### 模擬斷線重連

啟動 client 後，先 Ctrl+C 把 server 關掉再重新 `npm start`，預設 `Manager` 會自動重連（看 client 端是否再次印出 `connected`）。

---

## 疑難排解

| 症狀 | 處理 |
|---|---|
| `go mod download` 卡在下載 | 檢查網路、`GOPROXY`；可改用 `$env:GOPROXY="https://proxy.golang.org,direct"`。**不要**用 `go mod tidy` 或 `go get ...@latest` 嘗試「修復」，會破壞版本鎖（見 [CLAUDE.md](CLAUDE.md) 第 7 條） |
| `connect_error` | 確認 server 已啟動、port 3000 沒被佔用：`netstat -ano \| findstr :3000` |
| client 卡住沒輸出 | 防火牆可能擋 localhost；或客戶端 transports 設定錯誤 |
| ack 沒回來 | server handler 必須收 `(payload, ack)` 兩個參數，且只能呼叫 `ack(...)` 一次 |
| Go server 編譯出現 `undefined: quic.ConnectionTracingID` 或 `qpack.NewDecoder` 參數錯誤 | `quic-go` / `qpack` / `gin` 版本被拉升了；在 [server-go/go.mod](server-go/go.mod) 鎖回 `quic-go v0.53.0`、`qpack v0.5.1`、`gin v1.10.1`。詳見 [CLAUDE.md](CLAUDE.md) 第 7 條開發守則 |

更多細節請見 [CLAUDE.md](CLAUDE.md)。
