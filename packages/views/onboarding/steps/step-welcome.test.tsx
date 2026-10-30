import { describe, expect, it, vi } from "vitest";
import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { renderWithI18n } from "../../test/i18n";
import { StepWelcome } from "./step-welcome";
vi.mock("@multica/core/api", () => ({ api: { getBaseUrl: () => "https://service.example.test/api" } }));
describe("service setup", () => {
  it("shows the configured service and continues without marketing or profile collection", async () => {
    const next = vi.fn();
    renderWithI18n(<StepWelcome onNext={next} />);
    expect(screen.getByText("https://service.example.test/api")).toBeInTheDocument();
    expect(screen.queryByRole("link")).toBeNull();
    await userEvent.click(screen.getByRole("button", { name: "Continue to workspace" }));
    expect(next).toHaveBeenCalledOnce();
  });
  it("translates the service step", () => {
    renderWithI18n(<StepWelcome onNext={vi.fn()} />, { locale: "zh-Hans" });
    expect(screen.getByRole("heading", { name: "连接你的服务" })).toBeInTheDocument();
  });
});
