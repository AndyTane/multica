"use client";

import { useCallback, useEffect, useMemo, useState, type ReactNode } from "react";
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

interface RegistrationPageProps {
  logo?: ReactNode;
  /** Called after a successful registration; defaults to the onboarding route. */
  onSuccess?: () => void;
}

const INVITE_FRAGMENT_PREFIX = "#token=";

function readInviteTokenFromFragment(): string {
  if (typeof window === "undefined") return "";
  const hash = window.location.hash || "";
  if (!hash.startsWith(INVITE_FRAGMENT_PREFIX)) return "";
  const token = decodeURIComponent(hash.slice(INVITE_FRAGMENT_PREFIX.length));
  // Remove the secret from the address bar/history immediately; it must not
  // reach Referer, history entries, screenshots or storage.
  window.history.replaceState(null, "", window.location.pathname + window.location.search);
  return token;
}

function humanError(message: string): string {
  switch (message) {
    case "invalid_invite":
      return "This invitation is invalid or has expired.";
    case "registration_disabled":
      return "Open registration is disabled on this instance.";
    case "already_activated":
      return "This account already has a password.";
    case "username_taken":
      return "That login name is already in use.";
    case "email_taken":
      return "That email is already registered.";
    case "invalid_password":
      return "Password must be at least 12 characters and at most 72 bytes.";
    case "invalid_username":
      return "Login name must be 3-64 letters, digits, dots, underscores or hyphens.";
    case "invalid_email":
      return "Please enter a valid email address.";
    case "invalid_display_name":
      return "Please enter a display name.";
    default:
      return "Something went wrong. Please try again.";
  }
}

export function RegistrationPage({ logo, onSuccess }: RegistrationPageProps) {
  const openRegistrationEnabled = useConfigStore((s) => s.openRegistrationEnabled);
  const localPasswordLoginEnabled = useConfigStore((s) => s.localPasswordLoginEnabled);
  const [token, setToken] = useState("");
  const [name, setName] = useState("");
  const [username, setUsername] = useState("");
  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    setToken(readInviteTokenFromFragment());
  }, []);

  const mode = useMemo<"invite" | "open" | "closed">(() => {
    if (token.trim()) return "invite";
    if (openRegistrationEnabled) return "open";
    return "closed";
  }, [openRegistrationEnabled, token]);

  const submit = useCallback(
    async (event?: React.FormEvent) => {
      event?.preventDefault();
      if (loading) return;
      setError("");
      if (!name.trim()) {
        setError("Please enter a display name.");
        return;
      }
      if (!username.trim()) {
        setError("Please enter a login name.");
        return;
      }
      if (password !== confirm) {
        setError("The two passwords do not match.");
        return;
      }
      setLoading(true);
      try {
        if (mode === "invite") {
          await api.redeemRegistrationInvite({ token: token.trim(), name, username, password });
        } else if (mode === "open") {
          await api.openRegistration({ email, name, username, password });
        } else {
          throw new Error("registration_disabled");
        }
        setPassword("");
        setConfirm("");
        if (onSuccess) onSuccess();
        else window.location.assign("/onboarding");
      } catch (err) {
        // Clear only the secrets; the invite, display name and login name stay
        // so a retry is one edit away (web checklist for P5).
        setPassword("");
        setConfirm("");
        setError(humanError(err instanceof Error ? err.message : ""));
        setLoading(false);
      }
    },
    [confirm, email, loading, mode, name, onSuccess, password, token, username],
  );

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-md">
        <CardHeader className="space-y-3 text-center">
          {logo}
          <CardTitle className="text-display-sm">Set up your account</CardTitle>
          <CardDescription>
            {mode === "invite"
              ? "Use your invitation to choose a login name and password."
              : mode === "open"
                ? "Create an account on this instance."
                : "This instance requires an invitation to register."}
          </CardDescription>
        </CardHeader>
        <CardContent className="space-y-4">
          {mode === "closed" ? (
            <Alert>
              <AlertDescription>
                Ask an instance administrator for an invitation link.
              </AlertDescription>
            </Alert>
          ) : null}
          {mode === "invite" ? (
            <div className="space-y-2">
              <Label htmlFor="registration-invite">Invitation token</Label>
              <Input
                id="registration-invite"
                type="text"
                autoComplete="off"
                value={token}
                onChange={(e) => setToken(e.target.value)}
                placeholder="Paste the invitation token"
              />
            </div>
          ) : null}
          {mode === "open" ? (
            <div className="space-y-2">
              <Label htmlFor="registration-email">Email</Label>
              <Input
                id="registration-email"
                type="email"
                autoComplete="email"
                value={email}
                onChange={(e) => setEmail(e.target.value)}
              />
            </div>
          ) : null}
          <div className="space-y-2">
            <Label htmlFor="registration-name">Display name</Label>
            <Input
              id="registration-name"
              type="text"
              autoComplete="name"
              value={name}
              onChange={(e) => setName(e.target.value)}
              disabled={mode === "closed" || loading}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="registration-username">Login name</Label>
            <Input
              id="registration-username"
              type="text"
              autoComplete="username"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              disabled={mode === "closed" || loading}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="registration-password">Password</Label>
            <Input
              id="registration-password"
              type="password"
              autoComplete="new-password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              disabled={mode === "closed" || loading}
            />
          </div>
          <div className="space-y-2">
            <Label htmlFor="registration-confirm">Confirm password</Label>
            <Input
              id="registration-confirm"
              type="password"
              autoComplete="new-password"
              value={confirm}
              onChange={(e) => setConfirm(e.target.value)}
              disabled={mode === "closed" || loading}
            />
          </div>
          {error ? <p className="text-body text-destructive">{error}</p> : null}
        </CardContent>
        <CardFooter className="flex flex-col gap-3">
          <Button
            className="w-full"
            size="lg"
            disabled={loading || mode === "closed" || !localPasswordLoginEnabled}
            onClick={() => void submit()}
          >
            {loading ? "Setting up..." : "Set password and continue"}
          </Button>
          <Button variant="ghost" className="w-full" onClick={() => window.location.assign("/login")}>
            Back to sign in
          </Button>
        </CardFooter>
      </Card>
    </div>
  );
}
