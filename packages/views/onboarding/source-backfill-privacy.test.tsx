import { render } from "@testing-library/react";
import { expect, it } from "vitest";
import { SourceBackfillModal } from "./source-backfill-modal";

it("does not mount the attribution prompt or its data queries in the local build", () => {
  // No providers are needed: a disabled prompt must not mount its query hooks.
  const { container } = render(<SourceBackfillModal />);
  expect(container).toBeEmptyDOMElement();
});
