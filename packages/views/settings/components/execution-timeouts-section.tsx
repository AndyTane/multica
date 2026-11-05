"use client";

import { useEffect, useId, useState } from "react";
import {
  DEFAULT_EXECUTION_TIMEOUTS,
  MAX_TIMEOUT_SECONDS,
  type ExecutionTimeouts,
} from "@multica/core/agents/execution-timeouts";
import { Button } from "@multica/ui/components/ui/button";
import { Input } from "@multica/ui/components/ui/input";
import { Switch } from "@multica/ui/components/ui/switch";
import { useT } from "../../i18n";
import { SettingsCard, SettingsRow, SettingsSaveState, SettingsSection, type SettingsSaveStatus } from "./settings-layout";

interface Props {
  scope: "workspace" | "agent";
  value: ExecutionTimeouts | null;
  inherited?: ExecutionTimeouts | null;
  canEdit: boolean;
  provider?: string;
  clientSupported?: boolean;
  serverSupported?: boolean;
  invalid?: boolean;
  onSave: (value: ExecutionTimeouts | null) => Promise<void>;
}

// New policies follow the general wait now that the OpenCode override is hidden.
// Existing policies retain their stored extension values when editing the two waits.
const NEW_EXECUTION_TIMEOUTS: ExecutionTimeouts = {
  ...DEFAULT_EXECUTION_TIMEOUTS,
  opencodeIdleSeconds: 0,
};

function DurationField({ label, value, zeroLabel, disabled, onChange }: {
  label: string;
  value: number;
  zeroLabel: string;
  disabled: boolean;
  onChange: (value: number) => void;
}) {
  const { t } = useT("settings");
  const id = useId();
  const [text, setText] = useState(() => value === 0 ? "" : String(value / 3600));
  useEffect(() => {
    setText((current) => Number(current || "0") * 3600 === value ? current : value === 0 ? "" : String(value / 3600));
  }, [value]);
  const valid = Number.isInteger(value) && value >= 0 && value <= MAX_TIMEOUT_SECONDS;
  return (
    <SettingsRow label={<label htmlFor={id}>{label}</label>} size="text">
      <div className="flex flex-col gap-2 sm:items-end">
        <div className="flex items-center gap-2">
          <Input id={id} type="number" aria-label={t(($) => $.execution_timeouts.duration, { label })}
            aria-invalid={!valid} disabled={disabled} min={0} step="any" placeholder={zeroLabel}
            className="w-14 appearance-none px-2 text-right tabular-nums [appearance:textfield] [&::-webkit-inner-spin-button]:appearance-none [&::-webkit-outer-spin-button]:appearance-none"
            value={text}
            onChange={(e) => { setText(e.target.value); onChange(e.target.value === "" ? 0 : Number(e.target.value) * 3600); }}
            onBlur={() => { if (value === 0) setText(""); }} />
          <span className="text-body text-muted-foreground">{t(($) => $.execution_timeouts.hours)}</span>
        </div>
        {!valid && <span role="alert" className="text-caption text-destructive">{t(($) => $.execution_timeouts.invalid)}</span>}
      </div>
    </SettingsRow>
  );
}

export function ExecutionTimeoutsSection({ scope, value: storedValue, inherited: storedInherited, canEdit, clientSupported, serverSupported = true, invalid, onSave }: Props) {
  const { t } = useT("settings");
  const value = serverSupported ? storedValue : null;
  const inherited = serverSupported ? storedInherited : null;
  const editable = canEdit && serverSupported;
  const id = useId();
  const savedKey = JSON.stringify(value);
  const [enabled, setEnabled] = useState(value !== null);
  const [draft, setDraft] = useState(value ?? inherited ?? NEW_EXECUTION_TIMEOUTS);
  const [status, setStatus] = useState<SettingsSaveStatus>("idle");
  const [error, setError] = useState("");
  useEffect(() => {
    setEnabled(value !== null);
    setDraft(value ?? inherited ?? NEW_EXECUTION_TIMEOUTS);
    setStatus((current) =>
      (current === "saving" || current === "saved") && savedKey === JSON.stringify(enabled ? draft : null)
        ? "saved" : "idle",
    );
    setError("");
    // Serialized policy identity avoids resetting drafts on unrelated cache edits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [savedKey, scope, serverSupported]);
  const effective = enabled ? draft : inherited;
  const policy = enabled ? draft : null;
  const changed = JSON.stringify(policy) !== savedKey;
  const valid = [draft.idleSeconds, draft.toolIdleSeconds, draft.runSeconds, draft.opencodeIdleSeconds]
    .every((n) => Number.isInteger(n) && n >= 0 && n <= MAX_TIMEOUT_SECONDS) &&
    (draft.opencodeShellSeconds === undefined || Number.isInteger(draft.opencodeShellSeconds) && draft.opencodeShellSeconds > 0 && draft.opencodeShellSeconds <= MAX_TIMEOUT_SECONDS);
  const disabled = !editable || !enabled || status === "saving";
  const shown = effective ?? NEW_EXECUTION_TIMEOUTS;
  async function save() {
    setStatus("saving"); setError("");
    try { await onSave(policy); setStatus("saved"); }
    catch (err) { setStatus("error"); setError(err instanceof Error ? err.message : t(($) => $.auto_save.failed)); }
  }
  return (
    <SettingsSection title={t(($) => $.execution_timeouts.title)} anchor="execution-timeouts"
      action={<SettingsSaveState status={status} savingLabel={t(($) => $.auto_save.saving)} savedLabel={t(($) => $.auto_save.saved)} errorLabel={t(($) => $.auto_save.failed)} />}>
      {!serverSupported && <p role="status" className="text-body text-muted-foreground">{t(($) => $.execution_timeouts.server_upgrade_required)}</p>}
      {serverSupported && invalid && <p role="alert" className="text-body text-destructive">{t(($) => $.execution_timeouts.invalid_saved)}</p>}
      {serverSupported && scope === "agent" && (value || inherited) && clientSupported === false &&
        <p role="status" className="text-body text-warning-foreground">{t(($) => $.execution_timeouts.upgrade_required)}</p>}
      <SettingsCard>
        <SettingsRow label={<label htmlFor={id}>{scope === "agent" ? t(($) => $.execution_timeouts.inherit) : t(($) => $.execution_timeouts.manage)}</label>}>
          <Switch id={id} disabled={!editable || status === "saving"} checked={scope === "agent" ? !enabled : enabled}
            onCheckedChange={(checked) => {
              const next = scope === "agent" ? !checked : checked;
              if (next && !enabled) setDraft(value ?? inherited ?? NEW_EXECUTION_TIMEOUTS);
              setEnabled(next); setStatus("idle");
            }} />
        </SettingsRow>
        {(enabled || effective) && <>
          <DurationField label={t(($) => $.execution_timeouts.idle)} value={shown.idleSeconds}
            zeroLabel={t(($) => $.execution_timeouts.off)} disabled={disabled}
            onChange={(n) => setDraft({ ...draft, idleSeconds: n })} />
          <DurationField label={t(($) => $.execution_timeouts.tool_idle)} value={shown.toolIdleSeconds}
            zeroLabel={t(($) => $.execution_timeouts.unlimited)} disabled={disabled}
            onChange={(n) => setDraft({ ...draft, toolIdleSeconds: n })} />
        </>}
      </SettingsCard>
      {error && <p role="alert" className="text-body text-destructive">{error}</p>}
      {editable && <Button variant="outline" size="sm" disabled={!changed || enabled && !valid || status === "saving"} onClick={() => void save()}>
        {t(($) => $.execution_timeouts.save)}
      </Button>}
    </SettingsSection>
  );
}
