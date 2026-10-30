// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  captureEvent, captureException, captureSignupSource, identify,
  initAnalytics, resetAnalytics, setPersonProperties,
} from "./index";
import { EXTERNAL_REPORTING_ENABLED } from "./privacy";

afterEach(() => {
  vi.clearAllMocks();
  vi.unstubAllEnvs();
  window.history.replaceState({}, "", "/");
  document.cookie = "multica_signup_source=; path=/; max-age=0";
});

describe("local build reporting policy", () => {
  it.each(["", "0", "false", "1", "true"])("cannot be enabled by old environment settings (%s)", (disabled) => {
    vi.stubEnv("ANALYTICS_DISABLED", disabled);
    vi.stubEnv("POSTHOG_API_KEY", "test-project-key");
    vi.stubEnv("POSTHOG_HOST", "https://collector.invalid");
    vi.stubEnv("NEXT_PUBLIC_POSTHOG_KEY", "test-project-key");
    expect(EXTERNAL_REPORTING_ENABLED).toBe(false);
    const onCaptured = vi.fn();
    identify("test-user", { email: "private@example.test" });
    captureEvent("before-init", { content: "private" });
    expect(initAnalytics({ key: "test-project-key", host: "https://collector.invalid" })).toBe(false);
    identify("test-user");
    captureEvent("after-init", {}, { sendInstantly: true, onCaptured });
    captureException(new Error("private error"));
    setPersonProperties({ source: ["search"] });
    resetAnalytics();
    expect(initAnalytics({ key: "another-project", host: "https://collector.invalid" })).toBe(false);
    expect(onCaptured).not.toHaveBeenCalled();
  });

  it("removes old attribution cookies without collecting a new source", () => {
    document.cookie = "multica_signup_source=previous-campaign; path=/";
    window.history.replaceState({}, "", "/?utm_source=private-campaign");
    captureSignupSource();
    expect(document.cookie).not.toContain("multica_signup_source");
  });
});
