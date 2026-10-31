import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../test/i18n";
import { configStore } from "@multica/core/config";
import { ApiError } from "@multica/core/api";
import enAuth from "../locales/en/auth.json";

const navigation = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("../navigation", () => ({
  useNavigation: () => ({
    push: navigation.push,
    replace: vi.fn(),
  }),
}));

const apiMocks = vi.hoisted(() => ({
  redeemRegistrationInvite: vi.fn(),
  openRegistration: vi.fn(),
}));
vi.mock("@multica/core/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@multica/core/api")>();
  return {
    ...actual,
    api: {
      ...actual.api,
      redeemRegistrationInvite: apiMocks.redeemRegistrationInvite,
      openRegistration: apiMocks.openRegistration,
    },
  };
});

import { RegistrationPage } from "./registration-page";

const USER = {
  id: "user-1",
  name: "New Person",
  email: "new@example.test",
} as never;

function setConfig(overrides: {
  authConfigLoaded?: boolean;
  openRegistrationEnabled?: boolean;
  localPasswordLoginEnabled?: boolean;
}) {
  act(() => {
    configStore.setState({
      authConfigLoaded: true,
      openRegistrationEnabled: false,
      localPasswordLoginEnabled: true,
      ...overrides,
    });
  });
}

beforeEach(() => {
  apiMocks.redeemRegistrationInvite.mockReset();
  apiMocks.openRegistration.mockReset();
  navigation.push.mockReset();
  setConfig({});
  window.history.replaceState(null, "", "/register");
});

afterEach(() => {
  cleanup();
});

describe("RegistrationPage mode", () => {
  it("renders only the title until the auth config resolves", () => {
    setConfig({ authConfigLoaded: false });
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    expect(screen.getByText(enAuth.registration.title)).toBeInTheDocument();
    expect(document.querySelector("#registration-form")).toBeNull();
  });

  it("shows an empty invitation field when open registration is off", () => {
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    const invite = screen.getByLabelText(
      enAuth.registration.invite_label,
    ) as HTMLInputElement;
    expect(invite.value).toBe("");
    expect(screen.getByText(enAuth.registration.invite_only_hint)).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: enAuth.registration.mode_open })).toBeNull();
  });

  it("offers the invite/open switch when open registration is enabled", async () => {
    setConfig({ openRegistrationEnabled: true });
    const user = userEvent.setup();
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    expect(screen.getByText(enAuth.registration.email_label)).toBeInTheDocument();
    await user.click(
      screen.getByRole("button", { name: enAuth.registration.mode_invite }),
    );
    expect(
      screen.getByLabelText(enAuth.registration.invite_label),
    ).toBeInTheDocument();
  });

  it("prefills from the URL fragment and strips it from the address bar", () => {
    window.history.replaceState(null, "", "/register#token=fragment-secret");
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    const invite = screen.getByLabelText(
      enAuth.registration.invite_label,
    ) as HTMLInputElement;
    expect(invite.value).toBe("fragment-secret");
    expect(window.location.hash).toBe("");
  });

  it("shows an unavailable notice when password login is disabled", () => {
    setConfig({ localPasswordLoginEnabled: false });
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    expect(screen.getByText(enAuth.registration.unavailable)).toBeInTheDocument();
    expect(document.querySelector("#registration-form")).toBeNull();
  });
});

describe("RegistrationPage submission", () => {
  it("extracts the token from a pasted invitation link", async () => {
    const user = userEvent.setup();
    apiMocks.redeemRegistrationInvite.mockResolvedValue({ token: "jwt", user: USER });
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    const invite = screen.getByLabelText(enAuth.registration.invite_label);
    await user.type(invite, "https://app.example/register#token=pasted-secret");
    await user.type(
      screen.getByLabelText(enAuth.registration.name_label),
      "New Person",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.username_label),
      "new.person",
    );
    const password = screen.getByLabelText(enAuth.registration.password_label);
    await user.type(password, "a sufficiently long password");
    await user.type(
      screen.getByLabelText(enAuth.registration.confirm_label),
      "a sufficiently long password{enter}",
    );
    await waitFor(() =>
      expect(apiMocks.redeemRegistrationInvite).toHaveBeenCalledWith({
        token: "pasted-secret",
        name: "New Person",
        username: "new.person",
        password: "a sufficiently long password",
      }),
    );
  });

  it("clears only the password fields when the server refuses", async () => {
    const user = userEvent.setup();
    apiMocks.redeemRegistrationInvite.mockRejectedValue(
      new ApiError("invalid or expired invitation", 400, "Bad Request", {
        code: "invalid_invite",
      }),
    );
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    const invite = screen.getByLabelText(enAuth.registration.invite_label) as HTMLInputElement;
    await user.type(invite, "pasted-secret");
    const name = screen.getByLabelText(enAuth.registration.name_label) as HTMLInputElement;
    await user.type(name, "New Person");
    const username = screen.getByLabelText(
      enAuth.registration.username_label,
    ) as HTMLInputElement;
    await user.type(username, "new.person");
    const password = screen.getByLabelText(
      enAuth.registration.password_label,
    ) as HTMLInputElement;
    await user.type(password, "a sufficiently long password");
    const confirm = screen.getByLabelText(
      enAuth.registration.confirm_label,
    ) as HTMLInputElement;
    await user.type(confirm, "a sufficiently long password{enter}");
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(/invalid or has expired/i),
    );
    expect(invite.value).toBe("pasted-secret");
    expect(name.value).toBe("New Person");
    expect(username.value).toBe("new.person");
    expect(password.value).toBe("");
    expect(confirm.value).toBe("");
    // The refusal names the credential, so focus goes to the invitation field —
    // the first thing this retry has to change.
    await waitFor(() => expect(document.activeElement).toBe(invite));
  });

  it("reports the localized server error instead of a raw code", async () => {
    const user = userEvent.setup();
    apiMocks.redeemRegistrationInvite.mockRejectedValue(
      new ApiError("invalid or expired invitation", 400, "Bad Request", {
        code: "invalid_invite",
      }),
    );
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    await user.type(
      screen.getByLabelText(enAuth.registration.invite_label),
      "pasted-secret",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.name_label),
      "New Person",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.username_label),
      "new.person",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.password_label),
      "a sufficiently long password",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.confirm_label),
      "a sufficiently long password{enter}",
    );
    await waitFor(() =>
      expect(screen.getByRole("alert")).toHaveTextContent(
        /invalid or has expired/i,
      ),
    );
    expect(screen.queryByText("invalid_invite")).toBeNull();
  });

  it("hands the session to the host without navigating", async () => {
    const user = userEvent.setup();
    const onSuccess = vi.fn();
    apiMocks.redeemRegistrationInvite.mockResolvedValue({ token: "jwt", user: USER });
    renderWithI18n(<RegistrationPage onSuccess={onSuccess} />);
    await user.type(
      screen.getByLabelText(enAuth.registration.invite_label),
      "pasted-secret",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.name_label),
      "New Person",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.username_label),
      "new.person",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.password_label),
      "a sufficiently long password",
    );
    await user.type(
      screen.getByLabelText(enAuth.registration.confirm_label),
      "a sufficiently long password{enter}",
    );
    await waitFor(() =>
      expect(onSuccess).toHaveBeenCalledWith({ token: "jwt", user: USER }),
    );
    expect(navigation.push).not.toHaveBeenCalled();
  });

  it("routes the back action through the navigation adapter", async () => {
    const user = userEvent.setup();
    renderWithI18n(<RegistrationPage onSuccess={vi.fn()} />);
    await user.click(
      screen.getByRole("button", { name: enAuth.registration.back }),
    );
    expect(navigation.push).toHaveBeenCalledWith("/login");
  });
});
