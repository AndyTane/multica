"use client";

import { createContext, useCallback, useContext, useEffect, useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Button } from "@multica/ui/components/ui/button";
import { useT } from "../../i18n";

// Owned by the issue page so virtualized rows retain the reader's choice.
export const CommentBodySession = createContext<{
  choices: Map<string, boolean>;
  postedId: string | null;
} | null>(null);

export function useCommentBodyExpansion(commentId: string, autoExpand: boolean) {
  const session = useContext(CommentBodySession);
  const choices = session?.choices;
  const reveal = autoExpand || session?.postedId === commentId;
  const [expanded, setExpanded] = useState(() => reveal || choices?.get(commentId) === true);
  const changeExpanded = useCallback((value: boolean) => {
    choices?.set(commentId, value);
    setExpanded(value);
  }, [commentId, choices]);
  useEffect(() => {
    if (reveal) changeExpanded(true);
  }, [reveal, changeExpanded]);
  return [expanded, changeExpanded] as const;
}

export function CommentBody({ children, expanded, onExpandedChange }: {
  children: ReactNode;
  expanded: boolean;
  onExpandedChange: (expanded: boolean) => void;
}) {
  const { t } = useT("issues");
  const id = useId();
  const contentRef = useRef<HTMLDivElement>(null);
  const [isLong, setIsLong] = useState(false);
  useLayoutEffect(() => {
    const content = contentRef.current;
    if (!content) return;
    const measure = () => setIsLong(content.getBoundingClientRect().height > 320);
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(measure);
    observer.observe(content);
    return () => observer.disconnect();
  }, [children]);
  const collapsed = isLong && !expanded;
  return (
    <div className="min-w-0 max-w-full" data-comment-body={collapsed ? "collapsed" : "expanded"}>
      <div
        id={id}
        className="min-w-0 max-w-full"
        style={collapsed ? {
          maxHeight: 200,
          overflow: "clip",
          maskImage: "linear-gradient(to bottom, black 140px, transparent)",
        } : undefined}
        onFocusCapture={() => { if (collapsed) onExpandedChange(true); }}
      >
        <div ref={contentRef} className="flow-root min-w-0 max-w-full">{children}</div>
      </div>
      {isLong && (
        <Button type="button" variant="ghost" size="xs" className="mt-1 text-muted-foreground"
          aria-expanded={expanded} aria-controls={id} onClick={() => onExpandedChange(!expanded)}>
          {expanded ? t(($) => $.comment.collapse_body) : t(($) => $.comment.expand_body)}
        </Button>
      )}
    </div>
  );
}
