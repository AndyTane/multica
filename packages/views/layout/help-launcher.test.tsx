import { cloneElement, type ReactElement, type ReactNode } from "react";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { configStore } from "@multica/core/config";
import enLayout from "../locales/en/layout.json";
import { isDesktopShell } from "../platform/local-directory";
import { HelpLauncher } from "./help-launcher";

const apiMock = vi.hoisted(() => ({ getConfig: vi.fn() }));
vi.mock("@multica/core/api", () => ({ api: apiMock }));
const clipboardMock = vi.hoisted(() => ({ copyText: vi.fn() }));
vi.mock("@multica/ui/lib/clipboard", () => clipboardMock);
const toastMock = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({ toast: toastMock }));

// The download entry is gated on the desktop-shell probe, which reads a
// preload-injected bridge that jsdom never has. Mock it so both platforms are
// reachable from the same suite.
vi.mock("../platform/local-directory", () => ({
  isDesktopShell: vi.fn(() => false),
}));

// The UI language the mocked i18n instance reports; drives locale-aware links.
const i18nState = vi.hoisted(() => ({ language: "en" }));

// react-i18next isn't initialised in the views test env, so resolve the
// selector against the real en/layout.json to assert on actual copy.
vi.mock("../i18n", () => ({
  useT: () => ({
    i18n: i18nState,
    t: (
      sel: (r: typeof enLayout) => string,
      vars?: Record<string, string>,
    ) => {
      const template = sel(enLayout);
      return vars
        ? template.replace(/\{\{(\w+)\}\}/g, (_, key) => String(vars[key] ?? ""))
        : template;
    },
  }),
}));

// Follows the app-sidebar.test.tsx convention of flattening the Base UI
// dropdown primitives to plain children so the menu content is always in
// the DOM, instead of exercising the real portal/open-state interaction.
//
// The mock deliberately preserves ONE real invariant: DropdownMenuLabel wraps
// Base UI's Menu.GroupLabel, whose useMenuGroupRootContext() throws when it has
// no Menu.Group ancestor. A plain-<div> mock silently swallowed that contract,
// which is exactly how MUL-4819 shipped — a version row rendered outside a
// DropdownMenuGroup crashed the whole app (no error boundary above the sidebar)
// the moment the Help menu opened. Mirroring the throw here keeps the guard.
// The group context lives inside the factory so it survives vi.mock hoisting.
vi.mock("@multica/ui/components/ui/dropdown-menu", async () => {
  const { createContext, useContext } = await import("react");
  const GroupContext = createContext(false);
  return {
    // Exposes onOpenChange as a button so tests can simulate opening.
    DropdownMenu: ({
      children,
      onOpenChange,
    }: {
      children: ReactNode;
      onOpenChange?: (open: boolean) => void;
    }) => (
      <>
        <button type="button" onClick={() => onOpenChange?.(true)}>
          open-menu
        </button>
        {children}
      </>
    ),
    DropdownMenuContent: ({ children }: { children: ReactNode }) => <>{children}</>,
    // Base UI's `render` prop swaps in a caller-supplied element (here the
    // <a>) and adopts the item's children. Flattening it to a bare fragment —
    // as this mock originally did — would drop the anchor entirely and make an
    // href assertion silently unfalsifiable.
    DropdownMenuItem: ({
      children,
      render,
      onClick,
    }: {
      children: ReactNode;
      render?: ReactElement;
      onClick?: () => void;
    }) =>
      render ? (
        cloneElement(render, undefined, children)
      ) : (
        <button type="button" onClick={onClick}>
          {children}
        </button>
      ),
    DropdownMenuGroup: ({ children }: { children: ReactNode }) => (
      <GroupContext.Provider value={true}>{children}</GroupContext.Provider>
    ),
    DropdownMenuLabel: ({ children }: { children: ReactNode }) => {
      if (!useContext(GroupContext)) {
        throw new Error(
          "Base UI: MenuGroupRootContext is missing. Menu group parts must be used within <Menu.Group>.",
        );
      }
      return <div>{children}</div>;
    },
    DropdownMenuSeparator: () => null,
    DropdownMenuTrigger: ({ children }: { children: ReactNode }) => <>{children}</>,
  };
});

beforeEach(() => {
  vi.mocked(isDesktopShell).mockReturnValue(false);
  i18nState.language = "en";
});

afterEach(() => {
  configStore.getState().setServerVersion("");
  configStore.getState().setWebBuildVersion("");
  vi.clearAllMocks();
});

describe("HelpLauncher", () => {
  it("does not show a version row when the server omits it", () => {
    render(<HelpLauncher />);
    expect(screen.queryByText(/U Workers/)).not.toBeInTheDocument();
  });

  it("shows the server version and commit once /api/config resolves them", () => {
    configStore.getState().setServerVersion("v0.2.8", "012edaa");
    render(<HelpLauncher />);
    expect(screen.getByText("U Workers v0.2.8")).toBeInTheDocument();
    expect(screen.getByText("Commit 012edaa")).toBeInTheDocument();
  });

  it("copies version and commit together", async () => {
    clipboardMock.copyText.mockResolvedValue(true);
    configStore.getState().setServerVersion("v0.2.8", "012edaa");
    render(<HelpLauncher />);
    fireEvent.click(screen.getByText("U Workers v0.2.8"));
    expect(clipboardMock.copyText).toHaveBeenCalledWith("U Workers v0.2.8 (012edaa)");
    await waitFor(() => expect(toastMock.success).toHaveBeenCalledWith("Version copied"));
  });

  // A tab left open across an upgrade keeps the config it booted with, so
  // opening the menu must re-read the live server version.
  it("re-reads the server version when the menu opens", async () => {
    apiMock.getConfig.mockResolvedValue({ server_version: "v0.2.8", server_commit: "012edaa" });
    configStore.getState().setServerVersion("v0.2.7", "abc1234");
    render(<HelpLauncher />);
    fireEvent.click(screen.getByText("open-menu"));
    expect(await screen.findByText("U Workers v0.2.8")).toBeInTheDocument();
  });

  it("offers a reload when the page is older than the server", () => {
    configStore.getState().setWebBuildVersion("v0.2.7");
    configStore.getState().setServerVersion("v0.2.8");
    render(<HelpLauncher />);
    expect(screen.getByText("Reload to update")).toBeInTheDocument();
  });

  it("does not offer a reload for a matching or untagged web build", () => {
    configStore.getState().setServerVersion("v0.2.8");
    configStore.getState().setWebBuildVersion("v0.2.8");
    const { unmount } = render(<HelpLauncher />);
    expect(screen.queryByText(/Reload to update/)).not.toBeInTheDocument();
    unmount();
    // The package.json fallback of an untagged build is not a release tag.
    configStore.getState().setWebBuildVersion("0.2.7");
    render(<HelpLauncher />);
    expect(screen.queryByText(/Reload to update/)).not.toBeInTheDocument();
  });

  // MUL-6462: after web onboarding the desktop download CTA was unreachable —
  // no entry anywhere in the app, so users had to remember the URL or detour
  // through the marketing site. The Help menu is the persistent home for it.

  it.each([
    ["en", "/docs"],
    ["zh-Hans", "/docs/zh"],
    ["fr", "/docs/fr"],
  ])("links Docs to the %s docs", (language, href) => {
    i18nState.language = language;
    render(<HelpLauncher />);
    expect(screen.getByRole("link", { name: /Docs/ })).toHaveAttribute(
      "href",
      href,
    );
  });

  // AppSidebar is shared: apps/desktop renders the same component tree. Without
  // this gate the desktop app would offer to download the desktop app.
  it("hides the download entry inside the desktop shell", () => {
    vi.mocked(isDesktopShell).mockReturnValue(true);
    render(<HelpLauncher />);
    expect(screen.queryByText("Desktop app")).not.toBeInTheDocument();
    // The rest of the menu is unaffected by the gate.
    expect(screen.getByText("Docs")).toBeInTheDocument();
  });
});
