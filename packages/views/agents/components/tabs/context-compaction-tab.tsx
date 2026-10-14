"use client";

import { useEffect, useId, useMemo, useRef, useState } from "react";
import type { Agent } from "@multica/core/types";
import { contextCompactionSchema, mergeContextCompaction, parseContextCompaction } from "@multica/core/agents/context-compaction";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { Loader2, Save } from "lucide-react";
import { toast } from "sonner";
import { useT } from "../../../i18n";

type FormState = { enabled: boolean; threshold: string };
const equal = (a: FormState, b: FormState) => a.enabled === b.enabled && a.threshold === b.threshold;

export function ContextCompactionTab({ agent, canEdit, onSave, onDirtyChange }: {
  agent: Agent;
  canEdit: boolean;
  onSave: (updates: { runtime_config: Record<string, unknown> }) => Promise<void>;
  onDirtyChange?: (dirty: boolean) => void;
}) {
  const { t } = useT("agents");
  const id = useId();
  const original = useMemo(() => parseContextCompaction(agent.runtime_config), [agent.runtime_config]);
  const originalForm = useMemo(() => ({ enabled: original.enabled, threshold: String(original.threshold_tokens) }), [original]);
  const [state, setState] = useState(originalForm);
  const [saving, setSaving] = useState(false);
  const previous = useRef(originalForm);
  useEffect(() => {
    const before = previous.current;
    previous.current = originalForm;
    setState((current) => equal(current, before) ? originalForm : current);
  }, [originalForm]);
  const dirty = !equal(state, originalForm);
  useEffect(() => { onDirtyChange?.(dirty); }, [dirty, onDirtyChange]);
  const parsed = contextCompactionSchema.safeParse({ enabled: state.enabled, threshold_tokens: /^\d+$/.test(state.threshold) ? Number(state.threshold) : NaN });
  const valid = parsed.success;
  const save = async () => {
    if (!canEdit || saving || !dirty || state.enabled && !valid) return;
    const policy = parsed.success ? parsed.data : { enabled: false, threshold_tokens: original.threshold_tokens };
    setSaving(true);
    try {
      await onSave({ runtime_config: mergeContextCompaction(agent.runtime_config, policy) });
      setState({ enabled: policy.enabled, threshold: String(policy.threshold_tokens) });
      toast.success(t(($) => $.tab_body.context_compaction.saved_toast));
    } catch (error) {
      toast.error(error instanceof Error ? error.message : t(($) => $.tab_body.context_compaction.save_failed_toast));
    } finally { setSaving(false); }
  };
  return (
    <div className="space-y-5">
      <div className="flex items-center justify-between gap-4">
        <Label htmlFor={`${id}-enabled`}>{t(($) => $.tab_body.context_compaction.enabled)}</Label>
        <Switch id={`${id}-enabled`} checked={state.enabled} disabled={!canEdit || saving}
          onCheckedChange={(enabled) => setState((current) => ({ ...current, enabled }))} />
      </div>
      <p className="text-caption text-muted-foreground">{t(($) => $.tab_body.context_compaction.help)}</p>
      <div className="space-y-2">
        <Label htmlFor={`${id}-threshold`}>{t(($) => $.tab_body.context_compaction.threshold)}</Label>
        <Input id={`${id}-threshold`} type="number" inputMode="numeric" min={1000} max={2000000} step={1}
          value={state.threshold} disabled={!canEdit || saving || !state.enabled}
          aria-invalid={state.enabled && !valid} aria-describedby={state.enabled && !valid ? `${id}-error` : undefined}
          onChange={(event) => setState((current) => ({ ...current, threshold: event.target.value }))} />
        {state.enabled && !valid && <p id={`${id}-error`} role="alert" className="text-caption text-destructive">{t(($) => $.tab_body.context_compaction.invalid)}</p>}
      </div>
      {canEdit && <Button onClick={() => { void save(); }} disabled={!dirty || saving || state.enabled && !valid} aria-busy={saving} size="sm">
        {saving ? <Loader2 aria-hidden className="size-3.5 animate-spin" /> : <Save aria-hidden className="size-3.5" />}
        {t(($) => $.tab_body.common.save)}
      </Button>}
    </div>
  );
}
