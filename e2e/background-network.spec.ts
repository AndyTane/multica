import { test, expect, type Browser, type BrowserContext, type Page } from "@playwright/test";
import { execFileSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { Socket } from "node:net";
import { join } from "node:path";

import { TestApiClient } from "./fixtures";

/**
 * CLAWSTA-80 Task 8 — background network acceptance.
 *
 * Runs against a real isolated deployment built from production artifacts (Go
 * server binary + Next.js standalone server + its own PostgreSQL cluster) and
 * observes four layers:
 *
 *  1. Browser requests, WebSockets, service workers and navigation through
 *     Playwright context/page events.
 *  2. Browser and Node/Go network attempts through
 *     scripts/e2e/recording-proxy.mjs, which records destination metadata and
 *     refuses every non-loopback connection with 502. An attempt therefore
 *     fails; it is never silently swallowed.
 *  3. The Go API and Next.js server processes: the deployment is started with
 *     HTTP(S)_PROXY pointing at the recorder and NO_PROXY covering loopback.
 *     The proxy path is proven live with exact Node and Go control requests
 *     and browser self-test requests before the deployment starts.
 *  4. A Windows TCP-table sample of the API/web PIDs as a sampling backstop;
 *     it is not exhaustive process-egress proof and the spec fails if the
 *     sample cannot be taken.
 *
 * Evidence stores only sanitized destination metadata: scheme + host + path,
 * never query strings, userinfo, tokens or user agents. The spec requires the
 * acceptance harness environment; a bare deployment cannot prove the Node/Go
 * observation layer is wired up, so missing observer configuration fails.
 */

const WEB_ORIGIN = new URL(
  process.env.PLAYWRIGHT_BASE_URL ?? process.env.FRONTEND_ORIGIN ?? "http://localhost:3000",
).origin;
const PROXY_URL = process.env.E2E_PROXY_URL ?? "";
const PROXY_LOG = process.env.E2E_PROXY_LOG ?? "";
const PROXY_PID = Number(process.env.E2E_PROXY_PID ?? "0");
const SERVER_PID = Number(process.env.E2E_SERVER_PID ?? "0");
const WEB_PID = Number(process.env.E2E_WEB_PID ?? "0");
const EVIDENCE_DIR = process.env.E2E_EVIDENCE_DIR ?? "";
const ACCEPTANCE_ROOT = process.env.E2E_ACCEPTANCE_ROOT ?? "";
const RUN_ID = process.env.E2E_RUN_ID ?? `${Date.now().toString(36)}-${process.pid.toString(36)}`;
const WORKER = process.env.TEST_PARALLEL_INDEX ?? process.env.TEST_WORKER_INDEX ?? "0";

const EMAIL = `e2e-network-${WORKER}-${RUN_ID}@multica.ai`;
const WORKSPACE_NAME = `E2E Network Workspace ${WORKER}`;
const WORKSPACE_SLUG = `e2e-network-${WORKER}-${RUN_ID}`;
const ALT_WORKSPACE_NAME = `E2E Network Alt ${WORKER}`;
const ALT_WORKSPACE_SLUG = `e2e-network-alt-${WORKER}-${RUN_ID}`;

// Exact destinations/triggers the observation harness is allowed to see. Any
// other host, and any marker header outside this set, is a violation. This is
// deliberately not a wildcard: a broad ".invalid is fine" rule would let a
// real attempted destination hide behind an observer name.
const OBSERVER_HOSTS = new Set([
  "uw80-node-control.invalid:80",
  "uw80-go-control.invalid",
  "uw80-browser-selftest.invalid",
  "uw80-browser-selftest-tls.invalid:443",
]);
const OBSERVER_MARKERS = new Set(["node", "go", "browser-http", "browser-tls"]);
const SELF_TEST_ORIGINS = new Set([
  "http://uw80-browser-selftest.invalid",
  "https://uw80-browser-selftest-tls.invalid",
]);

const LOCAL_HOSTNAMES = new Set(["localhost", "127.0.0.1", "::1", "[::1]"]);

type ProxyLogEntry = {
  ts?: string;
  kind?: string;
  method?: string;
  host?: string;
  marker?: string | null;
  result?: string;
};

type TcpSample = { available: boolean; error?: string; external: string[] };

type ScenarioRecord = {
  scenario: string;
  finishedAt: string;
  proxyViolations: number;
  browserExternalRequests: number;
  browserExternalWebsockets: number;
  browserExternalNavigations: number;
  tcpSampleAvailable: boolean;
  tcpExternalConnections: string[];
};

/**
 * Sanitize a URL before it may enter an observation array or evidence file.
 * Keeps scheme + host + path only and drops userinfo, query string and hash,
 * which can carry tokens, search text or file names.
 */
function sanitizeUrl(raw: string): string {
  try {
    const url = new URL(raw);
    if (!["http:", "https:", "ws:", "wss:"].includes(url.protocol)) {
      return url.protocol;
    }
    // Scheme + host only: paths can carry task keys, filenames or tokens.
    return `${url.protocol}//${url.hostname}`;
  } catch {
    return "invalid-url";
  }
}

function isLoopbackHostname(hostname: string) {
  const normalized = hostname.toLowerCase();
  return (
    LOCAL_HOSTNAMES.has(normalized) ||
    normalized.startsWith("127.") ||
    normalized.endsWith(".localhost")
  );
}

function isLocalUrl(raw: string) {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return true;
  }
  if (["data:", "blob:", "about:", "chrome:", "devtools:", "file:"].includes(url.protocol)) {
    return true;
  }
  return isLoopbackHostname(url.hostname);
}

function isSelfTestUrl(raw: string) {
  try {
    const url = new URL(raw);
    return SELF_TEST_ORIGINS.has(`${url.protocol}//${url.hostname}`);
  } catch {
    return false;
  }
}

const observation = {
  requests: 0,
  externalRequests: [] as string[],
  websockets: [] as string[],
  externalWebsockets: [] as string[],
  navigations: [] as string[],
  externalNavigations: [] as string[],
  serviceWorkers: [] as string[],
  failedRequests: [] as string[],
  selftestAttempts: [] as string[],
  scenarios: [] as ScenarioRecord[],
};

let browser: Browser;
let context: BrowserContext;
let page: Page;
let api: TestApiClient;
let workspaceSlug = "";
let altWorkspaceSlug = "";

function requireObserver(condition: boolean, message: string) {
  if (!condition) {
    throw new Error(
      message +
        "\n\nThis spec only runs against the isolated acceptance deployment " +
        "described in docs/outbound-data-verification.md. Export E2E_PROXY_URL / " +
        "E2E_PROXY_LOG / E2E_PROXY_PID / E2E_SERVER_PID / E2E_WEB_PID / " +
        "E2E_ACCEPTANCE_ROOT / E2E_OBSERVER_CONTROLS after the recorder and " +
        "deployment are started from that runbook.",
    );
  }
}

function readJsonFile(path: string): Record<string, unknown> {
  return JSON.parse(readFileSync(path, "utf8").replace(/^\uFEFF/, "")) as Record<string, unknown>;
}

/**
 * Refuse to talk to whatever database happens to be configured: the acceptance
 * run must be the isolated deployment named by E2E_ACCEPTANCE_ROOT. This stops
 * a stray shell environment from pointing this spec at a real deployment.
 */
function assertAcceptanceTarget() {
  requireObserver(existsSync(ACCEPTANCE_ROOT), `acceptance root does not exist: ${ACCEPTANCE_ROOT}`);
  const markerPath = join(ACCEPTANCE_ROOT, "acceptance-root.json");
  requireObserver(
    existsSync(markerPath),
    `acceptance root marker is missing: ${markerPath} (the runbook in docs/outbound-data-verification.md creates it for the isolated root)`,
  );
  const marker = readJsonFile(markerPath);
  requireObserver(
    typeof marker.purpose === "string" && marker.purpose.includes("isolated acceptance"),
    "acceptance root marker does not describe an isolated acceptance environment",
  );
  const deployment = readJsonFile(join(ACCEPTANCE_ROOT, "deployment.json")) as {
    Root?: string;
    WebPort?: number;
    ApiPort?: number;
    DatabasePort?: number;
    Database?: string;
  };
  requireObserver(
    typeof deployment.Root === "string" &&
      deployment.Root.toLowerCase() === ACCEPTANCE_ROOT.toLowerCase(),
    "deployment.json belongs to a different root than E2E_ACCEPTANCE_ROOT",
  );

  const webUrl = new URL(WEB_ORIGIN);
  requireObserver(
    Number(webUrl.port) === Number(deployment.WebPort),
    `PLAYWRIGHT_BASE_URL port ${webUrl.port} is not the isolated web port ${deployment.WebPort}`,
  );
  const apiUrl = new URL(process.env.NEXT_PUBLIC_API_URL ?? "");
  requireObserver(
    Number(apiUrl.port) === Number(deployment.ApiPort),
    `NEXT_PUBLIC_API_URL port ${apiUrl.port} is not the isolated API port ${deployment.ApiPort}`,
  );
  const databaseUrl = new URL(process.env.DATABASE_URL ?? "");
  requireObserver(
    databaseUrl.hostname === "127.0.0.1" &&
      Number(databaseUrl.port) === Number(deployment.DatabasePort) &&
      databaseUrl.pathname.replace(/^\//, "") === deployment.Database,
    "DATABASE_URL does not point at the isolated acceptance database",
  );
}

function noteRequest(request: { url(): string; resourceType(): string }) {
  observation.requests += 1;
  const raw = request.url();
  const sanitized = sanitizeUrl(raw);
  if (isSelfTestUrl(raw)) {
    observation.selftestAttempts.push(sanitized);
    return;
  }
  if (!isLocalUrl(raw)) observation.externalRequests.push(sanitized);
}

function noteWebsocket(url: string) {
  const sanitized = sanitizeUrl(url);
  if (isSelfTestUrl(url)) {
    observation.selftestAttempts.push(sanitized);
    return;
  }
  observation.websockets.push(sanitized);
  if (!isLocalUrl(url)) observation.externalWebsockets.push(sanitized);
}

function noteNavigation(url: string) {
  const sanitized = sanitizeUrl(url);
  if (isSelfTestUrl(url)) {
    observation.selftestAttempts.push(sanitized);
    return;
  }
  observation.navigations.push(sanitized);
  if (!isLocalUrl(url)) observation.externalNavigations.push(sanitized);
}

const attachedPages = new WeakSet<Page>();

function attachPage(target: Page) {
  // Context-level request listeners cover every page; attach each page once for
  // the per-page events so duplicated listeners cannot inflate the counts.
  if (attachedPages.has(target)) return;
  attachedPages.add(target);
  target.on("websocket", (socket) => noteWebsocket(socket.url()));
  target.on("framenavigated", (frame) => {
    if (frame === target.mainFrame()) noteNavigation(frame.url());
  });
  target.on("requestfailed", (request) => {
    const raw = request.url();
    if (isSelfTestUrl(raw)) observation.selftestAttempts.push(sanitizeUrl(raw));
    else if (!isLocalUrl(raw)) observation.failedRequests.push(sanitizeUrl(raw));
  });
}

function readProxyLog(): ProxyLogEntry[] {
  requireObserver(existsSync(PROXY_LOG), `recording proxy log is missing: ${PROXY_LOG}`);
  return readFileSync(PROXY_LOG, "utf8")
    .split(/\r?\n/)
    .filter((line) => line.trim().length > 0)
    .map((line, index) => {
      try {
        return JSON.parse(line) as ProxyLogEntry;
      } catch {
        throw new Error(`Invalid recorder metadata at line ${index + 1}; cannot verify outbound traffic.`);
      }
    });
}

function isExpectedObserverEntry(entry: ProxyLogEntry) {
  if (entry.kind === "listening") return true;
  const host = typeof entry.host === "string" ? entry.host : "";
  if (!OBSERVER_HOSTS.has(host)) return false;
  // A matching host is the trigger we created. A marker is optional (CONNECT
  // cannot carry custom headers) but if present it must be one of ours.
  return entry.marker === undefined || entry.marker === null || OBSERVER_MARKERS.has(entry.marker);
}

function proxyViolations() {
  return readProxyLog()
    .filter((entry) => !isExpectedObserverEntry(entry))
    .filter((entry) => entry.kind === "http" || entry.kind === "connect" || entry.kind === "upgrade")
    .map((entry) => ({
      kind: entry.kind ?? null,
      host: typeof entry.host === "string" ? entry.host : null,
      method: entry.method ?? null,
    }));
}

function probeProxyDown(url: string) {
  const target = new URL(url);
  return new Promise<string>((resolve) => {
    const socket = new Socket();
    socket.setTimeout(1000);
    socket.once("connect", () => {
      socket.destroy();
      resolve("up");
    });
    socket.once("timeout", () => {
      socket.destroy();
      resolve("down");
    });
    socket.once("error", () => resolve("down"));
    socket.connect(Number(target.port || 80), target.hostname);
  });
}

/**
 * Windows TCP table sample for the API/web PIDs. This is a sampling backstop,
 * not exhaustive proof: short-lived connections can be missed. It must be
 * available on this platform; an unavailable sample is a failure, never a
 * silent zero.
 */
function sampleExternalTcp(pids: number[]): TcpSample {
  if (process.platform !== "win32") {
    return { available: false, error: `TCP sampling is not implemented on ${process.platform}`, external: [] };
  }
  const targets = new Set(pids.filter((pid) => Number.isInteger(pid) && pid > 0));
  if (targets.size !== 2) {
    return { available: false, error: `expected two owned service PIDs, got ${targets.size}`, external: [] };
  }
  let output = "";
  try {
    output = execFileSync("netstat.exe", ["-ano", "-p", "tcp"], {
      encoding: "utf8",
      timeout: 20000,
      windowsHide: true,
    });
  } catch (error) {
    return { available: false, error: `netstat failed: ${String(error)}`, external: [] };
  }
  const external: string[] = [];
  for (const line of output.split(/\r?\n/)) {
    const parts = line.trim().split(/\s+/);
    if (parts.length < 5) continue;
    const [protocol, , remote, state, pidRaw] = parts;
    if (protocol !== "TCP" && protocol !== "TCPv6") continue;
    if (state !== "ESTABLISHED" && state !== "SYN_SENT") continue;
    if (!targets.has(Number(pidRaw))) continue;
    const host = remote.startsWith("[")
      ? remote.slice(1, remote.lastIndexOf("]"))
      : remote.slice(0, remote.lastIndexOf(":"));
    if (host === "127.0.0.1" || host === "::1" || host === "0.0.0.0" || host === "::") continue;
    external.push(remote);
  }
  return { available: true, external: Array.from(new Set(external)) };
}

function recordScenario(scenario: string) {
  const tcp = sampleExternalTcp([SERVER_PID, WEB_PID]);
  const record: ScenarioRecord = {
    scenario,
    finishedAt: new Date().toISOString(),
    proxyViolations: proxyViolations().length,
    browserExternalRequests: observation.externalRequests.length,
    browserExternalWebsockets: observation.externalWebsockets.length,
    browserExternalNavigations: observation.externalNavigations.length,
    tcpSampleAvailable: tcp.available,
    tcpExternalConnections: tcp.external,
  };
  observation.scenarios.push(record);
  expect(tcp.available, `${scenario}: TCP backstop sample unavailable: ${tcp.error ?? "unknown"}`).toBe(
    true,
  );
  expect(
    {
      proxyViolations: record.proxyViolations,
      browserExternalRequests: record.browserExternalRequests,
      browserExternalWebsockets: record.browserExternalWebsockets,
      browserExternalNavigations: record.browserExternalNavigations,
      tcpExternalConnections: record.tcpExternalConnections,
    },
    `${scenario}: unsolicited outbound traffic detected`,
  ).toEqual({
    proxyViolations: 0,
    browserExternalRequests: 0,
    browserExternalWebsockets: 0,
    browserExternalNavigations: 0,
    tcpExternalConnections: [],
  });
}

function writeEvidenceFile(name: string, data: unknown) {
  if (!EVIDENCE_DIR) return;
  mkdirSync(EVIDENCE_DIR, { recursive: true });
  writeFileSync(join(EVIDENCE_DIR, name), JSON.stringify(data, null, 2) + "\n");
}

type ObserverControls = {
  node?: { recorded?: boolean; exitCode?: number };
  go?: { recorded?: boolean; exitCode?: number };
  backendEnvKeys?: string[];
  frontendEnvKeys?: string[];
};

function readObserverControls(): ObserverControls | null {
  const path = process.env.E2E_OBSERVER_CONTROLS;
  if (!path || !existsSync(path)) return null;
  try {
    return readJsonFile(path) as ObserverControls;
  } catch {
    return null;
  }
}

test.describe.configure({ mode: "serial" });

test.describe("background network boundary (isolated acceptance deployment)", () => {
  test.beforeAll(async ({ browser: browserFixture }) => {
    browser = browserFixture;
    requireObserver(Boolean(PROXY_URL) && Boolean(PROXY_LOG), "E2E_PROXY_URL and E2E_PROXY_LOG are not set.");
    requireObserver(SERVER_PID > 0 && WEB_PID > 0, "E2E_SERVER_PID and E2E_WEB_PID are not set.");
    requireObserver(Boolean(ACCEPTANCE_ROOT), "E2E_ACCEPTANCE_ROOT is not set.");
    assertAcceptanceTarget();

    api = new TestApiClient();
    await api.login(EMAIL, "E2E Network User");
    const workspace = await api.ensureWorkspace(WORKSPACE_NAME, WORKSPACE_SLUG);
    await api.markUserOnboarded();
    workspaceSlug = workspace.slug;
    const altWorkspace = await api.createWorkspace(ALT_WORKSPACE_NAME, ALT_WORKSPACE_SLUG);
    altWorkspaceSlug = altWorkspace.slug;

    context = await browser.newContext({
      baseURL: WEB_ORIGIN,
      // Loopback must bypass the recorder, otherwise the product's own
      // localhost requests would be refused and the scenario would fail for
      // the wrong reason.
      proxy: { server: PROXY_URL, bypass: "localhost,127.0.0.1,::1" },
    });
    context.on("request", (request) => noteRequest(request));
    context.on("page", (newPage) => attachPage(newPage));
    context.on("serviceworker", (worker) => observation.serviceWorkers.push(sanitizeUrl(worker.url())));
    page = await context.newPage();
    attachPage(page);
  });

  test.afterAll(async () => {
    writeEvidenceFile("browser-observation.json", {
      runId: RUN_ID,
      webOrigin: WEB_ORIGIN,
      generatedAt: new Date().toISOString(),
      observation: {
        ...observation,
        requests: observation.requests,
        scenarios: observation.scenarios,
      },
      proxyViolations: proxyViolations(),
      tcpPids: { server: SERVER_PID, web: WEB_PID },
      boundary:
        "Browser events plus a recorder that refuses non-loopback requests. " +
        "Node/Go traffic is covered through the deployment's proxy environment " +
        "and its exact control requests; the TCP sample is a sampling backstop.",
    });
    if (api) {
      try {
        await api.deleteWorkspace();
      } catch {
        /* The isolated database is discarded with the acceptance environment. */
      }
    }
    if (context) await context.close();
  });

  test("observation layers are live (exact Node and Go controls plus browser self-test)", async () => {
    const controls = readObserverControls();
    expect(controls, "observer control report is missing").not.toBeNull();
    expect(controls?.node?.recorded, "Node control request was not recorded").toBe(true);
    expect(controls?.go?.recorded, "Go control request was not recorded").toBe(true);
    expect(controls?.node?.exitCode, "Node control request did not fail").toBe(0);
    expect(controls?.go?.exitCode, "Go control request did not fail").toBe(0);
    expect(controls?.backendEnvKeys ?? []).toEqual(
      expect.arrayContaining(["HTTPS_PROXY", "HTTP_PROXY", "NO_PROXY"]),
    );
    expect(controls?.frontendEnvKeys ?? []).toEqual(
      expect.arrayContaining(["NODE_USE_ENV_PROXY", "HTTPS_PROXY", "NO_PROXY"]),
    );

    // Prove the browser context is routed through the recorder with exactly two
    // deliberate self-test destinations; both must be recorded and both must
    // fail in the browser. Those two entries are then removed from the
    // business observation, so they cannot mask a real external attempt.
    const selftest = await page.evaluate(async () => {
      const results: string[] = [];
      try {
        // No custom header: a CORS preflight would consume the attempt before
        // the real request, and the host itself identifies this self-test.
        await fetch("http://uw80-browser-selftest.invalid/");
        results.push("http:unexpected-success");
      } catch {
        results.push("http:failed");
      }
      try {
        await fetch("https://uw80-browser-selftest-tls.invalid/");
        results.push("tls:unexpected-success");
      } catch {
        results.push("tls:failed");
      }
      return results;
    });
    expect(selftest).toEqual(["http:failed", "tls:failed"]);

    await expect
      .poll(
        () => {
          const hosts = new Set(readProxyLog().map((entry) => entry.host));
          return [
            hosts.has("uw80-browser-selftest.invalid"),
            hosts.has("uw80-browser-selftest-tls.invalid:443"),
          ].filter(Boolean).length;
        },
        { timeout: 15000 },
      )
      .toBe(2);

    const selftestEntries = readProxyLog().filter((entry) =>
      ["uw80-browser-selftest.invalid", "uw80-browser-selftest-tls.invalid:443"].includes(
        String(entry.host),
      ),
    );
    expect(selftestEntries.length).toBeGreaterThanOrEqual(2);
    expect(
      selftestEntries.some((entry) => entry.host === "uw80-browser-selftest.invalid"),
      "browser HTTP self-test was not recorded",
    ).toBe(true);
    expect(
      selftestEntries.some((entry) => entry.host === "uw80-browser-selftest-tls.invalid:443"),
      "browser HTTPS self-test was not recorded",
    ).toBe(true);

    // Remove the exact self-test destinations from business observations.
    observation.externalRequests = observation.externalRequests.filter(
      (value) => !SELF_TEST_ORIGINS.has(value.replace(/\/$/, "")),
    );
    observation.failedRequests = observation.failedRequests.filter(
      (value) => !SELF_TEST_ORIGINS.has(value.replace(/\/$/, "")),
    );
    observation.selftestAttempts = [];
  });

  test("cold start on the login page: no unsolicited network", async () => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Sign in to U Workers")).toBeVisible({ timeout: 30000 });
    await page.waitForLoadState("networkidle");
    recordScenario("cold-start-login-page");
  });

  test("email-code login: no unsolicited network", async () => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    const emailField = page.getByRole("textbox", { name: "Email" });
    await expect(emailField).toBeVisible({ timeout: 30000 });
    await emailField.fill(EMAIL);
    await page.getByRole("button", { name: "Continue" }).click();
    // input-otp renders its focusable field with data-input-otp (it overlays
    // the styled slot group). Typing six digits auto-submits the form.
    const otp = page.locator('input[data-input-otp="true"]');
    await expect(otp).toBeVisible({ timeout: 30000 });
    await otp.click();
    await otp.pressSequentially("888888", { delay: 30 });
    await page.waitForURL(new RegExp(`/${workspaceSlug}/issues`), { timeout: 30000 });
    await expect(page.getByRole("button", { name: "New Issue" })).toBeVisible({ timeout: 30000 });
    // networkidle is not a fit here: the authenticated shell keeps a live
    // WebSocket and cache polling, so give the route a bounded settle window.
    await page.waitForTimeout(1500);
    recordScenario("email-code-login");
  });

  test("browse workspace routes: no unsolicited network", async () => {
    for (const route of ["issues", "inbox", "agents", "runtimes", "settings"] as const) {
      await page.goto(`/${workspaceSlug}/${route}`, { waitUntil: "domcontentloaded" });
      await page.waitForTimeout(1000);
      await expect(page.locator("body")).not.toBeEmpty();
    }
    recordScenario("browse-workspace-routes");
  });

  test("switch workspace: no unsolicited network", async () => {
    await page.goto(`/${workspaceSlug}/issues`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "New Issue" })).toBeVisible({ timeout: 30000 });
    const switcher = page.getByRole("button", { name: new RegExp(WORKSPACE_NAME) }).first();
    await expect(switcher).toBeVisible();
    await switcher.click();
    const altEntry = page.getByRole("menuitem", { name: new RegExp(ALT_WORKSPACE_NAME) });
    await expect(altEntry).toBeVisible({ timeout: 15000 });
    await altEntry.click();
    await page.waitForURL(new RegExp(`/${altWorkspaceSlug}/`), { timeout: 30000 });
    await expect(page.getByRole("button", { name: "New Issue" })).toBeVisible({ timeout: 30000 });
    recordScenario("switch-workspace");
  });

  test("idle with a live workspace view: no unsolicited network", async () => {
    await page.goto(`/${altWorkspaceSlug}/issues`, { waitUntil: "domcontentloaded" });
    await expect(page.getByRole("button", { name: "New Issue" })).toBeVisible({ timeout: 30000 });
    await page.waitForTimeout(8000);
    recordScenario("idle-workspace");
    expect(observation.websockets.length, "no local WebSocket connection observed").toBeGreaterThan(0);
  });

  test("external network unreachable: local task management and attachments keep working", async () => {
    requireObserver(PROXY_PID > 0, "E2E_PROXY_PID is not set.");
    // Stop the recorder: the browser context proxy and the deployment's
    // HTTP(S)_PROXY now point at a closed port, so external destinations are
    // unreachable for browser, Next.js server and Go API. Loopback services are
    // unaffected. (Only this isolated deployment's egress path is closed; no
    // firewall or machine-wide setting is changed.)
    try {
      process.kill(PROXY_PID);
    } catch {
      /* already gone */
    }
    await expect.poll(() => probeProxyDown(PROXY_URL), { timeout: 20000 }).toBe("down");

    const storage = await context.storageState();
    const offline = await browser.newContext({
      baseURL: WEB_ORIGIN,
      proxy: { server: PROXY_URL, bypass: "localhost,127.0.0.1,::1" },
      storageState: storage,
    });
    // The offline scenario exercises the manual create flow, not the agent
    // composer, so pin the create mode before the app boots in this profile.
    await offline.addInitScript(() => {
      localStorage.setItem(
        "multica_create_mode",
        JSON.stringify({ state: { lastMode: "manual" }, version: 0 }),
      );
    });
    const offlinePage = await offline.newPage();
    try {
      await offlinePage.goto(`/${workspaceSlug}/issues`, { waitUntil: "domcontentloaded" });
      await expect(offlinePage.getByRole("button", { name: "New Issue" })).toBeVisible({
        timeout: 30000,
      });

      const title = `Offline acceptance ${Date.now()}`;
      await offlinePage.getByRole("button", { name: "New Issue" }).click();
      const titleInput = offlinePage.getByRole("textbox", { name: "Issue title" });
      await expect(titleInput).toBeVisible();
      await titleInput.fill(title);
      await offlinePage.getByRole("button", { name: "Create Issue" }).click();
      await expect(offlinePage.getByText("Issue created")).toBeVisible({ timeout: 15000 });

      const upload = await offlinePage.evaluate(
        async ({ slug }) => {
          const token = localStorage.getItem("multica_token");
          const cookies = Object.fromEntries(
            document.cookie
              .split("; ")
              .filter(Boolean)
              .map((part) => {
                const index = part.indexOf("=");
                return [part.slice(0, index), decodeURIComponent(part.slice(index + 1))];
              }),
          );
          // Cookie-backed sessions require the CSRF cookie value in the header
          // for state-changing requests; the app client does the same.
          const csrf = cookies.multica_csrf_session ?? cookies.multica_csrf ?? "";
          const form = new FormData();
          form.append(
            "file",
            new File([new Uint8Array([0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a])], "offline.png", {
              type: "image/png",
            }),
          );
          const response = await fetch("/api/upload-file", {
            method: "POST",
            body: form,
            headers: {
              ...(token ? { Authorization: `Bearer ${token}` } : {}),
              ...(csrf ? { "X-CSRF-Token": csrf } : {}),
              "X-Workspace-Slug": slug,
            },
          });
          return { status: response.status, body: (await response.text()).slice(0, 120) };
        },
        { slug: workspaceSlug },
      );
      expect(upload.status, `local attachment upload failed: ${upload.body}`).toBe(200);

      const external = await offlinePage.evaluate(async () => {
        try {
          await fetch("http://uw80-offline-sentinel.invalid/", {
            signal: AbortSignal.timeout(5000),
          });
          return "reachable";
        } catch {
          return "unreachable";
        }
      });
      expect(external).toBe("unreachable");

      await offlinePage.reload({ waitUntil: "domcontentloaded" });
      await expect(offlinePage.getByText(title)).toBeVisible({ timeout: 30000 });
      writeEvidenceFile("offline-scenario.json", {
        scenario: "external-network-unreachable",
        recorderPortClosed: true,
        localIssueCreated: true,
        localAttachmentUploadStatus: upload.status,
        externalSentinel: external,
        boundary:
          "The browser and the isolated deployment's proxy environment point at " +
          "a closed recorder port. This is not a machine-wide firewall change.",
      });
    } finally {
      await offline.close();
    }
  });
});
