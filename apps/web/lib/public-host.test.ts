// @vitest-environment node
import { describe, expect, it } from "vitest";

import { resolvePublicAppUrl } from "./public-host";

describe("public metadata URL", () => {
  it("omits public URLs without explicit configuration", () => {
    expect(resolvePublicAppUrl({})).toBeUndefined();
  });
  it("uses the configured public address without query or fragment", () => {
    expect(resolvePublicAppUrl({ NEXT_PUBLIC_APP_URL: "https://work.example.test/?x=1#top" })).toBe("https://work.example.test");
  });
  it.each(["/relative", "file:///tmp/site", "https://user:secret@example.test"])("rejects %s", (value) => {
    expect(resolvePublicAppUrl({ NEXT_PUBLIC_APP_URL: value })).toBeUndefined();
  });
});
