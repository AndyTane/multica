import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { ReviewBadge } from "./review-badge";

vi.mock("@multica/core/auth", () => ({ useAuthStore: (s: (state: unknown) => unknown) => s({ user: { id: "me" } }) }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({ getActorName: () => "Reviewer" }) }));
vi.mock("../../i18n", () => ({ useT: () => ({ t: (s: (v: unknown) => string, options?: { name?: string }) => s({ awaiting: { you: "等你", other: `等 ${options?.name}`, specialist: "等验收", specialist_hint: `等 ${options?.name} 验收` } }) }) }));
afterEach(cleanup);

describe("review badge", () => {
  const issue = (metadata: Issue["metadata"], status = "in_review") => ({ status, metadata } as Issue);
  it("renders the current user's badge and action, while other recipients only get a muted label", () => {
    const card = issue({ review_by: "user", review_action: "Run the build", review_user: "me" });
    const { rerender } = render(<><ReviewBadge issue={card} /><ReviewBadge issue={card} actionOnly /></>);
    expect(screen.getByText("等你")).toBeTruthy(); expect(screen.getByText("Run the build")).toBeTruthy();
    const other = issue({ ...card.metadata, review_user: "other" });
    rerender(<><ReviewBadge issue={other} /><ReviewBadge issue={other} actionOnly /></>);
    expect(screen.getByText("等 Reviewer")).toBeTruthy(); expect(screen.queryByText("Run the build")).toBeNull();
  });
  it("renders a specialist label and leaves unmarked or non-review cards alone", () => {
    const { rerender, container } = render(<ReviewBadge issue={issue({ review_by: "specialist", review_agent: "agent" })} />);
    expect(screen.getByText("等验收")).toBeTruthy();
    rerender(<ReviewBadge issue={issue({})} />); expect(container.textContent).toBe("");
    rerender(<ReviewBadge issue={issue({ review_by: "user" }, "done")} />); expect(container.textContent).toBe("");
  });
});
