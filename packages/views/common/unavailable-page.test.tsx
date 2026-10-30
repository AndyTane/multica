import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../test/i18n";
const replace = vi.hoisted(() => vi.fn());
vi.mock("../navigation", () => ({ useOptionalNavigation: () => ({ replace }) }));
import { UnavailablePage } from "./unavailable-page";
describe("retired deep links", () => {
  it("focuses a local workspace return action", async () => {
    renderWithI18n(<UnavailablePage workspaceHref="/acme/issues" />);
    const button = screen.getByRole("button", { name: "Return to workspace" });
    expect(button).toHaveFocus();
    expect(screen.queryByRole("link")).toBeNull();
    await userEvent.click(button);
    expect(replace).toHaveBeenCalledWith("/acme/issues");
  });
});
