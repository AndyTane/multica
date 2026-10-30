// @vitest-environment node
import { afterEach, describe, expect, it, vi } from "vitest";
import { captureSignupSource, detectClientType, initAnalytics } from "./index";

afterEach(() => vi.unstubAllGlobals());

describe("analytics caller boundary", () => {
  it("remains safe during server rendering", () => {
    expect(initAnalytics(undefined)).toBe(false);
    expect(initAnalytics(null)).toBe(false);
    expect(captureSignupSource).not.toThrow();
    expect(detectClientType()).toBe("web");
  });

  it("keeps runtime classification local", () => {
    vi.stubGlobal("window", {});
    vi.stubGlobal("navigator", { userAgent: "Mozilla/5.0" });
    expect(detectClientType()).toBe("web");
    vi.stubGlobal("window", { desktopAPI: {} });
    expect(detectClientType()).toBe("desktop");
    vi.stubGlobal("window", { electron: {} });
    expect(detectClientType()).toBe("desktop");
    vi.stubGlobal("window", {});
    vi.stubGlobal("navigator", { userAgent: "Electron/1.0" });
    expect(detectClientType()).toBe("desktop");
  });
});
