"use client";

import { useEffect, useRef, useState } from "react";
import { useMutation } from "@tanstack/react-query";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { useAccountErrorText, useT } from "../../i18n";
import { markPasswordChanged } from "../../auth/password-changed-notice";
import { useNavigation } from "../../navigation";
import { SettingsCard, SettingsSection } from "./settings-layout";

/**
 * Password change for the signed-in account.
 *
 * A successful change revokes every session for the account — including this
 * one — so the only honest success state is: clear the fields, sign out, and let
 * the login page carry a one-shot notice. Showing "changed" inside a session
 * that is already dead is a message the user usually never sees (the next
 * request 401s and the shell unmounts). Failures clear all three fields and put
 * the focus back on the current-password input.
 */
export function PasswordChangeCard() {
  const { t } = useT("settings");
  const errorText = useAccountErrorText();
  const navigation = useNavigation();
  const logout = useAuthStore((state) => state.logout);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const currentRef = useRef<HTMLInputElement>(null);

  // Focus is applied after the commit so it survives the click that submitted
  // the form; a synchronous focus inside the rejection handler loses to the
  // browser's own focus handling for the button.
  useEffect(() => {
    if (!error) return;
    currentRef.current?.focus();
  }, [error]);

  const change = useMutation({
    mutationFn: () =>
      api.changeCurrentPassword({
        current_password: currentPassword,
        new_password: newPassword,
      }),
    onSuccess: () => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirm("");
      setError("");
      // The marker survives the shell's own redirect to /login; the URL query is
      // best-effort and the sign-in page reads both.
      markPasswordChanged();
      logout();
      navigation.push("/login?reason=password_changed");
    },
    onError: (mutationError) => {
      setCurrentPassword("");
      setNewPassword("");
      setConfirm("");
      setError(errorText(mutationError));
      currentRef.current?.focus();
    },
  });

  return (
    <SettingsSection
      title={t(($) => $.password_change.title)}
      description={t(($) => $.password_change.description)}
    >
      <SettingsCard>
        <form
          className="space-y-3"
          noValidate
          onSubmit={(event) => {
            event.preventDefault();
            if (change.isPending) return;
            setError("");
            if (newPassword !== confirm) {
              setError(t(($) => $.password_change.error_mismatch));
              currentRef.current?.focus();
              return;
            }
            change.mutate();
          }}
        >
          <div className="space-y-1">
            <Label htmlFor="current-password">
              {t(($) => $.password_change.current_label)}
            </Label>
            <Input
              id="current-password"
              ref={currentRef}
              name="current-password"
              type="password"
              autoComplete="current-password"
              value={currentPassword}
              onChange={(event) => setCurrentPassword(event.target.value)}
              disabled={change.isPending}
              aria-invalid={error ? true : undefined}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="new-password">
              {t(($) => $.password_change.new_label)}
            </Label>
            <Input
              id="new-password"
              name="new-password"
              type="password"
              autoComplete="new-password"
              value={newPassword}
              onChange={(event) => setNewPassword(event.target.value)}
              disabled={change.isPending}
              aria-invalid={error ? true : undefined}
              aria-describedby="new-password-hint"
            />
            <p
              id="new-password-hint"
              className="text-caption text-muted-foreground"
            >
              {t(($) => $.password_change.password_hint)}
            </p>
          </div>
          <div className="space-y-1">
            <Label htmlFor="confirm-new-password">
              {t(($) => $.password_change.confirm_label)}
            </Label>
            <Input
              id="confirm-new-password"
              name="confirm-new-password"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(event) => setConfirm(event.target.value)}
              disabled={change.isPending}
              aria-invalid={error ? true : undefined}
            />
          </div>
          {error ? (
            <p role="alert" className="text-body text-destructive">
              {error}
            </p>
          ) : null}
          <Button
            type="submit"
            disabled={
              change.isPending || !currentPassword || !newPassword || !confirm
            }
          >
            {change.isPending
              ? t(($) => $.password_change.submitting)
              : t(($) => $.password_change.submit)}
          </Button>
        </form>
      </SettingsCard>
    </SettingsSection>
  );
}
