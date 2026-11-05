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

function DurationField({ label, value, zeroLabel, optional, disabled, onChange }: {
  label: string;
  value: number | undefined;
  zeroLabel: string;
  optional?: boolean;
  disabled: boolean;
  onChange: (value: number | undefined) => void;
}) {
  const { t } = useT("settings");
  const id = useId();
  const [unit, setUnit] = useState(() => value && value % 3600 === 0 ? 3600 : value && value % 60 !== 0 ? 1 : 60);
  const custom = optional ? value !== undefined : value !== 0;
  const valid = value === undefined ? !!optional : Number.isInteger(value) && value >= (optional ? 1 : 0) && value <= MAX_TIMEOUT_SECONDS;
  return (
    <SettingsRow label={<label htmlFor={id}>{label}</label>} size="text">
      <div className="flex flex-col gap-2 sm:items-end">
        <div className="flex flex-wrap items-center gap-2">
          <select id={id} aria-label={label} disabled={disabled} value={custom ? "custom" : "zero"}
            className="h-8 rounded-md border bg-background px-2 text-body disabled:opacity-50"
            onChange={(e) => onChange(e.target.value === "zero" ? optional ? undefined : 0 : 7200)}>
            <option value="zero">{zeroLabel}</option>
            <option value="custom">{t(($) => $.execution_timeouts.custom)}</option>
          </select>
          {custom && <>
            <Input type="number" aria-label={t(($) => $.execution_timeouts.duration, { label })}
              aria-invalid={!valid} disabled={disabled} min={optional ? 1 / unit : 0} step="any"
              className="w-24" value={Number.isFinite(value) ? (value ?? 0) / unit : ""}
              onChange={(e) => onChange(e.target.value === "" ? NaN : Number(e.target.value) * unit)} />
            <select aria-label={t(($) => $.execution_timeouts.unit, { label })} value={unit} disabled={disabled}
              className="h-8 rounded-md border bg-background px-2 text-body disabled:opacity-50"
              onChange={(e) => setUnit(Number(e.target.value))}>
              <option value={60}>{t(($) => $.execution_timeouts.minutes)}</option>
              <option value={3600}>{t(($) => $.execution_timeouts.hours)}</option>
              <option value={1}>{t(($) => $.execution_timeouts.seconds)}</option>
            </select>
          </>}
        </div>
        {!valid && <span role="alert" className="text-caption text-destructive">{t(($) => $.execution_timeouts.invalid)}</span>}
      </div>
    </SettingsRow>
  );
}

export function ExecutionTimeoutsSection({ scope, value: storedValue, inherited: storedInherited, canEdit, provider, clientSupported, serverSupported = true, invalid, onSave }: Props) {
  const { t } = useT("settings");
  const value = serverSupported ? storedValue : null;
  const inherited = serverSupported ? storedInherited : null;
  const editable = canEdit && serverSupported;
  const id = useId();
  const savedKey = JSON.stringify(value);
  const [enabled, setEnabled] = useState(value !== null);
  const [draft, setDraft] = useState(value ?? inherited ?? DEFAULT_EXECUTION_TIMEOUTS);
  const [status, setStatus] = useState<SettingsSaveStatus>("idle");
  const [error, setError] = useState("");
  useEffect(() => {
    setEnabled(value !== null);
    setDraft(value ?? inherited ?? DEFAULT_EXECUTION_TIMEOUTS);
    setStatus((current) =>
      (current === "saving" || current === "saved") && savedKey === JSON.stringify(enabled ? draft : null)
        ? "saved" : "idle",
    );
    setError("");
    // Serialized policy identity avoids resetting drafts on unrelated cache edits.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [savedKey, scope, serverSupported]);
  const effective = enabled ? draft : inherited;
  const source = value ? scope === "agent" ? t(($) => $.execution_timeouts.source_agent) : t(($) => $.execution_timeouts.source_workspace)
    : inherited ? t(($) => $.execution_timeouts.source_workspace) : t(($) => $.execution_timeouts.source_client);
  const policy = enabled ? draft : null;
  const changed = JSON.stringify(policy) !== savedKey;
  const valid = [draft.idleSeconds, draft.toolIdleSeconds, draft.runSeconds, draft.opencodeIdleSeconds]
    .every((n) => Number.isInteger(n) && n >= 0 && n <= MAX_TIMEOUT_SECONDS) &&
    (draft.opencodeShellSeconds === undefined || Number.isInteger(draft.opencodeShellSeconds) && draft.opencodeShellSeconds > 0 && draft.opencodeShellSeconds <= MAX_TIMEOUT_SECONDS);
  const disabled = !editable || !enabled || status === "saving";
  const shown = effective ?? DEFAULT_EXECUTION_TIMEOUTS;
  const openCode = scope === "workspace" || provider === "opencode" || provider === "codearts";
  const shell = scope === "workspace" || provider === "opencode";
  async function save() {
    setStatus("saving"); setError("");
    try { await onSave(policy); setStatus("saved"); }
    catch (err) { setStatus("error"); setError(err instanceof Error ? err.message : t(($) => $.auto_save.failed)); }
  }
  return (
    <SettingsSection title={t(($) => $.execution_timeouts.title)} anchor="execution-timeouts"
      action={<SettingsSaveState status={status} savingLabel={t(($) => $.auto_save.saving)} savedLabel={t(($) => $.auto_save.saved)} errorLabel={t(($) => $.auto_save.failed)} />}>
      <p className="text-caption text-muted-foreground">{source}</p>
      {!serverSupported && <p role="status" className="text-body text-muted-foreground">{t(($) => $.execution_timeouts.server_upgrade_required)}</p>}
      {serverSupported && invalid && <p role="alert" className="text-body text-destructive">{t(($) => $.execution_timeouts.invalid_saved)}</p>}
      {serverSupported && scope === "agent" && (value || inherited) && !clientSupported &&
        <p role="status" className="text-body text-warning-foreground">{t(($) => $.execution_timeouts.upgrade_required)}</p>}
      <SettingsCard>
        <SettingsRow label={<label htmlFor={id}>{scope === "agent" ? t(($) => $.execution_timeouts.inherit) : t(($) => $.execution_timeouts.manage)}</label>}>
          <Switch id={id} disabled={!editable || status === "saving"} checked={scope === "agent" ? !enabled : enabled}
            onCheckedChange={(checked) => {
              const next = scope === "agent" ? !checked : checked;
              if (next && !enabled) setDraft(value ?? inherited ?? DEFAULT_EXECUTION_TIMEOUTS);
              setEnabled(next); setStatus("idle");
            }} />
        </SettingsRow>
        {(enabled || effective) && <>
          <DurationField label={t(($) => $.execution_timeouts.idle)} value={shown.idleSeconds}
            zeroLabel={t(($) => $.execution_timeouts.disabled)} disabled={disabled}
            onChange={(n) => setDraft({ ...draft, idleSeconds: n ?? 0 })} />
          {shown.idleSeconds === 0 && <p className="px-4 py-3 text-caption text-muted-foreground">{t(($) => $.execution_timeouts.disabled_help)}</p>}
          <DurationField label={t(($) => $.execution_timeouts.tool_idle)} value={shown.toolIdleSeconds}
            zeroLabel={t(($) => $.execution_timeouts.unlimited)} disabled={disabled}
            onChange={(n) => setDraft({ ...draft, toolIdleSeconds: n ?? 0 })} />
          <DurationField label={t(($) => $.execution_timeouts.run)} value={shown.runSeconds}
            zeroLabel={t(($) => $.execution_timeouts.unlimited)} disabled={disabled}
            onChange={(n) => setDraft({ ...draft, runSeconds: n ?? 0 })} />
          {openCode && <>
            <SettingsRow label={t(($) => $.execution_timeouts.opencode)}>
              <span className="text-caption text-muted-foreground">{t(($) => $.execution_timeouts.opencode_help)}</span>
            </SettingsRow>
            <DurationField label={t(($) => $.execution_timeouts.opencode_idle)} value={shown.opencodeIdleSeconds}
              zeroLabel={t(($) => $.execution_timeouts.follow)} disabled={disabled}
              onChange={(n) => setDraft({ ...draft, opencodeIdleSeconds: n ?? 0 })} />
            {shown.idleSeconds > 0 && shown.opencodeIdleSeconds > shown.idleSeconds &&
              <p className="px-4 py-3 text-caption text-muted-foreground">{t(($) => $.execution_timeouts.effective_wait, {
                value: shown.idleSeconds / (shown.idleSeconds % 3600 === 0 ? 3600 : shown.idleSeconds % 60 === 0 ? 60 : 1),
                unit: shown.idleSeconds % 3600 === 0 ? t(($) => $.execution_timeouts.hours) : shown.idleSeconds % 60 === 0 ? t(($) => $.execution_timeouts.minutes) : t(($) => $.execution_timeouts.seconds),
              })}</p>}
            {shell && <DurationField label={t(($) => $.execution_timeouts.shell)} value={shown.opencodeShellSeconds} optional
              zeroLabel={t(($) => $.execution_timeouts.engine_default)} disabled={disabled}
              onChange={(n) => setDraft({ ...draft, opencodeShellSeconds: n })} />}
            {shell && <p className="px-4 py-3 text-caption text-muted-foreground">{t(($) => $.execution_timeouts.shell_help)}</p>}
          </>}
        </>}
      </SettingsCard>
      {error && <p role="alert" className="text-body text-destructive">{error}</p>}
      {editable && <Button variant="outline" size="sm" disabled={!changed || enabled && !valid || status === "saving"} onClick={() => void save()}>
        {t(($) => $.execution_timeouts.save)}
      </Button>}
    </SettingsSection>
  );
}
