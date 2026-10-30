import { test, expect, _electron as electron } from "@playwright/test";
import { execFileSync, spawn, type ChildProcess } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, readdirSync, rmSync, writeFileSync } from "node:fs";
import { homedir } from "node:os";
import { join, resolve } from "node:path";
import { Socket } from "node:net";

import {
  assertInsideRoot,
  classifyRecorderEntries,
  guardedStopScript,
  makeControlWindows,
  parseRecorderLog,
  parseSamplerLog,
  planProcessCleanup,
  summarizePhase,
} from "../scripts/e2e/desktop-observer.mjs";
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
 * The packaged desktop case (batch D2) runs one continuous observation pass
 * from before the app process starts, through startup, engine controls, login,
 * browsing and a 60 s idle window. Recorder entries and TCP samples are
 * collected continuously and split into explicit phases; a phase only passes
 * when its own window has samples, no sampler errors, no unexpected
 * non-loopback connection, no recorder traffic outside exact control windows
 * and no browser-layer egress. Process cleanup is verified against a pre-run
 * snapshot, the owned executable path, creation time and the workspace daemon
 * PID reported by the product CLI.
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
const DESKTOP_SKIP_SCREENSHOTS = process.env.E2E_DESKTOP_SKIP_SCREENSHOTS === "1";
const DESKTOP_RUN_ID = process.env.E2E_RUN_ID ?? "d2-" + Date.now().toString(36);
const DESKTOP_EMAIL = "e2e-desktop-" + DESKTOP_RUN_ID + "@multica.ai";
const DESKTOP_WORKSPACE_NAME = "E2E Desktop Workspace";
const DESKTOP_WORKSPACE_SLUG = "e2e-desktop-" + DESKTOP_RUN_ID;
const DESKTOP_SAMPLER_INTERVAL_MS = 500;

// Exact control destinations; nothing here is a wildcard. The recorder sees
// HTTP destinations without a port and CONNECT targets with :80/:443, so each
// exact host lists the exact variants it can produce.
const DESKTOP_SELFTEST_HTTP_HOST = "uw80-desktop-selftest.invalid";
const DESKTOP_SELFTEST_TLS_HOST = "uw80-desktop-selftest-tls.invalid:443";
const DESKTOP_NODE_CONTROL_HOST = "uw80-desktop-nodecontrol.invalid";
const DESKTOP_GO_CONTROL_HOST = "uw80-desktop-go-control.invalid";
const DESKTOP_CONTROL_HOSTNAMES = new Set([
  DESKTOP_SELFTEST_HTTP_HOST,
  "uw80-desktop-selftest-tls.invalid",
]);
const SYSTEM_LAYER_CONTROL_REMOTE = "192.0.2.1:80"; // TEST-NET-1, not routable
const DAEMON_PROFILE_NEEDLE_PREFIX = "u-workers-desktop-127.0.0.1-";

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

type ProcessInfo = {
  pid: number;
  ppid: number;
  name: string;
  executablePath: string;
  commandLine: string;
  creationDate: string | null;
};

type WorkspaceDaemonStatus = {
  pid: number | null;
  source: string;
  port: number | null;
  cliPath: string | null;
};

type BrowserLayer = {
  externalRequests: number;
  externalWebsockets: number;
  externalNavigations: number;
  failedRequests: number;
};

type BrowserObservation = BrowserLayer & {
  requests: number;
  externalRequestUrls: string[];
  externalWebsocketUrls: string[];
  externalNavigationUrls: string[];
  failedRequestUrls: string[];
  localWebsockets: string[];
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

function sanitizeUrl(raw: string): string {
  try {
    const url = new URL(raw);
    if (!["http:", "https:", "ws:", "wss:"].includes(url.protocol)) return url.protocol;
    return url.protocol + "//" + url.hostname;
  } catch {
    return "invalid-url";
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

function isDesktopControlUrl(raw: string): boolean {
  try {
    return DESKTOP_CONTROL_HOSTNAMES.has(new URL(raw).hostname.toLowerCase());
  } catch {
    return false;
  }
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

function writeHelperScript(root: string, name: string, content: string): string {
  const dir = join(root, "state", "d2-helpers");
  mkdirSync(dir, { recursive: true });
  const path = join(dir, name);
  writeFileSync(path, content + "\n", "utf8");
  return path;
}

function writeEvidenceFile(name: string, content: string) {
  mkdirSync(DESKTOP_EVIDENCE_DIR, { recursive: true });
  writeFileSync(join(DESKTOP_EVIDENCE_DIR, name), content, "utf8");
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

function readRecorderRaw(): string {
  requireDesktopHarness(
    Boolean(DESKTOP_PROXY_LOG) && existsSync(DESKTOP_PROXY_LOG),
    "recording proxy log is missing: " + (DESKTOP_PROXY_LOG || "(E2E_PROXY_LOG is not set)"),
  );
  return readFileSync(DESKTOP_PROXY_LOG, "utf8");
}

function readRecorderEntries() {
  return parseRecorderLog(readRecorderRaw());
}

function readSamplerRaw(path: string): string {
  if (!existsSync(path)) return "";
  return readFileSync(path, "utf8");
}

function readSamplerSamples(path: string) {
  return parseSamplerLog(readSamplerRaw(path));
}

async function waitForRecorderHost(host: string, timeoutMs: number) {
  await expect
    .poll(
      () =>
        readRecorderEntries().some(
          (entry) => (entry.kind === "http" || entry.kind === "connect" || entry.kind === "upgrade") && entry.host === host,
        ),
      { timeout: timeoutMs },
    )
    .toBe(true);
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

function readProcessSnapshot(): ProcessInfo[] {
  const script = [
    "$ErrorActionPreference = 'Stop'",
    "Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate | ForEach-Object {",
    "  $created = $_.CreationDate",
    "  [pscustomobject]@{",
    "    pid = [int]$_.ProcessId",
    "    ppid = [int]$_.ParentProcessId",
    "    name = [string]$_.Name",
    "    executablePath = ([string]$_.ExecutablePath)",
    "    commandLine = ([string]$_.CommandLine)",
    "    creationDate = if ($created) { $created.ToUniversalTime().ToString('o') } else { '' }",
    "  }",
    "} | ConvertTo-Json -Compress -Depth 3",
  ].join("\n");
  const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], {
    encoding: "utf8",
    timeout: 60000,
    windowsHide: true,
  });
  const trimmed = output.replace(/^\uFEFF/, "").trim();
  if (!trimmed) return [];
  const parsed = JSON.parse(trimmed) as ProcessInfo | ProcessInfo[];
  const list = Array.isArray(parsed) ? parsed : [parsed];
  return list.map((row) => ({
    pid: Number(row.pid),
    ppid: Number(row.ppid),
    name: String(row.name ?? ""),
    executablePath: String(row.executablePath ?? ""),
    commandLine: String(row.commandLine ?? ""),
    creationDate: row.creationDate ? String(row.creationDate) : null,
  }));
}

function readListeningPort(pid: number): number | null {
  const script =
    "$connections = @(Get-NetTCPConnection -State Listen -ErrorAction SilentlyContinue | " +
    "Where-Object { $_.OwningProcess -eq " + pid + " }); if ($connections.Count -gt 0) { [int]$connections[0].LocalPort }";
  try {
    const output = execFileSync("powershell.exe", ["-NoProfile", "-NonInteractive", "-Command", script], {
      encoding: "utf8",
      timeout: 30000,
      windowsHide: true,
    });
    const parsed = Number(output.replace(/^\uFEFF/, "").trim());
    return Number.isInteger(parsed) && parsed > 0 ? parsed : null;
  } catch {
    return null;
  }
}

function readWorkspaceDaemonStatus(): WorkspaceDaemonStatus {
  let snapshot: ProcessInfo[] = [];
  try {
    snapshot = readProcessSnapshot();
  } catch {
    return { pid: null, source: "process snapshot failed", port: null, cliPath: null };
  }
  const workspaceDaemon = snapshot.find(
    (row) =>
      row.name === "multica.exe" &&
      row.commandLine.includes("daemon start") &&
      !row.commandLine.includes(DAEMON_PROFILE_NEEDLE_PREFIX),
  );
  const resolvedPid = workspaceDaemon?.pid ?? null;
  const port = resolvedPid ? readListeningPort(resolvedPid) : null;
  let cliPath: string | null = null;
  try {
    const whereOutput = execFileSync("where.exe", ["multica"], { encoding: "utf8", timeout: 15000, windowsHide: true });
    const first = whereOutput.split(/\r?\n/).map((line) => line.trim()).find(Boolean);
    if (first) cliPath = first;
  } catch {
    cliPath = null;
  }
  if (cliPath && port) {
    try {
      const statusOutput = execFileSync(cliPath, ["daemon", "status", "--output", "json"], {
        encoding: "utf8",
        timeout: 30000,
        windowsHide: true,
        env: { ...process.env, MULTICA_DAEMON_PORT: String(port) },
      });
      const status = JSON.parse(statusOutput) as { pid?: unknown };
      const cliPid = Number(status.pid);
      if (Number.isInteger(cliPid) && cliPid > 0) {
        return { pid: cliPid, source: "multica daemon status --output json", port, cliPath };
      }
    } catch {
      /* fall through to the process-scan answer below */
    }
  }
  return {
    pid: resolvedPid,
    source: resolvedPid ? "process scan fallback (daemon status CLI unavailable)" : "not found",
    port,
    cliPath,
  };
}

function samePath(left: string, right: string): boolean {
  const normalize = (value: string) => String(value ?? "").replace(/\//g, "\\").toLowerCase();
  return normalize(left) === normalize(right);
}

function stopOwnedProcess(candidate: ProcessInfo, protectedPids: number[]) {
  execFileSync("powershell.exe", ["-NoProfile", "-Command", guardedStopScript(candidate, protectedPids)], {
    timeout: 15000,
    windowsHide: true,
  });
}

function removeDirVerified(targetPath: string, rootPath: string, label: string) {
  assertInsideRoot(resolve(targetPath), resolve(rootPath), label);
  for (let attempt = 0; attempt < 8; attempt += 1) {
    try {
      rmSync(targetPath, { recursive: true, force: true });
    } catch {
      /* a closing child may still hold a handle; retry briefly */
    }
    if (!existsSync(targetPath)) return true;
    execFileSync("powershell.exe", ["-NoProfile", "-Command", "Start-Sleep -Milliseconds 400"], {
      timeout: 10000,
      windowsHide: true,
    });
  }
  throw new Error(label + " could not be removed after retries: " + targetPath);
}

const TCP_SAMPLER_SCRIPT = String.raw`param(
    [string]$OutPath,
    [int]$IntervalMs = 500,
    [string]$AppExePath,
    [string]$DaemonExePath,
    [string]$DaemonProfileNeedle,
    [string]$RunStartedAt,
    [string]$ExtraPids = ""
)
$ErrorActionPreference = 'Continue'
$stopFile = $OutPath + '.stop'
$utf8 = [Text.UTF8Encoding]::new($false)
$runStarted = [DateTime]::Parse($RunStartedAt, [Globalization.CultureInfo]::InvariantCulture, [Globalization.DateTimeStyles]::RoundtripKind).ToUniversalTime()
function Write-Sample {
    param($Record)
    try {
        [IO.File]::AppendAllText($OutPath, ($Record | ConvertTo-Json -Compress -Depth 6) + [Environment]::NewLine, $utf8)
    } catch { }
}
while (-not (Test-Path -LiteralPath $stopFile)) {
    $startedAt = Get-Date
    try {
        $procs = @(Get-CimInstance Win32_Process -Property ProcessId,ParentProcessId,Name,ExecutablePath,CommandLine,CreationDate -ErrorAction Stop)
        $appPids = New-Object System.Collections.Generic.List[int]
        $daemonPids = New-Object System.Collections.Generic.List[int]
        foreach ($p in $procs) {
            $exe = [string]$p.ExecutablePath
            if ($exe -and $exe.Equals($AppExePath, [System.StringComparison]::OrdinalIgnoreCase)) {
                $candidate = [int]$p.ProcessId
                if (-not $appPids.Contains($candidate)) { $appPids.Add($candidate) }
            }
            if ($p.Name -eq 'multica.exe' -and $exe -and $exe.Equals($DaemonExePath, [System.StringComparison]::OrdinalIgnoreCase)) {
                $created = $p.CreationDate
                $startedRecently = $created -and ($created.ToUniversalTime() -ge $runStarted.AddSeconds(-2))
                $profileMatches = $p.CommandLine -and $p.CommandLine.Contains($DaemonProfileNeedle)
                if ($startedRecently -and $profileMatches) {
                    $candidate = [int]$p.ProcessId
                    if (-not $daemonPids.Contains($candidate)) { $daemonPids.Add($candidate) }
                }
            }
        }
        $changed = $true
        while ($changed) {
            $changed = $false
            foreach ($p in $procs) {
                $childPid = [int]$p.ProcessId
                if ($appPids.Contains($childPid)) { continue }
                if ($appPids.Contains([int]$p.ParentProcessId)) {
                    $appPids.Add($childPid)
                    $changed = $true
                }
            }
        }
        $extraPidList = New-Object System.Collections.Generic.List[int]
        foreach ($extra in ($ExtraPids -split ',')) {
            $parsed = 0
            if ([int]::TryParse($extra.Trim(), [ref]$parsed) -and $parsed -gt 0) { $extraPidList.Add($parsed) }
        }
        $owned = New-Object 'System.Collections.Generic.HashSet[int]'
        foreach ($pidValue in $appPids) { [void]$owned.Add($pidValue) }
        foreach ($pidValue in $daemonPids) { [void]$owned.Add($pidValue) }
        foreach ($pidValue in $extraPidList) { [void]$owned.Add($pidValue) }
        $nonLoopback = @()
        $tcp = @(Get-NetTCPConnection -ErrorAction Stop)
        foreach ($connection in $tcp) {
            $owner = [int]$connection.OwningProcess
            if (-not $owned.Contains($owner)) { continue }
            $remoteAddress = [string]$connection.RemoteAddress
            if ($remoteAddress -eq '127.0.0.1' -or $remoteAddress -eq '::1' -or $remoteAddress -eq '0.0.0.0' -or $remoteAddress -eq '::' -or $remoteAddress.StartsWith('127.')) { continue }
            $nonLoopback += [pscustomobject]@{
                remote = ('{0}:{1}' -f $remoteAddress, $connection.RemotePort)
                state = [string]$connection.State
                pid = $owner
            }
        }
        Write-Sample ([pscustomobject]@{
            ts = (Get-Date).ToUniversalTime().ToString('o')
            intervalMs = $IntervalMs
            pids = [pscustomobject]@{
                app = @($appPids)
                daemon = @($daemonPids)
                extra = @($extraPidList)
            }
            nonLoopback = @($nonLoopback)
        })
    } catch {
        Write-Sample ([pscustomobject]@{
            ts = (Get-Date).ToUniversalTime().ToString('o')
            intervalMs = $IntervalMs
            pids = [pscustomobject]@{ app = @(); daemon = @(); extra = @() }
            nonLoopback = @()
            error = [string]$_.Exception.Message
        })
    }
    $elapsed = ((Get-Date) - $startedAt).TotalMilliseconds
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
Write-Output ('captured {0}x{1}' -f $b.Width, $b.Height)`;

function startTcpSampler(options: {
  root: string;
  outPath: string;
  appExePath: string;
  daemonExePath: string;
  daemonProfileNeedle: string;
  runStartedAt: string;
  extraPids?: number[];
  intervalMs?: number;
}): ChildProcess {
  const scriptPath = writeHelperScript(options.root, "desktop-tcp-sampler.ps1", TCP_SAMPLER_SCRIPT);
  return spawn(
    "powershell.exe",
    [
      "-NoProfile",
      "-NonInteractive",
      "-ExecutionPolicy",
      "Bypass",
      "-File",
      scriptPath,
      "-OutPath",
      options.outPath,
      "-IntervalMs",
      String(options.intervalMs ?? DESKTOP_SAMPLER_INTERVAL_MS),
      "-AppExePath",
      options.appExePath,
      "-DaemonExePath",
      options.daemonExePath,
      "-DaemonProfileNeedle",
      options.daemonProfileNeedle,
      "-RunStartedAt",
      options.runStartedAt,
      "-ExtraPids",
      (options.extraPids ?? []).join(","),
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

const GO_PROXY_CONTROL_SOURCE = [
  "package main",
  "",
  "import (",
  '  "fmt"',
  '  "net/http"',
  '  "os"',
  ")",
  "",
  "func main() {",
  '  response, err := http.Get("http://uw80-desktop-go-control.invalid/")',
  "  if err != nil {",
  '    fmt.Println("refused")',
  "    os.Exit(0)",
  "  }",
  "  if response.StatusCode == 502 {",
  '    fmt.Println("refused-by-recorder")',
  "    os.Exit(0)",
  "  }",
  '  fmt.Println("unexpected status", response.StatusCode)',
  "  os.Exit(3)",
  "}",
].join("\n");

function sleep(ms: number) {
  return new Promise((resolveWait) => setTimeout(resolveWait, ms));
}

test.describe("packaged desktop identity and outbound observation", () => {
  const skipReason = desktopSkipReason();
  test.skip(Boolean(skipReason), skipReason ?? "");

  test("packaged app boots from isolated config, logs in, and stays loopback-only", async () => {
    test.setTimeout(900_000);

    requireDesktopHarness(Boolean(DESKTOP_PROXY_URL), "E2E_PROXY_URL is not set");
    requireDesktopHarness(Boolean(DESKTOP_PROXY_LOG), "E2E_PROXY_LOG is not set");
    requireDesktopHarness(Boolean(DESKTOP_EXE) && existsSync(DESKTOP_EXE), "E2E_DESKTOP_EXE is not usable");

    mkdirSync(DESKTOP_EVIDENCE_DIR, { recursive: true });
    const evidencePath = (name: string) => join(DESKTOP_EVIDENCE_DIR, name);

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

    const daemonExePath = resolve(DESKTOP_EXE, "..", "resources", "app.asar.unpacked", "resources", "bin", "multica.exe");
    requireDesktopHarness(
      existsSync(daemonExePath),
      "the packaged bundled CLI is missing, so daemon attribution cannot be verified: " + daemonExePath,
    );
    const daemonProfileNeedle = DAEMON_PROFILE_NEEDLE_PREFIX + String(deployment.ApiPort);

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
    // A real Windows profile always has AppData; Chromium resolves appData
    // through the redirected profile and fails when the layout is absent.
    mkdirSync(join(isolatedHome, "AppData", "Roaming"), { recursive: true });
    mkdirSync(join(isolatedHome, "AppData", "Local"), { recursive: true });
    expect(existsSync(userDataPath), "userData must still not exist before launch").toBe(false);

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

    let codeRevision: Record<string, unknown> = { head: "unknown" };
    try {
      const head = execFileSync("git", ["rev-parse", "HEAD"], { encoding: "utf8", timeout: 15000 }).trim();
      const dirty = execFileSync("git", ["status", "--porcelain"], { encoding: "utf8", timeout: 15000 }).trim().length > 0;
      codeRevision = { head, dirty };
    } catch {
      /* not a git checkout; evidence records unknown */
    }

    // --- Cleanup attribution inputs: taken before anything of this run starts.
    const workspaceDaemonStatus = readWorkspaceDaemonStatus();
    requireDesktopHarness(
      workspaceDaemonStatus.pid !== null && workspaceDaemonStatus.source === "multica daemon status --output json",
      "cannot identify the workspace daemon PID; refusing to run a cleanup that cannot protect it",
    );
    const protectedPids = [workspaceDaemonStatus.pid as number];
    const runStartedAt = new Date().toISOString();
    const preLaunchSnapshot = readProcessSnapshot();
    const preExistingAppPids = preLaunchSnapshot
      .filter((row) => samePath(row.executablePath, DESKTOP_EXE))
      .map((row) => row.pid);
    const preExistingDaemonPids = preLaunchSnapshot
      .filter(
        (row) =>
          row.name === "multica.exe" &&
          samePath(row.executablePath, daemonExePath) &&
          row.commandLine.includes(daemonProfileNeedle),
      )
      .map((row) => row.pid);
    expect(preExistingDaemonPids, "an isolated desktop daemon already existed before this run").toEqual([]);

    const api = new TestApiClient();
    await api.login(DESKTOP_EMAIL, "E2E Desktop User");
    await api.ensureWorkspace(DESKTOP_WORKSPACE_NAME, DESKTOP_WORKSPACE_SLUG);
    await api.markUserOnboarded();

    await expect.poll(() => probeRecorder(), { timeout: 15000 }).toBe("up");
    const recorderBaselineOffset = readRecorderEntries().length;

    const desktopEnv = buildDesktopEnv(isolatedHome);
    const samplerPath = join(DESKTOP_ROOT, "state", "tcp-samples-d2.jsonl");
    const ensurePhaseSamples = async (startIso: string, minimum: number, timeoutMs = 30000) => {
      const startMs = Date.parse(startIso);
      const deadline = Date.now() + timeoutMs;
      while (Date.now() < deadline) {
        const samples = readSamplerSamples(samplerPath);
        const count = samples.filter((sample) => Date.parse(sample.ts) >= startMs).length;
        if (count >= minimum) return count;
        await sleep(250);
      }
      return readSamplerSamples(samplerPath).filter((sample) => Date.parse(sample.ts) >= startMs).length;
    };
    rmSync(samplerPath, { force: true });
    rmSync(samplerPath + ".stop", { force: true });

    const browserObs: BrowserObservation = {
      requests: 0,
      externalRequests: 0,
      externalWebsockets: 0,
      externalNavigations: 0,
      failedRequests: 0,
      externalRequestUrls: [],
      externalWebsocketUrls: [],
      externalNavigationUrls: [],
      failedRequestUrls: [],
      localWebsockets: [],
    };
    const browserSnapshot = (): BrowserLayer => ({
      externalRequests: browserObs.externalRequests,
      externalWebsockets: browserObs.externalWebsockets,
      externalNavigations: browserObs.externalNavigations,
      failedRequests: browserObs.failedRequests,
    });
    const browserDeltaFrom = (from: BrowserLayer, to: BrowserLayer = browserSnapshot()): BrowserLayer => ({
      externalRequests: to.externalRequests - from.externalRequests,
      externalWebsockets: to.externalWebsockets - from.externalWebsockets,
      externalNavigations: to.externalNavigations - from.externalNavigations,
      failedRequests: to.failedRequests - from.failedRequests,
    });
    let snapshotStartup: BrowserLayer = browserSnapshot();
    let snapshotControlB: BrowserLayer = browserSnapshot();
    let snapshotLogin: BrowserLayer = browserSnapshot();
    let snapshotBrowse: BrowserLayer = browserSnapshot();
    let isolatedApiRequests = 0;
    let browserAttachedAt = "";
    let browserAttachRequests = 0;
    let browserAttachFailedRequests = 0;
    let chromiumControlActive = false;

    let desktopApp: Awaited<ReturnType<typeof electron.launch>> | undefined;
    let desktopPid = 0;
    let sampler: ChildProcess | undefined;
    let phases: Array<Record<string, unknown>> = [];
    let daemonPids: number[] = [];
    let cleanupEvidence: Record<string, unknown> = {};
    const controlWindowsRaw: Array<{ label: string; start: string; end: string; hosts: string[] }> = [];
    const evidence: Record<string, unknown> = {
      batch: "CLAWSTA-80 desktop egress observation D2",
      generatedAt: new Date().toISOString(),
      codeRevision,
      command: {
        spec: "pnpm exec playwright test e2e/u-workers-brand.spec.ts --project=chromium --grep 'packaged app boots' --reporter=line",
        observationMode: "continuous single pass: sampler and recorder baseline start before the app process; phases are slices of the same pass",
        desktopExe: DESKTOP_EXE,
        acceptanceRoot: DESKTOP_ROOT,
        recordingProxy: DESKTOP_PROXY_URL,
        launchArgs: ["--proxy-server=" + DESKTOP_PROXY_URL, "--proxy-bypass-list=localhost;127.0.0.1;::1"],
        screenshotsSkipped: DESKTOP_SKIP_SCREENSHOTS,
        samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
      },
      exeVersionInfo: versionInfo,
      isolation: {
        userDataPath,
        realAppDataPath,
        isolatedHome,
        isolatedDesktopConfig: desktopConfigPath,
        realAppDataExistedBeforeRun: false,
        appDataParentsPrepared: true,
        isolatedMulticaExistedBefore,
        realMulticaProfilesBeforeCount: realProfilesBefore.length,
      },
      app: {} as Record<string, unknown>,
      observation: {
        model:
          "one continuous pass: the TCP sampler starts before launch and the recorder baseline is taken before any control or app traffic; every phase is a time slice of the same pass",
        controls: {} as Record<string, unknown>,
        phases: [] as Array<Record<string, unknown>>,
        sampler: {} as Record<string, unknown>,
        recorder: {} as Record<string, unknown>,
        daemon: {} as Record<string, unknown>,
        boundaries: [
          "Chromium network is covered by the refusing recorder from app process start plus Playwright events from the first attached window; the recorder baseline is taken before launch, so startup traffic is measured, not assumed zero.",
          "The TCP layer is a sampling backstop (interval recorded, gaps reported), not a complete connection log; missing samples or sampler errors fail a phase instead of counting as zero.",
          "The environment-proxy contract is verified with the same runtimes (Node and Go) in minimal local controls; that proves the contract and those runtimes, not that every runtime inside the app obeys it. The actual Go daemon is covered by its detached PIDs in the TCP sampler.",
          "Only exact control destinations inside their own timestamp windows are excluded; there is no wildcard allowlist.",
        ],
      },
      cleanup: {} as Record<string, unknown>,
      screenshots: [] as string[],
      scenarios: [] as Array<Record<string, unknown>>,
    };

    try {
      // Observation starts before the app process exists.
      const sentinel = spawn(
        process.execPath,
        [
          "-e",
          "const net=require('net');const s=net.connect({host:'192.0.2.1',port:80});s.on('error',()=>{});setTimeout(()=>process.exit(0),30000);",
        ],
        { stdio: "ignore", windowsHide: true },
      );
      requireDesktopHarness(typeof sentinel.pid === "number", "system-layer control process did not start");
      const observationStartedAt = new Date().toISOString();
      sampler = startTcpSampler({
        root: DESKTOP_ROOT,
        outPath: samplerPath,
        appExePath: DESKTOP_EXE,
        daemonExePath,
        daemonProfileNeedle,
        runStartedAt,
        extraPids: [sentinel.pid as number],
        intervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
      });

      // --- Control phase A (before the app exists): Node env-proxy control,
      // Go-runtime env-proxy control, and the system-layer TEST-NET-1 probe.
      const controlAStart = new Date().toISOString();
      execFileSync(
        process.execPath,
        [
          "-e",
          "fetch('http://uw80-desktop-nodecontrol.invalid/').then((r)=>process.exit(r.status===502?0:4),()=>process.exit(0));",
        ],
        { env: desktopEnv, encoding: "utf8", timeout: 30000, windowsHide: true },
      );
      await waitForRecorderHost(DESKTOP_NODE_CONTROL_HOST + ":80", 30000);
      await waitForRecorderHost(DESKTOP_NODE_CONTROL_HOST, 30000).catch(() => undefined);

      const goControl: Record<string, unknown> = { attempted: false, recorded: false, skipped: false };
      try {
        const goDir = join(DESKTOP_ROOT, "state", "go-proxy-control");
        mkdirSync(goDir, { recursive: true });
        writeFileSync(join(goDir, "main.go"), GO_PROXY_CONTROL_SOURCE + "\n", "utf8");
        goControl.attempted = true;
        execFileSync("go", ["run", "main.go"], {
          cwd: goDir,
          env: desktopEnv,
          encoding: "utf8",
          timeout: 90000,
          windowsHide: true,
        });
        await waitForRecorderHost(DESKTOP_GO_CONTROL_HOST, 30000).catch(() => undefined);
        // Go's http proxy may use CONNECT (:80) or absolute-form; accept either exact form.
        const goRecorded = readRecorderEntries().some(
          (entry) =>
            (entry.kind === "http" || entry.kind === "connect" || entry.kind === "upgrade") &&
            (entry.host === DESKTOP_GO_CONTROL_HOST || entry.host === DESKTOP_GO_CONTROL_HOST + ":80"),
        );
        goControl.recorded = goRecorded;
        if (!goRecorded) throw new Error("Go runtime proxy control request was not recorded by the recorder");
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        if (goControl.attempted && message.includes("Command failed")) {
          // The probe refused as expected but the recorder entry was missing: a real observation failure.
          throw new Error("Go runtime proxy control failed: " + message);
        }
        goControl.skipped = true;
        goControl.reason = message.split("\n")[0];
      }
      requireDesktopHarness(goControl.recorded === true, "Go environment-proxy control was not recorded");

      await expect
        .poll(
          () =>
            readSamplerSamples(samplerPath).some((sample) =>
              sample.nonLoopback.some((connection) => connection.remote === SYSTEM_LAYER_CONTROL_REMOTE),
            ),
          { timeout: 45000 },
        )
        .toBe(true);
      await ensurePhaseSamples(controlAStart, 2);
      const controlAEnd = new Date().toISOString();
      sentinel.kill();
      controlWindowsRaw.push(
        {
          label: "node-env-proxy-control",
          start: controlAStart,
          end: controlAEnd,
          hosts: [DESKTOP_NODE_CONTROL_HOST, DESKTOP_NODE_CONTROL_HOST + ":80"],
        },
        {
          label: "go-runtime-proxy-control",
          start: controlAStart,
          end: controlAEnd,
          hosts: [DESKTOP_GO_CONTROL_HOST, DESKTOP_GO_CONTROL_HOST + ":80"],
        },
      );

      // --- Startup phase: from launching the packaged process until the
      // branded login page is visible. The recorder baseline predates launch.
      const launchStartedAt = new Date().toISOString();
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
      // The desktop layout needs a real window size before the sidebar
      // navigation can be clicked; maximize before attaching the observers.
      await app.evaluate(({ BrowserWindow }) => {
        const window = BrowserWindow.getAllWindows()[0];
        if (window) {
          window.maximize();
          window.show();
          window.focus();
        }
      });

      browserAttachedAt = new Date().toISOString();
      browserAttachRequests = browserObs.requests;
      browserAttachFailedRequests = browserObs.failedRequests;
      const noteRequest = (request: { url(): string }) => {
        browserObs.requests += 1;
        const raw = request.url();
        if (raw.startsWith(runtimeConfig.apiUrl ?? "")) isolatedApiRequests += 1;
        if (chromiumControlActive && isDesktopControlUrl(raw)) return;
        if (!isLocalUrl(raw)) browserObs.externalRequests += 1;
        if (!isLocalUrl(raw)) browserObs.externalRequestUrls.push(sanitizeUrl(raw));
      };
      const context = page.context();
      context.on("request", noteRequest);
      context.on("page", (newPage) => {
        newPage.on("websocket", (socket) => {
          const sanitized = sanitizeUrl(socket.url());
          browserObs.localWebsockets.push(sanitized);
          if (!isLocalUrl(socket.url())) {
            browserObs.externalWebsockets += 1;
            browserObs.externalWebsocketUrls.push(sanitized);
          }
        });
        newPage.on("framenavigated", (frame) => {
          if (frame !== newPage.mainFrame()) return;
          if (!isLocalUrl(frame.url())) {
            browserObs.externalNavigations += 1;
            browserObs.externalNavigationUrls.push(sanitizeUrl(frame.url()));
          }
        });
        newPage.on("requestfailed", (request) => {
          if (chromiumControlActive && isDesktopControlUrl(request.url())) return;
          if (!isLocalUrl(request.url())) {
            browserObs.failedRequests += 1;
            browserObs.failedRequestUrls.push(sanitizeUrl(request.url()));
          }
        });
      });
      page.on("websocket", (socket) => {
        const sanitized = sanitizeUrl(socket.url());
        browserObs.localWebsockets.push(sanitized);
        if (!isLocalUrl(socket.url())) {
          browserObs.externalWebsockets += 1;
          browserObs.externalWebsocketUrls.push(sanitized);
        }
      });
      page.on("framenavigated", (frame) => {
        if (frame !== page.mainFrame()) return;
        if (!isLocalUrl(frame.url())) {
          browserObs.externalNavigations += 1;
          browserObs.externalNavigationUrls.push(sanitizeUrl(frame.url()));
        }
      });
      page.on("requestfailed", (request) => {
        if (chromiumControlActive && isDesktopControlUrl(request.url())) return;
        if (!isLocalUrl(request.url())) {
          browserObs.failedRequests += 1;
          browserObs.failedRequestUrls.push(sanitizeUrl(request.url()));
        }
      });

      await expect(page.locator('input[type="email"]').first()).toBeVisible({ timeout: 60_000 });
      const appName = await app.evaluate(({ app: electronApp }) => electronApp.getName());
      expect(appName, "packaged app.getName()").toBe(PRODUCT_NAME);
      const loginWindowTitle = await page.title();
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
      expect(existsSync(realAppDataPath), "the real roaming userData directory must never be created").toBe(false);

      // Deterministic English + light UI for screenshots and stable selectors.
      await page.evaluate(() => {
        localStorage.setItem("multica-locale", "en");
        localStorage.setItem("theme", "light");
      });
      await page.reload({ waitUntil: "domcontentloaded" });
      await expect(page.getByText("Sign in to U Workers")).toBeVisible({ timeout: 60_000 });
      await expect(page.locator("html")).toHaveClass(/light/);
      if (!DESKTOP_SKIP_SCREENSHOTS) {
        await captureFullWindow(app, DESKTOP_ROOT, evidencePath("login.png"));
        (evidence.screenshots as string[]).push("login.png");
      }
      await ensurePhaseSamples(launchStartedAt, 2);
      const startupEndedAt = new Date().toISOString();
      snapshotStartup = browserSnapshot();
      expect(snapshotStartup, "non-control browser egress after attaching during startup").toEqual({
        externalRequests: 0, externalWebsockets: 0, externalNavigations: 0, failedRequests: 0,
      });

      // --- Control phase B: Chromium self-test, after the app is running.
      const controlBStart = new Date().toISOString();
      chromiumControlActive = true;
      const selftest = await page.evaluate(async () => {
        const results: string[] = [];
        try {
          const response = await fetch("http://uw80-desktop-selftest.invalid/");
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
      await waitForRecorderHost(DESKTOP_SELFTEST_HTTP_HOST, 30000);
      await waitForRecorderHost(DESKTOP_SELFTEST_TLS_HOST, 30000);
      await sleep(1500);
      await ensurePhaseSamples(controlBStart, 2);
      const controlBEnd = new Date().toISOString();
      chromiumControlActive = false;
      snapshotControlB = browserSnapshot();
      controlWindowsRaw.push({
        label: "chromium-self-test",
        start: controlBStart,
        end: controlBEnd,
        hosts: [DESKTOP_SELFTEST_HTTP_HOST, DESKTOP_SELFTEST_TLS_HOST],
      });

      // --- Login phase.
      const loginStart = new Date().toISOString();
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
      await expect.poll(() => browserObs.localWebsockets.length, { timeout: 30000 }).toBeGreaterThan(0);
      expect(isolatedApiRequests, "the packaged app must talk to the isolated API").toBeGreaterThan(0);
      const mainWindowTitle = await page.title();
      await ensurePhaseSamples(loginStart, 2);
      const loginEnd = new Date().toISOString();
      snapshotLogin = browserSnapshot();

      // --- Browse phase.
      const browseStart = new Date().toISOString();
      await page.locator('a[href*="/inbox"]').first().click();
      await expect(page.getByRole("heading", { name: "Inbox" }).first()).toBeVisible({ timeout: 30000 });
      await page.locator('a[href*="/agents"]').first().click();
      await expect(page.getByRole("heading", { name: "Agents" }).first()).toBeVisible({ timeout: 30000 });
      await page.locator('a[href*="/issues"]').first().click();
      await expect(page.getByRole("heading", { name: "Issues" }).first()).toBeVisible({ timeout: 30000 });
      await expect(page.getByRole("button", { name: "New Issue" }).first()).toBeVisible({ timeout: 30000 });
      if (!DESKTOP_SKIP_SCREENSHOTS) {
        await captureFullWindow(app, DESKTOP_ROOT, evidencePath("main.png"));
        (evidence.screenshots as string[]).push("main.png");
      }
      await ensurePhaseSamples(browseStart, 2);
      const browseEnd = new Date().toISOString();
      snapshotBrowse = browserSnapshot();

      // --- Idle phase: 60 s without interaction.
      const idleStart = new Date().toISOString();
      await page.waitForTimeout(60_000);
      const idleEnd = new Date().toISOString();

      await stopTcpSampler(sampler, samplerPath);
      sampler = undefined;

      const recorderEntries = readRecorderEntries().slice(recorderBaselineOffset);
      const samples = readSamplerSamples(samplerPath);
      const controlWindows = makeControlWindows(controlWindowsRaw);
      expect(classifyRecorderEntries(recorderEntries, {
        start: observationStartedAt, end: idleEnd, controlWindows,
      }).violations, "non-control recorder traffic anywhere in the continuous pass").toEqual([]);
      const startupBrowser: BrowserLayer | null = null;
      const controlBBrowser = browserDeltaFrom(snapshotStartup, snapshotControlB);
      const loginBrowser = browserDeltaFrom(snapshotControlB, snapshotLogin);
      const browseBrowser = browserDeltaFrom(snapshotLogin, snapshotBrowse);
      const idleBrowser = browserDeltaFrom(snapshotBrowse);

      phases = [
        summarizePhase({
          name: "control-a-node-go-system",
          start: controlAStart,
          end: controlAEnd,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          allowedNonLoopback: [SYSTEM_LAYER_CONTROL_REMOTE],
        }),
        summarizePhase({
          name: "startup",
          start: launchStartedAt,
          end: startupEndedAt,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          browserDelta: startupBrowser,
        }),
        summarizePhase({
          name: "control-b-chromium-selftest",
          start: controlBStart,
          end: controlBEnd,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          browserDelta: controlBBrowser,
        }),
        summarizePhase({
          name: "login",
          start: loginStart,
          end: loginEnd,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          browserDelta: loginBrowser,
        }),
        summarizePhase({
          name: "browse",
          start: browseStart,
          end: browseEnd,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          browserDelta: browseBrowser,
        }),
        summarizePhase({
          name: "idle-60s",
          start: idleStart,
          end: idleEnd,
          samplerIntervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
          samples,
          recorderEntries,
          controlWindows,
          browserDelta: idleBrowser,
        }),
      ];
      for (const phase of phases) {
        expect(phase.ok, "observation phase failed: " + String(phase.name) + ": " + (phase.errors as string[]).join("; ")).toBe(true);
      }
      (evidence.observation as Record<string, unknown>).phases = phases;

      const daemonSamples = samples.filter((sample) => (sample.pids?.daemon ?? []).length > 0);
      daemonPids = Array.from(new Set(daemonSamples.flatMap((sample) => sample.pids.daemon)));
      const daemonFirstSeenAt = daemonSamples.length > 0 ? daemonSamples[0].ts : null;
      const daemonProfileDir = join(isolatedHome, ".multica", "profiles", daemonProfileNeedle.replace(/-$/, ""));
      (evidence.observation as Record<string, unknown>).daemon = {
        observed: daemonPids.length > 0,
        pids: daemonPids,
        firstSeenAt: daemonFirstSeenAt,
        profileDir: daemonProfileDir,
        profileInitialized: existsSync(join(daemonProfileDir, "config.json")),
        executablePath: daemonExePath,
        identification: "multica.exe with the packaged CLI executable path, the isolated profile needle and a creation time inside this run",
      };
      expect(daemonPids.length, "the desktop auto-started daemon was not sampled in the TCP layer").toBeGreaterThan(0);

      const sampleTimestamps = samples.map((sample) => Date.parse(sample.ts)).sort((a, b) => a - b);
      const sampleGaps: number[] = [];
      for (let index = 1; index < sampleTimestamps.length; index += 1) {
        sampleGaps.push(sampleTimestamps[index] - sampleTimestamps[index - 1]);
      }
      (evidence.observation as Record<string, unknown>).sampler = {
        file: "tcp-samples.jsonl",
        intervalMs: DESKTOP_SAMPLER_INTERVAL_MS,
        totalSamples: samples.length,
        firstSampleAt: samples.length > 0 ? samples[0].ts : null,
        lastSampleAt: samples.length > 0 ? samples[samples.length - 1].ts : null,
        maxGapMs: sampleGaps.length > 0 ? Math.max(...sampleGaps) : null,
        medianGapMs:
          sampleGaps.length > 0
            ? sampleGaps.slice().sort((a, b) => a - b)[Math.floor(sampleGaps.length / 2)]
            : null,
        samplerErrors: samples.filter((sample) => typeof sample.error === "string").length,
        note: "sampling backstop, not a complete connection log; phases fail when their own window has no samples or a sampler error",
      };
      (evidence.observation as Record<string, unknown>).recorder = {
        file: "recorder.jsonl",
        baselineOffset: recorderBaselineOffset,
        entriesAfterBaseline: recorderEntries.length,
        trafficEntries: recorderEntries.filter(
          (entry) => entry.kind === "http" || entry.kind === "connect" || entry.kind === "upgrade",
        ).length,
      };
      (evidence.observation as Record<string, unknown>).controls = {
        controlWindows: controlWindowsRaw,
        nodeEnvProxy: { host: DESKTOP_NODE_CONTROL_HOST, recorded: true },
        goRuntimeProxy: goControl,
        chromiumSelfTest: { results: selftest },
        systemTcpControl: { remote: SYSTEM_LAYER_CONTROL_REMOTE, sampled: true },
      };
      (evidence.app as Record<string, unknown>) = {
        name: appName,
        loginWindowTitle,
        mainWindowTitle,
        userDataPath: reportedUserData,
        browserAttachedAt,
        requestsAtAttach: browserAttachRequests,
        failedRequestsAtAttach: browserAttachFailedRequests,
      };
      (evidence.scenarios as Array<Record<string, unknown>>).push(
        {
          scenario: "pre-launch controls",
          phases: ["control-a-node-go-system"],
        },
        {
          scenario: "startup",
          phases: ["startup"],
          browserLayer: "not attached yet; recorder covers Chromium from process start",
        },
        {
          scenario: "control-b chromium self-test",
          phases: ["control-b-chromium-selftest"],
        },
        { scenario: "login", phases: ["login"] },
        { scenario: "browse", phases: ["browse"] },
        { scenario: "idle", phases: ["idle-60s"] },
      );
      writeEvidenceFile("recorder.jsonl", recorderEntries.map((entry) => JSON.stringify(entry)).join("\n") + "\n");
      writeEvidenceFile("tcp-samples.jsonl", samples.map((sample) => JSON.stringify(sample)).join("\n") + "\n");
    } catch (error) {
      evidence.error = error instanceof Error ? error.message : String(error);
      throw error;
    } finally {
      if (sampler) await stopTcpSampler(sampler, samplerPath);
      const appExeCandidates: ProcessInfo[] = [];
      if (desktopApp) {
        try {
          const beforeClose = readProcessSnapshot();
          for (const row of beforeClose) {
            if (samePath(row.executablePath, DESKTOP_EXE) && !preExistingAppPids.includes(row.pid)) {
              appExeCandidates.push(row);
            }
          }
        } catch {
          /* the post-close scan below is authoritative */
        }
        await desktopApp.close().catch(() => undefined);
      }
      await sleep(2000);

      const afterCloseSnapshot = readProcessSnapshot();
      const currentWorkspaceDaemon = readWorkspaceDaemonStatus();
      requireDesktopHarness(
        currentWorkspaceDaemon.pid !== null && currentWorkspaceDaemon.source === "multica daemon status --output json",
        "cannot revalidate workspace daemon before cleanup",
      );
      protectedPids.push(currentWorkspaceDaemon.pid as number);
      const appCandidates = afterCloseSnapshot.filter(
        (row) => samePath(row.executablePath, DESKTOP_EXE) && !preExistingAppPids.includes(row.pid),
      );
      const daemonCandidates = afterCloseSnapshot.filter(
        (row) =>
          row.name === "multica.exe" &&
          samePath(row.executablePath, daemonExePath) &&
          row.commandLine.includes(daemonProfileNeedle) &&
          !preExistingDaemonPids.includes(row.pid),
      );
      const appPlan = planProcessCleanup(appCandidates, {
        preExistingPids: preLaunchSnapshot.map((row) => row.pid),
        protectedPids,
        runStartedAt,
        expectedExecutablePath: DESKTOP_EXE,
      });
      const daemonPlan = planProcessCleanup(daemonCandidates, {
        preExistingPids: preLaunchSnapshot.map((row) => row.pid),
        protectedPids,
        runStartedAt,
        expectedExecutablePath: daemonExePath,
      });
      const cleanupErrors: string[] = [];
      for (const pid of appPlan.approved) {
        try {
          stopOwnedProcess(appCandidates.find((row) => row.pid === pid)!, protectedPids);
        } catch (error) {
          cleanupErrors.push(error instanceof Error ? error.message : String(error));
        }
      }
      for (const pid of daemonPlan.approved) {
        try {
          stopOwnedProcess(daemonCandidates.find((row) => row.pid === pid)!, protectedPids);
        } catch (error) {
          cleanupErrors.push(error instanceof Error ? error.message : String(error));
        }
      }
      await sleep(2500);

      const afterCleanupSnapshot = readProcessSnapshot();
      const remainingApp = afterCleanupSnapshot.filter(
        (row) => samePath(row.executablePath, DESKTOP_EXE) && !preExistingAppPids.includes(row.pid),
      );
      const remainingDaemon = afterCleanupSnapshot.filter(
        (row) =>
          row.name === "multica.exe" &&
          samePath(row.executablePath, daemonExePath) &&
          row.commandLine.includes(daemonProfileNeedle) &&
          !preExistingDaemonPids.includes(row.pid),
      );
      const workspaceDaemonAlive = afterCleanupSnapshot.some((row) => row.pid === workspaceDaemonStatus.pid);

      const deletionRecords: Array<Record<string, unknown>> = [];
      for (const [label, targetPath] of [
        ["isolated userData", userDataPath],
        ["helper scripts", join(DESKTOP_ROOT, "state", "d2-helpers")],
        ["go proxy control", join(DESKTOP_ROOT, "state", "go-proxy-control")],
      ] as Array<[string, string]>) {
        if (!existsSync(targetPath)) {
          deletionRecords.push({ label, path: targetPath, existed: false, removed: true });
          continue;
        }
        try {
          removeDirVerified(targetPath, DESKTOP_ROOT, label);
          deletionRecords.push({ label, path: targetPath, existed: true, removed: true });
        } catch (error) {
          deletionRecords.push({
            label,
            path: targetPath,
            existed: true,
            removed: false,
            error: error instanceof Error ? error.message : String(error),
          });
        }
      }

      cleanupEvidence = {
        errors: cleanupErrors,
        workspaceDaemon: {
          pid: workspaceDaemonStatus.pid,
          source: workspaceDaemonStatus.source,
          port: workspaceDaemonStatus.port,
          cliPath: workspaceDaemonStatus.cliPath,
          aliveAfterCleanup: workspaceDaemonAlive,
        },
        preLaunch: {
          snapshotAt: runStartedAt,
          preExistingAppPids,
          preExistingDaemonPids,
        },
        plans: [
          {
            group: "packaged app process tree",
            expectedExecutablePath: DESKTOP_EXE,
            approved: appPlan.approved,
            rejected: appPlan.rejected,
            candidatesSeenAtClose: appExeCandidates.map((row) => ({ pid: row.pid, name: row.name, creationDate: row.creationDate })),
          },
          {
            group: "bundled CLI daemon",
            expectedExecutablePath: daemonExePath,
            profileNeedle: daemonProfileNeedle,
            approved: daemonPlan.approved,
            rejected: daemonPlan.rejected,
            candidatesSeenAtClose: daemonCandidates.map((row) => ({ pid: row.pid, creationDate: row.creationDate })),
          },
        ],
        remainingRunOwnedProcesses: {
          app: remainingApp.map((row) => row.pid),
          daemon: remainingDaemon.map((row) => row.pid),
        },
        recursiveDeletions: deletionRecords,
      };
      (evidence.cleanup as Record<string, unknown>) = cleanupEvidence;
      writeEvidenceFile("run-summary.json", JSON.stringify(evidence, null, 2) + "\n");

      const realProfilesAfter = existsSync(realMulticaProfiles) ? readdirSync(realMulticaProfiles).sort() : [];
      const newRealDesktopProfiles = realProfilesAfter.filter(
        (name) => !realProfilesBefore.includes(name) && name.startsWith("u-workers-desktop-"),
      );

      if (evidence.error === undefined) {
        expect(cleanupErrors, "process identity or termination check failed").toEqual([]);
        expect([...appPlan.rejected, ...daemonPlan.rejected], "process ownership could not be confirmed").toEqual([]);
        expect(remainingApp, "packaged app processes survived the verified cleanup").toEqual([]);
        expect(remainingDaemon, "the bundled CLI daemon survived the verified cleanup").toEqual([]);
        expect(workspaceDaemonAlive, "the workspace daemon disappeared during cleanup").toBe(true);
        expect(
          deletionRecords.filter((record) => record.removed !== true),
          "an isolated recursive deletion failed",
        ).toEqual([]);
        expect(newRealDesktopProfiles, "a real ~/.multica desktop profile was created despite the isolated HOME").toEqual(
          [],
        );
        expect(existsSync(userDataPath), "isolated userData directory still exists after cleanup").toBe(false);
        expect(existsSync(realAppDataPath), "the real roaming userData directory was created during the run").toBe(false);
        expect(daemonPids.length, "the auto-started daemon was not part of the sampled process set").toBeGreaterThan(0);
      }
    }
  });
});
