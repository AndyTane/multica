"use client";

import { useState } from "react";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { api } from "@multica/core/api";
import { useAuthStore } from "@multica/core/auth";
import { SettingsCard, SettingsSection } from "./settings-layout";

export function PasswordChangeCard() {
  const logout = useAuthStore((s) => s.logout);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState("");
  const [done, setDone] = useState(false);

  const submit = async () => {
    if (busy) return;
    setError("");
    setMessage("");
    if (newPassword !== confirm) {
      setError("The two new passwords do not match.");
      return;
    }
    setBusy(true);
    try {
      await api.changeCurrentPassword({ current_password: currentPassword, new_password: newPassword });
      setCurrentPassword("");
      setNewPassword("");
      setConfirm("");
      setDone(true);
      setMessage("Password changed. Sign in again with the new password.");
      logout();
    } catch (err) {
      const raw = err instanceof Error ? err.message : "";
      if (raw === "invalid_current_password") setError("The current password is incorrect.");
      else if (raw === "no_password_set") setError("This account has no local password yet.");
      else if (raw === "invalid_password") setError("Password must be at least 12 characters and at most 72 bytes.");
      else setError(raw || "Failed to change the password.");
    } finally {
      setBusy(false);
    }
  };

  return (
    <SettingsSection title="Password" description="Change the password used to sign in on this instance.">
      <SettingsCard>
        <div className="space-y-3">
          <div className="space-y-1">
            <Label htmlFor="current-password">Current password</Label>
            <Input id="current-password" type="password" autoComplete="current-password" value={currentPassword} onChange={(e) => { setCurrentPassword(e.target.value); setMessage(""); setError(""); }} disabled={busy} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="new-password">New password</Label>
            <Input id="new-password" type="password" autoComplete="new-password" value={newPassword} onChange={(e) => setNewPassword(e.target.value)} disabled={busy} />
          </div>
          <div className="space-y-1">
            <Label htmlFor="confirm-new-password">Confirm new password</Label>
            <Input id="confirm-new-password" type="password" autoComplete="new-password" value={confirm} onChange={(e) => setConfirm(e.target.value)} disabled={busy} />
          </div>
          {error ? <p className="text-body text-destructive">{error}</p> : null}
          {message ? <p className="text-body text-muted-foreground">{message}</p> : null}
          <Button disabled={busy || !currentPassword || !newPassword || !confirm} onClick={() => void submit()}>
            {busy ? "Saving..." : "Change password"}
          </Button>
          {done ? (
            <Button variant="outline" onClick={() => window.location.assign("/login")}>
              Go to sign in
            </Button>
          ) : null}
        </div>
      </SettingsCard>
    </SettingsSection>
  );
}
