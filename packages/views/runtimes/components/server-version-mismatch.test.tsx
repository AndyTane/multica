// @vitest-environment jsdom

import { afterEach, describe, expect, it } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { I18nProvider } from "@multica/core/i18n/react";
import { configStore } from "@multica/core/config";
import enCommon from "../../locales/en/common.json";
import enRuntimes from "../../locales/en/runtimes.json";
import { ServerVersionMismatch } from "./server-version-mismatch";

const TEST_RESOURCES = { en: { common: enCommon, runtimes: enRuntimes } };

function renderMismatch(currentVersion: string | null, launchedBy?: string | null) {
  return render(
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <ServerVersionMismatch currentVersion={currentVersion} launchedBy={launchedBy} />
    </I18nProvider>,
  );
}

afterEach(() => {
  cleanup();
  configStore.getState().setServerVersion("");
});

// CLAWSTA-127: after a server upgrade, a machine still on the old Client-Kit
// must be visible from the runtime page without asking its owner.
describe("ServerVersionMismatch", () => {
  it("flags a CLI on a different release than the server", () => {
    configStore.getState().setServerVersion("v0.2.8");
    renderMismatch("v0.2.7");
    expect(screen.getByText("CLI v0.2.7 doesn't match server v0.2.8")).toBeInTheDocument();
  });

  it.each([
    ["the CLI matches the server", "0.2.8", null],
    ["the CLI is a local build", "v0.2.7-3-gabc1234", null],
    ["the CLI is Desktop-managed", "v0.2.7", "desktop"],
    ["the CLI version is unknown", null, null],
  ])("stays quiet when %s", (_, cli, launchedBy) => {
    configStore.getState().setServerVersion("v0.2.8");
    renderMismatch(cli, launchedBy);
    expect(screen.queryByText(/doesn't match server/)).not.toBeInTheDocument();
  });

  it("stays quiet before the server version is known", () => {
    renderMismatch("v0.2.7");
    expect(screen.queryByText(/doesn't match server/)).not.toBeInTheDocument();
  });
});
