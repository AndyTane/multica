export const COMPOSIO_MCP_APPS_FLAG = "composio_mcp_apps";
export const PLUGINS_V1_FLAG = "plugins_v1";
export const BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG =
  "billing_workspace_subscriptions";

// IM channel switches (CLAWSTA-102). Names match the server's
// server/internal/featureflags keys one-for-one: Feishu is the only channel
// shipped enabled, and every other channel is listed only while its switch is
// on, so the client never shows a channel the server has not wired.
export const CHANNEL_SLACK_FLAG = "channel_slack";
export const CHANNEL_TELEGRAM_FLAG = "channel_telegram";
export const CHANNEL_DINGTALK_FLAG = "channel_dingtalk";
export const CHANNEL_WECOM_FLAG = "channel_wecom";
