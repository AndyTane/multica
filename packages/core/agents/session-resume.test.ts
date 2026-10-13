// @vitest-environment node
import { describe, expect, it } from "vitest";
import { sessionResumeCommand } from "./session-resume";

const ids = {
  id: "01a136db-92cf-75b1-a804-f42445c19c9b",
  agent_id: "68ca5d11-c32c-453c-82bf-ec44dbcf66a8",
  session_id: "01a136db-d24d-7533-a0a0-b646d5ee0439",
  status: "completed",
};
const runtime = {
  provider: "codex", workspace_id: "8daf5cf8-6287-44d5-84cd-6e7d72e80f35",
  metadata: { session_resume: { cli_path: "D:\\Tools with spaces\\multica.exe", profile: "", workspaces_root: "C:\\Users\\Tane\\multica_workspaces", os: "windows" } },
};

describe("session recovery clipboard command", () => {
  it("quotes the real Windows launcher and includes the workspace and root", () => {
    expect(sessionResumeCommand(ids, runtime)).toBe("& 'D:\\Tools with spaces\\multica.exe' '--workspace-id' '8daf5cf8-6287-44d5-84cd-6e7d72e80f35' 'agent' 'resume' '68ca5d11-c32c-453c-82bf-ec44dbcf66a8' '01a136db-92cf-75b1-a804-f42445c19c9b' '--workspaces-root' 'C:\\Users\\Tane\\multica_workspaces'");
  });
  it("keeps shell metacharacters in paths literal and quotes POSIX apostrophes", () => {
    const metadata = { session_resume: { cli_path: "/tools/a'$(whoami)/multica", profile: "dev", workspaces_root: "/tmp/my 'root'", os: "linux" } };
    const command = sessionResumeCommand(ids, { ...runtime, metadata });
    expect(command).toContain("'/tools/a'\"'\"'$(whoami)/multica'");
    expect(command).toContain("'--profile' 'dev'");
    expect(command).toContain("'/tmp/my '\"'\"'root'\"'\"''");
  });
  it("does not fabricate commands for old or malformed runtime metadata", () => {
    for (const metadata of [{}, { session_resume: null }, { session_resume: { ...runtime.metadata.session_resume, os: "future" } }, { session_resume: { ...runtime.metadata.session_resume, cli_path: "multica.exe" } }, { session_resume: { ...runtime.metadata.session_resume, cli_path: "D:\\evil.exe" } }, { session_resume: { ...runtime.metadata.session_resume, workspaces_root: 7 } }]) {
      expect(sessionResumeCommand(ids, { ...runtime, metadata })).toBeUndefined();
    }
  });
  it("rejects active runs, unsupported providers and unsafe or missing identities", () => {
    for (const status of ["running", "queued", "dispatched", "future"]) expect(sessionResumeCommand({ ...ids, status }, runtime)).toBeUndefined();
    expect(sessionResumeCommand(ids, { ...runtime, provider: "dsh" })).toBeUndefined();
    expect(sessionResumeCommand({ ...ids, session_id: "" }, runtime)).toBeUndefined();
    expect(sessionResumeCommand({ ...ids, id: "run'; bad" }, runtime)).toBeUndefined();
  });
});
