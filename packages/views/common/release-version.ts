/**
 * Release-version helpers shared by the version surfaces (Help menu, runtime
 * CLI row). UWorkers ships the server, web bundle and Client-Kit under one
 * release tag, so equal release versions mean the pieces match.
 */

/**
 * Parses a released CLI version ("v0.4.17" / "0.4.17") into comparable parts,
 * or null when the string is not a release version.
 *
 * A daemon built from source reports a `git describe` string
 * ("v0.4.17-12-gabc1234") or the ldflags default ("dev"), and neither can be
 * ordered against a release tag. This mirrors `IsReleaseVersion` in
 * server/internal/cli/update.go, which is how the daemon's own auto-update
 * loop decides the same question.
 */
export function parseReleaseVersion(v: string): number[] | null {
  const parts = v.trim().replace(/^v/, "").split(".");
  if (parts.length !== 3) return null;
  const parsed: number[] = [];
  for (const part of parts) {
    if (!/^\d+$/.test(part)) return null;
    parsed.push(Number(part));
  }
  return parsed;
}

/**
 * True when both strings are release versions naming the same release.
 * Anything unparseable (dev builds, `git describe` strings, empty) compares
 * as "unknown" and returns null, so callers never claim a mismatch they
 * could not actually read.
 */
export function sameReleaseVersion(a: string | null | undefined, b: string | null | undefined): boolean | null {
  const pa = a ? parseReleaseVersion(a) : null;
  const pb = b ? parseReleaseVersion(b) : null;
  if (!pa || !pb) return null;
  return pa.every((part, i) => part === pb[i]);
}
