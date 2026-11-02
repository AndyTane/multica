// @vitest-environment node
import { describe, expect, it } from "vitest";
import { normalizePublicAppUrl } from "./public-app-url";
import { configStore } from "./index";

describe("public frontend origin", () => {
  it.each([
    [" https://work.example.test:14443/ ", "https://work.example.test:14443"],
    ["http://192.168.50.10:3000", "http://192.168.50.10:3000"],
    ["https://[fd00::10]:8443", "https://[fd00::10]:8443"],
  ])("normalizes %s", (raw, want) => {
    expect(normalizePublicAppUrl(raw)).toBe(want);
  });

  it.each([
    undefined, null, 42, {}, "", "not a URL", "ftp://work.example.test", "//work.example.test",
    "https://user:password@work.example.test", "https://work.example.test/path",
    "https://work.example.test?token=secret", "https://work.example.test#secret",
    "https://work.example.test:0", "https://work.example.test:99999", "https://work.example.test:",
    "http://localhost:3000", "http://LOCALHOST.:3000", "http://foo.localhost", "http://127.4.3.2",
    "http://127.1", "http://2130706433", "http://0x7f000001", "http://0.0.0.0", "http://[::]",
    "http://[::1]", "http://[::ffff:127.0.0.1]", "http://[fe80::1%25eth0]",
    "https://bad_host", "https://bad..host", "https://work.example.test\\other", "https://work.example.test\n",
  ])("ignores unsafe or missing value %j", (raw) => {
    expect(normalizePublicAppUrl(raw)).toBe("");
  });

  it("stores and clears the public address independently of daemon config", () => {
    const state = configStore.getState();
    state.setDaemonConfig({ daemonAppUrl: "https://daemon.example.test" });
    state.setPublicAppUrl("https://work.example.test:14443/");
    expect(configStore.getState().publicAppUrl).toBe("https://work.example.test:14443");
    state.setPublicAppUrl();
    expect(configStore.getState().publicAppUrl).toBe("");
    state.setPublicAppUrl("http://localhost:3000");
    expect(configStore.getState().publicAppUrl).toBe("");
    expect(configStore.getState().daemonAppUrl).toBe("https://daemon.example.test");
    state.setDaemonConfig({});
  });
});
