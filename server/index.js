const { Server } = require("socket.io");
const jwt = require("jsonwebtoken");

const PORT = Number(process.env.PORT || 3000);
const JWT_SECRET = process.env.JWT_SECRET || "dev-secret-change-me";

const io = new Server(PORT, {
  cors: { origin: "*" },
});

console.log(`[server] Socket.IO 4.x listening on http://localhost:${PORT}`);

function extractToken(socket) {
  // auth payload (preferred), query, Authorization header
  const auth = socket.handshake.auth || {};
  if (auth.token) return auth.token;
  const q = socket.handshake.query || {};
  if (q.token) return Array.isArray(q.token) ? q.token[0] : q.token;
  const h = socket.handshake.headers || {};
  if (h.authorization) return h.authorization.replace(/^Bearer\s+/i, "");
  return "";
}

function jwtMiddleware(nsp) {
  return (socket, next) => {
    const token = extractToken(socket);
    if (!token) {
      console.log(`[${nsp}] auth reject sid=${socket.id}: missing token`);
      const err = new Error("unauthorized");
      err.data = { code: "AUTH_FAILED", reason: "missing token" };
      return next(err);
    }
    try {
      const claims = jwt.verify(token, JWT_SECRET, { algorithms: ["HS256"] });
      socket.data.claims = claims;
      next();
    } catch (e) {
      console.log(`[${nsp}] auth reject sid=${socket.id}: ${e.message}`);
      const err = new Error("unauthorized");
      err.data = { code: "AUTH_FAILED", reason: e.message };
      next(err);
    }
  };
}

io.use(jwtMiddleware("/"));

io.on("connection", (socket) => {
  const c = socket.data.claims || {};
  console.log(`[/] connected: ${socket.id} (uid=${c.uid} role=${c.role})`);

  socket.emit("welcome", {
    message: "hello from socket.io 4.x server",
    id: socket.id,
    ts: Date.now(),
  });

  // Echo with acknowledgement: client may pass a callback as the last arg.
  socket.on("ping", (payload, ack) => {
    console.log(`[/] ping from ${socket.id}:`, payload);
    if (typeof ack === "function") {
      ack({ pong: true, echo: payload, ts: Date.now() });
    }
  });

  // Broadcast chat to everyone (including sender).
  socket.on("chat", (msg) => {
    console.log(`[/] chat from ${socket.id}: ${msg}`);
    io.emit("chat", { from: socket.id, msg, ts: Date.now() });
  });

  // Room demo: join a room, then broadcast to that room only.
  socket.on("join", (room) => {
    socket.join(room);
    console.log(`[/] ${socket.id} joined room "${room}"`);
    io.to(room).emit("system", `${socket.id} joined ${room}`);
  });

  socket.on("roomMsg", ({ room, msg }) => {
    io.to(room).emit("roomMsg", { from: socket.id, room, msg });
  });

  socket.on("disconnect", (reason) => {
    console.log(`[/] disconnected ${socket.id}: ${reason}`);
  });
});

// Namespace demo (also requires JWT)
const admin = io.of("/admin");
admin.use(jwtMiddleware("/admin"));
admin.on("connection", (socket) => {
  console.log(`[/admin] connected: ${socket.id}`);
  socket.emit("welcome", { ns: "/admin", id: socket.id });

  socket.on("op", (payload, ack) => {
    console.log(`[/admin] op:`, payload);
    if (typeof ack === "function") ack({ ok: true });
  });
});

process.on("SIGINT", () => {
  console.log("[server] shutting down...");
  io.close(() => process.exit(0));
});
