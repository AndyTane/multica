// @vitest-environment jsdom
import { afterEach, expect, it, vi } from "vitest";

afterEach(() => vi.restoreAllMocks());

it("does not report or replace the network APIs used by authorized work", async () => {
  const fetchRequest = vi.fn(async () => ({ ok: true }));
  const beaconRequest = vi.fn(() => false);
  const xhrRequest = vi.spyOn(XMLHttpRequest.prototype, "send").mockImplementation(() => {});
  const addListener = vi.spyOn(window, "addEventListener");
  vi.stubGlobal("fetch", fetchRequest);
  vi.stubGlobal("navigator", { sendBeacon: beaconRequest });
  try {
    const xhr = XMLHttpRequest;
    const webSocket = WebSocket;
    const analytics = await import("./index");
    expect(analytics.initAnalytics({ key: "test-project-key", host: "https://collector.invalid" })).toBe(false);
    analytics.identify("test-user");
    analytics.captureEvent("test-event", {}, { sendInstantly: true });
    analytics.captureException(new Error("test-error"));
    analytics.setPersonProperties({ source: ["search"] });
    analytics.resetAnalytics();
    expect(fetchRequest).not.toHaveBeenCalled();
    expect(xhrRequest).not.toHaveBeenCalled();
    expect(beaconRequest).not.toHaveBeenCalled();
    expect(addListener).not.toHaveBeenCalled();
    expect(globalThis.fetch).toBe(fetchRequest);
    expect(XMLHttpRequest).toBe(xhr);
    expect(WebSocket).toBe(webSocket);

    // Same global API stays available for explicitly configured connections.
    const response = await fetch("https://user-model.invalid/v1/messages", { method: "POST" });
    expect(response.ok).toBe(true);
    expect(fetchRequest).toHaveBeenCalledExactlyOnceWith(
      "https://user-model.invalid/v1/messages", { method: "POST" },
    );
  } finally {
    vi.unstubAllGlobals();
  }
});
