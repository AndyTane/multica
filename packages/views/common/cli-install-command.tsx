"use client";

import type { ReactNode } from "react";
import {
  Tabs,
  TabsContent,
  TabsList,
  TabsTrigger,
} from "@multica/ui/components/ui/tabs";

/** Use a bundled/local CLI. These commands never fetch a remote installer. */
export const CLI_INSTALL_COMMANDS = {
  macosLinux:
    "export U_WORKERS_CLI_PATH=/absolute/path/to/multica",
  windows:
    "pwsh -File scripts/build-local-cli.ps1",
} as const;

export type CliInstallPlatform = keyof typeof CLI_INSTALL_COMMANDS;

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
          {children(CLI_INSTALL_COMMANDS[platform])}
        </TabsContent>
      ))}
    </Tabs>
  );
}
