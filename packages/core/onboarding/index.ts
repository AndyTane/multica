export type {
  OnboardingStep,
  OnboardingCompletionPath,
  QuestionnaireAnswers,
  Source,
  Role,
  UseCase,
} from "./types";
export {
  completeOnboarding,
} from "./store";
export { ONBOARDING_STEP_ORDER } from "./step-order";
export {
  bootstrapMika,
  useBootstrapMika,
  type BootstrapMikaInput,
  type BootstrapMikaResult,
  type MikaOnboardingLanguage,
} from "./use-bootstrap-mika";
export {
  useWelcomeStore,
  type WelcomeSignal,
} from "./welcome-store";
export {
  MIKA_SYSTEM_KEY,
  isMikaAgent,
  memberNeedsMikaSetup,
  workspaceNeedsMika,
} from "./mika";
