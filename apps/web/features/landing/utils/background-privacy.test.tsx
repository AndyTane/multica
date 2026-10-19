import { renderHook, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { useGithubStars } from "./use-github-stars";
import { fetchLatestRelease } from "./github-release";

afterEach(() => vi.unstubAllGlobals());

it("does not query GitHub for marketing stars or official releases", async () => {
  const request = vi.fn(() => Promise.reject(new Error("Network disabled in privacy test")));
  vi.stubGlobal("fetch", request);
  const { result } = renderHook(() => useGithubStars());
  const release = await fetchLatestRelease();
  await waitFor(() => expect(result.current).toBeNull());
  expect(release.version).toBeNull();
  expect(request).not.toHaveBeenCalled();
});
