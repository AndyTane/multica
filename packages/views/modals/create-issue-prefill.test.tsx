// @vitest-environment jsdom
import type { ReactNode } from "react";
import { afterEach, beforeAll, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen } from "@testing-library/react";
import { useIssueDraftStore } from "@multica/core/issues/stores/draft-store";

// CLAWSTA-151 B13 one-click child issue: the caller text must arrive as an
// isolated manual draft, never through the shared draft the user may be
// writing in. Closing the dialog has to give that draft back untouched, and
// nothing but the card text plus the chosen owner may be copied.

vi.mock("@multica/ui/components/ui/dialog", () => ({
  Dialog: ({ children }: { children: ReactNode }) => <div>{children}</div>,
  DialogContent: ({ children }: { children: ReactNode }) => (
    <div data-testid="dialog-content">{children}</div>
  ),
}));

vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({
    data: undefined,
    isLoading: false,
    isFetching: false,
    isError: false,
    error: null,
    refetch: vi.fn(),
  }),
}));

vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));

vi.mock("@multica/core/issues/queries", () => ({
  sourceContextPreviewOptions: () => ({ queryKey: ["source-context-preview"] }),
}));

vi.mock("@multica/core/issues/stores/create-mode-store", () => ({
  useCreateModeStore: Object.assign(
    (selector: (state: { setLastMode: () => void }) => unknown) =>
      selector({ setLastMode: () => {} }),
    { getState: () => ({ setLastMode: () => {} }) },
  ),
}));

vi.mock("./quick-create-issue", () => ({
  AgentCreatePanel: () => <div data-testid="agent-panel" />,
}));

// The real store, so "restored untouched" is an assertion about the store the
// rest of the app reads, not about a mock. The panel reads it the same way the
// real manual form does.
vi.mock("./create-issue", async () => {
  const { useIssueDraftStore: useRealDraftStore } = await vi.importActual<
    typeof import("@multica/core/issues/stores/draft-store")
  >("@multica/core/issues/stores/draft-store");
  function ManualPanelStub() {
    const draft = useRealDraftStore((s) => s.draft);
    return (
      <div data-testid="manual-panel">
        {JSON.stringify({
          activeMode: draft.activeMode,
          manual: draft.manual,
          attachments: draft.shared.attachments,
          prompt: draft.agent.prompt,
        })}
      </div>
    );
  }
  return {
    ManualCreatePanel: ManualPanelStub,
    manualDialogContentClass: () => "manual-dialog-content",
  };
});

import { CreateIssueDialog } from "./create-issue-dialog";

// Node ships a partial localStorage shim under jsdom; persist needs a real
// in-memory Storage to round-trip the isolated draft backup.
beforeAll(() => {
  if (typeof globalThis.localStorage?.clear !== "function") {
    const values = new Map<string, string>();
    const storage: Storage = {
      get length() { return values.size; },
      clear: () => values.clear(),
      getItem: (k) => values.get(k) ?? null,
      key: (i) => Array.from(values.keys())[i] ?? null,
      removeItem: (k) => { values.delete(k); },
      setItem: (k, v) => { values.set(k, v); },
    };
    Object.defineProperty(globalThis, "localStorage", { configurable: true, value: storage });
    Object.defineProperty(window, "localStorage", { configurable: true, value: storage });
  }
});

function resetDraft() {
  const store = useIssueDraftStore.getState();
  store.endIsolatedDraft();
  store.clearDraft();
  store.setAgent({ prompt: "", actorType: undefined, actorId: undefined });
  store.setActiveMode("manual");
}

function panelState() {
  return JSON.parse(screen.getByTestId("manual-panel").textContent ?? "{}") as {
    activeMode: string;
    manual: Record<string, unknown>;
    attachments: unknown[];
    prompt: string;
  };
}

const PREFILL = {
  title: "Fix the build",
  description: "The build fails on Windows.",
  assigneeType: "agent",
  assigneeId: "agent-1",
};

function renderPrefill(extra: Record<string, unknown> = {}) {
  return render(
    <CreateIssueDialog
      onClose={vi.fn()}
      initialMode="manual"
      data={{
        parent_issue_id: "parent-1",
        parent_issue_identifier: "CLAW-9",
        prefill: { ...PREFILL, ...extra },
      }}
    />,
  );
}

beforeEach(resetDraft);
afterEach(cleanup);

describe("create-issue prefill", () => {
  it("shows the caller text in the manual form", () => {
    renderPrefill();
    const state = panelState();
    expect(state.activeMode).toBe("manual");
    expect(state.manual).toMatchObject(PREFILL);
    expect(screen.queryByTestId("agent-panel")).toBeNull();
  });

  it("restores the draft the user was writing when the dialog closes", () => {
    const store = useIssueDraftStore.getState();
    store.setManual({
      title: "Draft the user was writing",
      description: "half done",
      labelIds: ["label-1"],
    });
    store.setShared({ priority: "high" });
    const ordinary = useIssueDraftStore.getState().draft;

    const view = renderPrefill();
    expect(panelState().manual).toMatchObject({ title: "Fix the build" });
    expect(useIssueDraftStore.getState().draft.manual.labelIds).toEqual([]);

    view.unmount();
    expect(useIssueDraftStore.getState().draft).toEqual(ordinary);
    expect(useIssueDraftStore.getState().isolatedDraftBackup).toBeUndefined();
  });

  it("keeps an agent-mode draft and its prompt while the prefill is open", () => {
    const store = useIssueDraftStore.getState();
    store.setAgent({ prompt: "ask the agent to plan the release" });
    store.setActiveMode("agent");
    const ordinary = useIssueDraftStore.getState().draft;

    const view = renderPrefill();
    expect(panelState().activeMode).toBe("manual");
    expect(panelState().manual).toMatchObject(PREFILL);

    view.unmount();
    expect(useIssueDraftStore.getState().draft).toEqual(ordinary);
    expect(useIssueDraftStore.getState().draft.agent.prompt).toBe(
      "ask the agent to plan the release",
    );
  });

  it("copies only the card text and owner, never review marks, dispatcher or attachments", () => {
    renderPrefill({
      metadata: { review_by: "user", review_action: "Approve" },
      dispatcher_user_id: "user-2",
      attachments: [{ id: "attachment-1" }],
    });
    const state = panelState();
    expect(state.manual).toMatchObject(PREFILL);
    expect(state.manual).not.toHaveProperty("metadata");
    expect(state.manual).not.toHaveProperty("dispatcher_user_id");
    expect(state.attachments).toEqual([]);
    expect(Object.keys(state.manual).sort()).toEqual([
      "assigneeId",
      "assigneeType",
      "description",
      "labelIds",
      "propertyValues",
      "startDate",
      "status",
      "title",
    ]);
  });
});
