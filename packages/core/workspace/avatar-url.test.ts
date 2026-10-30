import { describe, expect, it, vi } from "vitest";
import { resolvePublicFileUrl, resolvePublicFileUrlWithBase } from "./avatar-url";

vi.mock("../api", () => ({
  api: {
    getBaseUrl: () => "http://127.0.0.1:8080",
  },
}));

describe("resolvePublicFileUrlWithBase", () => {
  it.each(["https://www.gravatar.com/avatar/hash", "//secure.gravatar.com/avatar/hash", "https://cdn.libravatar.org/avatar/hash"])("omits unsolicited avatar services: %s", (url) => {
    expect(resolvePublicFileUrlWithBase(url, "http://localhost:8080")).toBeNull();
  });
  it("resolves root-relative URLs against base URL", () => {
    expect(resolvePublicFileUrlWithBase("/uploads/a.png", "http://127.0.0.1:8080")).toBe(
      "http://127.0.0.1:8080/uploads/a.png",
    );
  });

  it("trims trailing slash in base URL", () => {
    expect(resolvePublicFileUrlWithBase("/upload/a.png", "http://127.0.0.1:8080/")).toBe(
      "http://127.0.0.1:8080/upload/a.png",
    );
  });

  it("keeps absolute URLs unchanged", () => {
    expect(resolvePublicFileUrlWithBase("https://cdn.example.com/a.png", "http://127.0.0.1:8080")).toBe(
      "https://cdn.example.com/a.png",
    );
  });

  it("returns null for empty values", () => {
    expect(resolvePublicFileUrlWithBase(null, "http://127.0.0.1:8080")).toBeNull();
    expect(resolvePublicFileUrlWithBase(undefined, "http://127.0.0.1:8080")).toBeNull();
  });
});

describe("resolvePublicFileUrl", () => {
  it("uses API base URL implicitly", () => {
    expect(resolvePublicFileUrl("/uploads/a.png")).toBe("http://127.0.0.1:8080/uploads/a.png");
  });
});
