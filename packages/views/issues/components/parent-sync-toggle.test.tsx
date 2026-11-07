import { cleanup, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { renderWithI18n } from "../../test/i18n";
import { ParentSyncToggle } from "./parent-sync-toggle";

// CLAWSTA-151 B11: the server sends stable reason codes for a sync that cannot
// be delivered. The toggle must explain them in words; printing the code would
// leak an identifier, and a code this client does not know must degrade to the
// generic sentence instead of disappearing or being shown raw.

function renderToggle(reason: string | null, kind: "unavailable" | "owner" = "unavailable") {
  return renderWithI18n(
    <ParentSyncToggle
      kind={kind}
      name={kind === "unavailable" ? null : "Ada"}
      reason={reason}
      checked={false}
      locked={false}
      disabled={kind === "unavailable"}
      onCheckedChange={vi.fn()}
    />,
  );
}

afterEach(cleanup);

describe("ParentSyncToggle reason copy", () => {
  it("names an unavailable destination in words", () => {
    renderToggle("no_fallback_recipient");
    expect(screen.getByText(/No one can receive this copy/)).toBeTruthy();
    expect(screen.queryByText(/no_fallback_recipient/)).toBeNull();
  });

  it("explains that the card has no parent to sync to", () => {
    renderToggle("issue_has_no_parent");
    expect(screen.getByText(/This card has no parent card to sync to/)).toBeTruthy();
  });

  it("falls back to the generic sentence for an unknown reason", () => {
    renderToggle("sync_backend_melted");
    expect(screen.getByText(/could not be delivered to the parent card/)).toBeTruthy();
    expect(screen.queryByText(/sync_backend_melted/)).toBeNull();
  });

  it("keeps the plain destination label when there is nothing to explain", () => {
    renderToggle(null, "owner");
    expect(screen.getByText(/Sync to parent/)).toBeTruthy();
  });
});
