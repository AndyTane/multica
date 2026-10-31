"use client";

/**
 * One-shot "your password changed, sign in again" marker.
 *
 * A password change revokes the session, so the only place the confirmation can
 * be read is the sign-in page. The query string cannot carry it reliably: the
 * shell redirects an unauthenticated visitor to /login and that redirect can
 * land after the page's own navigation, dropping the parameter. A short-lived
 * sessionStorage marker survives those redirects and any remount of the login
 * card, and is cleared on the first keystroke or a successful sign-in.
 */
const KEY = "multica:password-changed";
const MAX_AGE_MS = 60_000;

export function markPasswordChanged(): void {
  try {
    sessionStorage.setItem(KEY, String(Date.now()));
  } catch {
    // A browser that blocks session storage simply does not get the notice.
  }
}

export function passwordChangedNoticeVisible(): boolean {
  try {
    const raw = sessionStorage.getItem(KEY);
    if (!raw) return false;
    const at = Number(raw);
    if (!Number.isFinite(at) || Date.now() - at > MAX_AGE_MS) {
      sessionStorage.removeItem(KEY);
      return false;
    }
    return true;
  } catch {
    return false;
  }
}

export function clearPasswordChangedNotice(): void {
  try {
    sessionStorage.removeItem(KEY);
  } catch {
    // ignore
  }
}
