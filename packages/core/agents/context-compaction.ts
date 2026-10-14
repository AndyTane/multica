import { z } from "zod";
import { parseWithFallback } from "../api/schema";

export const contextCompactionSchema = z.object({
  enabled: z.boolean().default(true),
  threshold_tokens: z.number().int().min(1000).max(2000000).default(80000),
});
export type ContextCompaction = z.infer<typeof contextCompactionSchema>;
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
