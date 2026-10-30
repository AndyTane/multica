// @vitest-environment node
import { describe, expect, it } from "vitest";
import { docsHref, docsLocalePrefix } from "./docs-locale";

describe("docsLocalePrefix", () => {
  it.each([
    ["en", ""],
    [undefined, ""],
    ["zh-Hans", "/zh"],
    ["ja", "/ja"],
    ["ko", "/ko"],
    ["fr", "/fr"],
  ])("maps %s to %j", (language, expected) => {
    expect(docsLocalePrefix(language)).toBe(expected);
  });
});

it("uses the same origin unless the Desktop app URL is provided", () => {
  expect(docsHref("agents", "fr")).toBe("/docs/fr/agents");
  expect(docsHref("agents", "zh-Hans", "https://my-server.test/")).toBe("https://my-server.test/docs/zh/agents");
});
