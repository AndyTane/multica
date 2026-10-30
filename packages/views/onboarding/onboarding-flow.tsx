"use client";

import { useState } from "react";
import { toast } from "sonner";
import { useAuthStore } from "@multica/core/auth";
import { completeOnboarding, useBootstrapMika, useWelcomeStore, type OnboardingStep } from "@multica/core/onboarding";
import { useWorkspaceList } from "@multica/core/workspace";
import type { AgentRuntime, Workspace } from "@multica/core/types";
import { Button } from "@multica/ui/components/ui/button";
import { StepWelcome } from "./steps/step-welcome";
import { StepShell, StepHeading, StepFooter } from "./components/step-shell";
import { StepWorkspace } from "./steps/step-workspace";
import { StepRuntimeConnect } from "./steps/step-runtime-connect";
import { OnboardingLogoutButton } from "./components/onboarding-logout-button";
import { getMikaOnboarding, pickContentLang } from "./templates";
import { useT } from "../i18n";

interface OnboardingFlowProps {
  onComplete: (workspace?: Workspace, destination?: OnboardingDestination) => void;
  mode?: OnboardingMode;
  onCancel?: () => void;
  runtimeInstructions?: React.ReactNode;
  onRuntimeRefresh?: () => void | Promise<void>;
  runtimesPending?: boolean;
}

/** Authenticated service → workspace → runtime/model → first task. No profile survey. */
export function OnboardingFlow({ onComplete, mode = "first_run", onCancel, runtimeInstructions, onRuntimeRefresh, runtimesPending }: OnboardingFlowProps) {
  const { t, i18n } = useT("onboarding");
  const user = useAuthStore((s) => s.user);
  if (!user) throw new Error("OnboardingFlow requires an authenticated user");
  const isNewWorkspace = mode === "new_workspace";
  const [step, setStep] = useState<OnboardingStep>(isNewWorkspace ? "workspace" : "welcome");
  const [workspace, setWorkspace] = useState<Workspace | null>(null);
  const [runtime, setRuntime] = useState<AgentRuntime | null>(null);
  const [model, setModel] = useState<string>();
  const [busy, setBusy] = useState(false);
  const bootstrapMika = useBootstrapMika(workspace?.id ?? "");
  const { workspaces, ready } = useWorkspaceList({ enabled: step === "welcome" || step === "workspace" });

  async function finish() {
    if (!workspace || busy) return;
    setBusy(true);
    try {
      if (runtime) {
        // Opening the guided task is explicit; selecting a runtime never starts a model.
        const result = await bootstrapMika.mutateAsync({ workspaceSlug: workspace.slug, runtimeId: runtime.id, model, ...getMikaOnboarding(pickContentLang(i18n.language)) });
        await completeOnboarding("full", workspace.id);
        onComplete(workspace, { kind: "chat", sessionId: result.chatSession.id });
      } else {
        await completeOnboarding("runtime_skipped", workspace.id);
        useWelcomeStore.getState().set({ workspaceId: workspace.id, choice: "skip" });
        onComplete(workspace);
      }
    } catch (err) {
      toast.error(err instanceof Error ? err.message : t(($) => $.errors.mika_setup_failed));
    } finally { setBusy(false); }
  }

  if (step === "welcome") return <>
    <OnboardingLogoutButton />
    <StepWelcome onNext={() => setStep("workspace")} onSkip={ready && workspaces.length ? async () => {
      try { await completeOnboarding("skip_existing", workspaces[0]?.id); onComplete(workspaces[0]); }
      catch (err) { toast.error(err instanceof Error ? err.message : t(($) => $.errors.skip_failed)); }
    } : undefined} />
  </>;

  const back = step === "workspace"
    ? (isNewWorkspace ? onCancel : () => setStep("welcome"))
    : step === "task" ? () => setStep("runtime") : undefined;
  return <StepShell currentStep={step} onBack={back} backDisabled={busy} chromeFooter={<OnboardingLogoutButton inline />}>
    {step === "workspace" && <StepWorkspace existing={isNewWorkspace ? workspace : workspace ?? workspaces[0] ?? null} onCreated={(ws) => { setWorkspace(ws); setStep("runtime"); }} onBusyChange={setBusy} />}
    {step === "runtime" && workspace && <>
      {runtimeInstructions}
      <StepRuntimeConnect wsId={workspace.id} wsSlug={workspace.slug} currentUserId={user.id} onNext={(rt, selectedModel) => { setRuntime(rt); setModel(selectedModel); setStep("task"); }} onRefresh={onRuntimeRefresh} runtimesPending={runtimesPending} />
    </>}
    {step === "task" && <>
      <StepHeading title={t(($) => $.task.title)} description={runtime ? t(($) => $.task.connected) : t(($) => $.task.no_runtime)} />
      {runtime && <p className="mt-6 break-words text-body text-muted-foreground">{runtime.name}{model ? ` · ${model}` : ""}</p>}
      <StepFooter><Button size="lg" onClick={() => void finish()} disabled={busy}>{busy ? t(($) => $.task.starting) : runtime ? t(($) => $.task.start) : t(($) => $.task.open_workspace)}</Button></StepFooter>
    </>}
  </StepShell>;
}

export type OnboardingMode = "first_run" | "new_workspace";
export type OnboardingDestination = { kind: "issue"; issueId: string } | { kind: "chat"; sessionId: string };
export type { OnboardingStep };
