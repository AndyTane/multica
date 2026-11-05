import { afterEach, describe, expect, it, vi } from "vitest";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import type { Agent } from "@multica/core/types";
import { renderWithI18n } from "../../test/i18n";
import { AgentDetailInspector } from "./agent-detail-inspector";

vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => null,
}));

vi.mock("@tanstack/react-query", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@tanstack/react-query")>()),
  useQuery: () => ({ data: undefined, isSuccess: false }),
}));

// The picker is the only way agent labels ever reached the agent settings
// form. Mocking it means the assertion below fails loudly if the row is
// re-added, instead of passing because a real picker rendered nothing.
vi.mock("../../labels/resource-label-picker", () => ({
  ResourceLabelPicker: () => <div data-testid="resource-label-picker" />,
}));

vi.mock("../../common/avatar-upload-control", () => ({
  AvatarUploadControl: () => <div data-testid="avatar-upload" />,
}));

vi.mock("./inspector/model-picker", () => ({
  ModelPicker: () => <div data-testid="model-picker" />,
}));

vi.mock("./inspector/runtime-picker", () => ({
  RuntimePicker: () => <div data-testid="runtime-picker" />,
}));

vi.mock("./inspector/thinking-prop-row", () => ({
  ThinkingSettingField: () => <div data-testid="thinking-field" />,
}));

vi.mock("./inspector/service-tier-setting-field", () => ({
  ServiceTierSettingField: () => <div data-testid="service-tier-field" />,
}));

const agent = {
  execution_timeouts_supported: true,
  id: "agent-1",
  workspace_id: "workspace-1",
  name: "Lambda",
  description: "Test agent",
  runtime_id: "runtime-1",
} as Agent;

describe("AgentDetailInspector labels", () => {
  afterEach(() => {
    cleanup();
  });

  it("wires an explicit timeout override to the existing agent update mutation", async () => {
    const update = vi.fn(async () => {});
    renderWithI18n(<AgentDetailInspector agent={agent} runtime={null} runtimes={[]} members={[]}
      currentUserId="user-1" canEdit onUpdate={update} />);
    fireEvent.click(screen.getByRole("switch", { name: "Use workspace defaults" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(update).toHaveBeenCalledWith("agent-1", {
      runtime_config: { execution_timeouts: { idle_seconds: 7200, tool_idle_seconds: 7200, run_seconds: 0, opencode_idle_seconds: 600 } },
    }));
  });

  // Agent labels were removed from the product (MUL-5600). Label Settings no
  // longer manages an agent catalog, so an attach-only picker here would be a
  // dead end pointing at a catalog the user cannot populate.
  it("does not offer a label picker", () => {
    renderWithI18n(
      <AgentDetailInspector
        agent={agent}
        runtime={null}
        runtimes={[]}
        members={[]}
        currentUserId="user-1"
        canEdit
        onUpdate={vi.fn(async () => {})}
      />,
    );

    // Sanity check: the profile card actually rendered.
    expect(screen.getByLabelText("Name")).toBeInTheDocument();

    expect(screen.queryByTestId("resource-label-picker")).toBeNull();
    expect(screen.queryByText("Labels")).toBeNull();
  });
});
