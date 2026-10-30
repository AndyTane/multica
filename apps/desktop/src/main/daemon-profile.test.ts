// @vitest-environment node
import { homedir } from "os";
import { join } from "path";
import { describe, expect, it } from "vitest";

import {
  DEFAULT_HEALTH_PORT,
  deriveProfileName,
  healthPortForProfile,
  profileArgs,
  profileConfigPath,
  profileDir,
  profileLogPath,
  profilePidPath,
  profileUserIdPath,
} from "./daemon-profile";

const MULTICA_DIR = join(homedir(), ".multica");
const DEFAULT_CLI_CONFIG = join(MULTICA_DIR, "config.json");

describe("deriveProfileName", () => {
  it("names the profile after the target host", () => {
    expect(deriveProfileName("https://api.example.test")).toBe(
      "u-workers-desktop-api.example.test",
    );
  });

  it("replaces the port colon so the name is path-safe", () => {
    expect(deriveProfileName("http://localhost:8080")).toBe(
      "u-workers-desktop-localhost-8080",
    );
  });

  it("rejects an unparseable URL", () => {
    expect(() => deriveProfileName("not a url")).toThrow(/valid API URL/);
  });
});

describe("profile paths", () => {
  it("always resolves under profiles/<name>", () => {
    const dir = join(MULTICA_DIR, "profiles", "u-workers-desktop-api.example.test");
    expect(profileDir("u-workers-desktop-api.example.test")).toBe(dir);
    expect(profileConfigPath("u-workers-desktop-api.example.test")).toBe(
      join(dir, "config.json"),
    );
    expect(profileLogPath("u-workers-desktop-api.example.test")).toBe(
      join(dir, "daemon.log"),
    );
    expect(profilePidPath("u-workers-desktop-api.example.test")).toBe(
      join(dir, "daemon.pid"),
    );
    expect(profileUserIdPath("u-workers-desktop-api.example.test")).toBe(
      join(dir, ".desktop-user-id"),
    );
  });

  // Regression: an unresolved profile used to resolve to ~/.multica, so Desktop
  // could overwrite server_url and token in the user's own CLI config. #6399.
  it("refuses to build a path for an unresolved profile", () => {
    expect(() => profileDir("")).toThrow(/unresolved/);
    expect(() => profileConfigPath("")).toThrow(/unresolved/);
    expect(() => profileLogPath("")).toThrow(/unresolved/);
    expect(() => profilePidPath("")).toThrow(/unresolved/);
    expect(() => profileUserIdPath("")).toThrow(/unresolved/);
  });

  it("never yields the default CLI config path for any input", () => {
    for (const name of ["u-workers-desktop-api.example.test", "u-workers-desktop-localhost-8080"]) {
      expect(profileConfigPath(name)).not.toBe(DEFAULT_CLI_CONFIG);
    }
    expect(() => profileConfigPath("")).toThrow();
  });
});

describe("profileArgs", () => {
  it("selects the Desktop-owned profile", () => {
    expect(profileArgs("u-workers-desktop-api.example.test")).toEqual([
      "--profile",
      "u-workers-desktop-api.example.test",
    ]);
  });

  // Regression: this returned [] for an unresolved profile, so the bundled CLI
  // ran against the user's default profile instead of Desktop's. #6399.
  it("refuses to spawn the CLI without a profile flag", () => {
    expect(() => profileArgs("")).toThrow(/unresolved/);
  });
});

describe("healthPortForProfile", () => {
  // Regression: this returned 19514 — the default profile's port — for an
  // unresolved profile, so Desktop would probe the user's own CLI daemon and
  // report it as its own. #6399.
  it("refuses to hand out a port for an unresolved profile", () => {
    expect(() => healthPortForProfile("")).toThrow(/unresolved/);
  });

  it("never derives the default profile's port", () => {
    for (const name of ["u-workers-desktop-api.example.test", "desktop", "x", "a".repeat(50)]) {
      expect(healthPortForProfile(name)).not.toBe(DEFAULT_HEALTH_PORT);
    }
  });

  it("derives a stable per-profile port above the default", () => {
    const port = healthPortForProfile("u-workers-desktop-api.example.test");
    expect(port).toBeGreaterThan(DEFAULT_HEALTH_PORT);
    expect(port).toBe(healthPortForProfile("u-workers-desktop-api.example.test"));
  });
});
