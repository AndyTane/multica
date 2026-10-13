"use client";

import { Copy, ExternalLink } from "lucide-react";
import type { AgentTask } from "@multica/core/types/agent";
import { Button } from "@multica/ui/components/ui/button";
import { copyText } from "@multica/ui/lib/clipboard";
import { useT } from "../../i18n";

export function DshSessionLink({ task }: { task: AgentTask }) {
  const { t } = useT("agents");
  if (!task.session_id?.startsWith("multica-web-") || !task.session_web_url) return null;
  return (
    <span className="flex shrink-0 items-center gap-0.5">
      <a href={task.session_web_url} target="_blank" rel="noopener noreferrer"
        className="inline-flex items-center gap-1 rounded-md px-2 py-1 text-caption text-muted-foreground hover:bg-accent hover:text-foreground focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring">
        <ExternalLink aria-hidden className="size-3.5" />
        {t(($) => $.transcript.open_dsh_web)}
      </a>
      <Button variant="ghost" size="icon-sm" aria-label={t(($) => $.transcript.copy_dsh_session)}
        title={task.session_id} onClick={() => { void copyText(task.session_id!); }}>
        <Copy aria-hidden className="size-3.5" />
      </Button>
    </span>
  );
}
