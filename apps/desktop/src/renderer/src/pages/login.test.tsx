import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
const config = vi.hoisted(() => ({ googleClientId: "" }));
vi.mock("@multica/core/config", () => ({ useConfigStore: (select: (s: typeof config) => unknown) => select(config) }));
vi.mock("@multica/views/platform", () => ({ DragStrip: () => null }));
vi.mock("@multica/views/auth", () => ({ LoginPage: ({ onGoogleLogin }: { onGoogleLogin?: () => void }) => <><span>Email login</span>{onGoogleLogin && <button onClick={onGoogleLogin}>Google</button>}</> }));
import { DesktopLoginPage } from "./login";
beforeEach(() => {
  config.googleClientId = "";
  window.desktopAPI = { runtimeConfig: { ok: true, config: { schemaVersion: 1, apiUrl: "http://localhost:8080", wsUrl: "ws://localhost:8080/ws", appUrl: "http://localhost:3000" } } } as typeof window.desktopAPI;
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
});
