import { screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { useModalStore } from "@multica/core/modals";
import { renderWithI18n } from "../test/i18n";
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-test" }));
import { IssueLimitUpgradeDialog } from "./issue-limit-upgrade-dialog";
describe("local limit recovery", () => {
  it("keeps the rejection visible without upgrade, portal or cloud actions", () => {
    useModalStore.getState().showIssueLimitRecovery("ws-test", "issue_limit");
    renderWithI18n(<IssueLimitUpgradeDialog />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText(/contact your workspace administrator/)).toBeInTheDocument();
    expect(screen.queryByText(/upgrade|billing portal/i)).toBeNull();
    expect(screen.queryByRole("link")).toBeNull();
  });
});
