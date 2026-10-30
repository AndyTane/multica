import { render, screen } from "@testing-library/react";
import { expect, it } from "vitest";
import { BlockingRuntimeConfigError } from "./runtime-config-error";

it("renders the configuration path and JSON instructions without starting the app", () => {
  const message = 'Missing C:/isolated/desktop.json\n{"apiUrl":"http://localhost:8080","wsUrl":"ws://localhost:8080/ws","appUrl":"http://localhost:3000"}';
  const { container } = render(<BlockingRuntimeConfigError message={message} />);
  expect(screen.getByRole("heading").textContent).toBe("Desktop configuration error");
  expect(container.querySelector("pre")?.textContent).toBe(message);
  expect(container.querySelector("a, img, iframe, script")).toBeNull();
});
