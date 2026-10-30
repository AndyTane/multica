// @vitest-environment jsdom

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { UpdateSection } from "./update-section";

vi.mock("@multica/core/analytics/privacy", () => ({ EXTERNAL_BACKGROUND_REQUESTS_ENABLED: true }));

const TEST_RESOURCES = { en: { common: enCommon, runtimes: enRuntimes } };

vi.mock("@multica/core/api", () => ({
  api: {
    initiateUpdate: vi.fn(),
    getUpdateResult: vi.fn(),
  },
}));

function renderSection(props: {
  runtimeId: string | null;
  launchedBy?: string | null;
  currentVersion?: string;
}) {
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <UpdateSection
        runtimeId={props.runtimeId}
        currentVersion={props.currentVersion ?? "v0.4.0"}
        isOnline
        launchedBy={props.launchedBy}
      />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

describe("UpdateSection read-only status", () => {
  it("shows the installed version without unsolicited freshness claims", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ tag_name: "v0.4.0" }),
      }),
    );

    renderSection({ runtimeId: null });

    expect(screen.queryByText("Latest")).toBeNull();
    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(screen.queryByText("Read-only")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Update" }),
    ).not.toBeInTheDocument();
  });

  it("shows the Desktop manager without exposing an update action", () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ tag_name: "v0.4.0" }),
      }),
    );

    renderSection({ runtimeId: null, launchedBy: "desktop" });

    expect(screen.getByText("Managed by Desktop")).toBeInTheDocument();
    expect(screen.queryByText("Read-only")).not.toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: "Update" }),
    ).not.toBeInTheDocument();
  });

  it("does not advertise an unfetched version to a read-only viewer", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ tag_name: "v0.4.0" }),
      }),
    );

    renderSection({ runtimeId: null, currentVersion: "v0.3.17" });

    expect(screen.queryByText("available")).toBeNull();
    expect(screen.queryByText("Read-only")).toBeNull();
    expect(
      screen.queryByRole("button", { name: "Update" }),
    ).not.toBeInTheDocument();
  });
});

// A daemon built from source reports either the ldflags default ("dev") or a
// `git describe` string. Neither can be ordered against a release tag, so the
// UI must not claim an upgrade is available — a manual update is legitimate
// precisely because a human made an informed decision, and that decision has to
// rest on something we actually parsed.
describe("UpdateSection non-release versions", () => {
  const LATEST = "v0.4.20";

  let clock = Date.now();

  beforeEach(() => {
    clock += 60 * 60 * 1000;
    vi.spyOn(Date, "now").mockReturnValue(clock);
    vi.stubGlobal(
      "fetch",
      vi.fn().mockResolvedValue({
        ok: true,
        json: async () => ({ tag_name: LATEST }),
      }),
    );
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  const NON_RELEASE_VERSIONS = [
    "dev",
    "v0.4.17-12-gabc1234",
    "v0.4.17-dirty",
    "v0.4.17-rc1",
    "v0.4",
  ];

  for (const currentVersion of NON_RELEASE_VERSIONS) {
    it(`offers no update and claims no state for "${currentVersion}"`, async () => {
      renderSection({ runtimeId: "runtime-1", currentVersion });

      expect(await screen.findByText("Local build")).toBeInTheDocument();
      expect(
        screen.queryByRole("button", { name: "Update" }),
      ).not.toBeInTheDocument();
      expect(screen.queryByText("available")).not.toBeInTheDocument();
      // "Latest" would be just as unfounded as "update available".
      expect(screen.queryByText("Latest")).not.toBeInTheDocument();
    });
  }

  it("does not offer an upstream update for a released version", async () => {
    renderSection({ runtimeId: "runtime-1", currentVersion: "v0.4.17" });

    expect(
      screen.queryByRole("button", { name: "Update" }),
    ).toBeNull();
    expect(screen.queryByText("available")).toBeNull();
    expect(screen.queryByText("Local build")).not.toBeInTheDocument();
  });

  it("does not infer freshness from a locally displayed release tag", async () => {
    renderSection({ runtimeId: "runtime-1", currentVersion: LATEST });

    expect(screen.queryByText("Latest")).toBeNull();
    expect(globalThis.fetch).not.toHaveBeenCalled();
    expect(
      screen.queryByRole("button", { name: "Update" }),
    ).not.toBeInTheDocument();
    expect(screen.queryByText("Local build")).not.toBeInTheDocument();
  });
});
