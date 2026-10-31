"use client";

import { useCallback, useEffect, useState } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
} from "@multica/ui/components/ui/card";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import type {
  InstanceAdminUser,
  InstanceUsersResponse,
  RegistrationSettings,
} from "@multica/core/api/schemas";

function errorText(err: unknown): string {
  const message = err instanceof Error ? err.message : "";
  switch (message) {
    case "email_taken":
      return "That email is already registered.";
    case "invalid_email":
      return "Please enter a valid email address.";
    case "already_activated":
      return "This account already has a password.";
    case "cannot_delete_self":
      return "You cannot delete your own account.";
    case "last_admin":
      return "The last instance administrator cannot be deleted.";
    case "owns_workspaces":
      return "Transfer or delete this user's workspaces first.";
    case "owns_runtimes":
    case "owns_agents":
      return "This user still owns agent runtimes or agents.";
    case "instance_admin_required":
      return "Instance administrator permission is required.";
    default:
      return message || "Something went wrong. Please try again.";
  }
}

export function InstanceUsersPage() {
  const user = useAuthStore((s) => s.user);
  const isLoading = useAuthStore((s) => s.isLoading);
  const [users, setUsers] = useState<InstanceAdminUser[]>([]);
  const [settings, setSettings] = useState<RegistrationSettings | null>(null);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(true);
  const [addName, setAddName] = useState("");
  const [addEmail, setAddEmail] = useState("");
  const [busy, setBusy] = useState(false);
  const [issuedInvite, setIssuedInvite] = useState<{ email: string; token: string; expires: string } | null>(null);
  const [copied, setCopied] = useState(false);

  const load = useCallback(async () => {
    setLoading(true);
    setError("");
    try {
      const [userList, registration] = await Promise.all([
        api.listInstanceUsers(),
        api.getRegistrationSettings(),
      ]);
      setUsers((userList as InstanceUsersResponse).users ?? []);
      setSettings(registration);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    if (isLoading) return;
    if (!user) {
      window.location.assign("/login");
      return;
    }
    if (!user.is_instance_admin) {
      setLoading(false);
      return;
    }
    void load();
  }, [isLoading, load, user]);

  if (isLoading || (loading && users.length === 0 && !error)) {
    return (
      <div className="flex min-h-svh items-center justify-center text-body text-muted-foreground">
        Loading users...
      </div>
    );
  }

  if (!user || !user.is_instance_admin) {
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>Instance administrators only</CardTitle>
            <CardDescription>
              This page manages instance accounts. A workspace owner without the
              instance administrator flag cannot open it.
            </CardDescription>
          </CardHeader>
          <CardContent>
            <Button onClick={() => window.location.assign("/")}>Back</Button>
          </CardContent>
        </Card>
      </div>
    );
  }

  const copyInvite = async (token: string) => {
    try {
      await navigator.clipboard.writeText(token);
      setCopied(true);
    } catch {
      setCopied(false);
    }
  };

  const addUser = async () => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api.addInstanceUser({ name: addName.trim(), email: addEmail.trim() });
      setAddName("");
      setAddEmail("");
      setIssuedInvite({ email: result.user.email, token: result.invite_token, expires: result.invite_expires_at });
      await load();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const regenerate = async (target: InstanceAdminUser) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const result = await api.regenerateInstanceInvite(target.id);
      setIssuedInvite({ email: target.email, token: result.invite_token, expires: result.invite_expires_at });
      await load();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const revoke = async (target: InstanceAdminUser) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      await api.revokeInstanceInvite(target.id);
      setIssuedInvite(null);
      await load();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const remove = async (target: InstanceAdminUser) => {
    if (busy) return;
    if (!window.confirm("Delete " + target.email + "? This removes the account, its password and invitations. Workspace-owned content is not deleted.")) {
      return;
    }
    setBusy(true);
    setError("");
    try {
      await api.deleteInstanceUser(target.id);
      await load();
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  const setOpenRegistration = async (enabled: boolean) => {
    if (busy) return;
    setBusy(true);
    setError("");
    try {
      const next = await api.updateRegistrationSettings({ open_registration_enabled: enabled });
      setSettings(next);
    } catch (err) {
      setError(errorText(err));
    } finally {
      setBusy(false);
    }
  };

  return (
    <div className="mx-auto flex w-full max-w-3xl flex-col gap-6 p-6">
      <Card>
        <CardHeader>
          <CardTitle>Instance users</CardTitle>
          <CardDescription>
            Manage accounts on this instance. You are signed in as {user.email}.
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {error ? (
            <Alert>
              <AlertDescription>{error}</AlertDescription>
            </Alert>
          ) : null}
          {issuedInvite ? (
            <div className="space-y-2 rounded-lg border p-4">
              <p className="text-body font-medium">Invitation for {issuedInvite.email}</p>
              <p className="text-caption text-muted-foreground">
                Shown once and never again. Expires {new Date(issuedInvite.expires).toLocaleString()}.
              </p>
              <div className="flex flex-wrap items-center gap-2">
                <code className="max-w-full break-all rounded bg-muted px-2 py-1 text-caption">{issuedInvite.token}</code>
                <Button size="sm" variant="outline" onClick={() => void copyInvite(issuedInvite.token)}>
                  {copied ? "Copied" : "Copy"}
                </Button>
              </div>
              <p className="text-caption text-muted-foreground">
                Open the registration page with HTTPS and paste this token in the invitation field.
              </p>
            </div>
          ) : null}

          <div className="rounded-lg border p-4">
            <p className="mb-2 text-body font-medium">Add a pending user</p>
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="admin-add-name">Display name</Label>
                <Input id="admin-add-name" value={addName} onChange={(e) => setAddName(e.target.value)} />
              </div>
              <div className="space-y-1">
                <Label htmlFor="admin-add-email">Email</Label>
                <Input id="admin-add-email" type="email" value={addEmail} onChange={(e) => setAddEmail(e.target.value)} />
              </div>
            </div>
            <Button className="mt-3" disabled={busy || !addName.trim() || !addEmail.trim()} onClick={() => void addUser()}>
              Add user and create invitation
            </Button>
          </div>

          <div className="rounded-lg border p-4">
            <div className="flex items-center justify-between gap-3">
              <div>
                <p className="text-body font-medium">Open registration</p>
                <p className="text-caption text-muted-foreground">
                  When off (default), only invitation links can create accounts.
                </p>
              </div>
              <Button
                variant={settings?.open_registration_enabled ? "default" : "outline"}
                disabled={busy || !settings}
                onClick={() => void setOpenRegistration(!(settings?.open_registration_enabled ?? false))}
              >
                {settings?.open_registration_enabled ? "Enabled" : "Disabled"}
              </Button>
            </div>
          </div>

          <div className="space-y-2">
            {users.map((entry) => (
              <div key={entry.id} className="flex flex-wrap items-center justify-between gap-3 rounded-lg border p-3">
                <div className="min-w-0">
                  <p className="truncate text-body font-medium">
                    {entry.name} <span className="text-muted-foreground">({entry.email})</span>
                  </p>
                  <p className="text-caption text-muted-foreground">
                    {entry.is_instance_admin ? "Instance administrator" : "User"}
                    {" - "}
                    {entry.has_password ? "Password set" : "Pending activation"}
                    {entry.invite_expires_at ? " - invitation active" : ""}
                  </p>
                </div>
                <div className="flex flex-wrap gap-2">
                  {!entry.has_password ? (
                    <>
                      <Button size="sm" variant="outline" disabled={busy} onClick={() => void regenerate(entry)}>
                        New invitation
                      </Button>
                      <Button size="sm" variant="ghost" disabled={busy} onClick={() => void revoke(entry)}>
                        Revoke
                      </Button>
                    </>
                  ) : null}
                  <Button size="sm" variant="destructive" disabled={busy || entry.id === user.id} onClick={() => void remove(entry)}>
                    Delete
                  </Button>
                </div>
              </div>
            ))}
          </div>
        </CardContent>
      </Card>
      <Button variant="ghost" className="self-start" onClick={() => window.location.assign("/")}>
        Back to workspaces
      </Button>
    </div>
  );
}
