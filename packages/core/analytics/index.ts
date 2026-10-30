// Inert reporting boundary for existing callers. This build has no analytics
// SDK, identity/event buffer, exception listener, or reporting transport.
// Local diagnostics and user-configured networking operate independently.

export const EVENT_SCHEMA_VERSION = 2;

export interface AnalyticsConfig {
  key: string;
  host: string;
  appVersion?: string;
  environment?: string;
}

export type ClientType = "desktop" | "web";

export function detectClientType(): ClientType {
  if (typeof window === "undefined") return "web";
  const w = window as unknown as { electron?: unknown; desktopAPI?: unknown };
  if (w.electron || w.desktopAPI) return "desktop";
  if (typeof navigator !== "undefined" && /Electron/i.test(navigator.userAgent)) {
    return "desktop";
  }
  return "web";
}

/** Server configuration cannot enable reporting in this build. */
export function initAnalytics(_config: AnalyticsConfig | null | undefined): boolean {
  return false;
}

export function identify(_userId: string, _userProperties?: Record<string, unknown>): void {}

export function resetAnalytics(): void {}

export interface CaptureEventOptions {
  sendInstantly?: boolean;
  /** Never called in this build: no event is handed off for reporting. */
  onCaptured?: () => void;
}

export function captureEvent(
  _name: string,
  _props?: Record<string, unknown>,
  _options?: CaptureEventOptions,
): void {}

export function captureException(_error: unknown, _props?: Record<string, unknown>): void {}

export function setPersonProperties(_props: Record<string, unknown>): void {}

/** Remove attribution left by older versions without collecting a new source. */
export function captureSignupSource(): void {
  if (typeof document !== "undefined") {
    document.cookie = "multica_signup_source=; path=/; max-age=0; samesite=lax";
  }
}
