"use client";

import { useCallback } from "react";
import { BookOpen, CircleHelp, MessageCircle, RefreshCw } from "lucide-react";
import { toast } from "sonner";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuSeparator, DropdownMenuTrigger } from "@multica/ui/components/ui/dropdown-menu";
import { copyText } from "@multica/ui/lib/clipboard";
import { api } from "@multica/core/api";
import { useModalStore } from "@multica/core/modals";
import { configStore, useConfigStore } from "@multica/core/config";
import { PRODUCT_NAME } from "@multica/core/config/brand";
import { useT } from "../i18n";
import { docsHref } from "../common/docs-locale";
import { sameReleaseVersion } from "../common/release-version";

export function HelpLauncher() {
  const { t, i18n } = useT("layout");
  const version = useConfigStore((s) => s.serverVersion);
  const commit = useConfigStore((s) => s.serverCommit);
  const webBuildVersion = useConfigStore((s) => s.webBuildVersion);
  // Only a tagged release build ("v0.2.7") is comparable; the package.json
  // fallback of an untagged build would otherwise read as a stale page.
  const stale =
    webBuildVersion.startsWith("v") &&
    sameReleaseVersion(webBuildVersion, version) === false;

  // The config loads once per page, so a tab left open across an upgrade
  // still shows the old version. Re-read it whenever the menu opens: this is
  // exactly when someone is checking which release is running.
  const refreshVersion = useCallback((open: boolean) => {
    if (!open) return;
    void api.getConfig().then(
      (cfg) => configStore.getState().setServerVersion(cfg.server_version, cfg.server_commit),
      () => {},
    );
  }, []);

  const copyVersion = () => {
    const text = `${PRODUCT_NAME} ${version}${commit ? ` (${commit})` : ""}`;
    void copyText(text).then((ok) =>
      ok ? toast.success(t(($) => $.help.version_copied)) : toast.error(t(($) => $.help.version_copy_failed)),
    );
  };

  return <DropdownMenu onOpenChange={refreshVersion}>
    <DropdownMenuTrigger aria-label={t(($) => $.help.trigger)} title={t(($) => $.help.trigger)} className="inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><CircleHelp className="size-4" /></DropdownMenuTrigger>
    <DropdownMenuContent align="end" side="top" sideOffset={8} className="min-w-40 max-w-56">
      <DropdownMenuItem render={<a href={docsHref("", i18n.language)} target="_blank" rel="noopener noreferrer" />}><BookOpen className="size-3.5" />{t(($) => $.help.docs)}</DropdownMenuItem>
      <DropdownMenuItem onClick={() => useModalStore.getState().open("feedback")}><MessageCircle className="size-3.5" />{t(($) => $.help.feedback)}</DropdownMenuItem>
      {version && <><DropdownMenuSeparator /><DropdownMenuGroup>
        <DropdownMenuItem onClick={copyVersion} title={t(($) => $.help.copy_version)} className="flex-col items-start gap-0.5 break-all">
          <span className="text-caption font-medium text-foreground">{t(($) => $.help.product_version, { product: PRODUCT_NAME, version })}</span>
          {commit && <span className="font-mono text-caption text-muted-foreground">{t(($) => $.help.commit, { commit })}</span>}
        </DropdownMenuItem>
        {stale && <DropdownMenuItem onClick={() => window.location.reload()} title={t(($) => $.help.reload_stale_title, { version: webBuildVersion })} className="text-warning focus:text-warning"><RefreshCw className="size-3.5" />{t(($) => $.help.reload_stale)}</DropdownMenuItem>}
      </DropdownMenuGroup></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
