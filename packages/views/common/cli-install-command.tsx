"use client";

import type { ReactNode } from "react";
import { useConfigStore } from "@multica/core/config";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@multica/ui/components/ui/tabs";
import { CLI_COMMAND } from "./cli-command";

/** Shown when the server does not report a stamped release version. */
export const CLI_VERSION_PLACEHOLDER = "<SERVER_VERSION>";

/**
 * The local build must carry the server's release version, or version-gated
 * features refuse it. Servers report `1.2.3` or `v1.2.3`; anything else (dev
 * builds report nothing) falls back to a placeholder the user fills in.
 */
export function cliBuildVersion(serverVersion: string | undefined): string {
  const raw = serverVersion?.trim() ?? "";
  const version = /^[0-9]/.test(raw) ? `v${raw}` : raw;
  return /^v[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9]+-g[0-9a-f]+)?$/.test(version)
    ? version
    : CLI_VERSION_PLACEHOLDER;
}

/**
 * Build the CLI from the source checkout and put it on this terminal's PATH,
 * so the next step's `u-workers` resolves. Nothing is downloaded.
 */
export function cliInstallCommands(version: string) {
  return {
    macosLinux: `(cd server && go build -ldflags "-X main.version=${version}" -o bin/${CLI_COMMAND} ./cmd/multica)
export PATH="$PWD/server/bin:$PATH"`,
    windows: `pwsh -File scripts/build-local-cli.ps1 -Version ${version}
$env:Path = "$PWD\\server\\bin;$env:Path"`,
  } as const;
}

export type CliInstallPlatform = keyof ReturnType<typeof cliInstallCommands>;

/** Tab order. macOS / Linux stays first so the default matches the old single command. */
const PLATFORMS = [
  "macosLinux",
  "windows",
] as const satisfies readonly CliInstallPlatform[];

export interface CliInstallCommandLabels {
  /** Accessible name for the platform switch. */
  group: string;
  macosLinux: string;
  windows: string;
}

/**
 * Platform switch for the install command, built on the shared Tabs primitive
 * so tab semantics and arrow-key navigation come for free.
 *
 * The switch is the only shared UI: `children` receives the selected command
 * and renders it, so each surface keeps its own command row — the product
 * surfaces their token-styled row with an icon copy button, the landing page
 * its marketing palette with a spelled-out one.
 */
export function CliInstallCommand({
  labels,
  classNames,
  children,
}: {
  labels: CliInstallCommandLabels;
  /** Palette overrides for surfaces that do not use the product tokens. */
  classNames?: { list?: string; trigger?: string };
  children: (command: string) => ReactNode;
}) {
  const serverVersion = useConfigStore((s) => s.serverVersion);
  const commands = cliInstallCommands(cliBuildVersion(serverVersion));
  return (
    <Tabs defaultValue={PLATFORMS[0]}>
      <TabsList
        aria-label={labels.group}
        activateOnFocus
        className={classNames?.list}
      >
        {PLATFORMS.map((platform) => (
          <TabsTrigger
            key={platform}
            value={platform}
            className={classNames?.trigger}
          >
            {labels[platform]}
          </TabsTrigger>
        ))}
      </TabsList>
      {PLATFORMS.map((platform) => (
        <TabsContent key={platform} value={platform}>
          {children(commands[platform])}
        </TabsContent>
      ))}
    </Tabs>
  );
}
