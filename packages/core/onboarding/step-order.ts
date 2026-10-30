import type { OnboardingStep } from "./types";

/** Service is confirmed before these workspace-scoped setup steps. */
export const ONBOARDING_STEP_ORDER: readonly OnboardingStep[] = ["workspace", "runtime", "task"];
