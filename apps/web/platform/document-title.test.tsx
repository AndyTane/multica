// @vitest-environment jsdom
/**
 * MUL-6222 — the web half of the tab-title contract.
 *
 * What a URL is *named* is resolved by `@multica/core/paths` +
 * `useTabPresentation` and tested there; mocked out here so these cover only
 * the platform wiring web owns: the site suffix, the unknown-route and
 * navigation behavior of `document.title`.
 */
import { describe, expect, it, vi, beforeEach } from "vitest";
import type { ReactNode } from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { act, render, waitFor } from "@testing-library/react";

const route = vi.hoisted(() => ({ pathname: "/acme/issues", search: "" }));
vi.mock("next/navigation", () => ({
  usePathname: () => route.pathname,
  useSearchParams: () => new URLSearchParams(route.search),
}));

const getIssue = vi.hoisted(() => vi.fn());
vi.mock("@multica/core/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/api")>()),
  api: { getIssue },
}));
vi.mock("@multica/core/paths", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@multica/core/paths")>()),
  useCurrentWorkspace: () => ({ id: "ws1", slug: "acme" }),
}));
function makeWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: ReactNode }) => <QueryClientProvider client={qc}>{children}</QueryClientProvider>;
}

const presentation = vi.hoisted(() => ({
  title: "Issues",
  urls: [] as string[],
}));
vi.mock("@multica/views/layout", () => ({
  useTabPresentation: (url: string) => {
    presentation.urls.push(url);
    return { title: presentation.title, visual: { kind: "icon", icon: "ListTodo" } };
  },
}));

import {
  MAX_PAGE_TITLE_LENGTH,
  SITE_TITLE,
  formatDocumentTitle,
} from "./document-title";
import { WorkspaceDocumentTitle } from "./workspace-document-title";

function open(pathname: string, search = "") {
  route.pathname = pathname;
  route.search = search;
}

beforeEach(() => {
  getIssue.mockReset().mockReturnValue(new Promise(() => {}));
  presentation.urls = [];
  presentation.title = "Issues";
  document.title = SITE_TITLE;
});

describe("formatDocumentTitle", () => {
  it("puts the page name in front of the product name", () => {
    expect(formatDocumentTitle("MUL-123: Fix login")).toBe(
      "MUL-123: Fix login — U Workers",
    );
  });

  it("falls back to the site title rather than a bare suffix", () => {
    expect(formatDocumentTitle("")).toBe(SITE_TITLE);
    expect(formatDocumentTitle("   ")).toBe(SITE_TITLE);
    expect(formatDocumentTitle(null)).toBe(SITE_TITLE);
    expect(formatDocumentTitle(undefined)).toBe(SITE_TITLE);
  });

  it("clips an over-long title and keeps the identifying prefix", () => {
    const title = `MUL-123: ${"long ".repeat(60)}`;
    const formatted = formatDocumentTitle(title);

    expect(formatted.startsWith("MUL-123: long")).toBe(true);
    expect(formatted.endsWith("… — U Workers")).toBe(true);
    // Ellipsis replaces the clipped remainder, and no trailing space survives.
    expect(formatted).not.toContain(" … ");
  });

  it("clips on code points so an emoji is never cut in half", () => {
    const formatted = formatDocumentTitle("🎯".repeat(MAX_PAGE_TITLE_LENGTH + 10));
    const pageTitle = formatted.slice(0, formatted.indexOf(" — U Workers"));

    expect(Array.from(pageTitle)).toHaveLength(MAX_PAGE_TITLE_LENGTH + 1);
    expect(pageTitle).not.toContain("�");
    expect(pageTitle.endsWith("🎯…")).toBe(true);
  });

  it("leaves a title at the limit untouched", () => {
    const exact = "x".repeat(MAX_PAGE_TITLE_LENGTH);
    expect(formatDocumentTitle(exact)).toBe(`${exact} — U Workers`);
  });
});

describe("WorkspaceDocumentTitle", () => {
  it("resolves a direct issue URL from an empty cache and follows UUID updates", async () => {
    open("/acme/issues/CLAWSTA-124");
    presentation.title = "Issue";
    const issue = { id: "11111111-1111-4111-8111-111111111111", identifier: "CLAWSTA-124", title: "版本升级", status: "todo" };
    getIssue.mockResolvedValue(issue);
    const qc = new QueryClient({ defaultOptions: { queries: { retry: false, staleTime: Infinity } } });
    render(<QueryClientProvider client={qc}><WorkspaceDocumentTitle /></QueryClientProvider>);
    await waitFor(() => expect(document.title).toBe("CLAWSTA-124: 版本升级 — U Workers"));
    expect(getIssue).toHaveBeenCalledTimes(1);
    await act(async () => {
      qc.setQueryData(["issues", "ws1", "detail", issue.id], { ...issue, title: "升级完成" });
    });
    await waitFor(() => expect(document.title).toBe("CLAWSTA-124: 升级完成 — U Workers"));
  });
  it("names the tab after the open issue", () => {
    open("/acme/issues/MUL-123");
    presentation.title = "MUL-123: Fix login";

    render(<WorkspaceDocumentTitle />, { wrapper: makeWrapper() });

    expect(document.title).toBe("MUL-123: Fix login — U Workers");
  });

  it("resolves against the full URL so a container's selection titles the tab", () => {
    open("/acme/inbox", "issue=abc&view=archived");
    presentation.title = "MUL-9: Ping";

    render(<WorkspaceDocumentTitle />, { wrapper: makeWrapper() });

    expect(presentation.urls).toContain("/acme/inbox?issue=abc&view=archived");
    expect(document.title).toBe("MUL-9: Ping — U Workers");
  });

  it("keeps the site title on an unrecognized route", () => {
    open("/acme/not-a-route");
    presentation.title = "Unknown page";

    render(<WorkspaceDocumentTitle />, { wrapper: makeWrapper() });

    expect(document.title).toBe(SITE_TITLE);
  });

  it("restores the site title when the dashboard unmounts", () => {
    open("/acme/projects/p1");
    presentation.title = "Website redesign";

    const view = render(<WorkspaceDocumentTitle />, { wrapper: makeWrapper() });
    expect(document.title).toBe("Website redesign — U Workers");

    view.unmount();
    expect(document.title).toBe(SITE_TITLE);
  });

  it("re-asserts the title when a navigation keeps the same page name", () => {
    open("/acme/inbox");
    presentation.title = "Inbox";
    const view = render(<WorkspaceDocumentTitle />, { wrapper: makeWrapper() });
    expect(document.title).toBe("Inbox — U Workers");

    // A route change re-renders the target's metadata, resetting the title to
    // the root default before our effect gets to run again.
    document.title = SITE_TITLE;
    open("/acme/inbox", "view=archived");
    view.rerender(<WorkspaceDocumentTitle />);

    expect(document.title).toBe("Inbox — U Workers");
  });
});
