import { renderHook } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { fetchLatestRelease } from "./github-release";
import { useGithubStars } from "./use-github-stars";

afterEach(() => vi.unstubAllGlobals());

it("does not query release or star APIs on render", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  expect(renderHook(() => useGithubStars()).result.current).toBeNull();
  expect(await fetchLatestRelease()).toEqual({ version: null, publishedAt: null, htmlUrl: null, assets: {} });
  expect(fetch).not.toHaveBeenCalled();
});
