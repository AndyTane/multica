// @vitest-environment node
import { describe, expect, it } from "vitest";
import type { CommentParentSync } from "../types";
import {
  parentSyncDeliveryWarning,
  parentSyncToggleState,
  type ParentSyncOwnership,
} from "./parent-sync";

function ownership(overrides: Partial<ParentSyncOwnership> = {}): ParentSyncOwnership {
  return {
    parentAssignee: { type: "agent", id: "agent-parent" },
    subIssueAssignee: { type: "agent", id: "agent-child" },
    currentUserId: "user-1",
    ...overrides,
  };
}

function sync(overrides: Partial<CommentParentSync> = {}): CommentParentSync {
  return {
    parent_issue_id: "issue-parent",
    mode: "agent",
    recipients: [{ type: "agent", id: "agent-parent", name: "Lambda" }],
    already_routed: false,
    ...overrides,
  };
}

describe("parentSyncToggleState — labels by mode", () => {
  it("names the parent's agent owner", () => {
    expect(parentSyncToggleState(sync(), ownership())).toEqual({
      visible: true,
      kind: "owner",
      name: "Lambda",
      reason: null,
      locked: false,
    });
  });

  it("names the parent's member owner", () => {
    const state = parentSyncToggleState(
      sync({
        mode: "member",
        recipients: [{ type: "member", id: "user-2", name: "Tane" }],
      }),
      ownership({ parentAssignee: { type: "member", id: "user-2" } }),
    );
    expect(state).toMatchObject({ visible: true, kind: "owner", name: "Tane" });
  });

  it("names the fallback dispatcher recipient", () => {
    const state = parentSyncToggleState(
      sync({
        mode: "dispatcher",
        recipients: [{ type: "member", id: "user-2", name: "Tane" }],
      }),
      ownership(),
    );
    expect(state).toMatchObject({ visible: true, kind: "recipient", name: "Tane" });
  });

  it("names the fallback admin recipient", () => {
    const state = parentSyncToggleState(
      sync({
        mode: "admin",
        recipients: [{ type: "member", id: "user-3", name: "Owner" }],
      }),
      ownership(),
    );
    expect(state).toMatchObject({ visible: true, kind: "recipient", name: "Owner" });
  });

  it("greys out with the server's reason when nothing can be reached", () => {
    const state = parentSyncToggleState(
      sync({ mode: "unavailable", recipients: [], reason: "parent owner left the workspace" }),
      ownership(),
    );
    expect(state).toEqual({
      visible: true,
      kind: "unavailable",
      name: null,
      reason: "parent owner left the workspace",
      locked: false,
    });
  });

  it("keeps an owner label without a resolved name (unnamed owner)", () => {
    expect(parentSyncToggleState(sync({ recipients: [] }), ownership())).toMatchObject({
      visible: true,
      kind: "owner",
      name: null,
    });
  });

  it("hides an unnamed fallback recipient — the user cannot tell who gets it", () => {
    expect(
      parentSyncToggleState(sync({ mode: "dispatcher", recipients: [] }), ownership()),
    ).toMatchObject({ visible: false });
  });

  it("hides modes this client cannot name instead of guessing", () => {
    expect(parentSyncToggleState(sync({ mode: "future_mode" }), ownership())).toMatchObject({
      visible: false,
    });
  });

  it("hides when the server reported no parent-sync at all (older server)", () => {
    expect(parentSyncToggleState(null, ownership())).toMatchObject({ visible: false });
    expect(parentSyncToggleState(undefined, ownership())).toMatchObject({ visible: false });
  });
});

describe("parentSyncToggleState — hide rules", () => {
  it("hides when the parent owner is the sub-issue owner", () => {
    const state = parentSyncToggleState(
      sync(),
      ownership({
        parentAssignee: { type: "agent", id: "agent-same" },
        subIssueAssignee: { type: "agent", id: "agent-same" },
      }),
    );
    expect(state.visible).toBe(false);
  });

  it("hides when the commenter is the parent's member owner", () => {
    const state = parentSyncToggleState(
      sync({ mode: "member", recipients: [{ type: "member", id: "user-1", name: "Me" }] }),
      ownership({ parentAssignee: { type: "member", id: "user-1" }, currentUserId: "user-1" }),
    );
    expect(state.visible).toBe(false);
  });

  it("keeps the toggle when the commenter only shares the owner's id across actor types", () => {
    // A member id and an agent id are different namespaces: same string is not
    // the same person.
    const state = parentSyncToggleState(
      sync(),
      ownership({ parentAssignee: { type: "agent", id: "user-1" }, currentUserId: "user-1" }),
    );
    expect(state.visible).toBe(true);
  });

  it("locks the box when ordinary routing already reaches the owner", () => {
    const state = parentSyncToggleState(sync({ already_routed: true }), ownership());
    expect(state).toMatchObject({ visible: true, locked: true });
  });
});

describe("parentSyncDeliveryWarning", () => {
  it("stays quiet on success-shaped and missing delivery statuses", () => {
    for (const status of ["queued", "coalesced", "deferred", "steered", "delivered", undefined]) {
      expect(parentSyncDeliveryWarning(sync({ delivery_status: status }))).toBeNull();
    }
    expect(parentSyncDeliveryWarning(null)).toBeNull();
  });

  it("reports the server's reason for a failed delivery", () => {
    expect(
      parentSyncDeliveryWarning(
        sync({ mode: "unavailable", delivery_status: "failed", reason: "runtime offline" }),
      ),
    ).toEqual({ reason: "runtime offline" });
  });

  it("reports a failed delivery without a reason", () => {
    expect(parentSyncDeliveryWarning(sync({ delivery_status: "blocked" }))).toEqual({
      reason: null,
    });
  });
});
