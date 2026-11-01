import { beforeEach, describe, expect, it, vi } from "vitest";
import { fireEvent, screen } from "@testing-library/react";
import { configStore } from "@multica/core/config";
import {
  CHANNEL_DINGTALK_FLAG,
  CHANNEL_SLACK_FLAG,
  CHANNEL_TELEGRAM_FLAG,
  CHANNEL_WECOM_FLAG,
} from "@multica/core/feature-flags";
import { renderWithI18n } from "../../test/i18n";

const state = vi.hoisted(() => ({
  search: "",
  connectionError: false,
  pending: false,
  // The IM channels keep revoked rows for audit, so the fixture carries a real
  // status — a row without one is not evidence of a live connection (#8496).
  installationStatus: "active" as string,
  push: vi.fn(),
  // Every useQuery call this render made, so a test can prove a hidden channel
  // is not merely unrendered but not requested at all.
  queries: [] as { keys: string; enabled: boolean | undefined }[],
}));
vi.mock("../../navigation/context", async (importOriginal) => ({
  ...(await importOriginal<typeof import("../../navigation/context")>()),
  useNavigation: () => ({
    pathname: "/acme/settings",
    searchParams: new URLSearchParams(state.search),
    push: state.push,
  }),
}));
vi.mock("@multica/core/hooks", () => ({ useWorkspaceId: () => "ws-1" }));
vi.mock("@multica/core/paths", () => ({
  useCurrentWorkspace: () => ({ id: "ws-1", name: "Acme" }),
}));
vi.mock("@multica/core/permissions", () => ({
  useCurrentMember: () => ({ member: { role: "admin" } }),
}));
vi.mock("@tanstack/react-query", () => ({
  queryOptions: <T,>(opts: T) => opts,
  useQuery: (opts: {
    queryKey?: unknown;
    select?: (data: unknown) => unknown;
    enabled?: boolean;
  }) => {
    state.queries.push({
      keys: JSON.stringify(opts.queryKey ?? []),
      enabled: opts.enabled,
    });
    if (opts.enabled === false) {
      return { data: undefined, isPending: false, isError: false };
    }
    return {
      data: opts.select?.({
        installations: [{ id: "one", status: state.installationStatus }],
      }),
      isPending: state.pending,
      isError: state.connectionError,
    };
  },
}));
vi.mock("./lark-tab", () => ({ LarkTab: () => <div>Lark detail</div> }));
vi.mock("./slack-tab", () => ({ SlackTab: () => <div>Slack detail</div> }));
vi.mock("./dingtalk-tab", () => ({ DingTalkTab: () => <div>DingTalk detail</div> }));
vi.mock("./wecom-tab", () => ({ WecomTab: () => <div>WeCom detail</div> }));
vi.mock("./telegram-tab", () => ({ TelegramTab: () => <div>Telegram detail</div> }));

import { ChannelsTab } from "./channels-tab";

/** Only the channels a deployment ships enabled; everything else is off. */
function enableChannels(...flags: string[]) {
  configStore.getState().setFeatureFlags(
    Object.fromEntries(flags.map((flag) => [flag, true])),
  );
}

const ALL_CHANNEL_FLAGS = [
  CHANNEL_SLACK_FLAG,
  CHANNEL_DINGTALK_FLAG,
  CHANNEL_WECOM_FLAG,
  CHANNEL_TELEGRAM_FLAG,
];

function queryFor(fragment: string) {
  return state.queries.find((query) => query.keys.includes(fragment));
}

beforeEach(() => {
  state.search = "tab=channels";
  state.connectionError = false;
  state.pending = false;
  state.installationStatus = "active";
  state.push.mockClear();
  state.queries = [];
  configStore.getState().setFeatureFlags({});
});

describe("ChannelsTab", () => {
  it("lists Feishu only under the default channel switches", () => {
    renderWithI18n(<ChannelsTab />);

    expect(screen.getByRole("link", { name: /Lark/ })).toBeInTheDocument();
    for (const hidden of ["Slack", "DingTalk", "WeCom", "Telegram"]) {
      expect(screen.queryByText(hidden)).toBeNull();
    }
    // Hidden channels are not requested either: the settings page must not ask
    // the server about a channel it is not showing.
    expect(queryFor('"lark"')?.enabled).toBe(true);
    for (const fragment of ['"slack"', '"dingtalk"', '"wecom"', '"telegram"']) {
      expect(queryFor(fragment)?.enabled).toBe(false);
    }
  });

  it("lists only messaging channels, each with its own mark and status", () => {
    enableChannels(...ALL_CHANNEL_FLAGS);
    renderWithI18n(<ChannelsTab />);

    expect(screen.getByRole("link", { name: /Slack Connected/ })).toBeInTheDocument();
    expect(screen.queryByRole("link", { name: /GitHub/ })).toBeNull();
    expect(screen.queryByRole("link", { name: /Composio/ })).toBeNull();
    expect(screen.queryByText("Slack detail")).toBeNull();
    const shapes = ["lark", "slack", "dingtalk", "wecom", "telegram"].map(
      (channel) => screen.getByTestId(`integration-channel-icon-${channel}`).innerHTML,
    );
    expect(new Set(shapes).size).toBe(5);

    fireEvent.click(screen.getByRole("link", { name: /Slack Connected/ }));
    expect(state.push).toHaveBeenCalledWith("/acme/settings?tab=channels&integration=slack");
  });

  it("adds a channel back as soon as its switch is on", () => {
    enableChannels(CHANNEL_SLACK_FLAG);
    renderWithI18n(<ChannelsTab />);

    expect(screen.getByRole("link", { name: /Slack Connected/ })).toBeInTheDocument();
    expect(screen.queryByText("DingTalk")).toBeNull();
    expect(screen.queryByText("WeCom")).toBeNull();
    expect(screen.queryByText("Telegram")).toBeNull();
    expect(queryFor('"slack"')?.enabled).toBe(true);
  });

  it("does not report a revoked bot as connected", () => {
    enableChannels(CHANNEL_SLACK_FLAG);
    state.installationStatus = "revoked";
    renderWithI18n(<ChannelsTab />);
    expect(screen.getByRole("link", { name: /Slack Not connected/ })).toBeInTheDocument();
  });

  it.each([
    ["pending", "Checking..."],
    ["error", "Status unavailable"],
  ] as const)("never reports a %s read as disconnected", (kind, label) => {
    enableChannels(CHANNEL_SLACK_FLAG);
    if (kind === "pending") state.pending = true;
    else state.connectionError = true;
    renderWithI18n(<ChannelsTab />);
    expect(screen.getByRole("link", { name: new RegExp(`Slack ${label}`) })).toBeInTheDocument();
  });

  it("opens one channel with a breadcrumb back to the list", () => {
    enableChannels(CHANNEL_SLACK_FLAG);
    state.search = "tab=channels&integration=slack";
    renderWithI18n(<ChannelsTab />);

    expect(screen.getByText("Slack detail")).toBeInTheDocument();
    expect(screen.queryByText("Lark detail")).toBeNull();
    expect(screen.getByRole("link", { name: "Messaging" })).toHaveAttribute(
      "href",
      "/acme/settings?tab=channels",
    );
  });

  it("falls back to the channel list when a deep link points at a switched-off channel", () => {
    state.search = "tab=channels&integration=slack";
    renderWithI18n(<ChannelsTab />);

    expect(screen.queryByText("Slack detail")).toBeNull();
    expect(screen.getByRole("link", { name: /Lark/ })).toBeInTheDocument();
  });

  it("opens the retired Lark bookmark on the Lark page", () => {
    state.search = "tab=lark";
    renderWithI18n(<ChannelsTab />);
    expect(screen.getByText("Lark detail")).toBeInTheDocument();
  });
});
