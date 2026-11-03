"use client";

import { TriangleAlert } from "lucide-react";
import { useConfigStore } from "@multica/core/config";
import { useT } from "../../i18n";
import { sameReleaseVersion } from "../../common/release-version";

/**
 * Warns when a machine's CLI runs a different release than the server.
 *
 * The Client-Kit ships under the server's release tag, so a different release
 * means the machine was not re-connected after an upgrade. Desktop-managed
 * CLIs follow the desktop app's own versions, and source builds cannot be
 * compared, so both stay quiet.
 */
export function ServerVersionMismatch({
  currentVersion,
  launchedBy,
}: {
  currentVersion: string | null;
  launchedBy?: string | null;
}) {
  const { t } = useT("runtimes");
  const serverVersion = useConfigStore((s) => s.serverVersion);
  if (launchedBy === "desktop") return null;
  if (sameReleaseVersion(currentVersion, serverVersion) !== false) return null;

  return (
    <span
      className="inline-flex items-center gap-1 text-caption text-warning"
      title={t(($) => $.update.server_mismatch_title)}
    >
      <TriangleAlert className="h-3 w-3" />
      {t(($) => $.update.server_mismatch, {
        cli: currentVersion,
        version: serverVersion,
      })}
    </span>
  );
}
