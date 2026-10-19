// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";

const originalBeacon = Object.getOwnPropertyDescriptor(navigator, "sendBeacon");
afterEach(() => {
  vi.unstubAllGlobals();
  if (originalBeacon) Object.defineProperty(navigator, "sendBeacon", originalBeacon);
  else Reflect.deleteProperty(navigator, "sendBeacon");
});

it("loads the real analytics dependency without making network requests", async () => {
  const fetchRequest = vi.fn(() => Promise.reject(new Error("Network disabled in privacy test")));
  const xhrRequest = vi.fn(() => { throw new Error("Network disabled in privacy test"); });
  const beaconRequest = vi.fn(() => false);
  vi.stubGlobal("fetch", fetchRequest);
  vi.stubGlobal("XMLHttpRequest", class {
    // The SDK constructs an XHR for a capability probe; only send is network I/O.
    withCredentials = false;
    send() { xhrRequest(); }
  });
  Object.defineProperty(navigator, "sendBeacon", { configurable: true, value: beaconRequest });
  const analytics = await import("./index");
  expect(analytics.initAnalytics({ key: "test-project-key", host: "https://collector.invalid" })).toBe(false);
  analytics.identify("test-user");
  analytics.captureEvent("test-event", {}, { sendInstantly: true });
  analytics.captureException(new Error("test-error"));
  analytics.setPersonProperties({ source: ["search"] });
  expect(fetchRequest).not.toHaveBeenCalled();
  expect(xhrRequest).not.toHaveBeenCalled();
  expect(beaconRequest).not.toHaveBeenCalled();
});
