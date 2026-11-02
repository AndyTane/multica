"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Ellipsis } from "lucide-react";
import { toast } from "sonner";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Badge } from "@multica/ui/components/ui/badge";
import { Button } from "@multica/ui/components/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@multica/ui/components/ui/dropdown-menu";
import { Input } from "@multica/ui/components/ui/input";
import { Label } from "@multica/ui/components/ui/label";
import { Switch } from "@multica/ui/components/ui/switch";
import { useAuthStore } from "@multica/core/auth";
import { api } from "@multica/core/api";
import type { InstanceAdminUser } from "@multica/core/api/schemas";
import {
  instanceAdminKeys,
  instanceUsersOptions,
  registrationSettingsOptions,
} from "@multica/core/instance-admin/queries";
import { useOptionalNavigation } from "../../navigation";
import { useAccountErrorText, useLocale, useT } from "../../i18n";
import { DeleteInstanceUserDialog } from "./delete-instance-user-dialog";
import {
  SettingsCard,
  SettingsRow,
  SettingsSection,
  SettingsTab,
} from "./settings-layout";

interface IssuedInvite {
  email: string;
  token: string;
  expiresAt: string;
}

/**
 * Instance accounts and the registration switch, as a settings tab.
 *
 * The data is account-level: `instanceUsersOptions` / `registrationSettingsOptions`
 * carry no workspace id, so switching workspaces neither refetches nor drops
 * them. Every write is a mutation that waits for the server and then invalidates
 * the account-level keys — nothing here is applied optimistically.
 *
 * The invitation secret lives in this component's state only. It is never put in
 * a query cache, storage or telemetry, and leaving the tab (or pressing 完成)
 * drops it.
 */
export function InstanceUsersTab() {
  const { t } = useT("settings");
  const { t: tErrors } = useT("errors");
  const errorText = useAccountErrorText();
  const locale = useLocale();
  const navigation = useOptionalNavigation();
  const qc = useQueryClient();
  const user = useAuthStore((state) => state.user);
  const isAdmin = user?.is_instance_admin === true;

  const usersQuery = useQuery({ ...instanceUsersOptions(), enabled: isAdmin });
  const settingsQuery = useQuery({
    ...registrationSettingsOptions(),
    enabled: isAdmin,
  });

  const [addName, setAddName] = useState("");
  const [addEmail, setAddEmail] = useState("");
  const [formError, setFormError] = useState("");
  const [invite, setInvite] = useState<IssuedInvite | null>(null);
  const [copied, setCopied] = useState(false);
  const [copyFailed, setCopyFailed] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<InstanceAdminUser | null>(null);
  const [deleteOpen, setDeleteOpen] = useState(false);
  const linkRef = useRef<HTMLInputElement>(null);

  // Leaving the tab drops the one-time secret. It exists only in memory.
  useEffect(() => {
    return () => setInvite(null);
  }, []);

  const inviteLink = useMemo(() => {
    if (!invite?.token || !navigation) return null;
    return navigation.getExternalShareUrl(`/register#token=${invite.token}`);
  }, [invite, navigation]);

  const formatDate = useCallback(
    (value: string) => {
      const parsed = new Date(value);
      if (Number.isNaN(parsed.getTime())) return value;
      return new Intl.DateTimeFormat(locale, {
        dateStyle: "medium",
        timeStyle: "short",
      }).format(parsed);
    },
    [locale],
  );

  const invalidateUsers = () =>
    qc.invalidateQueries({ queryKey: instanceAdminKeys.users() });

  const addUser = useMutation({
    mutationFn: (input: { name: string; email: string }) => api.addInstanceUser(input),
    onSuccess: async (result) => {
      setAddName("");
      setAddEmail("");
      setFormError("");
      if (!result.invite_token) {
        toast.error(tErrors(($) => $.invalid_response));
        return;
      }
      setCopied(false);
      setCopyFailed(false);
      setInvite({
        email: result.user.email,
        token: result.invite_token,
        expiresAt: result.invite_expires_at,
      });
      await invalidateUsers();
    },
    onError: (error) => setFormError(errorText(error)),
  });

  const regenerate = useMutation({
    mutationFn: (target: InstanceAdminUser) => api.regenerateInstanceInvite(target.id),
    onSuccess: async (result, target) => {
      if (!result.invite_token) {
        toast.error(tErrors(($) => $.invalid_response));
        return;
      }
      setCopied(false);
      setCopyFailed(false);
      setInvite({
        email: target.email,
        token: result.invite_token,
        expiresAt: result.invite_expires_at,
      });
      await invalidateUsers();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const revoke = useMutation({
    mutationFn: (target: InstanceAdminUser) => api.revokeInstanceInvite(target.id),
    onSuccess: async () => {
      setInvite(null);
      toast.success(t(($) => $.instance_admin.revoke_toast));
      await invalidateUsers();
    },
    onError: (error) => toast.error(errorText(error)),
  });

  const toggleRegistration = useMutation({
    mutationFn: (enabled: boolean) =>
      api.updateRegistrationSettings({ open_registration_enabled: enabled }),
    onSuccess: async () => {
      await qc.invalidateQueries({
        queryKey: instanceAdminKeys.registrationSettings(),
      });
    },
    onError: (error) => {
      // Nothing was applied optimistically, so the switch already shows the
      // server's value; re-read to make that explicit after a failure.
      void qc.invalidateQueries({
        queryKey: instanceAdminKeys.registrationSettings(),
      });
      toast.error(
        `${errorText(error)} ${t(($) => $.instance_admin.open_registration_failed)}`,
      );
    },
  });

  const copyInvite = async () => {
    if (!inviteLink) return;
    try {
      await navigator.clipboard.writeText(inviteLink);
      setCopied(true);
      setCopyFailed(false);
    } catch {
      setCopied(false);
      setCopyFailed(true);
      // Selecting the text is the fallback that actually lets the operator copy
      // it when the clipboard API is unavailable (insecure origin, permission).
      linkRef.current?.focus();
      linkRef.current?.select();
    }
  };

  if (!isAdmin) {
    return (
      <SettingsTab title={t(($) => $.page.tabs.admin_users)} scope="account">
        <SettingsCard>
          <p className="px-4 py-4 text-body text-muted-foreground">
            {t(($) => $.instance_admin.admin_only)}
          </p>
        </SettingsCard>
      </SettingsTab>
    );
  }

  const settings = settingsQuery.data;
  const users = usersQuery.data?.users ?? [];

  return (
    <SettingsTab
      title={t(($) => $.page.tabs.admin_users)}
      description={t(($) => $.instance_admin.description)}
      scope="account"
    >
      <SettingsSection>
        <SettingsCard>
          {/* SettingsCard is px-0; SettingsRow owns the row padding and puts
              the switch in the shared control slot. */}
          <SettingsRow
            label={t(($) => $.instance_admin.open_registration)}
            description={t(($) => $.instance_admin.open_registration_hint)}
          >
            <Switch
              aria-label={t(($) => $.instance_admin.open_registration)}
              checked={settings?.open_registration_enabled === true}
              disabled={!settings || toggleRegistration.isPending}
              onCheckedChange={(enabled) => toggleRegistration.mutate(enabled)}
            />
          </SettingsRow>
        </SettingsCard>
      </SettingsSection>

      <SettingsSection title={t(($) => $.instance_admin.add_user_title)}>
        <SettingsCard>
          <form
            className="space-y-3 px-4 py-4"
            onSubmit={(event) => {
              event.preventDefault();
              if (addUser.isPending) return;
              setFormError("");
              addUser.mutate({ name: addName.trim(), email: addEmail.trim() });
            }}
          >
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="space-y-1">
                <Label htmlFor="instance-admin-add-name">
                  {t(($) => $.instance_admin.name_label)}
                </Label>
                <Input
                  id="instance-admin-add-name"
                  name="instance-admin-add-name"
                  autoComplete="off"
                  value={addName}
                  onChange={(event) => setAddName(event.target.value)}
                  disabled={addUser.isPending}
                  aria-invalid={formError ? true : undefined}
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="instance-admin-add-email">
                  {t(($) => $.instance_admin.email_label)}
                </Label>
                <Input
                  id="instance-admin-add-email"
                  name="instance-admin-add-email"
                  type="email"
                  autoComplete="off"
                  value={addEmail}
                  onChange={(event) => setAddEmail(event.target.value)}
                  disabled={addUser.isPending}
                  aria-invalid={formError ? true : undefined}
                />
              </div>
            </div>
            {formError ? (
              <p role="alert" className="text-body text-destructive">
                {formError}
              </p>
            ) : null}
            <Button
              type="submit"
              disabled={addUser.isPending || !addName.trim() || !addEmail.trim()}
            >
              {addUser.isPending
                ? t(($) => $.instance_admin.adding)
                : t(($) => $.instance_admin.add_user)}
            </Button>
          </form>
        </SettingsCard>
      </SettingsSection>

      {invite ? (
        <SettingsSection title={t(($) => $.instance_admin.invite_title)}>
          <SettingsCard>
            <div className="space-y-3 px-4 py-4">
              <p className="text-body font-medium">{invite.email}</p>
              <p className="text-caption text-muted-foreground">
                {t(($) => $.instance_admin.invite_expires, {
                  date: formatDate(invite.expiresAt),
                })}
              </p>
              <div className="space-y-1">
                <Label htmlFor="instance-admin-invite-link">
                  {t(($) => $.instance_admin.invite_link_label)}
                </Label>
                <Input
                  id="instance-admin-invite-link"
                  ref={linkRef}
                  readOnly
                  autoComplete="off"
                  spellCheck={false}
                  value={inviteLink ?? ""}
                  onFocus={(event) => event.currentTarget.select()}
                  className="font-mono text-caption"
                />
              </div>
              {copyFailed ? (
                <p role="alert" className="text-caption text-destructive">
                  {t(($) => $.instance_admin.copy_failed)}
                </p>
              ) : null}
              <div className="flex flex-wrap gap-2">
                <Button
                  type="button"
                  disabled={!inviteLink}
                  onClick={() => void copyInvite()}
                >
                  {copied
                    ? t(($) => $.instance_admin.copied)
                    : t(($) => $.instance_admin.copy_invite)}
                </Button>
                <Button
                  type="button"
                  variant="outline"
                  onClick={() => {
                    setInvite(null);
                    setCopied(false);
                    setCopyFailed(false);
                  }}
                >
                  {t(($) => $.instance_admin.done)}
                </Button>
              </div>
            </div>
          </SettingsCard>
        </SettingsSection>
      ) : null}

      <SettingsSection>
        <SettingsCard>
          {usersQuery.isLoading ? (
            <p className="px-4 py-3 text-body text-muted-foreground">
              {t(($) => $.instance_admin.loading)}
            </p>
          ) : null}
          {usersQuery.isError ? (
            <div className="px-4 py-3">
              <Alert>
                <AlertDescription>{errorText(usersQuery.error)}</AlertDescription>
              </Alert>
            </div>
          ) : null}
          <ul className="divide-y">
            {users.map((entry) => {
              const isSelf = entry.id === user?.id;
              const pending = !entry.has_password;
              return (
                <li
                  key={entry.id}
                  className="flex flex-wrap items-center justify-between gap-3 px-4 py-3"
                >
                  <div className="min-w-0">
                    <p className="truncate text-body font-medium">{entry.name}</p>
                    <p className="truncate text-caption text-muted-foreground">
                      {entry.email}
                    </p>
                  </div>
                  <div className="flex flex-wrap items-center gap-2">
                    {entry.is_instance_admin ? (
                      <Badge variant="secondary">
                        {t(($) => $.instance_admin.badge_admin)}
                      </Badge>
                    ) : null}
                    <Badge variant={pending ? "outline" : "secondary"}>
                      {pending
                        ? t(($) => $.instance_admin.badge_pending)
                        : t(($) => $.instance_admin.badge_active)}
                    </Badge>
                    {entry.invite_expires_at ? (
                      <Badge variant="outline">
                        {t(($) => $.instance_admin.badge_invite_active, {
                          date: formatDate(entry.invite_expires_at),
                        })}
                      </Badge>
                    ) : null}
                    {pending ? (
                      <>
                        <Button
                          type="button"
                          variant="outline"
                          size="sm"
                          disabled={regenerate.isPending}
                          onClick={() => regenerate.mutate(entry)}
                        >
                          {t(($) => $.instance_admin.action_regenerate)}
                        </Button>
                        <Button
                          type="button"
                          variant="ghost"
                          size="sm"
                          disabled={revoke.isPending}
                          onClick={() => revoke.mutate(entry)}
                        >
                          {t(($) => $.instance_admin.action_revoke)}
                        </Button>
                      </>
                    ) : null}
                    {!isSelf ? (
                      <DropdownMenu>
                        <DropdownMenuTrigger
                          render={
                            <Button
                              variant="ghost"
                              size="icon-sm"
                              aria-label={t(($) => $.instance_admin.action_menu)}
                            />
                          }
                        >
                          <Ellipsis aria-hidden="true" />
                        </DropdownMenuTrigger>
                        <DropdownMenuContent align="end">
                          <DropdownMenuItem
                            variant="destructive"
                            onClick={() => {
                              setDeleteTarget(entry);
                              setDeleteOpen(true);
                            }}
                          >
                            {t(($) => $.instance_admin.action_delete)}
                          </DropdownMenuItem>
                        </DropdownMenuContent>
                      </DropdownMenu>
                    ) : null}
                  </div>
                </li>
              );
            })}
          </ul>
        </SettingsCard>
      </SettingsSection>

      <DeleteInstanceUserDialog
        target={deleteTarget}
        open={deleteOpen}
        onOpenChange={(open) => {
          setDeleteOpen(open);
          if (!open) setDeleteTarget(null);
        }}
      />
    </SettingsTab>
  );
}
