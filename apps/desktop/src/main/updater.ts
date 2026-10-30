import { autoUpdater, type UpdateDownloadedEvent } from "electron-updater";
import { app, dialog, type BrowserWindow, ipcMain } from "electron";
import type { ManualUpdateCheckResult } from "../shared/updater-types";

autoUpdater.autoDownload = false;
autoUpdater.autoInstallOnAppQuit = false;

// Windows arm64 ships its own update metadata channel because
// electron-builder's `latest.yml` is not arch-suffixed on Windows — both
// arches would otherwise collide on the same file in the GitHub Release.
// See scripts/package.mjs (builderArgsForTarget) for the publish-side half
// of this pact. Pin the channel here so arm64 clients fetch
// `latest-arm64.yml` instead of the x64 metadata.
if (process.platform === "win32" && process.arch === "arm64") {
  autoUpdater.channel = "latest-arm64";
}

interface ChannelConfigurableUpdater {
  channel: string | null;
  allowDowngrade: boolean;
}

export function configureMacX64UpdateChannel(
  updater: ChannelConfigurableUpdater,
  platform: NodeJS.Platform = process.platform,
  arch: string = process.arch,
): void {
  if (platform !== "darwin" || arch !== "x64") return;

  // AppUpdater.channel enables allowDowngrade as a side effect. This channel
  // isolates a CPU architecture, not a release train, so preserve normal
  // monotonic version behavior after selecting the architecture feed.
  updater.channel = "latest-x64";
  updater.allowDowngrade = false;
}

// electron-builder does not architecture-suffix macOS update metadata.
// package.mjs publishes macOS x64 as `latest-x64-mac.yml`; the established
// arm64 feed and runtime path remain unchanged.
configureMacX64UpdateChannel(autoUpdater);

type RendererChannel =
  | "updater:update-available"
  | "updater:download-progress"
  | "updater:update-downloaded";

function isDestroyedObjectError(err: unknown): boolean {
  return err instanceof Error && err.message.includes("Object has been destroyed");
}

function sendToLiveRenderer(
  win: BrowserWindow | null,
  channel: RendererChannel,
  payload: unknown,
): void {
  if (!win || win.isDestroyed()) return;

  try {
    const { webContents } = win;
    if (webContents.isDestroyed()) return;
    webContents.send(channel, payload);
  } catch (err) {
    if (isDestroyedObjectError(err)) return;
    throw err;
  }
}


export function setupAutoUpdater(
  getMainWindow: () => BrowserWindow | null,
  options: { updateUrl?: string } = { updateUrl: process.env.U_WORKERS_UPDATE_URL },
): void {
  const source = options.updateUrl?.trim();
  let sourceError = "No update source configured.";
  let configured = false;
  if (source) {
    try {
      const url = new URL(source);
      if (!["https:", "http:"].includes(url.protocol) || url.username || url.password) {
        throw new Error("Update source must be an HTTP(S) URL without credentials.");
      }
      autoUpdater.setFeedURL({ provider: "generic", url: source });
      configured = true;
    } catch (error) {
      sourceError = error instanceof Error ? error.message : String(error);
    }
  }
  let available = false;
  let downloaded = false;
  let inFlightCheck: Promise<ManualUpdateCheckResult> | null = null;
  let inFlightDownload: Promise<unknown> | null = null;

  autoUpdater.on("update-available", (info) => {
    sendToLiveRenderer(getMainWindow(), "updater:update-available", {
      version: info.version, releaseNotes: info.releaseNotes,
    });
  });
  autoUpdater.on("download-progress", (progress) => {
    sendToLiveRenderer(getMainWindow(), "updater:download-progress", { percent: progress.percent });
  });
  autoUpdater.on("update-downloaded", (info: UpdateDownloadedEvent) => {
    downloaded = true;
    sendToLiveRenderer(getMainWindow(), "updater:update-downloaded", {
      version: info.version, releaseNotes: info.releaseNotes,
    });
  });
  autoUpdater.on("error", (err) => console.error("Updater error:", err));

  // Old preferences cannot reactivate background checks.
  ipcMain.handle("updater:get-preferences", () => ({ automaticUpdates: false }));
  ipcMain.handle("updater:set-automatic-updates", () => ({ automaticUpdates: false }));

  ipcMain.handle("updater:check", (): Promise<ManualUpdateCheckResult> => {
    if (!configured) return Promise.resolve({ ok: false, error: sourceError });
    if (inFlightCheck) return inFlightCheck;
    inFlightCheck = (async (): Promise<ManualUpdateCheckResult> => {
      try {
        const result = await autoUpdater.checkForUpdates();
        available = result?.isUpdateAvailable ?? false;
        return {
          ok: true, currentVersion: app.getVersion(),
          latestVersion: result?.updateInfo.version ?? app.getVersion(),
          available, sourceUrl: source!,
        };
      } catch (err) {
        available = false;
        return { ok: false, error: err instanceof Error ? err.message : String(err) };
      } finally {
        inFlightCheck = null;
      }
    })();
    return inFlightCheck;
  });

  ipcMain.handle("updater:download", async () => {
    if (!configured) throw new Error(sourceError);
    if (!available) throw new Error("Check for an available update first.");
    if (inFlightDownload) return inFlightDownload;
    const download = async () => {
      const { response } = await dialog.showMessageBox({
        type: "question", title: "Download update",
        message: "Download this update?", detail: source,
        buttons: ["Cancel", "Download"], defaultId: 0, cancelId: 0,
      });
      if (response !== 1) return;
      return autoUpdater.downloadUpdate();
    };
    inFlightDownload = download().finally(() => { inFlightDownload = null; });
    return inFlightDownload;
  });
  ipcMain.handle("updater:install", () => {
    if (!downloaded) throw new Error("Download an update first.");
    autoUpdater.quitAndInstall(false, true);
  });
}
