import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import type { Issue } from "@multica/core/types";
import { ReviewBadge } from "./review-badge";

vi.mock("@multica/core/auth", () => ({ useAuthStore: (s: (state: unknown) => unknown) => s({ user: { id: "me" } }) }));
vi.mock("@multica/core/workspace/hooks", () => ({ useActorName: () => ({ getActorName: () => "Reviewer" }) }));
vi.mock("../../i18n", () => ({
  useT: () => ({
    t: (s: (v: unknown) => string, options?: { name?: string }) =>
      s({
        awaiting: {
          you: "等你",
          other: `等 ${options?.name}`,
          specialist: "等验收",
          specialist_hint: `等 ${options?.name} 验收`,
        },
        review_badge_r4: { reply_you: "等你回复", act_you: "等你推进" },
      }),
  }),
}));
afterEach(cleanup);

// A review mark only routes to a member when the card has a resolvable
// dispatcher (B9/B10): the fixtures below carry the same dispatcher the server
// would resolve, so "mine" means "the dispatcher B9 hands it to".
const dispatched = { dispatcher_user_id: "me", dispatcher_active: true };

describe("review badge", () => {
  const issue = (metadata: Issue["metadata"], status = "in_review") => ({ status, metadata, ...dispatched } as Issue);
  it("renders the current user's badge and action, while other recipients only get a muted label", () => {
    const card = issue({ review_by: "user", review_action: "Run the build", review_user: "me" });
    const { rerender } = render(<><ReviewBadge issue={card} /><ReviewBadge issue={card} actionOnly /></>);
    expect(screen.getByText("等你")).toBeTruthy(); expect(screen.getByText("Run the build")).toBeTruthy();
    const other = issue({ ...card.metadata, review_user: "other" });
    rerender(<><ReviewBadge issue={other} /><ReviewBadge issue={other} actionOnly /></>);
    expect(screen.getByText("等 Reviewer")).toBeTruthy(); expect(screen.queryByText("Run the build")).toBeNull();
  });
  it("treats a legacy user mark without review_user as waiting on its dispatcher", () => {
    render(<ReviewBadge issue={issue({ review_by: "user", review_action: "Run the build" })} />);
    expect(screen.getByText("等你")).toBeTruthy();
  });

  it("renders a specialist label and leaves unmarked or non-review cards alone", () => {
    const { rerender, container } = render(<ReviewBadge issue={issue({ review_by: "specialist", review_agent: "agent" })} />);
    expect(screen.getByText("等验收")).toBeTruthy();
    rerender(<ReviewBadge issue={issue({})} />); expect(container.textContent).toBe("");
    rerender(<ReviewBadge issue={issue({ review_by: "user" }, "done")} />); expect(container.textContent).toBe("");
  });
});

// CLAWSTA-151 F2: a blocked card waits on a person for a reply or for a
// manual step, and the badge must say WHICH one. The approval wording above
// stays untouched; blocked uses the new review_badge_r4 copy.
describe("review badge on blocked cards", () => {
  const issue = (metadata: Issue["metadata"], status = "blocked") => ({ status, metadata, ...dispatched } as Issue);

  it("waits on my reply when the card is blocked for review_kind=reply", () => {
    const { rerender } = render(<ReviewBadge issue={issue({ review_by: "user", review_kind: "reply", review_user: "me", review_action: "Answer the question" })} />);
    expect(screen.getByText("等你回复")).toBeTruthy();
    // The action sentence is the same secondary line the approval badge shows.
    rerender(<ReviewBadge issue={issue({ review_by: "user", review_kind: "reply", review_user: "me", review_action: "Answer the question" })} actionOnly />);
    expect(screen.getByText("Answer the question")).toBeTruthy();
  });

  it("waits on my action when the card is blocked for review_kind=act", () => {
    render(<ReviewBadge issue={issue({ review_by: "user", review_kind: "act", review_user: "me" })} />);
    expect(screen.getByText("等你推进")).toBeTruthy();
  });

  it("names the other recipient when a blocked card waits on someone else", () => {
    render(<ReviewBadge issue={issue({ review_by: "user", review_kind: "reply", review_user: "other" })} />);
    expect(screen.getByText("等 Reviewer")).toBeTruthy();
    expect(screen.queryByText("等你回复")).toBeNull();
    expect(screen.queryByText("等你推进")).toBeNull();
  });

  it("renders nothing for blocked marks the contract rejects", () => {
    // A blocked card must be user + reply|act (B7). An old or malformed mark
    // is not a state we can name, so it must not render a badge at all (B10).
    const { rerender, container } = render(<ReviewBadge issue={issue({ review_by: "user" })} />);
    expect(container.textContent).toBe("");
    rerender(<ReviewBadge issue={issue({ review_by: "user", review_kind: "approve", review_user: "me" })} />);
    expect(container.textContent).toBe("");
    rerender(<ReviewBadge issue={issue({ review_by: "specialist", review_kind: "reply", review_agent: "agent" })} />);
    expect(container.textContent).toBe("");
  });
});
