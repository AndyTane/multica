import { cleanup, fireEvent, screen } from "@testing-library/react";
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

function renderLocked(onCheckedChange = vi.fn()) {
  return renderWithI18n(
    <ParentSyncToggle
      kind="owner"
      name="Ada"
      reason={null}
      checked
      locked
      disabled
      onCheckedChange={onCheckedChange}
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

// A locked box used to look exactly like a clickable checked box: Base UI marks
// disabled with `data-disabled`, so the primitive's `disabled:opacity-50` never
// applied. It must read as unavailable and say why.
describe("ParentSyncToggle locked state", () => {
  it("says why the box is locked", () => {
    renderLocked();
    expect(screen.getByText(/already notified/)).toBeTruthy();
  });

  it("fades a disabled box through its data-disabled state", () => {
    renderLocked();
    const box = screen.getByRole("checkbox");
    expect(box).toHaveAttribute("data-disabled");
    expect(box).toHaveClass("data-disabled:opacity-50");
  });

  it("does not flip when the locked box is clicked", () => {
    const onCheckedChange = vi.fn();
    renderLocked(onCheckedChange);
    const box = screen.getByRole("checkbox");
    expect(box).toHaveAttribute("aria-disabled", "true");
    fireEvent.click(box);
    expect(onCheckedChange).not.toHaveBeenCalled();
    expect(box).toHaveAttribute("aria-checked", "true");
  });
});
