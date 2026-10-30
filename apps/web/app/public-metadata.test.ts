// @vitest-environment node
import { afterEach, expect, it, vi } from "vitest";
import robots from "./robots";
import sitemap from "./sitemap";

afterEach(() => vi.unstubAllEnvs());

it("omits public discovery addresses when no public URL is configured", () => {
  vi.stubEnv("NEXT_PUBLIC_APP_URL", "");
  expect(robots().sitemap).toBeUndefined();
  expect(sitemap()).toEqual([]);
});

it("keeps retired marketing routes out of discovery even with a configured address", () => {
  vi.stubEnv("NEXT_PUBLIC_APP_URL", "https://work.example.test");
  expect(robots().rules).toEqual({ userAgent: "*", disallow: "/" });
  expect(robots().sitemap).toBeUndefined();
  expect(sitemap()).toEqual([]);
});
