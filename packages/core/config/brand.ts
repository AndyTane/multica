/**
 * Display identity and the temporary dual names for it.
 *
 * Every external identifier the product exposes has a current spelling and a
 * legacy Multica spelling while the rename is in flight: readers accept both,
 * writers emit the current one and — where an older peer may still read it —
 * the legacy one too. Keep the client copy of those names here; the Go server
 * mirrors it in server/internal/brand.
 *
 * Removal criteria live in docs/plans/2026-11-01-u-workers-brand-batch-c.md.
 */
export const PRODUCT_NAME = "U Workers";
export const LEGACY_PRODUCT_NAME = "Multica";

export const DESKTOP_APP_ID = "app.uworkers.desktop";
export const DESKTOP_CONFIG_DIRECTORY = ".u-workers";
export const DESKTOP_PROFILE_PREFIX = "u-workers-desktop-";

/** Executable name current releases install; the legacy name still works. */
export const CLI_COMMAND = "u-workers";
export const LEGACY_CLI_COMMAND = "multica";

/** Per-user directory owning CLI profiles and config.json, current then legacy. */
export const CONFIG_DIRECTORY = ".u-workers";
export const LEGACY_CONFIG_DIRECTORY = ".multica";

/** Environment-variable prefixes. */
export const ENV_PREFIX = "U_WORKERS_";
export const LEGACY_ENV_PREFIX = "MULTICA_";

/**
 * Session cookies. The server sets and reads both spellings during the
 * transition, so a browser holding either one stays signed in.
 */
export const AUTH_COOKIE = "u_workers_auth";
export const LEGACY_AUTH_COOKIE = "multica_auth";
export const CSRF_COOKIE = "u_workers_csrf";
export const LEGACY_CSRF_COOKIE = "multica_csrf";
export const SESSION_CSRF_COOKIE = "u_workers_csrf_session";
export const LEGACY_SESSION_CSRF_COOKIE = "multica_csrf_session";

/** Request headers, current then legacy. */
export const PLUGIN_INSTALLATION_HEADER = "X-U-Workers-Plugin-Installation";
export const LEGACY_PLUGIN_INSTALLATION_HEADER = "X-Multica-Plugin-Installation";
