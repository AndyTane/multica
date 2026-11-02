import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { renderWithI18n } from "../../test/i18n";
import { ApiError } from "@multica/core/api";
import enSettings from "../../locales/en/settings.json";

const navigation = vi.hoisted(() => ({
  push: vi.fn(),
  replace: vi.fn(),
  origin: "https://app.example",
}));
vi.mock("../../navigation", () => ({
  useOptionalNavigation: () => ({
    getExternalShareUrl: (path: string) => `${navigation.origin}${path}`,
    getShareableUrl: (path: string) => `http://localhost:3000${path}`,
  }),
  useNavigation: () => ({ push: navigation.push, replace: navigation.replace }),
}));

const toastMocks = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("sonner", () => ({
  toast: { success: toastMocks.success, error: toastMocks.error },
}));

const authState = vi.hoisted(() => ({
  user: { id: "admin-1", name: "Admin", email: "admin@example.test", is_instance_admin: true },
}));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: typeof authState) => unknown) => selector(authState),
}));

const adminUser = {
  id: "admin-1",
  name: "Admin",
  email: "admin@example.test",
  is_instance_admin: true,
  onboarded: true,
  has_password: true,
  invite_expires_at: null,
  created_at: "2026-01-01T00:00:00Z",
};
const pendingUser = {
  id: "pending-1",
  name: "Pending Person",
  email: "pending@example.test",
  is_instance_admin: false,
  onboarded: false,
  has_password: false,
  invite_expires_at: "2026-11-01T00:00:00Z",
  created_at: "2026-01-02T00:00:00Z",
};
const ownerUser = {
  id: "owner-1",
  name: "Owner Person",
  email: "owner@example.test",
  is_instance_admin: false,
  onboarded: true,
  has_password: true,
  invite_expires_at: null,
  created_at: "2026-01-03T00:00:00Z",
};

const apiMocks = vi.hoisted(() => ({
  listInstanceUsers: vi.fn(),
  getRegistrationSettings: vi.fn(),
  updateRegistrationSettings: vi.fn(),
  addInstanceUser: vi.fn(),
  regenerateInstanceInvite: vi.fn(),
  revokeInstanceInvite: vi.fn(),
  deleteInstanceUser: vi.fn(),
  getInstanceUserDeletePreview: vi.fn(),
}));
vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return { ...actual, api: { ...actual.api, ...apiMocks } };
});

import { InstanceUsersTab } from "./instance-users-tab";

const clipboard = { writeText: vi.fn() };
// user-event installs its own navigator.clipboard stub in setup(); creating the
// instance first keeps that from replacing the double this suite asserts on.
let user: ReturnType<typeof userEvent.setup>;

function renderTab() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <InstanceUsersTab />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  user = userEvent.setup();
  Object.values(apiMocks).forEach((mock) => mock.mockReset());
  toastMocks.success.mockReset();
  toastMocks.error.mockReset();
  clipboard.writeText.mockReset();
  clipboard.writeText.mockResolvedValue(undefined);
  vi.stubGlobal("navigator", { clipboard });
  authState.user = {
    id: "admin-1",
    name: "Admin",
    email: "admin@example.test",
    is_instance_admin: true,
  };
  apiMocks.listInstanceUsers.mockResolvedValue({
    users: [adminUser, pendingUser, ownerUser],
  });
  apiMocks.getRegistrationSettings.mockResolvedValue({
    open_registration_enabled: false,
  });
});

afterEach(() => {
  cleanup();
});

describe("InstanceUsersTab", () => {
  it("turns the registration switch into a server write and rolls back on failure", async () => {
    apiMocks.updateRegistrationSettings.mockRejectedValue(
      new ApiError("failed to save setting", 500, "Internal Server Error"),
    );
    apiMocks.getRegistrationSettings
      .mockResolvedValueOnce({ open_registration_enabled: false })
      .mockResolvedValue({ open_registration_enabled: false });
    renderTab();
    const toggle = await screen.findByRole("switch", {
      name: enSettings.instance_admin.open_registration,
    });
    expect(toggle).toHaveAttribute("aria-checked", "false");
    await user.click(toggle);
    await waitFor(() =>
      expect(apiMocks.updateRegistrationSettings).toHaveBeenCalledWith({
        open_registration_enabled: true,
      }),
    );
    await waitFor(() => expect(toastMocks.error).toHaveBeenCalled());
    // No optimistic update: the switch still shows the server's value.
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });

  it("shows the full invitation link, copies it, and clears it on 完成", async () => {
    apiMocks.addInstanceUser.mockResolvedValue({
      user: pendingUser,
      invite_token: "invite-secret-token",
      invite_expires_at: "2026-11-01T00:00:00Z",
    });
    renderTab();
    await user.type(
      screen.getByLabelText(enSettings.instance_admin.name_label),
      "Pending Person",
    );
    await user.type(
      screen.getByLabelText(enSettings.instance_admin.email_label),
      "pending@example.test",
    );
    await user.click(
      screen.getByRole("button", { name: enSettings.instance_admin.add_user }),
    );

    const link = await screen.findByRole("textbox", {
      name: enSettings.instance_admin.invite_link_label,
    });
    expect((link as HTMLInputElement).value).toBe(
      "https://app.example/register#token=invite-secret-token",
    );
    expect(link).toHaveAttribute("readonly");

    await user.click(
      screen.getByRole("button", { name: enSettings.instance_admin.copy_invite }),
    );
    expect(clipboard.writeText).toHaveBeenCalledWith(
      "https://app.example/register#token=invite-secret-token",
    );
    expect(
      screen.getByRole("button", { name: enSettings.instance_admin.copied }),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: enSettings.instance_admin.done }),
    );
    await waitFor(() =>
      expect(
        screen.queryByRole("textbox", {
          name: enSettings.instance_admin.invite_link_label,
        }),
      ).toBeNull(),
    );
    expect(screen.queryByText(/invite-secret-token/)).toBeNull();
  });

  it("reports a clipboard failure and selects the link for manual copying", async () => {
    clipboard.writeText.mockRejectedValue(new Error("denied"));
    apiMocks.addInstanceUser.mockResolvedValue({
      user: pendingUser,
      invite_token: "invite-secret-token",
      invite_expires_at: "2026-11-01T00:00:00Z",
    });
    renderTab();
    await user.type(
      screen.getByLabelText(enSettings.instance_admin.name_label),
      "Pending Person",
    );
    await user.type(
      screen.getByLabelText(enSettings.instance_admin.email_label),
      "pending@example.test",
    );
    await user.click(
      screen.getByRole("button", { name: enSettings.instance_admin.add_user }),
    );
    const link = await screen.findByRole("textbox", {
      name: enSettings.instance_admin.invite_link_label,
    });
    await user.click(
      screen.getByRole("button", { name: enSettings.instance_admin.copy_invite }),
    );
    await waitFor(() =>
      expect(
        screen.getByText(enSettings.instance_admin.copy_failed),
      ).toBeInTheDocument(),
    );
    const input = link as HTMLInputElement;
    expect(input.selectionStart).toBe(0);
    expect(input.selectionEnd).toBe(input.value.length);
  });

  it("never offers a delete entry for the signed-in account", async () => {
    renderTab();
    await screen.findByText("Pending Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    // self + pending + owner = 3 rows, but only the two other accounts get a menu.
    expect(menus).toHaveLength(2);
  });

  it("disables confirmation when the preview lists a blocker", async () => {
    apiMocks.getInstanceUserDeletePreview.mockResolvedValue({
      can_delete: false,
      blockers: [{ code: "workspace_owner", count: 1 }],
      removes: ["account", "membership"],
    });
    renderTab();
    await screen.findByText("Owner Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[1]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText(enSettings.instance_admin.blocker.workspace_owner),
      ).toBeInTheDocument(),
    );
    expect(
      within(dialog).getByRole("button", {
        name: enSettings.instance_admin.delete_confirm,
      }),
    ).toBeDisabled();
  });

  it("labels chat-history and active-task blockers and keeps confirmation disabled", async () => {
    apiMocks.getInstanceUserDeletePreview.mockResolvedValue({
      can_delete: false,
      blockers: [
        { code: "chat_history", count: 2 },
        { code: "active_task", count: 1 },
      ],
      removes: ["account"],
    });
    renderTab();
    await screen.findByText("Owner Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[1]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    expect(
      within(dialog).getByText(/Has chat history \(2\)/),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByText(/Has work in progress/),
    ).toBeInTheDocument();
    expect(
      within(dialog).getByRole("button", {
        name: enSettings.instance_admin.delete_confirm,
      }),
    ).toBeDisabled();
    // U2: the cancel control is a generic Cancel, never the invite panel's 完成.
    const cancel = within(dialog).getByRole("button", {
      name: enSettings.instance_admin.cancel,
    });
    expect(cancel).toBeInTheDocument();
    expect(
      within(dialog).queryByRole("button", {
        name: enSettings.instance_admin.done,
      }),
    ).toBeNull();
  });

  it("merges unknown blockers into one dependency count instead of dropping them", async () => {
    apiMocks.getInstanceUserDeletePreview.mockResolvedValue({
      can_delete: false,
      blockers: [
        { code: "future_dependency", count: 3 },
        { code: "another_future_dependency", count: 2 },
      ],
      removes: ["account"],
    });
    renderTab();
    await screen.findByText("Owner Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[1]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText("5 more dependencies must be resolved first"),
      ).toBeInTheDocument(),
    );
    expect(within(dialog).queryByText(/future_dependency/)).toBeNull();
    expect(
      within(dialog).getByRole("button", {
        name: enSettings.instance_admin.delete_confirm,
      }),
    ).toBeDisabled();
  });

  it("fails closed when the preview request fails", async () => {
    apiMocks.getInstanceUserDeletePreview.mockRejectedValue(
      new ApiError("failed to inspect", 500, "Internal Server Error"),
    );
    renderTab();
    await screen.findByText("Owner Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[1]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    await waitFor(() =>
      expect(
        within(dialog).getByText(enSettings.instance_admin.delete_preview_failed),
      ).toBeInTheDocument(),
    );
    expect(
      within(dialog).getByRole("button", {
        name: enSettings.instance_admin.delete_confirm,
      }),
    ).toBeDisabled();
  });

  it("renders the open-registration row with its consequence text and control", async () => {
    renderTab();
    expect(
      await screen.findByText(enSettings.instance_admin.open_registration),
    ).toBeInTheDocument();
    expect(
      screen.getByText(enSettings.instance_admin.open_registration_hint),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("switch", {
        name: enSettings.instance_admin.open_registration,
      }),
    ).toBeInTheDocument();
  });

  it("fails closed when the preview is malformed", async () => {
    apiMocks.getInstanceUserDeletePreview.mockResolvedValue({
      can_delete: false,
      blockers: [],
      removes: [],
    });
    renderTab();
    await screen.findByText("Pending Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[0]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    expect(
      within(dialog).getByRole("button", {
        name: enSettings.instance_admin.delete_confirm,
      }),
    ).toBeDisabled();
  });

  it("confirms a preview with no blockers and deletes the account", async () => {
    apiMocks.getInstanceUserDeletePreview.mockResolvedValue({
      can_delete: true,
      blockers: [],
      removes: ["account", "password"],
    });
    apiMocks.deleteInstanceUser.mockResolvedValue(undefined);
    renderTab();
    await screen.findByText("Pending Person");
    const menus = screen.getAllByRole("button", {
      name: enSettings.instance_admin.action_menu,
    });
    await user.click(menus[0]!);
    await user.click(
      await screen.findByRole("menuitem", {
        name: enSettings.instance_admin.action_delete,
      }),
    );
    const dialog = await screen.findByRole("alertdialog");
    const confirm = within(dialog).getByRole("button", {
      name: enSettings.instance_admin.delete_confirm,
    });
    await waitFor(() => expect(confirm).toBeEnabled());
    await user.click(confirm);
    await waitFor(() =>
      expect(apiMocks.deleteInstanceUser).toHaveBeenCalledWith("pending-1"),
    );
  });

  it("hides the whole tab from an ordinary account", async () => {
    authState.user = {
      id: "plain-1",
      name: "Plain",
      email: "plain@example.test",
      is_instance_admin: false,
    };
    renderTab();
    expect(
      await screen.findByText(enSettings.instance_admin.admin_only),
    ).toBeInTheDocument();
    expect(apiMocks.listInstanceUsers).not.toHaveBeenCalled();
  });
});
