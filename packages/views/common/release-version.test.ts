// @vitest-environment node
import { describe, expect, it } from "vitest";
import { sameReleaseVersion } from "./release-version";

describe("sameReleaseVersion", () => {
  it.each([
    ["v0.2.7", "v0.2.7", true],
    ["v0.2.7", "0.2.7", true],
    ["v0.2.7", "v0.2.8", false],
    ["v0.2.7-3-gabc1234", "v0.2.7", null],
    ["dev", "v0.2.7", null],
    ["", "v0.2.7", null],
    [undefined, "v0.2.7", null],
  ])("%s vs %s -> %s", (a, b, want) => {
    expect(sameReleaseVersion(a, b)).toBe(want);
  });
});
