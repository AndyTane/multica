import { getDefaultNormalizer, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import {
  CLI_VERSION_PLACEHOLDER,
  CliInstallCommand,
  cliBuildVersion,
  cliInstallCommands,
} from "./cli-install-command";

const CLI_INSTALL_COMMANDS = cliInstallCommands(CLI_VERSION_PLACEHOLDER);

// Build commands span two lines; match them as written, not collapsed.
const MULTILINE = { normalizer: getDefaultNormalizer({ collapseWhitespace: false }) };

const LABELS = {
  group: "Choose your platform",
  macosLinux: "macOS / Linux",
  windows: "Windows",
};

function renderSwitch() {
  return render(
    <CliInstallCommand labels={LABELS}>
      {(command) => <code>{command}</code>}
    </CliInstallCommand>,
  );
}

describe("CliInstallCommand", () => {
  it("defaults to the macOS / Linux installer", () => {
    renderSwitch();

    expect(
      screen.getByRole("tablist", { name: LABELS.group }),
    ).toBeInTheDocument();
    expect(screen.getByRole("tab", { name: "macOS / Linux" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(
      screen.getByText(CLI_INSTALL_COMMANDS.macosLinux, MULTILINE),
    ).toBeInTheDocument();
    expect(screen.queryByText(CLI_INSTALL_COMMANDS.windows, MULTILINE)).toBeNull();
  });

  it("hands the Windows command to the row when that tab is picked", async () => {
    const user = userEvent.setup();
    renderSwitch();

    await user.click(screen.getByRole("tab", { name: "Windows" }));

    expect(screen.getByRole("tab", { name: "Windows" })).toHaveAttribute(
      "aria-selected",
      "true",
    );
    expect(screen.getByText(CLI_INSTALL_COMMANDS.windows, MULTILINE)).toBeInTheDocument();
    expect(screen.queryByText(CLI_INSTALL_COMMANDS.macosLinux, MULTILINE)).toBeNull();
  });

  // A real tablist: arrow keys move between platforms without a click.
  it("switches platform with the arrow keys", async () => {
    const user = userEvent.setup();
    renderSwitch();

    await user.click(screen.getByRole("tab", { name: "macOS / Linux" }));
    await user.keyboard("{ArrowRight}");

    expect(screen.getByText(CLI_INSTALL_COMMANDS.windows, MULTILINE)).toBeInTheDocument();
  });
});

describe("cliInstallCommands", () => {
  // The next step runs `u-workers`, so both builds must name it that and put
  // its folder on PATH; an env var alone does not make the shell find it.
  it("builds u-workers and puts it on this terminal's PATH", () => {
    const commands = cliInstallCommands("v0.2.0");

    expect(commands.macosLinux).toBe(
      `(cd server && go build -ldflags "-X main.version=v0.2.0" -o bin/u-workers ./cmd/multica)
export PATH="$PWD/server/bin:$PATH"`,
    );
    expect(commands.windows).toBe(
      `pwsh -File scripts/build-local-cli.ps1 -Version v0.2.0
$env:Path = "$PWD\\server\\bin;$env:Path"`,
    );
  });
});

describe("cliBuildVersion", () => {
  it.each([
    ["0.2.0", "v0.2.0"],
    ["v0.2.0", "v0.2.0"],
    [" v1.4.2-3-gabc123 ", "v1.4.2-3-gabc123"],
    ["", CLI_VERSION_PLACEHOLDER],
    [undefined, CLI_VERSION_PLACEHOLDER],
    ["dev", CLI_VERSION_PLACEHOLDER],
    ["v0.2.0; rm -rf ~", CLI_VERSION_PLACEHOLDER],
  ])("maps %j to %j", (input, expected) => {
    expect(cliBuildVersion(input)).toBe(expected);
  });
});
