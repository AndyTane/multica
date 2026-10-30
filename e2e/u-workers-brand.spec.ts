import { test, expect, _electron as electron } from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { Socket } from "node:net";

import { TestApiClient } from "./fixtures";
import { loginAsDefault } from "./helpers";

/**
 * CLAWSTA-80 Task 8 — U Workers brand acceptance against the real built web
 * application (Next.js production server, isolated deployment).
 *
 * Asserts user-visible identity: document titles, product mark accessible
 * names, manifest/PWA metadata, bundled icons and the absence of upstream
 * official-cloud hosts in served HTML.
 *
 * The packaged desktop case runs only when a real win-unpacked executable and
 * the isolated acceptance root are supplied. It launches the packaged app with
 * a redirected USERPROFILE/HOME (isolated ~/.u-workers and ~/.multica), logs in
 * through the real UI, browses Issues/Inbox/Agents, idles for 60 seconds, and
 * requires three independent observation layers to see zero non-loopback
 * traffic: the Chromium proxy plus Playwright request events, the
 * main-process/child environment proxy contract, and a per-second
 * Get-NetTCPConnection sample of the packaged process tree.
 */

const PRODUCT_NAME = "U Workers";
const OLD_OFFICIAL_HOST = /multica\.ai/i;

// --- Packaged desktop harness configuration -------------------------------

const DESKTOP_EXE = process.env.E2E_DESKTOP_EXE ?? "";
const DESKTOP_ROOT = process.env.E2E_ACCEPTANCE_ROOT ?? "";
const DESKTOP_PROXY_URL = process.env.E2E_PROXY_URL ?? "";
const DESKTOP_PROXY_LOG = process.env.E2E_PROXY_LOG ?? "";
const DESKTOP_EVIDENCE_DIR =
  process.env.E2E_DESKTOP_EVIDENCE_DIR ?? resolve(process.cwd(), "evidence", "clawsta-80", "desktop");
const DESKTOP_RUN_ID = process.env.E2E_RUN_ID ?? "d1-" + Date.now().toString(36);
const DESKTOP_EMAIL = "e2e-desktop-" + DESKTOP_RUN_ID + "@multica.ai";
const DESKTOP_WORKSPACE_NAME = "E2E Desktop Workspace";
const DESKTOP_WORKSPACE_SLUG = "e2e-desktop-" + DESKTOP_RUN_ID;

// The only deliberate non-loopback destinations the harness creates. They are
// exact hosts, never a wildcard: a broad ".invalid is fine" rule would let a
// real attempted destination hide behind an observer name.
const DESKTOP_SELFTEST_HTTP_HOST = "uw80-desktop-selftest.invalid";
const DESKTOP_SELFTEST_TLS_HOST = "uw80-desktop-selftest-tls.invalid:443";
const DESKTOP_NODE_CONTROL_HOST = "uw80-desktop-nodecontrol.invalid";
const DESKTOP_CONTROL_HOSTNAMES = new Set([
  "uw80-desktop-selftest.invalid",
  "uw80-desktop-selftest-tls.invalid",
  "uw80-desktop-nodecontrol.invalid",
]);
const SYSTEM_LAYER_CONTROL_REMOTE = "192.0.2.1:80"; // TEST-NET-1, not routable

type DesktopDeployment = {
  Root?: string;
  Database?: string;
  DatabasePort?: number;
  ApiPort?: number;
  WebPort?: number;
};

type DesktopRuntimeConfig = {
  schemaVersion?: number;
  apiUrl?: string;
  wsUrl?: string;
  appUrl?: string;
};

type ExeVersionInfo = {
  ProductName?: string;
  FileDescription?: string;
  FileVersion?: string;
  ProductVersion?: string;
  OriginalFilename?: string;
};

type ProxyLogEntry = {
  ts?: string;
  kind?: string;
  method?: string;
  host?: string;
  marker?: string | null;
  result?: string;
};

type TcpConnection = { remote?: string; state?: string; pid?: number };

type TcpSample = {
  ts?: string;
  pids?: number[];
  connections?: TcpConnection[];
  error?: string;
};

type ProcessRow = { pid: number; ppid: number; name: string };

type BrowserObservation = {
  requests: number;
  externalRequests: string[];
  websockets: string[];
  externalWebsockets: string[];
  navigations: string[];
  externalNavigations: string[];
  failedRequests: string[];
};

function readJsonFile<T>(path: string): T {
  return JSON.parse(readFileSync(path, "utf8").replace(/^\uFEFF/, "")) as T;
}

function desktopSkipReason(): string | null {
  if (!DESKTOP_EXE) {
    return (
      "E2E_DESKTOP_EXE is not set: the packaged desktop case needs the win-unpacked build from " +
      "pnpm --filter @multica/desktop package -- --win --dir."
    );
  }
  if (!existsSync(DESKTOP_EXE)) return "E2E_DESKTOP_EXE does not exist: " + DESKTOP_EXE;
  if (!DESKTOP_ROOT) {
    return (
      "E2E_ACCEPTANCE_ROOT is not set: start the isolated acceptance deployment described in " +
      "docs/outbound-data-verification.md first."
    );
  }
  const markerPath = join(DESKTOP_ROOT, "acceptance-root.json");
  const deploymentPath = join(DESKTOP_ROOT, "deployment.json");
  if (!existsSync(markerPath) || !existsSync(deploymentPath)) {
    return "acceptance root marker or deployment.json is missing under " + DESKTOP_ROOT;
  }
  try {
    const marker = readJsonFile<{ purpose?: string }>(markerPath);
    if (typeof marker.purpose !== "string" || !marker.purpose.includes("isolated acceptance")) {
      return "acceptance root marker does not describe an isolated acceptance environment";
    }
  } catch {
    return "cannot read acceptance root metadata under " + DESKTOP_ROOT;
  }
  return null;
}

function requireDesktopHarness(condition: boolean, message: string) {
  if (!condition) {
    throw new Error(
      message +
        "\n\nThis case runs only against the isolated desktop acceptance harness: a packaged " +
        "win-unpacked build (E2E_DESKTOP_EXE), the isolated acceptance root " +
        "(E2E_ACCEPTANCE_ROOT) and the recording proxy (E2E_PROXY_URL / E2E_PROXY_LOG). " +
        "See docs/outbound-data-verification.md.",
    );
  }
}

function readExeVersionInfo(exePath: string): ExeVersionInfo {
  const script =
    "$v = (Get-Item -LiteralPath '" +
    exePath.replace(/'/g, "''") +
    "').VersionInfo; [pscustomobject]@{ ProductName = $v.ProductName; " +
    "FileDescription = $v.FileDescription; FileVersion = $v.FileVersion; " +
    "ProductVersion = $v.ProductVersion; OriginalFilename = $v.OriginalFilename } | " +
    "ConvertTo-Json -Compress";
  const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], {
    encoding: "utf8",
    timeout: 30000,
    windowsHide: true,
  });
  return JSON.parse(output.replace(/^\uFEFF/, "").trim()) as ExeVersionInfo;
}

function readProcessTable(): ProcessRow[] {
  const script =
    "Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name | " +
    "ForEach-Object { [pscustomobject]@{ pid = [int]$_.ProcessId; ppid = [int]$_.ParentProcessId; name = [string]$_.Name } } | " +
    "ConvertTo-Json -Compress";
  const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], {
    encoding: "utf8",
    timeout: 30000,
    windowsHide: true,
  });
  const trimmed = output.replace(/^\uFEFF/, "").trim();
  if (!trimmed) return [];
  const parsed = JSON.parse(trimmed) as ProcessRow | ProcessRow[];
  return Array.isArray(parsed) ? parsed : [parsed];
}

type DaemonProcess = { pid: number; name: string };

/**
 * Finds the bundled CLI daemon the packaged app auto-starts. It runs detached
 * from the app process (the CLI daemonizes), so it must be added to the TCP
 * sampling roots explicitly instead of hoping a live parent chain includes it.
 */
function readDesktopDaemonPids(apiPort: number): number[] {
  const needle = "--profile u-workers-desktop-127.0.0.1-" + apiPort;
  const script =
    "Get-CimInstance Win32_Process | Where-Object { $_.Name -eq 'multica.exe' -and $_.CommandLine -and " +
    "$_.CommandLine.Contains('" + needle + "') } | ForEach-Object { [int]$_.ProcessId } | ConvertTo-Json -Compress";
  const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], {
    encoding: "utf8",
    timeout: 30000,
    windowsHide: true,
  });
  const trimmed = output.replace(/^\uFEFF/, "").trim();
  if (!trimmed) return [];
  const parsed = JSON.parse(trimmed) as number | number[];
  return (Array.isArray(parsed) ? parsed : [parsed]).filter((pid) => Number.isInteger(pid) && pid > 0);
}

function stopProcessById(pid: number) {
  try {
    execFileSync(
      "powershell.exe",
      ["-NoProfile", "-Command", "Stop-Process -Id " + pid + " -Force -ErrorAction SilentlyContinue"],
      { timeout: 15000, windowsHide: true },
    );
  } catch {
    /* best-effort cleanup of exactly the PIDs this run started */
  }
}

function processTree(rows: ProcessRow[], rootPid: number): ProcessRow[] {
  const wanted = new Set<number>([rootPid]);
  let changed = true;
  while (changed) {
    changed = false;
    for (const row of rows) {
      if (wanted.has(row.pid)) continue;
      if (wanted.has(row.ppid)) {
        wanted.add(row.pid);
        changed = true;
      }
    }
  }
  return rows.filter((row) => wanted.has(row.pid));
}

const TCP_SAMPLER_SCRIPT = String.raw`param(
    [int]$MainPid,
    [string]$ExtraPids = "",
    [string]$OutPath,
    [int]$IntervalMs = 1000
)
$ErrorActionPreference = 'Continue'
$stopFile = $OutPath + '.stop'
$utf8 = [Text.UTF8Encoding]::new($false)
function Write-Sample {
    param($Record)
    try {
        [IO.File]::AppendAllText($OutPath, ($Record | ConvertTo-Json -Compress -Depth 5) + [Environment]::NewLine, $utf8)
    } catch { }
}
while (-not (Test-Path -LiteralPath $stopFile)) {
    $started = Get-Date
    $pids = New-Object System.Collections.Generic.List[int]
    $pids.Add($MainPid)
    foreach ($extra in ($ExtraPids -split ',')) {
        $parsed = 0
        if ([int]::TryParse($extra.Trim(), [ref]$parsed) -and $parsed -gt 0 -and -not $pids.Contains($parsed)) {
            $pids.Add($parsed)
        }
    }
    try {
        $procs = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId -ErrorAction Stop)
        $changed = $true
        while ($changed) {
            $changed = $false
            foreach ($p in $procs) {
                $childPid = [int]$p.ProcessId
                $parentPid = [int]$p.ParentProcessId
                if ($pids.Contains($parentPid) -and -not $pids.Contains($childPid)) {
                    $pids.Add($childPid)
                    $changed = $true
                }
            }
        }
        $connections = @()
        $tcp = @(Get-NetTCPConnection -ErrorAction Stop)
        foreach ($connection in $tcp) {
            $owner = [int]$connection.OwningProcess
            if ($pids.Contains($owner)) {
                $connections += [pscustomobject]@{
                    remote = ('{0}:{1}' -f $connection.RemoteAddress, $connection.RemotePort)
                    state = [string]$connection.State
                    pid = $owner
                }
            }
        }
        Write-Sample ([pscustomobject]@{
            ts = (Get-Date).ToUniversalTime().ToString('o')
            pids = @($pids)
            connections = @($connections)
        })
    } catch {
        Write-Sample ([pscustomobject]@{
            ts = (Get-Date).ToUniversalTime().ToString('o')
            pids = @($pids)
            error = [string]$_.Exception.Message
        })
    }
    $elapsed = ((Get-Date) - $started).TotalMilliseconds
    $remaining = [int]($IntervalMs - $elapsed)
    if ($remaining -lt 100) { $remaining = 100 }
    Start-Sleep -Milliseconds $remaining
}`;

const SCREEN_CAPTURE_SCRIPT = String.raw`param(
    [long]$Hwnd,
    [string]$OutPath
)
Add-Type @"
using System;
using System.Text;
using System.Collections.Generic;
using System.Runtime.InteropServices;
public static class CapWin3 {
  public delegate bool EnumProc(IntPtr h, IntPtr l);
  [DllImport("user32.dll")] public static extern bool SetProcessDPIAware();
  [DllImport("user32.dll")] public static extern bool SetForegroundWindow(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool ShowWindow(IntPtr hWnd, int nCmdShow);
  [DllImport("user32.dll")] public static extern bool BringWindowToTop(IntPtr hWnd);
  [DllImport("user32.dll")] public static extern bool EnumWindows(EnumProc cb, IntPtr l);
  [DllImport("user32.dll")] public static extern uint GetWindowThreadProcessId(IntPtr h, out uint pid);
  [DllImport("user32.dll")] public static extern bool IsWindowVisible(IntPtr h);
  [DllImport("user32.dll")] public static extern int GetClassName(IntPtr h, StringBuilder s, int n);
}
"@
[void][CapWin3]::SetProcessDPIAware()
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
# Other always-on-top desktop windows (an unrelated usage widget, the
# computer-use cursor overlay and the pinned tray overflow flyout) would draw
# over the evidence frame. Hide exactly those windows for the capture and
# restore them afterwards.
$overlayPids = @(Get-Process -Name 'ai-usage-widget','cua-driver' -ErrorAction SilentlyContinue | ForEach-Object { $_.Id })
$hidden = New-Object System.Collections.ArrayList
$enum = {
  param($h, $l)
  if (-not [CapWin3]::IsWindowVisible($h)) { return $true }
  $p = 0
  [void][CapWin3]::GetWindowThreadProcessId($h, [ref]$p)
  $cls = New-Object System.Text.StringBuilder 256
  [void][CapWin3]::GetClassName($h, $cls, 256)
  $hide = $false
  if ($overlayPids -contains [int]$p) { $hide = $true }
  if ($cls.ToString() -eq 'TopLevelWindowForOverflowXamlIsland') { $hide = $true }
  if ($hide) {
    [void][CapWin3]::ShowWindow($h, 0)
    [void]$hidden.Add([long]$h)
  }
  return $true
}
[void][CapWin3]::EnumWindows($enum, [IntPtr]::Zero)
Start-Sleep -Milliseconds 700
$h = [IntPtr]::new($Hwnd)
[void][CapWin3]::ShowWindow($h, 3)
[void][CapWin3]::BringWindowToTop($h)
[void][CapWin3]::SetForegroundWindow($h)
Start-Sleep -Milliseconds 1200
$b = [System.Windows.Forms.Screen]::PrimaryScreen.Bounds
$bmp = New-Object System.Drawing.Bitmap $b.Width, $b.Height
$g = [System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($b.Location, [System.Drawing.Point]::Empty, $b.Size)
$bmp.Save($OutPath, [System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose(); $bmp.Dispose()
foreach ($handle in $hidden) { [void][CapWin3]::ShowWindow([IntPtr]::new([long]$handle), 5) }
Write-Output ('captured {0}x{1}' -f $b.Width, $b.Height)`;;

function writeHelperScript(root: string, name: string, content: string): string {
  const dir = join(root, "state", "d1-helpers");
  mkdirSync(dir, { recursive: true });
  const path = join(dir, name);
  writeFileSync(path, content + "\n", "utf8");
  return path;
}

function readSamples(path: string): TcpSample[] {
  if (!existsSync(path)) return [];
  const samples: TcpSample[] = [];
  for (const line of readFileSync(path, "utf8").replace(/^\uFEFF/, "").split(/\r?\n/)) {
    if (!line.trim()) continue;
    try {
      samples.push(JSON.parse(line) as TcpSample);
    } catch {
      /* a sample cut short when the sampler exits is not evidence */
    }
  }
  return samples;
}

function isLoopbackRemote(remote: string): boolean {
  const host = remote.startsWith("[")
    ? remote.slice(1, remote.indexOf("]"))
    : remote.slice(0, remote.lastIndexOf(":"));
  return host === "::1" || host === "::" || host === "0.0.0.0" || host.startsWith("127.");
}

function startTcpSampler(
  root: string,
  mainPid: number,
  outPath: string,
  intervalMs = 1000,
  extraPids: number[] = [],
): ChildProcess {
  const scriptPath = writeHelperScript(root, "desktop-tcp-sampler.ps1", TCP_SAMPLER_SCRIPT);
  return spawn(
    "powershell.exe",
    [
      "-NoProfile",
      "-NonInteractive",
      "-ExecutionPolicy",
      "Bypass",
      "-File",
      scriptPath,
      "-MainPid",
      String(mainPid),
      "-ExtraPids",
      extraPids.join(","),
      "-OutPath",
      outPath,
      "-IntervalMs",
      String(intervalMs),
    ],
    { windowsHide: true, stdio: "ignore" },
  );
}

async function stopTcpSampler(child: ChildProcess, outPath: string) {
  writeFileSync(outPath + ".stop", "stop\n", "utf8");
  const deadline = Date.now() + 15000;
  while (child.exitCode === null && child.signalCode === null && Date.now() < deadline) {
    await new Promise((resolveWait) => setTimeout(resolveWait, 250));
  }
  if (child.exitCode === null && child.signalCode === null) child.kill();
}

function readProxyLog(): ProxyLogEntry[] {
  requireDesktopHarness(
    Boolean(DESKTOP_PROXY_LOG) && existsSync(DESKTOP_PROXY_LOG),
    "recording proxy log is missing: " + (DESKTOP_PROXY_LOG || "(E2E_PROXY_LOG is not set)"),
  );
  return readFileSync(DESKTOP_PROXY_LOG, "utf8")
    .split(/\r?\n/)
    .filter((line) => line.trim().length > 0)
    .map((line, index) => {
      try {
        return JSON.parse(line) as ProxyLogEntry;
      } catch {
        throw new Error("Invalid recorder metadata at line " + (index + 1) + "; cannot verify outbound traffic.");
      }
    });
}

function proxyTrafficEntries(entries: ProxyLogEntry[]): ProxyLogEntry[] {
  return entries.filter(
    (entry) => entry.kind === "http" || entry.kind === "connect" || entry.kind === "upgrade",
  );
}

function probeRecorder(): Promise<"up" | "down"> {
  const target = new URL(DESKTOP_PROXY_URL);
  return new Promise((resolveProbe) => {
    const socket = new Socket();
    socket.setTimeout(1500);
    socket.once("connect", () => {
      socket.destroy();
      resolveProbe("up");
    });
    socket.once("timeout", () => {
      socket.destroy();
      resolveProbe("down");
    });
    socket.once("error", () => resolveProbe("down"));
    socket.connect(Number(target.port || 80), target.hostname);
  });
}

function sanitizeUrl(raw: string): string {
  try {
    const url = new URL(raw);
    if (!["http:", "https:", "ws:", "wss:"].includes(url.protocol)) return url.protocol;
    return url.protocol + "//" + url.hostname;
  } catch {
    return "invalid-url";
  }
}

function isDesktopControlUrl(raw: string): boolean {
  try {
    return DESKTOP_CONTROL_HOSTNAMES.has(new URL(raw).hostname.toLowerCase());
  } catch {
    return false;
  }
}

function isLocalUrl(raw: string): boolean {
  let url: URL;
  try {
    url = new URL(raw);
  } catch {
    return true;
  }
  if (["data:", "blob:", "about:", "chrome:", "devtools:", "file:"].includes(url.protocol)) return true;
  const hostname = url.hostname.toLowerCase();
  return (
    hostname === "localhost" ||
    hostname === "127.0.0.1" ||
    hostname === "::1" ||
    hostname.startsWith("127.") ||
    hostname.endsWith(".localhost")
  );
}

function buildDesktopEnv(isolatedHome: string): Record<string, string> {
  const env: Record<string, string> = {};
  const passthrough = [
    "SystemRoot",
    "SystemDrive",
    "windir",
    "PATH",
    "PATHEXT",
    "TEMP",
    "TMP",
    "APPDATA",
    "LOCALAPPDATA",
    "COMSPEC",
    "PROGRAMFILES",
    "PROGRAMFILES(X86)",
    "PROGRAMDATA",
    "NUMBER_OF_PROCESSORS",
    "OS",
    "PROCESSOR_ARCHITECTURE",
    "USERNAME",
    "USERDOMAIN",
    "COMPUTERNAME",
    "LANG",
  ];
  for (const key of passthrough) {
    const value = process.env[key];
    if (value) env[key] = value;
  }
  env.USERPROFILE = isolatedHome;
  env.HOME = isolatedHome;
  env.HOMEDRIVE = isolatedHome.slice(0, 2);
  env.HOMEPATH = isolatedHome.slice(2);
  env.HTTP_PROXY = DESKTOP_PROXY_URL;
  env.HTTPS_PROXY = DESKTOP_PROXY_URL;
  env.NO_PROXY = "localhost,127.0.0.1,::1";
  env.NODE_USE_ENV_PROXY = "1";
  return env;
}

async function captureFullWindow(
  desktopApp: Awaited<ReturnType<typeof electron.launch>>,
  root: string,
  outPath: string,
): Promise<void> {
  const handle = await desktopApp.evaluate(({ BrowserWindow }) => {
    const window = BrowserWindow.getAllWindows()[0];
    if (!window) return "0";
    window.maximize();
    window.show();
    window.focus();
    const buffer = window.getNativeWindowHandle();
    return buffer.length >= 8 ? buffer.readBigInt64LE(0).toString() : String(buffer.readUInt32LE(0));
  });
  if (handle === "0") throw new Error("packaged window handle is unavailable for the acceptance screenshot");
  const scriptPath = writeHelperScript(root, "capture-full-window.ps1", SCREEN_CAPTURE_SCRIPT);
  execFileSync(
    "powershell.exe",
    [
      "-NoProfile",
      "-NonInteractive",
      "-ExecutionPolicy",
      "Bypass",
      "-File",
      scriptPath,
      "-Hwnd",
      handle,
      "-OutPath",
      outPath,
    ],
    { encoding: "utf8", timeout: 60000, windowsHide: true },
  );
  requireDesktopHarness(existsSync(outPath), "full-window screenshot was not written: " + outPath);
  const size = readFileSync(outPath).length;
  requireDesktopHarness(size > 10000, "full-window screenshot is implausibly small (" + size + " bytes)");
}

test.describe("U Workers brand surfaces", () => {
  test("login page presents U Workers and bundles its own assets", async ({ page }) => {
    await page.goto("/login", { waitUntil: "domcontentloaded" });
    await expect(page.getByText("Sign in to U Workers")).toBeVisible({ timeout: 30000 });
    await expect(page.getByRole("img", { name: PRODUCT_NAME }).first()).toBeVisible();
    await expect(page).toHaveTitle(/U Workers/);

    const html = await page.content();
    expect(html).not.toMatch(OLD_OFFICIAL_HOST);
    const visibleText = await page.locator("body").innerText();
    expect(visibleText).not.toMatch(/\bMultica\b/);

    await expect(page.locator('link[rel="manifest"]')).toHaveCount(1);
    await expect(page.locator('link[rel="manifest"]')).toHaveAttribute(
      "href",
      /manifest\.webmanifest/,
    );
    await expect(page.locator('link[rel="icon"]').first()).toHaveAttribute(
      "href",
      /favicon\.svg/,
    );
    await expect(page.locator('meta[name="apple-mobile-web-app-title"]')).toHaveAttribute(
      "content",
      PRODUCT_NAME,
    );
    await expect(page.locator('meta[name="theme-color"]').first()).toHaveCount(1);
  });

  test("web app manifest is branded, installable metadata and icons resolve locally", async ({
    request,
  }) => {
    const manifestResponse = await request.get("/manifest.webmanifest");
    expect(manifestResponse.ok(), "manifest.webmanifest must be served").toBe(true);
    expect(manifestResponse.headers()["content-type"]).toContain("application/manifest+json");

    const manifest = (await manifestResponse.json()) as {
      name?: string;
      short_name?: string;
      description?: string;
      display?: string;
      start_url?: string;
      icons?: { src: string; sizes?: string; type?: string; purpose?: string }[];
      shortcuts?: unknown[];
    };
    expect(manifest.name).toBe(PRODUCT_NAME);
    expect(manifest.short_name).toBe(PRODUCT_NAME);
    expect(manifest.display).toBe("standalone");
    expect(manifest.start_url).toBeTruthy();
    expect(manifest.start_url).not.toBe("/");
    expect(manifest.description ?? "").not.toMatch(OLD_OFFICIAL_HOST);
    expect(manifest.shortcuts?.length ?? 0).toBeGreaterThan(0);

    const icons = manifest.icons ?? [];
    const sizes = icons.map((icon) => icon.sizes);
    expect(sizes).toEqual(expect.arrayContaining(["192x192", "512x512"]));
    expect(icons.some((icon) => icon.purpose === "maskable")).toBe(true);
    expect(icons.length).toBeGreaterThanOrEqual(3);
    for (const icon of icons) {
      const iconResponse = await request.get(icon.src);
      expect(iconResponse.ok(), "missing manifest icon " + icon.src).toBe(true);
      expect(iconResponse.headers()["content-type"]).toContain("image/png");
    }

    const favicon = await request.get("/favicon.svg");
    expect(favicon.ok()).toBe(true);
    expect(favicon.headers()["content-type"]).toContain("image/svg+xml");

    const appleTouchIcon = await request.get("/icons/apple-touch-icon.png");
    expect(appleTouchIcon.ok()).toBe(true);
    expect(appleTouchIcon.headers()["content-type"]).toContain("image/png");

    const shell = await request.get("/login");
    expect(shell.ok()).toBe(true);
    expect(await shell.text()).not.toMatch(OLD_OFFICIAL_HOST);
  });

  test("workspace shell keeps U Workers identity in titles and the product mark", async ({
    page,
  }) => {
    const slug = await loginAsDefault(page);
    await expect(page.getByRole("img", { name: PRODUCT_NAME }).first()).toBeVisible();
    await expect(page).toHaveTitle("Issues | U Workers");

    // Client-side navigation in the production build can fall back to the
    // root document title (the root title is still U Workers). Assert the
    // brand identity on every route here; the pre-existing route-title
    // fallback is recorded in docs/outbound-data-verification.md.
    await page.getByRole("link", { name: "Inbox" }).click();
    await expect(page).toHaveURL(/\/inbox/, { timeout: 30000 });
    await expect.poll(() => page.title(), { timeout: 15000 }).toMatch(/U Workers/);

    await page.getByRole("link", { name: "Agents" }).click();
    await expect(page).toHaveURL(/\/agents/, { timeout: 30000 });
    await expect.poll(() => page.title(), { timeout: 15000 }).toMatch(/U Workers/);

    const html = await page.content();
    expect(html).not.toMatch(OLD_OFFICIAL_HOST);
    await expect(page.locator('a[href*="/' + slug + '/issues"]').first()).toBeVisible();
  });
});


test.describe("packaged desktop identity and outbound observation", () => {
  const skipReason = desktopSkipReason();
  test.skip(Boolean(skipReason), skipReason ?? "");

  test("packaged app boots from isolated config, logs in, and stays loopback-only", async () => {
    test.setTimeout(480_000);

    requireDesktopHarness(Boolean(DESKTOP_PROXY_URL), "E2E_PROXY_URL is not set");
    requireDesktopHarness(Boolean(DESKTOP_PROXY_LOG), "E2E_PROXY_LOG is not set");
    requireDesktopHarness(Boolean(DESKTOP_EXE) && existsSync(DESKTOP_EXE), "E2E_DESKTOP_EXE is not usable");

    const evidencePath = (name: string) => join(DESKTOP_EVIDENCE_DIR, name);
    mkdirSync(DESKTOP_EVIDENCE_DIR, { recursive: true });

    const marker = readJsonFile<{ purpose?: string }>(join(DESKTOP_ROOT, "acceptance-root.json"));
    const deployment = readJsonFile<DesktopDeployment>(join(DESKTOP_ROOT, "deployment.json"));
    requireDesktopHarness(
      typeof marker.purpose === "string" && marker.purpose.includes("isolated acceptance"),
      "acceptance root marker does not describe an isolated acceptance environment",
    );
    requireDesktopHarness(
      typeof deployment.Root === "string" && deployment.Root.toLowerCase() === DESKTOP_ROOT.toLowerCase(),
      "deployment.json belongs to a different root than E2E_ACCEPTANCE_ROOT",
    );

    const isolatedHome = join(DESKTOP_ROOT, "home");
    const desktopConfigPath = join(isolatedHome, ".u-workers", "desktop.json");
    requireDesktopHarness(existsSync(desktopConfigPath), "isolated desktop config is missing: " + desktopConfigPath);
    const runtimeConfig = readJsonFile<DesktopRuntimeConfig>(desktopConfigPath);
    requireDesktopHarness(runtimeConfig.schemaVersion === 1, "isolated desktop.json does not use schemaVersion 1");
    requireDesktopHarness(
      new URL(runtimeConfig.apiUrl ?? "").port === String(deployment.ApiPort),
      "isolated desktop.json apiUrl does not use the acceptance API port " + deployment.ApiPort,
    );
    requireDesktopHarness(
      new URL(runtimeConfig.appUrl ?? "").port === String(deployment.WebPort),
      "isolated desktop.json appUrl does not use the acceptance web port " + deployment.WebPort,
    );

    // Electron resolves userData relative to the redirected USERPROFILE on this
    // Windows build (observed: <isolated home>\AppData\Roaming\U Workers), so the
    // packaged profile must never land in the real roaming profile. Both paths
    // are asserted: the real one must never appear, the isolated one must start
    // clean and be removed after the run.
    const userDataPath = join(isolatedHome, "AppData", "Roaming", PRODUCT_NAME);
    const realAppDataPath = process.env.APPDATA ? join(process.env.APPDATA, PRODUCT_NAME) : "";
    requireDesktopHarness(Boolean(realAppDataPath), "APPDATA is not available to locate the real roaming profile");
    requireDesktopHarness(
      !existsSync(realAppDataPath),
      "the real roaming userData directory already exists; refusing to touch it (" + realAppDataPath + ")",
    );
    requireDesktopHarness(
      !existsSync(userDataPath),
      "the isolated desktop userData directory already exists before the run: " + userDataPath,
    );

    const realMulticaProfiles = join(homedir(), ".multica", "profiles");
    const realProfilesBefore = existsSync(realMulticaProfiles) ? readdirSync(realMulticaProfiles).sort() : [];
    const isolatedMulticaDir = join(isolatedHome, ".multica");
    const isolatedMulticaExistedBefore = existsSync(isolatedMulticaDir);

    const versionInfo = readExeVersionInfo(DESKTOP_EXE);
    expect(versionInfo.ProductName, "packaged exe ProductName").toBe(PRODUCT_NAME);
    expect(versionInfo.FileDescription, "packaged exe FileDescription").toBe(PRODUCT_NAME);
    expect(JSON.stringify(versionInfo), "packaged exe version info must not still be Electron").not.toMatch(
      /electron/i,
    );

    const api = new TestApiClient();
    await api.login(DESKTOP_EMAIL, "E2E Desktop User");
    await api.ensureWorkspace(DESKTOP_WORKSPACE_NAME, DESKTOP_WORKSPACE_SLUG);
    await api.markUserOnboarded();

    await expect.poll(() => probeRecorder(), { timeout: 15000 }).toBe("up");

    // System-layer liveness: the same sampler script used for the observation
    // window must report a deliberate SYN_SENT connection to the reserved
    // TEST-NET-1 address (no routable service, no payload). Without this, a
    // zero from the TCP layer would not be evidence of anything.
    const controlSamplesPath = join(DESKTOP_ROOT, "state", "tcp-sampler-control.jsonl");
    rmSync(controlSamplesPath, { force: true });
    rmSync(controlSamplesPath + ".stop", { force: true });
    const sentinel = spawn(
      process.execPath,
      [
        "-e",
        "const net=require('net');const s=net.connect({host:'192.0.2.1',port:80});s.on('error',()=>{});setTimeout(()=>process.exit(0),20000);",
      ],
      { stdio: "ignore", windowsHide: true },
    );
    requireDesktopHarness(typeof sentinel.pid === "number", "system-layer control process did not start");
    const controlSampler = startTcpSampler(DESKTOP_ROOT, sentinel.pid as number, controlSamplesPath, 500);
    let systemLayerControlObserved = false;
    const controlDeadline = Date.now() + 25000;
    while (!systemLayerControlObserved && Date.now() < controlDeadline) {
      await new Promise((resolveWait) => setTimeout(resolveWait, 500));
      systemLayerControlObserved = readSamples(controlSamplesPath).some((sample) =>
        (sample.connections ?? []).some((connection) => connection.remote === SYSTEM_LAYER_CONTROL_REMOTE),
      );
    }
    await stopTcpSampler(controlSampler, controlSamplesPath);
    sentinel.kill();
    expect(systemLayerControlObserved, "TCP sampler did not observe the system-layer control connection").toBe(
      true,
    );

    const desktopEnv = buildDesktopEnv(isolatedHome);
    const observation: BrowserObservation = {
      requests: 0,
      externalRequests: [],
      websockets: [],
      externalWebsockets: [],
      navigations: [],
      externalNavigations: [],
      failedRequests: [],
    };
    let isolatedApiRequests = 0;

    let desktopApp: Awaited<ReturnType<typeof electron.launch>> | undefined;
    let windowSampler: ChildProcess | undefined;
    const windowSamplerPath = join(DESKTOP_ROOT, "state", "tcp-sampler-window.jsonl");
    let appDataExistedDuringRun = false;
    let desktopPid = 0;
    let proxyOffset = 0;
    let browserCountsBefore = { externalRequests: 0, externalWebsockets: 0, externalNavigations: 0 };
    let realProfilesAfter: string[] = [];
    let isolatedMulticaCreated = false;
    let userDataRemoved = false;
    let loginWindowTitle = "";
    let mainWindowTitle = "";
    const scenarioRecords: Record<string, unknown>[] = [];
    const evidence: Record<string, unknown> = {
      batch: "CLAWSTA-80 desktop acceptance D1",
      generatedAt: new Date().toISOString(),
      command: {
        spec: "pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --reporter=line",
        desktopExe: DESKTOP_EXE,
        acceptanceRoot: DESKTOP_ROOT,
        recordingProxy: DESKTOP_PROXY_URL,
        launchArgs: ["--proxy-server=" + DESKTOP_PROXY_URL, "--proxy-bypass-list=localhost;127.0.0.1;::1"],
      },
      exeVersionInfo: versionInfo,
      isolation: {
        userDataPath,
        realAppDataPath,
        isolatedHome,
        isolatedDesktopConfig: desktopConfigPath,
        userDataExistedBeforeRun: false,
        realAppDataExistedBeforeRun: false,
        realAppDataExistedAfterRun: null as boolean | null,
        realAppDataEverObserved: false,
        isolatedMulticaExistedBefore,
        realMulticaProfilesBeforeCount: realProfilesBefore.length,
      },
      app: {} as Record<string, unknown>,
      observation: {
        layers: {
          chromium:
            "launch args --proxy-server plus an explicit loopback bypass, and Playwright context/page request, websocket and navigation events",
          environment:
            "HTTP_PROXY/HTTPS_PROXY/NO_PROXY/NODE_USE_ENV_PROXY in the main-process environment contract; children inherit it",
          system: "per-second Get-NetTCPConnection sample of the packaged process tree (recursive child PIDs)",
        },
        controls: {
          chromiumSelfTest: [DESKTOP_SELFTEST_HTTP_HOST, DESKTOP_SELFTEST_TLS_HOST],
          environmentProxyControl: false,
          environmentProxyControlHost: DESKTOP_NODE_CONTROL_HOST,
          systemLayerControl: systemLayerControlObserved,
          systemLayerControlRemote: SYSTEM_LAYER_CONTROL_REMOTE,
        },
        windowSeconds: 60,
        browser: {} as Record<string, unknown>,
        environmentProxyViolations: [] as unknown[],
        systemLayer: {} as Record<string, unknown>,
        pidTree: [] as ProcessRow[],
        targetHostsSeen: [] as string[],
        daemonChildObserved: false,
        localWebsockets: [] as string[],
      },
      screenshots: [] as string[],
      scenarios: scenarioRecords,
      boundaries: [
        "The TCP sample is a sampling backstop, not exhaustive process-egress proof.",
        "Cross-machine login/realtime is out of scope for this batch (needs a second authorized machine).",
        "NSIS install, OS-level installed icon association and the tray remain unverified for this batch.",
        "The packaged window title bar shows the localized document title after login (for example Issues); the product name is carried by the exe version resources, app.getName(), the login window title and the in-app brand mark.",
      ],
    };

    try {
      desktopApp = await electron.launch({
        executablePath: DESKTOP_EXE,
        cwd: isolatedHome,
        args: ["--proxy-server=" + DESKTOP_PROXY_URL, "--proxy-bypass-list=localhost;127.0.0.1;::1"],
        env: desktopEnv,
        timeout: 90_000,
      });
      const app = desktopApp;
      desktopPid = app.process().pid ?? 0;
      requireDesktopHarness(desktopPid > 0, "packaged app did not expose a process id");

      const page = await app.firstWindow({ timeout: 60_000 });
      await app.evaluate(({ BrowserWindow }) => {
        const window = BrowserWindow.getAllWindows()[0];
        if (window) {
          window.maximize();
          window.show();
          window.focus();
        }
      });

      const noteRequest = (request: { url(): string }) => {
        observation.requests += 1;
        const raw = request.url();
        if (raw.startsWith(runtimeConfig.apiUrl ?? "")) isolatedApiRequests += 1;
        const sanitized = sanitizeUrl(raw);
        if (isDesktopControlUrl(raw)) return;
        if (!isLocalUrl(raw)) observation.externalRequests.push(sanitized);
      };
      const context = page.context();
      context.on("request", noteRequest);
      context.on("page", (newPage) => {
        newPage.on("websocket", (socket) => {
          const sanitized = sanitizeUrl(socket.url());
          observation.websockets.push(sanitized);
          if (!isLocalUrl(socket.url())) observation.externalWebsockets.push(sanitized);
        });
        newPage.on("framenavigated", (frame) => {
          if (frame !== newPage.mainFrame()) return;
          const sanitized = sanitizeUrl(frame.url());
          observation.navigations.push(sanitized);
          if (!isLocalUrl(frame.url())) observation.externalNavigations.push(sanitized);
        });
        newPage.on("requestfailed", (request) => {
          const sanitized = sanitizeUrl(request.url());
          if (isDesktopControlUrl(request.url())) return;
          if (!isLocalUrl(request.url())) observation.failedRequests.push(sanitized);
        });
      });
      page.on("websocket", (socket) => {
        const sanitized = sanitizeUrl(socket.url());
        observation.websockets.push(sanitized);
        if (!isLocalUrl(socket.url())) observation.externalWebsockets.push(sanitized);
      });
      page.on("framenavigated", (frame) => {
        if (frame !== page.mainFrame()) return;
        const sanitized = sanitizeUrl(frame.url());
        observation.navigations.push(sanitized);
        if (!isLocalUrl(frame.url())) observation.externalNavigations.push(sanitized);
      });
      page.on("requestfailed", (request) => {
        const sanitized = sanitizeUrl(request.url());
        if (isDesktopControlUrl(request.url())) return;
        if (!isLocalUrl(request.url())) observation.failedRequests.push(sanitized);
      });

      // Identity and configuration of the packaged runtime. The window title is
      // the branded login document title at this point; after login the tab
      // system owns document.title (localized route name, no product suffix).
      // The initial document may already be localized to the OS language; the
      // login surface is identified by its email field, not by copy.
      await expect(page.locator('input[type="email"]').first()).toBeVisible({ timeout: 60_000 });
      const appName = await app.evaluate(({ app: electronApp }) => electronApp.getName());
      expect(appName, "packaged app.getName()").toBe(PRODUCT_NAME);
      loginWindowTitle = await page.title();
      expect(loginWindowTitle, "packaged login window title").toContain(PRODUCT_NAME);
      const reportedUserData = await app.evaluate(({ app: electronApp }) => electronApp.getPath("userData"));
      expect(reportedUserData.toLowerCase(), "packaged userData path").toBe(userDataPath.toLowerCase());
      const mainProcessEnv = await app.evaluate(() => ({
        USERPROFILE: process.env.USERPROFILE,
        HOME: process.env.HOME,
        HTTP_PROXY: process.env.HTTP_PROXY,
        HTTPS_PROXY: process.env.HTTPS_PROXY,
        NO_PROXY: process.env.NO_PROXY,
        NODE_USE_ENV_PROXY: process.env.NODE_USE_ENV_PROXY,
      }));
      expect(mainProcessEnv.USERPROFILE, "main process USERPROFILE").toBe(isolatedHome);
      expect(mainProcessEnv.HTTP_PROXY, "main process HTTP_PROXY").toBe(DESKTOP_PROXY_URL);
      expect(mainProcessEnv.NO_PROXY, "main process NO_PROXY").toBe("localhost,127.0.0.1,::1");
      expect(mainProcessEnv.NODE_USE_ENV_PROXY, "main process NODE_USE_ENV_PROXY").toBe("1");

      const loginPageText = await page.locator("body").innerText();
      expect(loginPageText, "isolated desktop.json must load, not the configuration error page").not.toMatch(
        /(Missing|Invalid) .*desktop\.json/,
      );
      expect(existsSync(userDataPath), "isolated userData directory must be created by the packaged run").toBe(true);
      appDataExistedDuringRun = true;
      expect(existsSync(realAppDataPath), "the real roaming userData directory must never be created").toBe(false);
      (evidence.isolation as Record<string, unknown>).realAppDataEverObserved = false;
      (evidence.isolation as Record<string, unknown>).userDataExistedDuringRun = true;

      // Deterministic English + light UI for the acceptance screenshots.
      await page.evaluate(() => {
        localStorage.setItem("multica-locale", "en");
        localStorage.setItem("theme", "light");
      });
      await page.reload({ waitUntil: "domcontentloaded" });
      await expect(page.getByText("Sign in to U Workers")).toBeVisible({ timeout: 60_000 });
      await expect(page.locator("html")).toHaveClass(/light/);
      loginWindowTitle = await page.title();

      // Chromium-layer liveness: two exact self-test destinations must be
      // recorded by the recorder and fail in the packaged renderer.
      const selftest = await page.evaluate(async () => {
        const results: string[] = [];
        try {
          const response = await fetch("http://uw80-desktop-selftest.invalid/");
          // The desktop renderer runs with webSecurity disabled, so a recorder
          // 502 refusal resolves instead of rejecting. The recorder body is the
          // proof that the destination went through the refusal path.
          const body = await response.text();
          results.push("http:status:" + response.status);
          results.push("http:recorder:" + String(body.includes("isolated acceptance recorder")));
        } catch {
          results.push("http:failed");
        }
        try {
          await fetch("https://uw80-desktop-selftest-tls.invalid/");
          results.push("tls:unexpected-success");
        } catch {
          results.push("tls:failed");
        }
        return results;
      });
      expect(selftest, "packaged renderer self-test must be refused by the recorder").toEqual([
        "http:status:502",
        "http:recorder:true",
        "tls:failed",
      ]);
      await expect
        .poll(
          () => {
            const hosts = new Set(readProxyLog().map((entry) => entry.host));
            return [hosts.has(DESKTOP_SELFTEST_HTTP_HOST), hosts.has(DESKTOP_SELFTEST_TLS_HOST)].filter(Boolean)
              .length;
          },
          { timeout: 20000 },
        )
        .toBe(2);

      // Environment-proxy layer liveness: a Node process started with the
      // same proxy environment contract must be recorded and refused.
      execFileSync(
        process.execPath,
        [
          "-e",
          "fetch('http://uw80-desktop-nodecontrol.invalid/').then((r)=>process.exit(r.status===502?0:4),()=>process.exit(0));",
        ],
        { env: desktopEnv, encoding: "utf8", timeout: 30000, windowsHide: true },
      );
      await expect
        .poll(
          () =>
            readProxyLog().some((entry) => String(entry.host ?? "").startsWith(DESKTOP_NODE_CONTROL_HOST)),
          { timeout: 20000 },
        )
        .toBe(true);
      (evidence.observation as Record<string, unknown>).controls = {
        chromiumSelfTest: [DESKTOP_SELFTEST_HTTP_HOST, DESKTOP_SELFTEST_TLS_HOST],
        chromiumSelfTestFailedInApp: selftest,
        environmentProxyControl: true,
        environmentProxyControlHost: DESKTOP_NODE_CONTROL_HOST,
        systemLayerControl: systemLayerControlObserved,
        systemLayerControlRemote: SYSTEM_LAYER_CONTROL_REMOTE,
      };

      // Drop the deliberate control traffic from the business observation.
      observation.externalRequests = [];
      observation.externalWebsockets = [];
      observation.externalNavigations = [];
      observation.failedRequests = [];
      observation.requests = 0;

      await captureFullWindow(app, DESKTOP_ROOT, evidencePath("login.png"));
      scenarioRecords.push({ scenario: "isolated-login-page", proxyViolations: 0, externalRequests: 0 });

      // Real login through the packaged UI (isolated deployment, fixed dev code).
      const emailField = page.locator('input[type="email"]').first();
      await expect(emailField).toBeVisible({ timeout: 30000 });
      await emailField.fill(DESKTOP_EMAIL);
      await emailField.press("Enter");
      const otp = page.locator('input[data-input-otp="true"]');
      await expect(otp).toBeVisible({ timeout: 30000 });
      await otp.click();
      await otp.pressSequentially("888888", { delay: 30 });
      await expect(page.getByRole("heading", { name: "Issues" }).first()).toBeVisible({ timeout: 90_000 });
      await expect(page.getByText(PRODUCT_NAME).first()).toBeVisible();
      await expect.poll(() => observation.websockets.length, { timeout: 30000 }).toBeGreaterThan(0);
      expect(isolatedApiRequests, "the packaged app must talk to the isolated API").toBeGreaterThan(0);
      mainWindowTitle = await page.title();
      scenarioRecords.push({ scenario: "isolated-login", proxyViolations: 0, externalRequests: 0 });

      // The desktop auto-starts the bundled CLI daemon after login. It is
      // detached, so resolve its exact PIDs by the isolated profile it uses and
      // keep them inside the TCP observation scope and the cleanup list.
      let daemonPids: number[] = [];
      const daemonDeadline = Date.now() + 30000;
      while (daemonPids.length === 0 && Date.now() < daemonDeadline) {
        daemonPids = readDesktopDaemonPids(Number(deployment.ApiPort));
        if (daemonPids.length === 0) await new Promise((resolveWait) => setTimeout(resolveWait, 500));
      }
      expect(daemonPids.length, "the packaged desktop did not auto-start the bundled CLI daemon").toBeGreaterThan(
        0,
      );
      const daemonProfileDir = join(
        isolatedHome,
        ".multica",
        "profiles",
        "u-workers-desktop-127.0.0.1-" + String(deployment.ApiPort),
      );
      const daemonProfileInitialized = existsSync(join(daemonProfileDir, "config.json"));

      // Browse the real workspace surfaces. Desktop navigation is in-app, so
      // the file:// URL does not change; assert the surface heading instead.
      await page.locator('a[href*="/inbox"]').first().click();
      await expect(page.getByRole("heading", { name: "Inbox" }).first()).toBeVisible({ timeout: 30000 });
      await page.locator('a[href*="/agents"]').first().click();
      await expect(page.getByRole("heading", { name: "Agents" }).first()).toBeVisible({ timeout: 30000 });
      await page.locator('a[href*="/issues"]').first().click();
      await expect(page.getByRole("heading", { name: "Issues" }).first()).toBeVisible({ timeout: 30000 });
      await expect(page.getByRole("button", { name: "New Issue" }).first()).toBeVisible({ timeout: 30000 });
      scenarioRecords.push({ scenario: "browse-issues-inbox-agents", proxyViolations: 0, externalRequests: 0 });

      // Light main UI screenshot (title bar + taskbar icon are in the frame).
      await captureFullWindow(app, DESKTOP_ROOT, evidencePath("main.png"));

      const pidTree = processTree(readProcessTable(), desktopPid);
      (evidence.observation as Record<string, unknown>).pidTree = pidTree;
      (evidence.observation as Record<string, unknown>).daemonChildObserved = pidTree.some(
        (row) => row.pid !== desktopPid && /multica/i.test(row.name),
      );
      (evidence.observation as Record<string, unknown>).daemon = {
        observed: daemonPids.length > 0,
        pids: daemonPids,
        profileDir: daemonProfileDir,
        profileInitialized: daemonProfileInitialized,
        boundary: "The CLI daemonizes, so it is not part of the app's live parent chain; its detached PIDs from the isolated profile are sampled as additional TCP roots.",
      };
      (evidence.observation as Record<string, unknown>).localWebsockets = Array.from(
        new Set(observation.websockets),
      ).slice(0, 10);

      // Observation window: 60 s idle with the packaged tree sampled each second.
      proxyOffset = readProxyLog().length;
      browserCountsBefore = {
        externalRequests: observation.externalRequests.length,
        externalWebsockets: observation.externalWebsockets.length,
        externalNavigations: observation.externalNavigations.length,
      };
      rmSync(windowSamplerPath, { force: true });
      rmSync(windowSamplerPath + ".stop", { force: true });
      windowSampler = startTcpSampler(DESKTOP_ROOT, desktopPid, windowSamplerPath, 1000, daemonPids);
      const windowStartedAt = new Date();
      await page.waitForTimeout(60_000);
      await stopTcpSampler(windowSampler, windowSamplerPath);
      windowSampler = undefined;
      const windowEndedAt = new Date();
      (evidence.observation as Record<string, unknown>).windowStartedAt = windowStartedAt.toISOString();
      (evidence.observation as Record<string, unknown>).windowEndedAt = windowEndedAt.toISOString();

      const windowProxyEntries = proxyTrafficEntries(readProxyLog().slice(proxyOffset));
      const windowSamples = readSamples(windowSamplerPath).filter((sample) => {
        if (!sample.ts) return false;
        const ts = Date.parse(sample.ts);
        return ts >= windowStartedAt.getTime() && ts <= windowEndedAt.getTime();
      });
      const windowSampleErrors = windowSamples.filter((sample) => sample.error);
      const windowNonLoopback = windowSamples.flatMap((sample) =>
        (sample.connections ?? [])
          .map((connection) => ({ ...connection, remote: connection.remote ?? "unknown" }))
          .filter((connection) => !isLoopbackRemote(connection.remote)),
      );
      const windowBrowser = {
        externalRequests: observation.externalRequests.length - browserCountsBefore.externalRequests,
        externalWebsockets: observation.externalWebsockets.length - browserCountsBefore.externalWebsockets,
        externalNavigations: observation.externalNavigations.length - browserCountsBefore.externalNavigations,
      };
      (evidence.observation as Record<string, unknown>).browser = {
        ...windowBrowser,
        requestsSinceControls: observation.requests,
        isolatedApiRequests,
        localWebsockets: Array.from(new Set(observation.websockets)).slice(0, 10),
        externalRequests: observation.externalRequests,
        externalWebsockets: observation.externalWebsockets,
        externalNavigations: observation.externalNavigations,
        failedRequests: observation.failedRequests,
      };
      (evidence.observation as Record<string, unknown>).environmentProxyViolations = windowProxyEntries.map(
        (entry) => ({ kind: entry.kind ?? null, host: entry.host ?? null, method: entry.method ?? null }),
      );
      (evidence.observation as Record<string, unknown>).systemLayer = {
        samples: windowSamples.length,
        sampleErrors: windowSampleErrors.length,
        nonLoopback: windowNonLoopback,
        observedProcesses: Array.from(new Set(windowSamples.flatMap((sample) => sample.pids ?? []))).length,
        daemonPidsSampled: daemonPids.length,
      };
      (evidence.observation as Record<string, unknown>).targetHostsSeen = Array.from(
        new Set(windowProxyEntries.map((entry) => String(entry.host ?? "unknown"))),
      );
      (evidence.app as Record<string, unknown>) = {
        name: appName,
        loginWindowTitle,
        mainWindowTitle,
        userDataPath: reportedUserData,
      };

      scenarioRecords.push({
        scenario: "email-code-login-browse-idle-60s",
        proxyViolations: windowProxyEntries.length,
        externalBrowserRequests: windowBrowser.externalRequests,
        externalBrowserWebsockets: windowBrowser.externalWebsockets,
        externalBrowserNavigations: windowBrowser.externalNavigations,
        tcpSamples: windowSamples.length,
        tcpExternalConnections: windowNonLoopback.length,
      });

      expect(windowSamples.length, "TCP observation window produced too few samples").toBeGreaterThanOrEqual(10);
      expect(windowSampleErrors, "TCP sampler reported errors during the observation window").toEqual([]);
      expect(windowProxyEntries, "recorder saw traffic during the observation window").toEqual([]);
      expect(windowBrowser.externalRequests, "packaged renderer made external HTTP requests").toBe(0);
      expect(windowBrowser.externalWebsockets, "packaged renderer opened external WebSockets").toBe(0);
      expect(windowBrowser.externalNavigations, "packaged renderer navigated to an external origin").toBe(0);
      expect(observation.failedRequests, "packaged renderer failed requests to non-loopback origins").toEqual([]);
      expect(windowNonLoopback, "packaged process tree held non-loopback TCP connections").toEqual([]);

      (evidence.screenshots as string[]).push(
        "evidence/clawsta-80/desktop/login.png",
        "evidence/clawsta-80/desktop/main.png",
      );
    } catch (error) {
      evidence.error = error instanceof Error ? error.message : String(error);
      throw error;
    } finally {
      if (windowSampler) await stopTcpSampler(windowSampler, windowSamplerPath);
      const treeBeforeClose = desktopPid > 0 ? processTree(readProcessTable(), desktopPid) : [];
      if (desktopApp) await desktopApp.close().catch(() => undefined);
      const remaining = new Map(readProcessTable().map((row) => [row.pid, row]));
      for (const row of treeBeforeClose) {
        if (row.pid === desktopPid) continue;
        if (!remaining.has(row.pid)) continue;
        try {
          execFileSync(
            "powershell.exe",
            ["-NoProfile", "-Command", "Stop-Process -Id " + row.pid + " -Force -ErrorAction SilentlyContinue"],
            { timeout: 15000, windowsHide: true },
          );
        } catch {
          /* best-effort cleanup of exactly the PIDs this run started */
        }
      }
      const remainingDaemons = readDesktopDaemonPids(Number(deployment.ApiPort));
      for (const pid of remainingDaemons) stopProcessById(pid);
      const daemonStopDeadline = Date.now() + 15000;
      let daemonPidsRemaining = readDesktopDaemonPids(Number(deployment.ApiPort)).length;
      while (daemonPidsRemaining > 0 && Date.now() < daemonStopDeadline) {
        await new Promise((resolveWait) => setTimeout(resolveWait, 500));
        daemonPidsRemaining = readDesktopDaemonPids(Number(deployment.ApiPort)).length;
      }
      (evidence.observation as Record<string, unknown>).daemonCleanup = {
        stoppedPids: remainingDaemons,
        remainingPids: daemonPidsRemaining,
      };
      const removeDirWithRetry = (path: string) => {
        for (let attempt = 0; attempt < 8; attempt += 1) {
          try {
            rmSync(path, { recursive: true, force: true });
          } catch {
            /* a closing child may still hold a handle; retry briefly */
          }
          if (!existsSync(path)) return true;
          try {
            execFileSync("powershell.exe", ["-NoProfile", "-Command", "Start-Sleep -Milliseconds 400"], {
              timeout: 10000,
              windowsHide: true,
            });
          } catch {
            /* ignore */
          }
        }
        return !existsSync(path);
      };
      userDataRemoved = removeDirWithRetry(userDataPath);
      realProfilesAfter = existsSync(realMulticaProfiles) ? readdirSync(realMulticaProfiles).sort() : [];
      isolatedMulticaCreated = !isolatedMulticaExistedBefore && existsSync(isolatedMulticaDir);
      const newRealDesktopProfiles = realProfilesAfter.filter(
        (name) => !realProfilesBefore.includes(name) && name.startsWith("u-workers-desktop-"),
      );
      const realAppDataAfter = existsSync(realAppDataPath);
      (evidence.isolation as Record<string, unknown>).appDataExistedDuringRun = appDataExistedDuringRun;
      (evidence.isolation as Record<string, unknown>).userDataRemovedAfterRun = userDataRemoved;
      (evidence.isolation as Record<string, unknown>).isolatedMulticaDirCreated = isolatedMulticaCreated;
      (evidence.isolation as Record<string, unknown>).newRealDesktopProfiles = newRealDesktopProfiles;
      (evidence.isolation as Record<string, unknown>).realAppDataExistedAfterRun = realAppDataAfter;
      writeFileSync(evidencePath("run-summary.json"), JSON.stringify(evidence, null, 2) + "\n", "utf8");
      rmSync(join(DESKTOP_ROOT, "state", "d1-helpers"), { recursive: true, force: true });
      expect(realAppDataAfter, "the real roaming userData directory was created during the run").toBe(false);
      expect(newRealDesktopProfiles, "a real ~/.multica desktop profile was created despite the isolated HOME").toEqual(
        [],
      );
      expect(userDataRemoved, "isolated userData directory was not removed after the run").toBe(true);
    }
  });
});
