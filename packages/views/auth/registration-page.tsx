"use client";

import { useCallback, useEffect, useRef, useState, type ReactNode } from "react";
import {
  Card,
  CardHeader,
  CardTitle,
  CardDescription,
  CardContent,
  CardFooter,
} from "@multica/ui/components/ui/card";
import { Alert, AlertDescription } from "@multica/ui/components/ui/alert";
import { Input } from "@multica/ui/components/ui/input";
import { Button } from "@multica/ui/components/ui/button";
import { Label } from "@multica/ui/components/ui/label";
import { api } from "@multica/core/api";
import { useConfigStore } from "@multica/core/config";
import type { User } from "@multica/core/types";
import { accountErrorKey, useAccountErrorText, useT } from "../i18n";
import { useNavigation } from "../navigation";
import { cn } from "@multica/ui/lib/utils";

const INVITE_FRAGMENT_PREFIX = "#token=";

/**
 * Accepts either a bare invitation secret or a full invitation link and returns
 * the secret. Pasting the link is the primary path (the admin panel copies a URL
 * with the token in the fragment), so the field has to understand both.
 */
export function extractInviteToken(raw: string): string {
  const value = raw.trim();
  if (!value) return "";
  const marker = value.indexOf(INVITE_FRAGMENT_PREFIX);
  if (marker >= 0) {
    const fragment = value.slice(marker + INVITE_FRAGMENT_PREFIX.length);
    const cut = fragment.search(/[&#?\s]/);
    return decodeURIComponent(cut >= 0 ? fragment.slice(0, cut) : fragment);
  }
  return value;
}

function readInviteTokenFromFragment(): string {
  if (typeof window === "undefined") return "";
  const hash = window.location.hash || "";
  if (!hash.startsWith(INVITE_FRAGMENT_PREFIX)) return "";
  const token = decodeURIComponent(hash.slice(INVITE_FRAGMENT_PREFIX.length));
  // Remove the secret from the address bar and history immediately; it must not
  // reach a Referer, a history entry, a screenshot or storage.
  window.history.replaceState(null, "", window.location.pathname + window.location.search);
  return token;
}

export interface RegistrationSuccess {
  token: string;
  user: User;
}

interface RegistrationPageProps {
  logo?: ReactNode;
  /**
   * Called after the server created the session. The host platform injects the
   * completion (cookie, auth store, destination), so this shared component never
   * navigates on success and never guesses where a new account should land.
   */
  onSuccess: (result: RegistrationSuccess) => void;
  /** Leave registration. Web returns to the login route; desktop goes back to
   *  its login step. */
  onBack?: () => void;
}

/**
 * Invitation / open registration.
 *
 * Mode is decided only after `authConfigLoaded`: deciding earlier flashes a
 * dead-end card while the capability request is still in flight. Invitation mode
 * always offers the paste field, even with no token in the URL — under an
 * invitation-only instance that field is the only way in.
 */
export function RegistrationPage({ logo, onSuccess, onBack }: RegistrationPageProps) {
  const { t } = useT("auth");
  const { t: tErrors } = useT("errors");
  const errorText = useAccountErrorText();
  const navigation = useNavigation();
  const authConfigLoaded = useConfigStore((state) => state.authConfigLoaded);
  const openRegistrationEnabled = useConfigStore((state) => state.openRegistrationEnabled);
  const localPasswordLoginEnabled = useConfigStore((state) => state.localPasswordLoginEnabled);

  // The fragment is read in an effect, not in a state initializer: React may
  // discard and re-run an initializer, and reading it twice would strip the
  // secret on the first pass and come back empty on the second. An effect runs
  // exactly once per mount, which is also the first moment a side effect like
  // rewriting the address bar is safe.
  const [fragmentToken, setFragmentToken] = useState("");
  const [fragmentChecked, setFragmentChecked] = useState(false);
  // Default to invitation mode: an invitation-only instance must never present
  // an open form, and with the capability still loading the paste field is the
  // honest state. The effect below promotes it to "open" only when the instance
  // explicitly allows open registration and the visitor has not chosen yet.
  const [mode, setMode] = useState<"invite" | "open">("invite");
  const modeTouched = useRef(false);
  const [inviteInput, setInviteInput] = useState(fragmentToken);
  const [email, setEmail] = useState("");
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [fieldError, setFieldError] = useState<
    "invite" | "email" | "name" | "username" | "password" | ""
  >("");
  const [submitting, setSubmitting] = useState(false);

  const inviteRef = useRef<HTMLInputElement>(null);
  const emailRef = useRef<HTMLInputElement>(null);
  const nameRef = useRef<HTMLInputElement>(null);
  const usernameRef = useRef<HTMLInputElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);

  // The URL decides the initial mode; the segmented control can still change it
  // when the instance allows open registration. A token in the fragment always
  // wins, because it is the credential the user arrived with.
  useEffect(() => {
    const token = readInviteTokenFromFragment();
    if (token) {
      setFragmentToken(token);
      setInviteInput(token);
      setMode("invite");
    }
    setFragmentChecked(true);
  }, []);

  useEffect(() => {
    if (fragmentToken || modeTouched.current) return;
    if (!authConfigLoaded) return;
    setMode(openRegistrationEnabled ? "open" : "invite");
  }, [fragmentToken, openRegistrationEnabled, authConfigLoaded]);

  const inviteMode = fragmentToken ? true : mode === "invite";
  const canChooseMode = !fragmentToken && openRegistrationEnabled;
  const unavailable = authConfigLoaded && !localPasswordLoginEnabled;

  const focusField = useCallback((field: typeof fieldError) => {
    switch (field) {
      case "invite":
        inviteRef.current?.focus();
        break;
      case "email":
        emailRef.current?.focus();
        break;
      case "name":
        nameRef.current?.focus();
        break;
      case "username":
        usernameRef.current?.focus();
        break;
      case "password":
        passwordRef.current?.focus();
        break;
      default:
        break;
    }
  }, []);

  // Focus lands after the commit, so the field the error is about is the one a
  // keyboard or screen-reader user arrives at even when the failing request was
  // submitted from a different field.
  useEffect(() => {
    if (!error || !fieldError) return;
    focusField(fieldError);
  }, [error, fieldError, focusField]);

  const fail = useCallback(
    (field: Exclude<typeof fieldError, "">, message: string) => {
      setError(message);
      setFieldError(field);
      focusField(field);
    },
    [focusField],
  );

  const submit = async (event: React.FormEvent) => {
    event.preventDefault();
    if (submitting || unavailable) return;
    setError("");
    setFieldError("");
    const token = extractInviteToken(inviteInput);
    if (inviteMode && !token) {
      fail("invite", t(($) => $.registration.error_invite_required));
      return;
    }
    if (!inviteMode && !email.trim()) {
      fail("email", t(($) => $.registration.error_email_required));
      return;
    }
    if (!name.trim()) {
      fail("name", t(($) => $.registration.error_name_required));
      return;
    }
    if (!username.trim()) {
      fail("username", t(($) => $.registration.error_username_required));
      return;
    }
    if (password !== confirm) {
      fail("password", t(($) => $.registration.error_password_mismatch));
      return;
    }
    setSubmitting(true);
    try {
      const result = inviteMode
        ? await api.redeemRegistrationInvite({
            token,
            name: name.trim(),
            username: username.trim(),
            password,
          })
        : await api.openRegistration({
            email: email.trim(),
            name: name.trim(),
            username: username.trim(),
            password,
          });
      setPassword("");
      setConfirm("");
      if (!result.token) {
        // A malformed response must not be treated as a created account.
        setError(tErrors(($) => $.invalid_response));
        setSubmitting(false);
        return;
      }
      onSuccess({ token: result.token, user: result.user });
    } catch (mutationError) {
      // Only the secrets are cleared; the invitation, display name and login
      // name stay so a retry is one edit away.
      setPassword("");
      setConfirm("");
      setError(errorText(mutationError));
      // A rejected credential names the field the retry should start from; the
      // password itself was cleared, so that is the honest default.
      const code = accountErrorKey(mutationError);
      const target =
        code === "invalid_invite" || code === "already_activated"
          ? "invite"
          : code === "email_taken" || code === "invalid_email"
            ? "email"
            : code === "username_taken" || code === "invalid_username"
              ? "username"
              : code === "invalid_display_name"
                ? "name"
                : "password";
      setFieldError(target);
      setSubmitting(false);
    }
  };

  const back = () => {
    if (onBack) {
      onBack();
      return;
    }
    navigation.push("/login");
  };

  if (!authConfigLoaded) {
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader className="space-y-3 text-center">
            {logo}
            <CardTitle className="text-display-sm">
              {t(($) => $.registration.title)}
            </CardTitle>
          </CardHeader>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-3 text-center">
          {logo}
          <CardTitle className="text-display-sm">
            {t(($) => $.registration.title)}
          </CardTitle>
          {inviteMode && fragmentChecked && !fragmentToken ? (
            <CardDescription>
              {t(($) => $.registration.invite_only_hint)}
            </CardDescription>
          ) : null}
        </CardHeader>
        <CardContent className="space-y-4">
          {unavailable ? (
            <Alert>
              <AlertDescription>
                {t(($) => $.registration.unavailable)}
              </AlertDescription>
            </Alert>
          ) : null}
          {!unavailable ? (
            <form
              id="registration-form"
              noValidate
              onSubmit={submit}
              className="space-y-4"
            >
              {canChooseMode ? (
                <div
                  role="group"
                  aria-label={t(($) => $.registration.title)}
                  className="grid grid-cols-2 gap-1 rounded-lg bg-muted p-1"
                >
                  <Button
                    type="button"
                    size="sm"
                    variant={inviteMode ? "default" : "ghost"}
                    aria-pressed={inviteMode}
                    onClick={() => {
                      modeTouched.current = true;
                      setMode("invite");
                      setError("");
                      setFieldError("");
                    }}
                  >
                    {t(($) => $.registration.mode_invite)}
                  </Button>
                  <Button
                    type="button"
                    size="sm"
                    variant={!inviteMode ? "default" : "ghost"}
                    aria-pressed={!inviteMode}
                    onClick={() => {
                      modeTouched.current = true;
                      setMode("open");
                      setError("");
                      setFieldError("");
                    }}
                  >
                    {t(($) => $.registration.mode_open)}
                  </Button>
                </div>
              ) : null}

              {inviteMode ? (
                <div className="space-y-2">
                  <Label htmlFor="registration-invite">
                    {t(($) => $.registration.invite_label)}
                  </Label>
                  <Input
                    id="registration-invite"
                    ref={inviteRef}
                    name="registration-invite"
                    type="text"
                    autoComplete="off"
                    spellCheck={false}
                    autoFocus
                    value={inviteInput}
                    onChange={(event) => setInviteInput(event.target.value)}
                    placeholder={t(($) => $.registration.invite_placeholder)}
                    disabled={submitting}
                    aria-invalid={fieldError === "invite" ? true : undefined}
                  />
                </div>
              ) : (
                <div className="space-y-2">
                  <Label htmlFor="registration-email">
                    {t(($) => $.registration.email_label)}
                  </Label>
                  <Input
                    id="registration-email"
                    ref={emailRef}
                    name="registration-email"
                    type="email"
                    autoComplete="email"
                    value={email}
                    onChange={(event) => setEmail(event.target.value)}
                    disabled={submitting}
                    aria-invalid={fieldError === "email" ? true : undefined}
                  />
                </div>
              )}

              <div className="space-y-2">
                <Label htmlFor="registration-name">
                  {t(($) => $.registration.name_label)}
                </Label>
                <Input
                  id="registration-name"
                  ref={nameRef}
                  name="registration-name"
                  type="text"
                  autoComplete="name"
                  value={name}
                  onChange={(event) => setName(event.target.value)}
                  disabled={submitting}
                  aria-invalid={fieldError === "name" ? true : undefined}
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="registration-username">
                  {t(($) => $.registration.username_label)}
                </Label>
                <Input
                  id="registration-username"
                  ref={usernameRef}
                  name="registration-username"
                  type="text"
                  autoComplete="username"
                  value={username}
                  onChange={(event) => setUsername(event.target.value)}
                  disabled={submitting}
                  aria-invalid={fieldError === "username" ? true : undefined}
                  aria-describedby="registration-username-hint"
                />
                <p
                  id="registration-username-hint"
                  className="text-caption text-muted-foreground"
                >
                  {t(($) => $.registration.username_hint)}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="registration-password">
                  {t(($) => $.registration.password_label)}
                </Label>
                <Input
                  id="registration-password"
                  ref={passwordRef}
                  name="registration-password"
                  type="password"
                  autoComplete="new-password"
                  value={password}
                  onChange={(event) => setPassword(event.target.value)}
                  disabled={submitting}
                  aria-invalid={fieldError === "password" ? true : undefined}
                  aria-describedby="registration-password-hint"
                />
                <p
                  id="registration-password-hint"
                  className="text-caption text-muted-foreground"
                >
                  {t(($) => $.registration.password_hint)}
                </p>
              </div>

              <div className="space-y-2">
                <Label htmlFor="registration-confirm">
                  {t(($) => $.registration.confirm_label)}
                </Label>
                <Input
                  id="registration-confirm"
                  name="registration-confirm"
                  type="password"
                  autoComplete="new-password"
                  value={confirm}
                  onChange={(event) => setConfirm(event.target.value)}
                  disabled={submitting}
                  aria-invalid={fieldError === "password" ? true : undefined}
                />
              </div>

              {error ? (
                <p role="alert" className={cn("text-body text-destructive")}>
                  {error}
                </p>
              ) : null}

              <Button
                type="submit"
                className="w-full"
                size="lg"
                disabled={submitting || unavailable}
              >
                {submitting
                  ? t(($) => $.registration.submitting)
                  : t(($) => $.registration.submit)}
              </Button>
            </form>
          ) : null}
        </CardContent>
        <CardFooter className="flex flex-col gap-3">
          <Button
            type="button"
            variant="ghost"
            className="w-full"
            onClick={back}
            disabled={submitting}
          >
            {t(($) => $.registration.back)}
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}
