import { readFileSync } from "node:fs";
import net from "node:net";
import { networkInterfaces } from "node:os";
import { resolve } from "node:path";
import { pathToFileURL } from "node:url";

// A TCP relay preserves HTTP streaming and WebSocket upgrades without changing
// the existing loopback-only deployment or its authentication headers.
export function createRelay({ address, port, targetPort, subnet, prefix }) {
  const allowed = new net.BlockList();
  allowed.addSubnet(subnet, prefix, "ipv4");
  const sockets = new Set();
  const server = net.createServer({ allowHalfOpen: true }, (client) => {
    if (!client.remoteAddress || !allowed.check(client.remoteAddress, "ipv4")) {
      client.destroy();
      return;
    }
    const upstream = net.createConnection({ host: "127.0.0.1", port: targetPort, allowHalfOpen: true });
    sockets.add(client);
    sockets.add(upstream);
    const deadline = setTimeout(() => upstream.destroy(), 10_000);
    upstream.once("connect", () => clearTimeout(deadline));
    upstream.once("close", () => { clearTimeout(deadline); sockets.delete(upstream); });
    client.once("close", () => { sockets.delete(client); upstream.destroy(); });
    client.on("error", () => upstream.destroy());
    upstream.on("error", () => client.destroy());
    upstream.once("close", () => client.destroy());
    client.setKeepAlive(true, 30_000);
    upstream.setKeepAlive(true, 30_000);
    client.pipe(upstream).pipe(client);
  });
  server.on("error", (error) => console.error(`Relay ${address}:${port}: ${error.code ?? error.message}`));
  return {
    server,
    listen: () => new Promise((accept, reject) => {
      server.once("error", reject);
      server.listen({ host: address, port, exclusive: true }, () => {
        server.removeListener("error", reject);
        accept(server.address());
      });
    }),
    close: () => new Promise((done) => {
      for (const socket of sockets) socket.destroy();
      server.close(done);
    }),
  };
}

async function run(configPath) {
  const config = JSON.parse(readFileSync(configPath, "utf8").replace(/^\uFEFF/, ""));
  if (!config.interface || !Array.isArray(config.ports) || !config.ports.length ||
      config.ports.some((port) => !Number.isInteger(port) || port < 1024 || port > 65535)) {
    throw new Error("Expected a Wi-Fi interface name and unprivileged TCP ports.");
  }
  let currentAddress;
  let relays = [];
  let busy = false;
  let stopping = false;
  const refresh = async () => {
    if (busy || stopping) return;
    busy = true;
    try {
      const link = networkInterfaces()[config.interface]?.find((item) =>
        item.family === "IPv4" && !item.internal && item.cidr);
      if (link?.address === currentAddress && relays.length === config.ports.length &&
          relays.every((relay) => relay.server.listening)) return;
      await Promise.all(relays.map((relay) => relay.close()));
      relays = [];
      currentAddress = link?.address;
      if (!link) return;
      const [subnet, prefix] = link.cidr.split("/");
      for (const port of config.ports) {
        const relay = createRelay({ address: link.address, port, targetPort: port, subnet, prefix: Number(prefix) });
        relays.push(relay);
        await relay.listen();
      }
      console.log(JSON.stringify({ at: new Date().toISOString(), address: link.address, ports: config.ports }));
    } catch (error) {
      console.error(error.message);
    } finally {
      busy = false;
    }
  };
  const timer = setInterval(refresh, 10_000);
  for (const signal of ["SIGINT", "SIGTERM"]) process.once(signal, async () => {
    stopping = true;
    clearInterval(timer);
    await Promise.all(relays.map((relay) => relay.close()));
  });
  await refresh();
}

if (process.argv[1] && import.meta.url === pathToFileURL(resolve(process.argv[1])).href) {
  run(process.argv[2]).catch((error) => { console.error(error.message); process.exitCode = 1; });
}
