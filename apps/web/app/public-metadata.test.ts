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

it("uses a configured remote address for discovery", () => {
  vi.stubEnv("NEXT_PUBLIC_APP_URL", "https://work.example.test");
  expect(robots().sitemap).toEqual(["https://work.example.test/sitemap.xml"]);
  expect(sitemap()[0]?.url).toBe("https://work.example.test");
});
