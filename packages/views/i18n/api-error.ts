"use client";

import { errorCode } from "@multica/core/api";
import { useT } from "./use-t";

/**
 * The stable error codes the account-administration and registration endpoints
 * emit. Everything the UI shows for a failed call goes through this list and the
 * `errors` translation namespace: no handler message and no raw code ever
 * reaches a user, and a code added by a newer server degrades to the generic
 * sentence instead of leaking an identifier.
 */
export const ACCOUNT_ERROR_CODES = [
  "invalid_invite",
  "registration_disabled",
  "already_activated",
  "username_taken",
  "email_taken",
  "invalid_password",
  "invalid_username",
  "invalid_email",
  "invalid_display_name",
  "invalid_current_password",
  "current_password_required",
  "no_password_set",
  "cannot_delete_self",
  "last_admin",
  "workspace_owner",
  "owns_workspaces",
  "runtime_owner",
  "agent_owner",
  "owns_runtimes",
  "owns_agents",
  "channel_installation",
  "lark_installation",
  "share_link_creator",
  "source_context_capture",
  "instance_admin_required",
  "human_actor_required",
  "invalid_user_id",
  "user_not_found",
  "rate_limited",
  "dependency_changed",
  "revocation_unavailable",
  "invalid_request_body",
  "invalid_content_type",
  "invalid_origin",
  "local_password_login_disabled",
  "invalid_response",
] as const;

export type AccountErrorCode = (typeof ACCOUNT_ERROR_CODES)[number];
const KNOWN_CODES = new Set<string>(ACCOUNT_ERROR_CODES);

const GENERIC_CODE = "generic";

/**
 * Pick the message key for a failed call. The structured `code` field wins;
 * legacy endpoints that only send `{ error }` may put the code in the message,
 * and anything unrecognised is the generic sentence. Never returns raw text.
 */
export function accountErrorKey(err: unknown): AccountErrorCode | typeof GENERIC_CODE {
  const code = errorCode(err);
  if (code && KNOWN_CODES.has(code)) return code as AccountErrorCode;
  return GENERIC_CODE;
}

/** Localized sentence for a failed account call. */
export function useAccountErrorText() {
  const { t } = useT("errors");
  return (err: unknown): string => t(($) => $[accountErrorKey(err)]);
}
