import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, fireEvent, screen } from "@testing-library/react";
import { renderWithI18n } from "../../test/i18n";
import { CommentBody, CommentBodySession, useCommentBodyExpansion } from "./comment-body";

let height = 400;
let resize: () => void;
function Body({ reveal = false }: { reveal?: boolean }) {
  const [expanded, setExpanded] = useCommentBodyExpansion("comment-1", reveal);
  return <CommentBody expanded={expanded} onExpandedChange={setExpanded}><p>Conclusion and searchable final paragraph</p></CommentBody>;
}

beforeEach(() => {
  height = 400;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(() => ({ height }) as DOMRect);
  vi.stubGlobal("ResizeObserver", class {
    constructor(callback: () => void) { resize = callback; }
    observe() {}
    disconnect() {}
  });
});
afterEach(() => { vi.restoreAllMocks(); vi.unstubAllGlobals(); });

describe("CommentBody", () => {
  it("clips measured long content at 200px while retaining searchable text", () => {
    const { container } = renderWithI18n(<Body />);
    const button = screen.getByRole("button", { name: "Show full comment" });
    expect(button).toHaveAttribute("aria-expanded", "false");
    expect(document.getElementById(button.getAttribute("aria-controls")!)).toHaveStyle({ maxHeight: "200px", overflow: "clip" });
    expect(container).toHaveTextContent("searchable final paragraph");
  });
  it("leaves content at or below 320px unfolded and remeasures async content", () => {
    height = 320;
    renderWithI18n(<Body />);
    expect(screen.queryByRole("button")).toBeNull();
    height = 321;
    act(() => resize());
    expect(screen.getByRole("button", { name: "Show full comment" })).toBeVisible();
  });
  it("toggles full text and collapse through an accessible button", () => {
    renderWithI18n(<Body />);
    fireEvent.click(screen.getByRole("button", { name: "Show full comment" }));
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute("aria-expanded", "true");
    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    expect(screen.getByRole("button", { name: "Show full comment" })).toHaveAttribute("aria-expanded", "false");
  });
  it("opens a located or edited comment and remembers expansion after the highlight ends", () => {
    const { rerender } = renderWithI18n(<Body />);
    rerender(<Body reveal />);
    expect(screen.getByRole("button", { name: "Show less" })).toHaveAttribute("aria-expanded", "true");
    rerender(<Body />);
    expect(screen.getByRole("button", { name: "Show less" })).toBeVisible();
  });
  it("keeps the choice across virtualized remounts in this page only", () => {
    const session = { choices: new Map<string, boolean>(), postedId: null };
    const { rerender } = renderWithI18n(<CommentBodySession.Provider value={session}><Body /></CommentBodySession.Provider>);
    fireEvent.click(screen.getByRole("button", { name: "Show full comment" }));
    rerender(<CommentBodySession.Provider value={session}>{null}</CommentBodySession.Provider>);
    rerender(<CommentBodySession.Provider value={session}><Body /></CommentBodySession.Provider>);
    expect(screen.getByRole("button", { name: "Show less" })).toBeVisible();
    rerender(<CommentBodySession.Provider value={{ choices: new Map(), postedId: null }}><Body key="new-page" /></CommentBodySession.Provider>);
    expect(screen.getByRole("button", { name: "Show full comment" })).toBeVisible();
  });
  it("opens a freshly posted comment even if its row was already mounted", () => {
    const choices = new Map<string, boolean>();
    const { rerender } = renderWithI18n(<CommentBodySession.Provider value={{ choices, postedId: null }}><Body /></CommentBodySession.Provider>);
    rerender(<CommentBodySession.Provider value={{ choices, postedId: "comment-1" }}><Body /></CommentBodySession.Provider>);
    expect(screen.getByRole("button", { name: "Show less" })).toBeVisible();
    // The page clears the posting marker after aligning the new row.
    const session = { choices, postedId: null };
    rerender(<CommentBodySession.Provider value={session}><Body /></CommentBodySession.Provider>);
    fireEvent.click(screen.getByRole("button", { name: "Show less" }));
    rerender(<CommentBodySession.Provider value={session}>{null}</CommentBodySession.Provider>);
    rerender(<CommentBodySession.Provider value={session}><Body /></CommentBodySession.Provider>);
    expect(screen.getByRole("button", { name: "Show full comment" })).toBeVisible();
  });
});
