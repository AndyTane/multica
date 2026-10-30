import type { DownloadAssets } from "./parse-release-assets";

export interface LatestRelease {
  version: string | null;
  publishedAt: string | null;
  htmlUrl: string | null;
  assets: DownloadAssets;
}

// No release source is configured for this distribution.
export async function fetchLatestRelease(): Promise<LatestRelease> {
  return { version: null, publishedAt: null, htmlUrl: null, assets: {} };
}
