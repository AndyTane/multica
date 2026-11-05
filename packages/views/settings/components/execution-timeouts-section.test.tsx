import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { DEFAULT_EXECUTION_TIMEOUTS, type ExecutionTimeouts } from "@multica/core/agents/execution-timeouts";
import { renderWithI18n } from "../../test/i18n";
import { ExecutionTimeoutsSection } from "./execution-timeouts-section";

afterEach(cleanup);
describe("run timeout settings", () => {
  it("does not label an unbound runtime as an outdated client", () => {
    renderWithI18n(<ExecutionTimeoutsSection scope="agent" value={DEFAULT_EXECUTION_TIMEOUTS} canEdit onSave={vi.fn()} />);
    expect(screen.queryByText(/Client upgrade required/)).toBeNull();
  });
  it("shows client fallback and prevents edits against a legacy server", () => {
    const save = vi.fn();
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={DEFAULT_EXECUTION_TIMEOUTS} canEdit serverSupported={false} onSave={save} />);
    expect(screen.getByText(/no server policy/)).toBeInTheDocument();
    expect(screen.getByRole("status")).toHaveTextContent("Server upgrade required");
    expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
    expect(screen.queryByRole("button", { name: "Save" })).toBeNull();
    expect(screen.queryByLabelText("No-message wait duration")).toBeNull();
    fireEvent.click(screen.getByRole("switch"));
    expect(save).not.toHaveBeenCalled();
  });
  it("keeps the saved acknowledgment when the server cache refreshes the value", async () => {
    function SavedPolicy() {
      const [value, setValue] = useState<ExecutionTimeouts | null>(null);
      return <ExecutionTimeoutsSection scope="workspace" value={value} canEdit onSave={async (next) => { setValue(next); }} />;
    }
    renderWithI18n(<SavedPolicy />);
    fireEvent.click(screen.getByRole("switch", { name: "Manage on server" }));
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByText("Source: workspace default")).toBeInTheDocument();
  });
  it("enables a complete policy and explicitly follows the general OpenCode wait", async () => {
    const save = vi.fn(async () => {});
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={null} canEdit onSave={save} />);
    expect(screen.getByText(/no server policy/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("switch", { name: "Manage on server" }));
    fireEvent.change(screen.getByLabelText("OpenCode no-message wait"), { target: { value: "zero" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith({ ...DEFAULT_EXECUTION_TIMEOUTS, opencodeIdleSeconds: 0 }));
  });
  it("shows inherited values, restores inheritance, and hides OpenCode fields on DSH", async () => {
    const save = vi.fn(async () => {});
    renderWithI18n(<ExecutionTimeoutsSection scope="agent" provider="dsh" value={DEFAULT_EXECUTION_TIMEOUTS}
      inherited={{ ...DEFAULT_EXECUTION_TIMEOUTS, idleSeconds: 10800 }} clientSupported canEdit onSave={save} />);
    expect(screen.queryByText("Tool default execution time")).toBeNull();
    fireEvent.click(screen.getByRole("switch", { name: "Use workspace defaults" }));
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(3);
    expect(screen.getByLabelText("No-message wait duration")).toBeDisabled();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith(null));
  });
  it("keeps failed edits visible and rejects negative durations", async () => {
    const save = vi.fn(async () => { throw new Error("Server rejected this change"); });
    renderWithI18n(<ExecutionTimeoutsSection scope="agent" provider="opencode" value={DEFAULT_EXECUTION_TIMEOUTS} canEdit clientSupported onSave={save} />);
    fireEvent.change(screen.getByLabelText("No-message wait duration"), { target: { value: "-1" } });
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(screen.getByRole("alert")).toHaveTextContent("whole seconds");
    fireEvent.change(screen.getByLabelText("No-message wait duration"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText("Server rejected this change")).toBeInTheDocument());
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(3);
  });
  it("shows the real upgrade state, permission read-only and Chinese copy", () => {
    renderWithI18n(<ExecutionTimeoutsSection scope="agent" provider="opencode" value={DEFAULT_EXECUTION_TIMEOUTS}
      clientSupported={false} canEdit={false} onSave={vi.fn()} />, { locale: "zh-Hans" });
    expect(screen.getByRole("status")).toHaveTextContent("客户端需要升级");
    expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
    expect(screen.queryByRole("button", { name: "保存" })).toBeNull();
    expect(screen.queryByText(/下次运行生效/)).toBeNull();
    expect(screen.getByText("工具执行时无消息等待")).toBeInTheDocument();
  });
});
