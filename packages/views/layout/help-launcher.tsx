"use client";

import { BookOpen, CircleHelp, MessageCircle } from "lucide-react";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuLabel, DropdownMenuSeparator, DropdownMenuTrigger } from "@multica/ui/components/ui/dropdown-menu";
import { useModalStore } from "@multica/core/modals";
import { useConfigStore } from "@multica/core/config";
import { useT } from "../i18n";
import { docsHref } from "../common/docs-locale";

export function HelpLauncher() {
  const { t, i18n } = useT("layout");
  const version = useConfigStore((s) => s.serverVersion);
  return <DropdownMenu>
    <DropdownMenuTrigger aria-label={t(($) => $.help.trigger)} title={t(($) => $.help.trigger)} className="inline-flex size-8 shrink-0 cursor-pointer items-center justify-center rounded-full text-muted-foreground hover:bg-accent hover:text-foreground"><CircleHelp className="size-4" /></DropdownMenuTrigger>
    <DropdownMenuContent align="end" side="top" sideOffset={8} className="min-w-40 max-w-56">
      <DropdownMenuItem render={<a href={docsHref("", i18n.language)} target="_blank" rel="noopener noreferrer" />}><BookOpen className="size-3.5" />{t(($) => $.help.docs)}</DropdownMenuItem>
      <DropdownMenuItem onClick={() => useModalStore.getState().open("feedback")}><MessageCircle className="size-3.5" />{t(($) => $.help.feedback)}</DropdownMenuItem>
      {version && <><DropdownMenuSeparator /><DropdownMenuGroup><DropdownMenuLabel className="break-words font-normal">{t(($) => $.help.server_version, { version })}</DropdownMenuLabel></DropdownMenuGroup></>}
    </DropdownMenuContent>
  </DropdownMenu>;
}
