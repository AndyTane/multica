import { app } from "electron";
import { existsSync } from "fs";
import { join } from "path";

export function managedCliPath(): string {
  return join(app.getPath("userData"), "bin", process.platform === "win32" ? "multica.exe" : "multica");
}

// Resolution is local only. Installation and updates belong to the user.
export async function ensureManagedCli(
  options: { binaryPath?: string } = {},
): Promise<string> {
  const target = options.binaryPath ?? managedCliPath();
  if (existsSync(target)) return target;
  throw new Error("CLI not found. Use the bundled CLI, install a local binary, or set U_WORKERS_CLI_PATH.");
}
