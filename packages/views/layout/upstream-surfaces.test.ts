// @vitest-environment node
import { readFileSync } from "node:fs";
import { describe, expect, it } from "vitest";

const read = (path: string) => readFileSync(new URL(path, import.meta.url), "utf8");
describe("self-hosted product surfaces", () => {
  it("has no official cloud, upgrade or promotional entry in navigation/settings/search", () => {
    expect(read("./app-sidebar.tsx")).not.toMatch(/JoinDiscordCard/);
    expect(read("../settings/components/settings-page.tsx")).not.toMatch(/BillingTab|BILLING_WORKSPACE_SUBSCRIPTIONS_FLAG/);
    expect(read("../settings/components/use-settings-search-index.ts")).not.toMatch(/tab: "billing"|tab: "cloud"|upgrade/i);
    expect(read("../search/search-command.tsx")).not.toMatch(/multica\.ai|cloudRuntime|billing|upgrade/i);
    expect(read("../runtimes/components/runtimes-page.tsx")).not.toMatch(/CloudRuntimeDialog|cloudRuntimeEnabled/);
    expect(read("./help-launcher.tsx")).not.toMatch(/multica\.ai|CHANGELOG_URL|DOWNLOAD_URL|DISCORD_URL/);
  });
  it("keeps channels, configured apps, model choice and local usage", () => {
    expect(read("../settings/components/settings-page.tsx")).toMatch(/ChannelsTab/);
    expect(read("../settings/components/settings-page.tsx")).toMatch(/ConnectedAppsTab/);
    expect(read("../runtimes/components/runtime-detail.tsx")).toMatch(/UsageSection/);
    expect(read("../runtimes/components/mika-runtime-choice.tsx")).toMatch(/model/i);
  });
});
