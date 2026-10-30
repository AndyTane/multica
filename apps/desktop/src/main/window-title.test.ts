// @vitest-environment node
import { describe, expect, it, vi } from "vitest";

import { formatWindowTitle, installWindowTitle } from "./window-title";

describe("formatWindowTitle", () => {
  it("appends the product name to a page title", () => {
    expect(formatWindowTitle("Issues")).toBe("Issues — U Workers");
    expect(formatWindowTitle("  MUL-12: Fix login ")).toBe(
      "MUL-12: Fix login — U Workers",
    );
  });

  it("keeps the bare product name for empty or pre-login titles", () => {
    expect(formatWindowTitle("")).toBe("U Workers");
    expect(formatWindowTitle(undefined)).toBe("U Workers");
    expect(formatWindowTitle("U Workers")).toBe("U Workers");
  });

  it("does not double the suffix", () => {
    expect(formatWindowTitle("Issues — U Workers")).toBe("Issues — U Workers");
  });
});

describe("installWindowTitle", () => {
  it("replaces the page-driven title with the suffixed one", () => {
    let handler: ((event: { preventDefault: () => void }, title: string) => void) | undefined;
    const window = {
      getTitle: vi.fn(() => "MUL-1: Ping"),
      setTitle: vi.fn(),
      on: vi.fn((name: string, fn: typeof handler) => {
        if (name === "page-title-updated") handler = fn;
      }),
    };

    installWindowTitle(window as never);
    expect(window.setTitle).toHaveBeenLastCalledWith("MUL-1: Ping — U Workers");

    const event = { preventDefault: vi.fn() };
    handler?.(event, "Projects");
    expect(event.preventDefault).toHaveBeenCalled();
    expect(window.setTitle).toHaveBeenLastCalledWith("Projects — U Workers");
  });
});
