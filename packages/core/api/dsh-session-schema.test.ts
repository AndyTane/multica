// @vitest-environment node
import { describe, expect, it } from "vitest";
import { AgentTaskSchema } from "./schemas";

describe("DSH session response metadata", () => {
  it("keeps old responses and malformed optional fields usable", () => {
    expect(AgentTaskSchema.parse({ id: "run" }).session_id).toBeUndefined();
    const task = AgentTaskSchema.parse({ id: "run", session_id: 7, session_web_url: { url: "private" } });
    expect(task.id).toBe("run");
    expect(task.session_id).toBeUndefined();
    expect(task.session_web_url).toBeUndefined();
  });
  it("accepts local homepages and discards credential-bearing or unrelated URLs", () => {
    expect(AgentTaskSchema.parse({ id: "run", session_web_url: "http://localhost:6006" }).session_web_url).toBe("http://localhost:6006/");
    for (const url of ["javascript:alert(1)", "https://example.com/", "http://localhost:6006/?token=secret", "http://user:secret@localhost:6006/", "http://localhost:6006/sessions/1"]) {
      expect(AgentTaskSchema.parse({ id: "run", session_web_url: url }).session_web_url).toBeUndefined();
    }
  });
});
