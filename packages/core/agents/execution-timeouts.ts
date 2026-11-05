import { z } from "zod";
import { parseWithFallback } from "../api/schema";

export const MAX_TIMEOUT_SECONDS = 9_223_372_036;
const seconds = z.number().int().min(0).max(MAX_TIMEOUT_SECONDS);

export const ExecutionTimeoutsSchema = z.object({
  idle_seconds: seconds,
  tool_idle_seconds: seconds,
  run_seconds: seconds,
  opencode_idle_seconds: seconds,
  opencode_shell_seconds: seconds.positive().optional(),
}).transform((p) => ({
  idleSeconds: p.idle_seconds,
  toolIdleSeconds: p.tool_idle_seconds,
  runSeconds: p.run_seconds,
  opencodeIdleSeconds: p.opencode_idle_seconds,
  opencodeShellSeconds: p.opencode_shell_seconds,
}));

export type ExecutionTimeouts = z.output<typeof ExecutionTimeoutsSchema>;
export type ExecutionTimeoutsWire = z.input<typeof ExecutionTimeoutsSchema>;

const SettingsContainerSchema = z.object({ execution_timeouts: z.unknown().optional() }).loose();
const ServerCapabilitySchema = z.object({
  execution_timeouts_supported: z.boolean().catch(false).default(false),
}).loose();

export function supportsServerExecutionTimeouts(response: unknown) {
  return parseWithFallback<z.output<typeof ServerCapabilitySchema>>(response, ServerCapabilitySchema,
    { execution_timeouts_supported: false }, { endpoint: "execution_timeout_server_capability" })
    .execution_timeouts_supported;
}

// Opting in starts from the legacy defaults; following the generic OpenCode
// window always requires the user's explicit selection.
export const DEFAULT_EXECUTION_TIMEOUTS: ExecutionTimeouts = {
  idleSeconds: 7200,
  toolIdleSeconds: 7200,
  runSeconds: 0,
  opencodeIdleSeconds: 600,
  opencodeShellSeconds: undefined,
};

export function readExecutionTimeouts(container: unknown) {
  const settings = parseWithFallback<z.output<typeof SettingsContainerSchema>>(container ?? {}, SettingsContainerSchema, {}, { endpoint: "execution_timeout_settings" });
  const raw = settings.execution_timeouts;
  if (raw == null) return null;
  return parseWithFallback<ExecutionTimeouts | null>(raw, ExecutionTimeoutsSchema, null, {
    endpoint: "execution_timeouts",
  });
}

export function executionTimeoutsPatch(policy: ExecutionTimeouts | null) {
  return {
    execution_timeouts: policy == null ? null : {
      idle_seconds: policy.idleSeconds,
      tool_idle_seconds: policy.toolIdleSeconds,
      run_seconds: policy.runSeconds,
      opencode_idle_seconds: policy.opencodeIdleSeconds,
      ...(policy.opencodeShellSeconds == null ? {} : { opencode_shell_seconds: policy.opencodeShellSeconds }),
    },
  };
}

export function supportsExecutionTimeouts(metadata: Record<string, unknown> | undefined) {
  return Array.isArray(metadata?.capabilities) &&
    metadata.capabilities.includes("execution-timeouts-v1");
}
