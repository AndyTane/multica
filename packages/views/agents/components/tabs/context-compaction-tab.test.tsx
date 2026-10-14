// @vitest-environment jsdom
import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { Agent } from "@multica/core/types";
import { I18nProvider } from "@multica/core/i18n/react";
import enCommon from "../../../locales/en/common.json";
import enAgents from "../../../locales/en/agents.json";
import { ContextCompactionTab } from "./context-compaction-tab";

const toastError = vi.hoisted(() => vi.fn());
vi.mock("sonner", () => ({ toast: { success: vi.fn(), error: toastError } }));
const agent = { id: "agent-1", runtime_config: { unrelated: { keep: true } } } as Agent;
const onSave = vi.fn();
function renderTab(canEdit = true) {
  return render(<I18nProvider locale="en" resources={{ en: { common: enCommon, agents: enAgents } }}>
    <ContextCompactionTab agent={agent} canEdit={canEdit} onSave={onSave} />
  </I18nProvider>);
}

describe("context compaction settings", () => {
  beforeEach(() => { vi.clearAllMocks(); onSave.mockResolvedValue(undefined); });
  it("shows defaults and saves the policy while preserving unrelated configuration", async () => {
    renderTab();
    expect(screen.getByRole("switch", { name: "Automatic context compaction" })).toBeChecked();
    const threshold = screen.getByRole("spinbutton", { name: "Compaction threshold (tokens)" });
    expect(threshold).toHaveValue(80000);
    fireEvent.change(threshold, { target: { value: "120000" } });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledWith({ runtime_config: { unrelated: { keep: true }, context_compaction: { enabled: true, threshold_tokens: 120000 } } });
  });
  it("keeps the draft and supports retry when saving fails", async () => {
    onSave.mockRejectedValueOnce(new Error("save unavailable"));
    renderTab();
    const threshold = screen.getByRole("spinbutton");
    fireEvent.change(threshold, { target: { value: "100000" } });
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(toastError).toHaveBeenCalledWith("save unavailable"));
    expect(threshold).toHaveValue(100000);
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });
  it("lets the user disable compaction even with an invalid threshold draft", async () => {
    renderTab();
    fireEvent.change(screen.getByRole("spinbutton"), { target: { value: "0" } });
    expect(screen.getByRole("alert")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    await userEvent.click(screen.getByRole("switch"));
    await userEvent.click(screen.getByRole("button", { name: "Save" }));
    expect(onSave).toHaveBeenCalledWith({ runtime_config: { unrelated: { keep: true }, context_compaction: { enabled: false, threshold_tokens: 80000 } } });
  });
  it("shows read-only settings without exposing a save action", () => {
    renderTab(false);
    expect(screen.getByRole("switch")).toHaveAttribute("aria-disabled", "true");
    expect(screen.getByRole("spinbutton")).toBeDisabled();
    expect(screen.queryByRole("button", { name: "Save" })).not.toBeInTheDocument();
  });
});
