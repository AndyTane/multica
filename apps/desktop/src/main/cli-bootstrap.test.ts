// @vitest-environment node
import { mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { afterEach, expect, it, vi } from "vitest";

vi.mock("electron", () => ({ app: { getPath: () => "unused-test-path" } }));
import { ensureManagedCli } from "./cli-bootstrap";

afterEach(() => vi.unstubAllGlobals());

it("uses a specified local binary without downloading or executing it", async () => {
  const root = await mkdtemp(join(tmpdir(), "cli-local-test-"));
  const binaryPath = join(root, "fake-cli");
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  try {
    await writeFile(binaryPath, "test fixture, not executable");
    await expect(ensureManagedCli({ binaryPath })).resolves.toBe(binaryPath);
    expect(fetch).not.toHaveBeenCalled();
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});

it("fails locally if the specified binary is missing", async () => {
  const fetch = vi.fn();
  vi.stubGlobal("fetch", fetch);
  await expect(ensureManagedCli({ binaryPath: "missing-test-cli" })).rejects.toThrow("CLI not found");
  expect(fetch).not.toHaveBeenCalled();
});
