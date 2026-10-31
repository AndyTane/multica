import { queryOptions } from "@tanstack/react-query";
import { api } from "../api";

/**
 * Instance (deployment) administration data.
 *
 * These keys are ACCOUNT-level on purpose: the user list and the registration
 * switch belong to the whole instance, not to the workspace the operator happens
 * to have open. No key here carries a `wsId`, so switching workspaces neither
 * refetches nor drops them, and a workspace-scoped invalidation cannot silently
 * leave a stale administrative view behind.
 */
export const instanceAdminKeys = {
  all: ["instance-admin"] as const,
  users: () => ["instance-admin", "users"] as const,
  registrationSettings: () => ["instance-admin", "registration-settings"] as const,
  deletePreview: (userId: string) =>
    ["instance-admin", "delete-preview", userId] as const,
};

export function instanceUsersOptions() {
  return queryOptions({
    queryKey: instanceAdminKeys.users(),
    queryFn: () => api.listInstanceUsers(),
  });
}

export function registrationSettingsOptions() {
  return queryOptions({
    queryKey: instanceAdminKeys.registrationSettings(),
    queryFn: () => api.getRegistrationSettings(),
  });
}

/**
 * What the server would remove and what blocks deleting this account. The query
 * is keyed by the target so two open dialogs cannot read each other's answer,
 * and it is only enabled once a target is chosen.
 */
export function instanceUserDeletePreviewOptions(userId: string) {
  return queryOptions({
    queryKey: instanceAdminKeys.deletePreview(userId),
    queryFn: () => api.getInstanceUserDeletePreview(userId),
    enabled: userId.length > 0,
    // A preview is authorization-neutral but must not be reused as the
    // operator's mental model of a changed account: always re-read on open.
    staleTime: 0,
    gcTime: 0,
  });
}
