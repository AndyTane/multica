"use client";

import { useEffect, useRef } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { DragStrip } from "../platform";
import { useOptionalNavigation } from "../navigation";
import { useT } from "../i18n";

/** Local landing for retired cloud and billing links. It performs no requests. */
export function UnavailablePage({ workspaceHref = "/", onReturn }: { workspaceHref?: string; onReturn?: () => void }) {
  const { t } = useT("common");
  const navigation = useOptionalNavigation();
  const button = useRef<HTMLButtonElement>(null);
  useEffect(() => { button.current?.focus(); }, []);
  return <div className="flex min-h-svh flex-col bg-background text-foreground">
    <DragStrip />
    <main className="m-auto flex w-full max-w-md flex-col items-start gap-5 px-6 py-16">
      <h1 className="text-display-sm font-semibold tracking-tight">{t(($) => $.unavailable.title)}</h1>
      <p className="text-body text-muted-foreground">{t(($) => $.unavailable.description)}</p>
      <Button ref={button} onClick={() => { if (onReturn) onReturn(); else navigation?.replace(workspaceHref); }}>{t(($) => $.unavailable.back)}</Button>
    </main>
  </div>;
}
