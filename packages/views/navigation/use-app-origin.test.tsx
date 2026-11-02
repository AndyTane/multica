import { describe, expect, it } from "vitest";
import { renderHook } from "@testing-library/react";
import { NavigationProvider } from "./context";
import { useAppOrigin, useAppOrigins } from "./use-app-origin";
import type { NavigationAdapter } from "./types";

const adapter: NavigationAdapter = {
  push() {}, replace() {}, back() {}, pathname: "/", hash: "",
  searchParams: new URLSearchParams(),
  getShareableUrl: (path) => `http://localhost:3000${path}`,
  getExternalShareUrl: (path) => `https://public.example:14443${path}`,
};

describe("app origins", () => {
  it("reports both origins while keeping the singular API on the current entry", () => {
    const { result } = renderHook(() => ({ origin: useAppOrigin(), origins: useAppOrigins() }), {
      wrapper: ({ children }) => <NavigationProvider value={adapter}>{children}</NavigationProvider>,
    });
    expect(result.current).toEqual({ origin: "http://localhost:3000", origins: ["http://localhost:3000", "https://public.example:14443"] });
  });

  it("tolerates a missing provider", () => {
    expect(renderHook(() => useAppOrigins()).result.current).toEqual([]);
    expect(renderHook(() => useAppOrigin()).result.current).toBeNull();
  });

  it("ignores relative SSR or unconfigured desktop URLs", () => {
    const relative = { ...adapter, getShareableUrl: (path: string) => path, getExternalShareUrl: (path: string) => path };
    expect(renderHook(() => useAppOrigins(), {
      wrapper: ({ children }) => <NavigationProvider value={relative}>{children}</NavigationProvider>,
    }).result.current).toEqual([]);
  });
});
