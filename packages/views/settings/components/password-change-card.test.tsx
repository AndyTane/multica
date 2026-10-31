import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, screen, waitFor } from "@testing-library/react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../../test/i18n";
import { ApiError } from "@multica/core/api";
import enSettings from "../../locales/en/settings.json";

const navigation = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("../../navigation", () => ({
  useNavigation: () => ({ push: navigation.push, replace: vi.fn() }),
}));

const apiMocks = vi.hoisted(() => ({
  changeCurrentPassword: vi.fn(),
}));
vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return {
    ...actual,
    api: {
      ...actual.api,
      changeCurrentPassword: apiMocks.changeCurrentPassword,
    },
  };
});

const authMocks = vi.hoisted(() => ({ logout: vi.fn() }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: (selector: (state: { logout: () => void }) => unknown) =>
    selector({ logout: authMocks.logout }),
}));

import { PasswordChangeCard } from "./password-change-card";

function renderCard() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false }, mutations: { retry: false } },
  });
  return renderWithI18n(
    <QueryClientProvider client={queryClient}>
      <PasswordChangeCard />
    </QueryClientProvider>,
  );
}

beforeEach(() => {
  apiMocks.changeCurrentPassword.mockReset();
  authMocks.logout.mockReset();
  navigation.push.mockReset();
});

afterEach(() => {
  cleanup();
});

async function fillForm(user: ReturnType<typeof userEvent.setup>) {
  await user.type(
    screen.getByLabelText(enSettings.password_change.current_label),
    "the current password",
  );
  await user.type(
    screen.getByLabelText(enSettings.password_change.new_label),
    "the replacement password",
  );
  await user.type(
    screen.getByLabelText(enSettings.password_change.confirm_label),
    "the replacement password",
  );
}

describe("PasswordChangeCard", () => {
  it("submits on Enter and signs out before showing the login notice", async () => {
    const user = userEvent.setup();
    apiMocks.changeCurrentPassword.mockResolvedValue({ reauth_required: true });
    renderCard();
    await fillForm(user);
    await user.type(
      screen.getByLabelText(enSettings.password_change.confirm_label),
      "{enter}",
    );
    await waitFor(() =>
      expect(apiMocks.changeCurrentPassword).toHaveBeenCalledWith({
        current_password: "the current password",
        new_password: "the replacement password",
      }),
    );
    await waitFor(() => expect(authMocks.logout).toHaveBeenCalled());
    expect(navigation.push).toHaveBeenCalledWith("/login?reason=password_changed");
  });

  it("clears all three fields, focuses the current password and localizes the error", async () => {
    const user = userEvent.setup();
    apiMocks.changeCurrentPassword.mockRejectedValue(
      new ApiError("current password is incorrect", 400, "Bad Request", {
        code: "invalid_current_password",
      }),
    );
    renderCard();
    await fillForm(user);
    await user.click(
      screen.getByRole("button", { name: enSettings.password_change.submit }),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/current password is incorrect/i),
    );
    expect(screen.queryByText("invalid_current_password")).toBeNull();
    for (const label of [
      enSettings.password_change.current_label,
      enSettings.password_change.new_label,
      enSettings.password_change.confirm_label,
    ]) {
      expect(
        (screen.getByLabelText(label) as HTMLInputElement).value,
      ).toBe("");
    }
    expect(document.activeElement).toBe(
      screen.getByLabelText(enSettings.password_change.current_label),
    );
    expect(authMocks.logout).not.toHaveBeenCalled();
  });

  it("reports a mismatch without calling the server", async () => {
    const user = userEvent.setup();
    renderCard();
    await user.type(
      screen.getByLabelText(enSettings.password_change.current_label),
      "the current password",
    );
    await user.type(
      screen.getByLabelText(enSettings.password_change.new_label),
      "the replacement password",
    );
    await user.type(
      screen.getByLabelText(enSettings.password_change.confirm_label),
      "something else entirely",
    );
    await user.click(
      screen.getByRole("button", { name: enSettings.password_change.submit }),
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        enSettings.password_change.error_mismatch,
      ),
    );
    expect(apiMocks.changeCurrentPassword).not.toHaveBeenCalled();
  });
});
