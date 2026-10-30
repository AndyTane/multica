import { api } from "../api";
import { useAuthStore } from "../auth";
import type { OnboardingCompletionPath } from "./types";

export async function completeOnboarding(
  completionPath?: OnboardingCompletionPath,
  workspaceId?: string,
): Promise<void> {
  await api.markOnboardingComplete(
    completionPath || workspaceId
      ? { completion_path: completionPath, workspace_id: workspaceId }
      : undefined,
  );
  await useAuthStore.getState().refreshMe();
}
