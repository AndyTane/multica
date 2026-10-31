import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../test/i18n";
import type { AgentRuntime, Workspace } from "@multica/core/types";
const fake = vi.hoisted(() => ({ complete: vi.fn(), bootstrap: vi.fn().mockResolvedValue({ chatSession: { id: "chat-test" } }) }));
vi.mock("@multica/core/auth", () => ({ useAuthStore: (select: (s: unknown) => unknown) => select({ user: { id: "user-test" } }) }));
vi.mock("@multica/core/workspace", () => ({ useWorkspaceList: () => ({ workspaces: [], ready: true }) }));
vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "https://service.example.test" } }));
vi.mock("../auth", () => ({ useLogout: () => vi.fn() }));
vi.mock("@multica/core/onboarding", async (original) => ({ ...await original<object>(), completeOnboarding: fake.complete, useBootstrapMika: () => ({ mutateAsync: fake.bootstrap }) }));
vi.mock("./steps/step-workspace", () => ({ StepWorkspace: ({ onCreated }: { onCreated: (ws: Workspace) => void }) => <button onClick={() => onCreated({ id: "ws-test", slug: "test" } as Workspace)}>Create fake workspace</button> }));
vi.mock("./steps/step-runtime-connect", () => ({ StepRuntimeConnect: ({ onNext }: { onNext: (rt: AgentRuntime, model: string) => void }) => <button onClick={() => onNext({ id: "rt-test", name: "Fake runtime" } as AgentRuntime, "model-test")}>Select fake model</button> }));
import { OnboardingFlow } from "./onboarding-flow";

describe("local onboarding", () => {
  it("visits service, workspace, runtime/model and task; model work requires the final action", async () => {
    const done = vi.fn();
    renderWithI18n(<OnboardingFlow onComplete={done} />);
    await userEvent.click(screen.getByRole("button", { name: "Continue to workspace" }));
    await userEvent.click(screen.getByRole("button", { name: "Create fake workspace" }));
    await userEvent.click(screen.getByRole("button", { name: "Select fake model" }));
    expect(screen.getByRole("heading", { name: "Start your first task" })).toBeInTheDocument();
    expect(fake.bootstrap).not.toHaveBeenCalled();
    expect(screen.queryByText(/about you|waitlist|download desktop/i)).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Start with Siman" }));
    expect(fake.bootstrap).toHaveBeenCalledWith(expect.objectContaining({ workspaceSlug: "test", runtimeId: "rt-test", model: "model-test" }));
    expect(fake.complete).toHaveBeenCalledWith("full", "ws-test");
    expect(done).toHaveBeenCalledWith(expect.objectContaining({ id: "ws-test" }), { kind: "chat", sessionId: "chat-test" });
  });
});
