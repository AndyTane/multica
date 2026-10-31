import { fireEvent, render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";

const config = vi.hoisted(() => ({
  googleClientId: "",
  authConfigLoaded: true,
  localPasswordLoginEnabled: true,
  emailCodeLoginEnabled: false,
  openRegistrationEnabled: false,
}));
vi.mock("@multica/core/config", () => ({
  useConfigStore: (select: (s: typeof config) => unknown) => select(config),
}));
const auth = vi.hoisted(() => ({ adoptSession: vi.fn() }));
vi.mock("@multica/core/auth", () => ({
  useAuthStore: Object.assign(() => ({}), {
    getState: () => ({ adoptSession: auth.adoptSession }),
  }),
}));
vi.mock("@multica/views/platform", () => ({ DragStrip: () => null }));
vi.mock("@multica/views/auth", () => ({
  LoginPage: ({
    onGoogleLogin,
    onRegister,
    configReady,
  }: {
    onGoogleLogin?: () => void;
    onRegister?: () => void;
    configReady?: boolean;
  }) => (
    <>
      <span>Email login</span>
      <span>{configReady ? "config ready" : "config pending"}</span>
      {onGoogleLogin && <button onClick={onGoogleLogin}>Google</button>}
      {onRegister && <button onClick={onRegister}>Register</button>}
    </>
  ),
  RegistrationPage: ({
    onSuccess,
    onBack,
  }: {
    onSuccess: (result: { token: string; user: { id: string } }) => void;
    onBack?: () => void;
  }) => (
    <>
      <span>Registration step</span>
      <button
        onClick={() => onSuccess({ token: "session-token", user: { id: "user-1" } })}
      >
        Complete
      </button>
      <button onClick={onBack}>Back to login</button>
    </>
  ),
}));

import { DesktopLoginPage } from "./login";

beforeEach(() => {
  config.googleClientId = "";
  config.authConfigLoaded = true;
  config.localPasswordLoginEnabled = true;
  config.emailCodeLoginEnabled = false;
  auth.adoptSession.mockReset();
  window.desktopAPI = {
    runtimeConfig: {
      ok: true,
      config: {
        schemaVersion: 1,
        apiUrl: "http://localhost:8080",
        wsUrl: "ws://localhost:8080/ws",
        appUrl: "http://localhost:3000",
      },
    },
  } as typeof window.desktopAPI;
});

describe("Desktop auth methods", () => {
  it("keeps email and hides unconfigured Google", () => {
    render(<DesktopLoginPage />);
    expect(screen.getByText("Email login")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Google" })).toBeNull();
  });

  it("offers Google when the server supplies its client ID", () => {
    config.googleClientId = "configured-test-client";
    render(<DesktopLoginPage />);
    expect(screen.getByRole("button", { name: "Google" })).toBeInTheDocument();
  });

  it("passes the resolved config state to the login page", () => {
    config.authConfigLoaded = false;
    render(<DesktopLoginPage />);
    expect(screen.getByText("config pending")).toBeInTheDocument();
  });

  it("hides the registration entry when local password login is off", () => {
    config.localPasswordLoginEnabled = false;
    render(<DesktopLoginPage />);
    expect(screen.queryByRole("button", { name: "Register" })).toBeNull();
  });

  it("runs registration as a step inside the login page and adopts the session", () => {
    render(<DesktopLoginPage />);
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    expect(screen.getByText("Registration step")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: "Complete" }));
    expect(auth.adoptSession).toHaveBeenCalledWith("session-token", { id: "user-1" });
    expect(screen.getByText("Email login")).toBeInTheDocument();
  });

  it("returns to the login step from registration", () => {
    render(<DesktopLoginPage />);
    fireEvent.click(screen.getByRole("button", { name: "Register" }));
    fireEvent.click(screen.getByRole("button", { name: "Back to login" }));
    expect(screen.getByText("Email login")).toBeInTheDocument();
  });
});
