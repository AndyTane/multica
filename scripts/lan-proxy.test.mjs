import assert from "node:assert/strict";
import { createServer as httpServer } from "node:http";
import net from "node:net";
import { once } from "node:events";
import { createHash } from "node:crypto";
import { test } from "node:test";
import { createRelay } from "./lan-proxy.mjs";

async function listen(server) {
  server.listen(0, "127.0.0.1");
  await once(server, "listening");
  return server.address().port;
}

async function setup(t, target, options = {}) {
  const targetPort = await listen(target);
  const relay = createRelay({ address: "127.0.0.1", port: 0, targetPort,
    subnet: "127.0.0.0", prefix: 8, ...options });
  const address = await relay.listen();
  t.after(async () => { await relay.close(); await new Promise((done) => target.close(done)); });
  return address.port;
}

test("forwards streamed HTTP bodies and bearer auth without rewriting headers", async (t) => {
  const target = httpServer(async (req, res) => {
    let body = "";
    for await (const chunk of req) body += chunk;
    assert.equal(req.headers.authorization, "Bearer fake-test-token");
    res.writeHead(200, { "content-type": "text/plain" });
    res.write(body.slice(0, 3));
    res.end(body.slice(3));
  });
  const port = await setup(t, target);
  const response = await fetch(`http://127.0.0.1:${port}/api/test`, {
    method: "POST", body: "streamed-request", headers: { Authorization: "Bearer fake-test-token" },
  });
  assert.equal(response.status, 200);
  assert.equal(await response.text(), "streamed-request");
});

test("preserves WebSocket upgrades and bidirectional frames", async (t) => {
  const target = httpServer();
  const connections = new Set();
  target.on("connection", (socket) => { connections.add(socket); socket.on("close", () => connections.delete(socket)); });
  t.after(() => { for (const socket of connections) socket.destroy(); });
  target.on("upgrade", (req, socket) => {
    const accept = createHash("sha1").update(req.headers["sec-websocket-key"] + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11").digest("base64");
    socket.write(`HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: ${accept}\r\n\r\n`);
    socket.once("data", (frame) => {
      assert.equal(frame[0] & 15, 1);
      const length = frame[1] & 127;
      const payload = Buffer.from(frame.subarray(6, 6 + length));
      for (let i = 0; i < length; i++) payload[i] ^= frame[2 + i % 4];
      assert.equal(payload.toString(), "hello");
      socket.write(Buffer.concat([Buffer.from([0x81, 5]), Buffer.from("world")]));
    });
  });
  const port = await setup(t, target);
  const socket = new WebSocket(`ws://127.0.0.1:${port}/ws`);
  t.after(() => socket.close());
  await once(socket, "open");
  const message = once(socket, "message");
  socket.send("hello");
  assert.equal((await message)[0].data, "world");
});

test("rejects clients outside the configured local subnet", async (t) => {
  let upstreamConnections = 0;
  const target = net.createServer(() => upstreamConnections++);
  const port = await setup(t, target, { subnet: "192.168.3.0", prefix: 24 });
  const client = net.createConnection({ host: "127.0.0.1", port });
  client.on("error", () => {});
  await once(client, "close");
  assert.equal(upstreamConnections, 0);
});

test("allows a response after the client half-closes its request", async (t) => {
  const target = net.createServer({ allowHalfOpen: true }, (socket) => {
    socket.on("data", () => {});
    socket.on("end", () => socket.end("response"));
  });
  const port = await setup(t, target);
  const client = net.createConnection({ host: "127.0.0.1", port, allowHalfOpen: true });
  let body = "";
  client.on("data", (chunk) => { body += chunk; });
  client.end("request");
  await once(client, "end");
  client.destroy();
  assert.equal(body, "response");
});
