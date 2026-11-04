import { pickLocale, type SupportedLocale } from "@multica/core/i18n";
import { createBrowserCookieLocaleAdapter } from "@multica/core/i18n/browser";

export function resolveEmergencyLocale(): SupportedLocale {
  const adapter = createBrowserCookieLocaleAdapter();
  let cookieLocale: string | null = null;

  try {
    cookieLocale = adapter.getUserChoice();
  } catch {
    // A blocked cookie should not be able to break the last-resort UI.
  }

  return pickLocale({ ...adapter, getUserChoice: () => cookieLocale });
}
