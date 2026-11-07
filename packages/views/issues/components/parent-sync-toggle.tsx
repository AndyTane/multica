"use client";

import { useId } from "react";
import { Checkbox } from "@multica/ui/components/ui/checkbox";
import { cn } from "@multica/ui/lib/utils";
import type { ParentSyncToggleKind } from "@multica/core/issues/parent-sync";
import { parentSyncReasonMessageKey } from "@multica/core/issues/server-codes";
import { useT } from "../../i18n";

export interface ParentSyncToggleProps {
  kind: ParentSyncToggleKind;
  /** Recipient display name; absent for a parent issue with no owner name yet. */
  name: string | null;
  /** Why the sync cannot be delivered (`unavailable` only). */
  reason: string | null;
  checked: boolean;
  locked: boolean;
  disabled: boolean;
  onCheckedChange: (checked: boolean) => void;
}

/**
 * "Also hand this comment to the parent issue's owner" (CLAWSTA-151 F3).
 *
 * One control, four readings: the parent's owner, a fallback recipient when the
 * commenter cannot wake that owner, an unavailable destination (reason shown),
 * and the locked-on state when the ordinary routing already reaches them.
 */
function ParentSyncToggle({
  kind,
  name,
  reason,
  checked,
  locked,
  disabled,
  onCheckedChange,
}: ParentSyncToggleProps) {
  const { t } = useT("issues");
  const fieldId = useId();
  const unavailable = kind === "unavailable";
  const label =
    kind === "recipient" && name
      ? t(($) => $.parent_sync.sync_to_recipient, { name })
      : name
        ? t(($) => $.parent_sync.sync_to_owner, { name })
        : t(($) => $.parent_sync.sync_to_owner_unnamed);

  return (
    <div className="flex min-w-0 items-start gap-2">
      {/* Base UI marks a disabled checkbox with `data-disabled` instead of the
          native attribute, so the shared primitive's `disabled:opacity-50` never
          applies. Fade it here, where the locked/unavailable reading is decided. */}
      <Checkbox
        id={fieldId}
        checked={checked}
        disabled={disabled || locked}
        onCheckedChange={(next) => onCheckedChange(next === true)}
        className="mt-0.5 data-disabled:opacity-50"
      />
      <label
        htmlFor={fieldId}
        className={cn(
          "min-w-0 cursor-pointer select-none text-caption leading-4 text-muted-foreground",
          (disabled || locked) && "cursor-default",
        )}
      >
        {label}
        {locked && <> · {t(($) => $.parent_sync.locked_hint)}</>}
        {unavailable && reason && (
          <> · {t(($) => $.parent_sync.reason[parentSyncReasonMessageKey(reason)])}</>
        )}
      </label>
    </div>
  );
}

export { ParentSyncToggle };
