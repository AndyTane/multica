// Brand host shown as the workspace URL prefix on the managed Multica Cloud,
// and the fallback whenever the deployment exposes no app URL. `/api/config`
// deliberately omits `daemon_app_url` for the managed cloud (and for any
// self-hosted server that has not set MULTICA_APP_URL / FRONTEND_ORIGIN), so
// this literal must remain the ultimate fallback.
const UNCONFIGURED_HOST = "";

/**
 * Host rendered as the `<host>/<slug>` workspace URL prefix in the
 * create-workspace and onboarding UI. Derived from the deployment's app URL
 * (`daemon_app_url` from `/api/config`, surfaced through the config store) so
 * self-hosted instances show their own domain instead of an upstream domain. Falls
 * back to the empty host when no app URL is configured.
 */
export function workspaceUrlHost(
  daemonAppUrl: string | null | undefined,
): string {
  const trimmed = daemonAppUrl?.trim();
  if (!trimmed) return UNCONFIGURED_HOST;
  try {
    return new URL(trimmed).host || UNCONFIGURED_HOST;
  } catch {
    // `daemon_app_url` may arrive without a scheme; treat it as a bare host
    // and strip any path/query/fragment so only the authority remains.
    const bare = trimmed
      .replace(/^.*?:\/\//, "")
      .replace(/[/?#].*$/, "")
      .trim();
    return bare || UNCONFIGURED_HOST;
  }
}
