import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "@multica/views/locales/en/common.json";
import enAuth from "@multica/views/locales/en/auth.json";
import enSettings from "@multica/views/locales/en/settings.json";
import { ApiError } from "@multica/core/api";
import type { ReactNode } from "react";

const TEST_RESOURCES = {
  en: { common: enCommon, auth: enAuth, settings: enSettings },
};

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => (
    <I18nProvider locale="en" resources={TEST_RESOURCES}>
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    </I18nProvider>
  );
}

const {
  mockGetShareLinkInfo,
  mockJoinByShareLink,
  mockListWorkspaces,
  mockRefreshMe,
  mockPush,
  searchParamsState,
  authStateRef,
} = vi.hoisted(() => ({
  mockGetShareLinkInfo: vi.fn(),
  mockJoinByShareLink: vi.fn(),
  mockListWorkspaces: vi.fn(),
  mockRefreshMe: vi.fn(),
  mockPush: vi.fn(),
  searchParamsState: { params: new URLSearchParams() },
  authStateRef: {
    state: {
      user: null as null | { id: string; email: string },
      isLoading: false,
      refreshMe: () => mockRefreshMe(),
    },
  },
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({ push: mockPush, replace: vi.fn() }),
  usePathname: () => "/join",
  useSearchParams: () => searchParamsState.params,
}));

vi.mock("@multica/core/auth", async () => {
  const actual = await vi.importActual<typeof import("@multica/core/auth")>(
    "@multica/core/auth",
  );
  const useAuthStore = Object.assign(
    (selector: (s: typeof authStateRef.state) => unknown) =>
      selector(authStateRef.state),
    { getState: () => authStateRef.state },
  );
  return { ...actual, useAuthStore };
});

vi.mock("@multica/core/api", async () => {
  const actual = await vi.importActual<typeof import("@multica/core/api")>(
    "@multica/core/api",
  );
  return {
    ...actual,
    api: {
      getShareLinkInfo: mockGetShareLinkInfo,
      joinByShareLink: mockJoinByShareLink,
      listWorkspaces: mockListWorkspaces,
    },
  };
});

import JoinPage from "./page";

const TEST_INFO = {
  workspace_name: "Acme",
  workspace_slug: "acme",
  creator_name: "Alice",
  role: "member",
};

describe("JoinPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    searchParamsState.params = new URLSearchParams({ code: "abc123" });
    authStateRef.state.user = null;
    authStateRef.state.isLoading = false;
    mockGetShareLinkInfo.mockResolvedValue(TEST_INFO);
    mockListWorkspaces.mockResolvedValue([]);
    mockRefreshMe.mockResolvedValue(undefined);
  });

  it("shows the workspace preview without joining", async () => {
    render(<JoinPage />, { wrapper: createWrapper() });

    await waitFor(() =>
      expect(screen.getByText(/You're invited to Acme/)).toBeInTheDocument(),
    );
    expect(screen.getByText("Invited by Alice")).toBeInTheDocument();
    expect(mockJoinByShareLink).not.toHaveBeenCalled();
  });

  it("shows an Administrator badge when the link grants admin access", async () => {
    mockGetShareLinkInfo.mockResolvedValue({
      ...TEST_INFO,
      role: "admin",
    });

    render(<JoinPage />, { wrapper: createWrapper() });

    await waitFor(() =>
      expect(
        screen.getByText("You'll join this workspace as"),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText("Administrator")).toBeInTheDocument();
  });

  it("shows a Member badge when the link grants member access", async () => {
    mockGetShareLinkInfo.mockResolvedValue({
      ...TEST_INFO,
      role: "member",
    });

    render(<JoinPage />, { wrapper: createWrapper() });

    await waitFor(() =>
      expect(
        screen.getByText("You'll join this workspace as"),
      ).toBeInTheDocument(),
    );
    expect(screen.getByText("Member")).toBeInTheDocument();
  });

  it("shows an error when the link is invalid", async () => {
    mockGetShareLinkInfo.mockRejectedValue(new Error("not found"));

    render(<JoinPage />, { wrapper: createWrapper() });

    await waitFor(() =>
      expect(
        screen.getByText("This invite link is invalid or has expired."),
      ).toBeInTheDocument(),
    );
    expect(mockJoinByShareLink).not.toHaveBeenCalled();
  });

  it("sends unauthenticated users to login when they click join", async () => {
    const user = userEvent.setup();
    authStateRef.state.user = null;
    render(<JoinPage />, { wrapper: createWrapper() });

    await user.click(
      await screen.findByRole("button", { name: "Log In to Join" }),
    );
    expect(mockPush).toHaveBeenCalledWith(
      "/login?next=" + encodeURIComponent("/join?code=abc123"),
    );
    expect(mockJoinByShareLink).not.toHaveBeenCalled();
  });

  it("joins only when an authenticated user clicks the button", async () => {
    const user = userEvent.setup();
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockResolvedValue({
      member: { id: "m1" },
      workspace_id: "ws1",
      workspace_slug: "acme",
    });

    render(<JoinPage />, { wrapper: createWrapper() });

    await user.click(
      await screen.findByRole("button", { name: "Join Workspace" }),
    );
    await waitFor(() => expect(mockJoinByShareLink).toHaveBeenCalledTimes(1));
    // The join succeeded — the page should transition to the Joined state
    // (navigation happens after a short delay).
    await waitFor(() =>
      expect(screen.getByText("Joined!")).toBeInTheDocument(),
    );
  });

  it("refreshes identity before entering the invite workspace for an existing member", async () => {
    const user = userEvent.setup();
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockRejectedValue(
      new Error("you are already a member of this workspace"),
    );
    // The account's first workspace differs from the invited one — the page
    // must route to the workspace THIS invite points at (acme), not the first.
    mockListWorkspaces.mockResolvedValue([{ id: "ws0", slug: "other" }]);

    render(<JoinPage />, { wrapper: createWrapper() });

    await user.click(
      await screen.findByRole("button", { name: "Join Workspace" }),
    );
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/acme/issues"));
    expect(mockRefreshMe).toHaveBeenCalledTimes(1);
  });

  it("waits for both identity and workspace synchronization before navigation", async () => {
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockResolvedValue({ workspace_id: "ws1", workspace_slug: "acme" });
    let finishIdentity!: () => void;
    let finishWorkspaces!: (value: unknown[]) => void;
    mockRefreshMe.mockReturnValue(new Promise<void>((resolve) => { finishIdentity = resolve; }));
    mockListWorkspaces.mockReturnValue(new Promise<unknown[]>((resolve) => { finishWorkspaces = resolve; }));
    render(<JoinPage />, { wrapper: createWrapper() });
    await userEvent.click(await screen.findByRole("button", { name: "Join Workspace" }));
    await waitFor(() => expect(mockRefreshMe).toHaveBeenCalledTimes(1));
    expect(mockPush).not.toHaveBeenCalled();
    await act(async () => finishIdentity());
    expect(mockPush).not.toHaveBeenCalled();
    finishWorkspaces([{ id: "ws1", slug: "acme" }]);
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/acme/issues"));
  });

  it.each(["identity", "workspaces"])("retries %s synchronization without joining twice", async (failure) => {
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockResolvedValue({ workspace_id: "ws1", workspace_slug: "acme" });
    const failingRequest = failure === "identity" ? mockRefreshMe : mockListWorkspaces;
    failingRequest.mockRejectedValueOnce(new Error("temporarily offline"));
    render(<JoinPage />, { wrapper: createWrapper() });
    await userEvent.click(await screen.findByRole("button", { name: "Join Workspace" }));
    expect(await screen.findByRole("alert")).toHaveTextContent("Your membership is saved");
    expect(mockPush).not.toHaveBeenCalled();
    await userEvent.click(screen.getByRole("button", { name: "Retry sync" }));
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/acme/issues"));
    expect(mockJoinByShareLink).toHaveBeenCalledTimes(1);
    expect(mockRefreshMe).toHaveBeenCalledTimes(2);
    expect(mockListWorkspaces).toHaveBeenCalledTimes(2);
  });

  it("keeps the existing member's invite target when synchronization needs retry", async () => {
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockRejectedValue(new Error("you are already a member of this workspace"));
    mockListWorkspaces.mockResolvedValue([{ id: "ws0", slug: "other" }]);
    mockRefreshMe.mockRejectedValueOnce(new Error("temporarily offline"));
    render(<JoinPage />, { wrapper: createWrapper() });
    await userEvent.click(await screen.findByRole("button", { name: "Join Workspace" }));
    await screen.findByRole("alert");
    await userEvent.click(screen.getByRole("button", { name: "Retry sync" }));
    await waitFor(() => expect(mockPush).toHaveBeenCalledWith("/acme/issues"));
    expect(mockJoinByShareLink).toHaveBeenCalledTimes(1);
    expect(mockPush).not.toHaveBeenCalledWith("/other/issues");
  });

  it("shows an error when join fails for another reason", async () => {
    const user = userEvent.setup();
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockRejectedValue(
      new Error("share link not found or expired"),
    );

    render(<JoinPage />, { wrapper: createWrapper() });

    await user.click(
      await screen.findByRole("button", { name: "Join Workspace" }),
    );
    await waitFor(() =>
      expect(
        screen.getByText("share link not found or expired"),
      ).toBeInTheDocument(),
    );
  });

  it.each([
    [
      "seat_capacity_full",
      "All purchased member seats are in use. Ask a workspace admin to add a seat before trying again.",
    ],
    [
      "seat_capacity_unavailable",
      "Member capacity could not be verified. Please try again.",
    ],
  ])("maps %s to a user-facing capacity message", async (code, message) => {
    const user = userEvent.setup();
    authStateRef.state.user = { id: "u1", email: "a@b.com" };
    mockJoinByShareLink.mockRejectedValue(
      new ApiError("server fallback", 409, "Conflict", { code }),
    );

    render(<JoinPage />, { wrapper: createWrapper() });

    await user.click(
      await screen.findByRole("button", { name: "Join Workspace" }),
    );
    await waitFor(() => expect(screen.getByText(message)).toBeInTheDocument());
  });
});
