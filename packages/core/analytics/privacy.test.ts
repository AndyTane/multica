// @vitest-environment jsdom
import { afterEach, describe, expect, it, vi } from "vitest";

vi.mock("posthog-js", () => ({
  default: {
    init: vi.fn(), register: vi.fn(), identify: vi.fn(), reset: vi.fn(),
    capture: vi.fn(), captureException: vi.fn(),
  },
}));

import posthog from "posthog-js";
import {
  captureEvent, captureException, captureSignupSource, identify,
  initAnalytics, resetAnalytics, setPersonProperties,
} from "./index";
import { EXTERNAL_REPORTING_ENABLED } from "./privacy";

afterEach(() => {
  vi.clearAllMocks();
  window.history.replaceState({}, "", "/");
  document.cookie = "multica_signup_source=; path=/; max-age=0";
});

describe("local build reporting policy", () => {
  it("never starts or emits to the SDK even with a configured project key", () => {
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
    for (const method of Object.values(posthog)) expect(method).not.toHaveBeenCalled();
    expect(onCaptured).not.toHaveBeenCalled();
  });

  it("removes old attribution cookies without collecting a new source", () => {
    document.cookie = "multica_signup_source=previous-campaign; path=/";
    window.history.replaceState({}, "", "/?utm_source=private-campaign");
    captureSignupSource();
    expect(document.cookie).not.toContain("multica_signup_source");
  });
});
