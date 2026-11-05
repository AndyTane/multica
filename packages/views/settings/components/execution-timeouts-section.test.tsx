import { afterEach, describe, expect, it, vi } from "vitest";
import { useState } from "react";
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(2);
  });
  it("creates a policy that follows the general wait without a hidden ten-minute override", async () => {
    const save = vi.fn(async () => {});
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={null} canEdit onSave={save} />);
    fireEvent.click(screen.getByRole("switch", { name: "Manage on server" }));
    expect(screen.getAllByRole("spinbutton")).toHaveLength(2);
    expect(screen.queryByRole("combobox")).toBeNull();
    expect(screen.queryByText("OpenCode extensions")).toBeNull();
    expect(screen.queryByText("Total run limit")).toBeNull();
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith({ ...DEFAULT_EXECUTION_TIMEOUTS, opencodeIdleSeconds: 0 }));
  });
  it("saves fractional hours and zero while preserving existing hidden policy values", async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => {});
    const value = { ...DEFAULT_EXECUTION_TIMEOUTS, runSeconds: 14400, opencodeIdleSeconds: 1800, opencodeShellSeconds: 5400 };
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={value} canEdit onSave={save} />);
    await user.clear(screen.getByLabelText("No-message wait duration"));
    await user.type(screen.getByLabelText("No-message wait duration"), "0.5");
    await user.tab();
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(0.5);
    fireEvent.change(screen.getByLabelText("No-message wait during tools duration"), { target: { value: "" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith({ ...value, idleSeconds: 1800, toolIdleSeconds: 0 }));
    expect(screen.queryByText(/Source:/)).toBeNull();
    expect(screen.queryByText(/Explicit tool-call/)).toBeNull();
  });
  it("accepts exact decimal hours in both duration inputs", async () => {
    const user = userEvent.setup();
    const save = vi.fn(async () => {});
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={DEFAULT_EXECUTION_TIMEOUTS} canEdit onSave={save} />);
    for (const [label, text] of [["No-message wait duration", "1.1"], ["No-message wait during tools duration", "0.07"]] as const) {
      const input = screen.getByLabelText(label);
      await user.clear(input);
      await user.type(input, text);
      await user.tab();
      expect(input).toHaveValue(Number(text));
      expect(input).toHaveAttribute("aria-invalid", "false");
    }
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(save).toHaveBeenCalledWith({ ...DEFAULT_EXECUTION_TIMEOUTS, idleSeconds: 3960, toolIdleSeconds: 252 }));
  });
  it.each(["workspace", "agent"] as const)("clears the old success message on either %s duration edit", async (scope) => {
    function SavedPolicy() {
      const [value, setValue] = useState<ExecutionTimeouts | null>(DEFAULT_EXECUTION_TIMEOUTS);
      return <ExecutionTimeoutsSection scope={scope} value={value} canEdit onSave={async (next) => { setValue(next); }} />;
    }
    renderWithI18n(<SavedPolicy />);
    for (const label of ["No-message wait duration", "No-message wait during tools duration"]) {
      fireEvent.change(screen.getByLabelText(label), { target: { value: "3" } });
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
      fireEvent.change(screen.getByLabelText(label), { target: { value: "4" } });
      expect(screen.queryByText("Saved")).toBeNull();
      expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
      await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
    }
  });
  it("keeps invalid decimal drafts visible and blocks saving them", () => {
    const save = vi.fn();
    renderWithI18n(<ExecutionTimeoutsSection scope="workspace" value={DEFAULT_EXECUTION_TIMEOUTS} canEdit onSave={save} />);
    const input = screen.getByLabelText("No-message wait duration");
    fireEvent.change(input, { target: { value: "0.0001" } });
    expect(input).toHaveValue(0.0001);
    expect(input).toHaveAttribute("aria-invalid", "true");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    expect(save).not.toHaveBeenCalled();
  });
  it("waits for the submission to finish before showing success after a cache refresh", async () => {
    let finish!: () => void;
    const pending = new Promise<void>((resolve) => { finish = resolve; });
    function SavedPolicy() {
      const [value, setValue] = useState<ExecutionTimeouts | null>(DEFAULT_EXECUTION_TIMEOUTS);
      return <ExecutionTimeoutsSection scope="workspace" value={value} canEdit onSave={async (next) => { setValue(next); await pending; }} />;
    }
    renderWithI18n(<SavedPolicy />);
    fireEvent.change(screen.getByLabelText("No-message wait duration"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(screen.getByText("Saving…")).toBeInTheDocument();
    expect(screen.queryByText("Saved")).toBeNull();
    expect(screen.getByLabelText("No-message wait duration")).toBeDisabled();
    await act(async () => { finish(); });
    expect(screen.getByText("Saved")).toBeInTheDocument();
    expect(screen.getByLabelText("No-message wait duration")).toBeEnabled();
  });
  it("retains the failed draft and error when the agent cache rolls back, then retries", async () => {
    let fail!: (error: Error) => void;
    const pending = new Promise<void>((_resolve, reject) => { fail = reject; });
    let rejectFirst = true;
    function SavedPolicy() {
      const [value, setValue] = useState<ExecutionTimeouts | null>(DEFAULT_EXECUTION_TIMEOUTS);
      return <ExecutionTimeoutsSection scope="agent" value={value} canEdit onSave={async (next) => {
        setValue(next);
        if (rejectFirst) {
          try { await pending; }
          catch (error) { setValue(DEFAULT_EXECUTION_TIMEOUTS); throw error; }
        }
      }} />;
    }
    renderWithI18n(<SavedPolicy />);
    fireEvent.change(screen.getByLabelText("No-message wait duration"), { target: { value: "3" } });
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await act(async () => { fail(new Error("Save failed after optimistic update")); });
    expect(screen.getByRole("alert")).toHaveTextContent("Save failed after optimistic update");
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(3);
    expect(screen.queryByText("Saved")).toBeNull();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
    rejectFirst = false;
    fireEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(screen.getByText("Saved")).toBeInTheDocument());
    expect(screen.getByLabelText("No-message wait duration")).toHaveValue(3);
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
