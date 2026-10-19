import { render } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { UpdateSection } from "./update-section";

afterEach(() => vi.unstubAllGlobals());

it("does not mount official update checks or controls in the local build", () => {
  const request = vi.fn();
  vi.stubGlobal("fetch", request);
  const { container } = render(
    <UpdateSection runtimeId="test-runtime" currentVersion="v1.0.0" isOnline />,
  );
  expect(container).toBeEmptyDOMElement();
  expect(request).not.toHaveBeenCalled();
});
