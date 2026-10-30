#!/usr/bin/env node
/**
 * Recording proxy for isolated acceptance runs (CLAWSTA-80 / Task 8).
 *
 * The acceptance deployment starts its Go API and Next.js server with
 * HTTP(S)_PROXY pointing here and NO_PROXY covering loopback, and the
 * Playwright browser context uses the same proxy with a loopback bypass. The
 * proxy records destination metadata and refuses every request with 502, so an
 * attempted unsolicited request fails instead of being silently swallowed.
 * Nothing is forwarded and loopback traffic never reaches this proxy.
 *
 * Evidence is deliberately narrow: only the target host (no userinfo, query,
 * path or port ambiguity beyond the CONNECT authority), method, trigger,
 * validated observer marker and result. Request URLs, headers (including user
 * agents and cookies) and bodies are never written. A marker header is written
 * only when its value is one of the fixed observer tokens below; arbitrary
 * header contents are dropped so they cannot be used to whitelist a request.
 *
 * A log write failure is fatal: this process exits non-zero instead of
 * continuing to serve while "zero traffic" would later be read from a partial
 * evidence file.
 *
 * Usage: node scripts/e2e/recording-proxy.mjs --port 19090 --log <path>
 */
import { appendFileSync } from "node:fs";
import { createServer } from "node:http";

const args = process.argv.slice(2);
function argValue(name, fallback) {
  const index = args.indexOf(name);
  if (index !== -1 && args[index + 1]) return args[index + 1];
  const inline = args.find((value) => value.startsWith(name + "="));
  return inline ? inline.slice(name.length + 1) : fallback;
}

const port = Number(argValue("--port", process.env.E2E_PROXY_PORT || "19090"));
const logPath = argValue("--log", process.env.E2E_PROXY_LOG);
if (!logPath) {
  console.error("recording-proxy: --log <path> or E2E_PROXY_LOG is required");
  process.exit(2);
}

const OBSERVER_MARKERS = new Set(["node", "go", "browser-http", "browser-tls"]);

function sanitizeHost(value) {
  if (typeof value !== "string") return "unknown";
  // CONNECT authority may be "host:port"; there is no userinfo in Host headers
  // but strip defensively and cut anything after the authority.
  const withoutUserInfo = value.includes("@") ? value.slice(value.lastIndexOf("@") + 1) : value;
  return withoutUserInfo.split(/[/?#]/)[0].slice(0, 255) || "unknown";
}

function observerMarker(headers) {
  const raw = headers["x-e2e-control"] ?? headers["x-e2e-selftest"];
  if (typeof raw !== "string") return null;
  return OBSERVER_MARKERS.has(raw) ? raw : null;
}

function append(entry) {
  try {
    appendFileSync(logPath, JSON.stringify(entry) + "\n");
  } catch (error) {
    console.error("recording-proxy: cannot append to evidence log:", error);
    process.exit(1);
  }
}

append({ ts: new Date().toISOString(), kind: "listening", trigger: "startup", result: "listening" });

const server = createServer((request, response) => {
  append({
    ts: new Date().toISOString(),
    kind: "http",
    trigger: "http-request",
    method: request.method,
    host: sanitizeHost(request.headers.host),
    marker: observerMarker(request.headers),
    result: "refused-502",
  });
  response.writeHead(502, { "content-type": "text/plain; charset=utf-8" });
  response.end("blocked by isolated acceptance recorder");
});

server.on("connect", (request, socket) => {
  append({
    ts: new Date().toISOString(),
    kind: "connect",
    trigger: "connect",
    method: "CONNECT",
    host: sanitizeHost(request.url),
    marker: observerMarker(request.headers),
    result: "refused-502",
  });
  socket.end("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n");
  socket.destroy();
});

server.on("upgrade", (request, socket) => {
  append({
    ts: new Date().toISOString(),
    kind: "upgrade",
    trigger: "upgrade",
    method: request.method,
    host: sanitizeHost(request.headers.host),
    marker: observerMarker(request.headers),
    result: "refused-502",
  });
  socket.end("HTTP/1.1 502 Bad Gateway\r\nConnection: close\r\n\r\n");
  socket.destroy();
});

server.on("clientError", (error, socket) => {
  append({
    ts: new Date().toISOString(),
    kind: "client-error",
    trigger: "client-error",
    result: String(error && error.code ? error.code : "client-error").slice(0, 40),
  });
  socket.destroy();
});

server.listen(port, "127.0.0.1", () => {
  console.log("recording-proxy listening on 127.0.0.1:" + port);
});

function shutdown() {
  server.close(() => process.exit(0));
  setTimeout(() => process.exit(0), 1500).unref();
}
process.on("SIGTERM", shutdown);
process.on("SIGINT", shutdown);
