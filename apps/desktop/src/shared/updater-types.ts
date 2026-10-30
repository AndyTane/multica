export interface UpdaterPreferences {
  automaticUpdates: boolean;
}

export type ManualUpdateCheckResult =
  | {
      ok: true;
      currentVersion: string;
      latestVersion: string;
      available: boolean;
      sourceUrl: string;
    }
  | { ok: false; error: string };
