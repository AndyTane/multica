import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { I18nProvider } from "@multica/core/i18n/react";
import type { SupportedLocale } from "@multica/core/i18n";
import { UI_THEME_BOOTSTRAP } from "@multica/ui/components/common/ui-theme-script";
import { UiThemeRow } from "./ui-theme-row";

function renderRow(locale: SupportedLocale = "en") {
  return render(
    <I18nProvider locale={locale} resources={{ [locale]: { settings: {} } }}>
      <UiThemeRow />
    </I18nProvider>,
  );
}

const pressed = (name: string) => screen.getByRole("button", { name }).getAttribute("aria-pressed");

beforeEach(() => {
  localStorage.clear();
  document.documentElement.removeAttribute("data-ui-theme");
  document.documentElement.className = "";
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  localStorage.clear();
  document.documentElement.removeAttribute("data-ui-theme");
  document.documentElement.className = "";
});

describe("UiThemeRow", () => {
  it("defaults to UWorkers without adding an attribute or storage entry", () => {
    renderRow();
    expect(pressed("UWorkers")).toBe("true");
    expect(document.documentElement).not.toHaveAttribute("data-ui-theme");
    expect(localStorage.getItem("multica-ui-theme")).toBeNull();
  });

  it("applies Classic immediately and persists it on this device", async () => {
    renderRow();
    await userEvent.click(screen.getByRole("button", { name: "Classic" }));
    expect(document.documentElement).toHaveAttribute("data-ui-theme", "classic");
    expect(localStorage.getItem("multica-ui-theme")).toBe("classic");
    expect(pressed("Classic")).toBe("true");
  });

  it("removes the attribute and persists UWorkers when switching back", async () => {
    document.documentElement.setAttribute("data-ui-theme", "classic");
    localStorage.setItem("multica-ui-theme", "classic");
    renderRow();
    expect(pressed("Classic")).toBe("true");
    await userEvent.click(screen.getByRole("button", { name: "UWorkers" }));
    expect(document.documentElement).not.toHaveAttribute("data-ui-theme");
    expect(localStorage.getItem("multica-ui-theme")).toBe("uworkers");
    expect(pressed("UWorkers")).toBe("true");
  });

  it.each(["", "dark"])("preserves color mode classes %j and the system preference", async (className) => {
    document.documentElement.className = className;
    localStorage.setItem("theme", "system");
    renderRow();
    await userEvent.click(screen.getByRole("button", { name: "Classic" }));
    expect(document.documentElement.className).toBe(className);
    await userEvent.click(screen.getByRole("button", { name: "UWorkers" }));
    expect(document.documentElement.className).toBe(className);
    expect(localStorage.getItem("theme")).toBe("system");
  });

  it("renders Chinese copy for a Chinese locale", () => {
    renderRow("zh-Hans");
    expect(within(screen.getByRole("group", { name: "界面风格" })).getByRole("button", { name: "原版" })).toBeVisible();
  });

  it("still switches when storage writes are unavailable", async () => {
    vi.spyOn(localStorage, "setItem").mockImplementation(() => { throw new Error("Storage disabled"); });
    renderRow();
    await userEvent.click(screen.getByRole("button", { name: "Classic" }));
    expect(document.documentElement).toHaveAttribute("data-ui-theme", "classic");
    expect(pressed("Classic")).toBe("true");
  });

  it("follows style changes and storage clears from another tab", () => {
    renderRow();
    act(() => window.dispatchEvent(new StorageEvent("storage", { key: "multica-ui-theme", newValue: "classic" })));
    expect(pressed("Classic")).toBe("true");
    act(() => window.dispatchEvent(new StorageEvent("storage", { key: null })));
    expect(document.documentElement).not.toHaveAttribute("data-ui-theme");
    expect(pressed("UWorkers")).toBe("true");
  });
});

describe("prepaint bootstrap", () => {
  it("ships the same generated parser-blocking Desktop script", () => {
    const desktop = readFileSync(resolve(process.cwd(), "../../apps/desktop/src/renderer/public/ui-theme-init.js"), "utf8");
    expect(desktop.split("\n").slice(1).join("\n").trim()).toBe(UI_THEME_BOOTSTRAP);
  });

  it.each([null, "uworkers", "unknown", "classic"])("restores stored style %j before React starts", (stored) => {
    if (stored) localStorage.setItem("multica-ui-theme", stored);
    new Function(UI_THEME_BOOTSTRAP)();
    expect(document.documentElement.getAttribute("data-ui-theme")).toBe(stored === "classic" ? "classic" : null);
  });

  it("keeps the default when storage reads are unavailable", () => {
    vi.spyOn(localStorage, "getItem").mockImplementation(() => { throw new Error("Storage disabled"); });
    expect(() => new Function(UI_THEME_BOOTSTRAP)()).not.toThrow();
    expect(document.documentElement).not.toHaveAttribute("data-ui-theme");
  });
});
