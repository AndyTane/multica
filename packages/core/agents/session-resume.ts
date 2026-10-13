import { z } from "zod";

const uuid = z.string().regex(/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i);
const localPath = z.string().min(1).max(4096).refine((value) =>
  Array.from(value).every((char) => char.charCodeAt(0) >= 32 && char.charCodeAt(0) !== 127),
);
const launcherSchema = z.object({
  cli_path: localPath.refine((value) => /[\\/]multica(?:\.exe)?$/i.test(value)),
  workspaces_root: localPath,
  profile: z.string().max(64).regex(/^(?:[a-z0-9][a-z0-9._-]*)?$/i).default(""),
  os: z.enum(["windows", "linux", "darwin"]),
}).refine((value) => {
  const absolute = (path: string) => value.os === "windows" ? /^[a-z]:[\\/]/i.test(path) : path.startsWith("/");
  return absolute(value.cli_path) && absolute(value.workspaces_root);
});
const sourceSchema = z.object({
  task: z.object({ id: uuid, agent_id: uuid, session_id: uuid, status: z.enum(["completed", "failed", "cancelled"]) }),
  runtime: z.object({ provider: z.enum(["claude", "codex"]), workspace_id: uuid, metadata: z.object({ session_resume: launcherSchema }) }),
});

/** Paths are used only in an explicit clipboard action, never in visible UI or tooltips. */
export function sessionResumeCommand(task: unknown, runtime: unknown): string | undefined {
  const parsed = sourceSchema.safeParse({ task, runtime });
  if (!parsed.success) return undefined;
  const { task: run, runtime: rt } = parsed.data;
  const launcher = rt.metadata.session_resume;
  const args = [launcher.cli_path, "--workspace-id", rt.workspace_id];
  if (launcher.profile) args.push("--profile", launcher.profile);
  args.push("agent", "resume", run.agent_id, run.id, "--workspaces-root", launcher.workspaces_root);
  const quote = launcher.os === "windows"
    ? (value: string) => `'${value.replaceAll("'", "''")}'`
    : (value: string) => `'${value.replaceAll("'", "'\"'\"'")}'`;
  return `${launcher.os === "windows" ? "& " : ""}${args.map(quote).join(" ")}`;
}
