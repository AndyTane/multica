import { z } from "zod";
import { parseWithFallback } from "../api/schema";

export const contextCompactionSchema = z.object({
  enabled: z.boolean().default(true),
  threshold_tokens: z.number().int().min(1000).max(2000000).default(80000),
});
export type ContextCompaction = z.infer<typeof contextCompactionSchema>;

export function isContextCompactionMessage(message: { type: string }): boolean {
  return message.type === "context_compaction_started"
    || message.type === "context_compaction_completed"
    || message.type === "context_compaction_failed";
}

/** Sequence order also handles a reconnect backfill arriving after live events. */
export function contextCompactionState(messages: readonly { type: string; seq: number }[]): "running" | "failed" | null {
  let latest: { type: string; seq: number } | undefined;
  for (const message of messages) {
    if (!latest || message.seq > latest.seq) latest = message;
  }
  if (latest?.type === "context_compaction_started") return "running";
  if (latest?.type === "context_compaction_failed") return "failed";
  return null;
}
const defaults: ContextCompaction = { enabled: true, threshold_tokens: 80000 };
const runtimeConfigSchema = z.object({ context_compaction: z.unknown().optional() });
const contextCompactionResponseSchema = contextCompactionSchema.extend({
  enabled: contextCompactionSchema.shape.enabled.catch(true),
  threshold_tokens: contextCompactionSchema.shape.threshold_tokens.catch(80000),
});

export function parseContextCompaction(runtimeConfig: unknown): ContextCompaction {
  const parsed = runtimeConfigSchema.safeParse(runtimeConfig);
  return parseWithFallback(
    parsed.success ? parsed.data.context_compaction : undefined,
    contextCompactionResponseSchema.default(defaults),
    defaults,
    { endpoint: "agent.runtime_config.context_compaction" },
  );
}

export function mergeContextCompaction(runtimeConfig: unknown, policy: ContextCompaction): Record<string, unknown> {
  const parsed = z.record(z.string(), z.unknown()).safeParse(runtimeConfig);
  return { ...(parsed.success ? parsed.data : {}), context_compaction: policy };
}
