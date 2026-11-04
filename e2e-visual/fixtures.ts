/**
 * Visual baseline fixtures (CLAWSTA-133).
 *
 * e2e-visual/baseline.spec.ts screenshots a workspace whose pages have
 * content. Some of that content cannot be produced through the product API:
 * the join page needs a share link, the issue page needs a real attachment,
 * and agents/runtimes only exist through a daemon or the database. These
 * helpers sit next to the visual spec and build on the existing
 * `TestApiClient` (e2e/fixtures.ts) instead of duplicating it: login,
 * workspaces, issues, projects and comments keep going through the client,
 * only the visual-only rows use raw HTTP or SQL.
 *
 * Everything created here is workspace-scoped and is removed by
 * `VisualBaselineData.cleanup()`, which must run before the workspace delete.
 */
import "../e2e/env";
import pg from "pg";
import { TestApiClient } from "../e2e/fixtures";

// `||` (not `??`) so an empty `NEXT_PUBLIC_API_URL=` still falls back to
// localhost, matching e2e/fixtures.ts.
const API_BASE =
  process.env.NEXT_PUBLIC_API_URL || `http://localhost:${process.env.PORT || "8080"}`;
const DATABASE_URL =
  process.env.DATABASE_URL ?? "postgres://multica:multica@localhost:5432/multica?sslmode=disable";

export interface VisualWorkspace {
  id: string;
  slug: string;
}

interface CreatedInboxItem {
  title: string;
  body?: string;
  type?: string;
  severity?: "action_required" | "attention" | "info";
  issueId?: string;
  read?: boolean;
}

interface VisualFixtureIds {
  skillIds: string[];
  squadIds: string[];
  autopilotIds: string[];
  agentIds: string[];
  runtimeIds: string[];
  inboxItemIds: string[];
}

/**
 * Visual-only data creation on top of a logged-in TestApiClient. The client
 * owns auth, workspace selection and the issue/project/comment fixtures; this
 * class owns the rows the product API cannot create.
 */
export class VisualBaselineData {
  private readonly api: TestApiClient;
  private readonly workspace: VisualWorkspace;
  private created: VisualFixtureIds = {
    skillIds: [],
    squadIds: [],
    autopilotIds: [],
    agentIds: [],
    runtimeIds: [],
    inboxItemIds: [],
  };

  constructor(api: TestApiClient, workspace: VisualWorkspace) {
    this.api = api;
    this.workspace = workspace;
  }

  /** Create a workspace share link; the join page is `/join?code=<code>`. */
  async createShareLink(role: "member" | "admin" = "member"): Promise<{ id: string; code: string }> {
    const res = await this.request(`/api/workspaces/${this.workspace.id}/share-links`, {
      method: "POST",
      body: JSON.stringify({ role }),
    });
    if (!res.ok) {
      throw new Error(`create share link failed: ${res.status} ${await res.text()}`);
    }
    return (await res.json()) as { id: string; code: string };
  }

  /**
   * Upload a file bound to an issue. Mirrors `api.uploadFile` in
   * packages/core/api/client.ts: multipart form, bearer auth, workspace header.
   */
  async uploadIssueAttachment(
    issueId: string,
    filename: string,
    content: Uint8Array,
    contentType = "application/octet-stream",
  ): Promise<{ id: string; url: string; filename?: string }> {
    const form = new FormData();
    // Copy into a plain ArrayBuffer-backed view: File only accepts Blob parts
    // backed by ArrayBuffer, not the ArrayBufferLike generic Uint8Array.
    const bytes = new Uint8Array(content.length);
    bytes.set(content);
    form.append("file", new File([bytes], filename, { type: contentType }));
    form.append("issue_id", issueId);
    const res = await fetch(`${API_BASE}/api/upload-file`, {
      method: "POST",
      body: form,
      headers: this.authHeaders(),
    });
    if (!res.ok) {
      throw new Error(`upload attachment failed: ${res.status} ${await res.text()}`);
    }
    return (await res.json()) as { id: string; url: string; filename?: string };
  }

  async createSkill(name: string, description = "", content = "") {
    const res = await this.request("/api/skills", {
      method: "POST",
      body: JSON.stringify({ name, description, content }),
    });
    if (!res.ok) {
      throw new Error(`create skill failed: ${res.status} ${await res.text()}`);
    }
    const skill = (await res.json()) as { id: string; name: string };
    this.created.skillIds.push(skill.id);
    return skill;
  }

  async createSquad(name: string, leaderId: string, description = "") {
    const res = await this.request("/api/squads", {
      method: "POST",
      body: JSON.stringify({ name, description, leader_id: leaderId }),
    });
    if (!res.ok) {
      throw new Error(`create squad failed: ${res.status} ${await res.text()}`);
    }
    const squad = (await res.json()) as { id: string; name: string };
    this.created.squadIds.push(squad.id);
    return squad;
  }

  async createAutopilot(
    title: string,
    assigneeId: string,
    executionMode: "create_issue" | "run_only" = "create_issue",
  ) {
    const res = await this.request("/api/autopilots", {
      method: "POST",
      body: JSON.stringify({ title, assignee_id: assigneeId, execution_mode: executionMode }),
    });
    if (!res.ok) {
      throw new Error(`create autopilot failed: ${res.status} ${await res.text()}`);
    }
    const autopilot = (await res.json()) as { id: string; title: string };
    this.created.autopilotIds.push(autopilot.id);
    return autopilot;
  }

  /**
   * Insert an online runtime plus an agent bound to it, directly in the
   * database. The product only registers runtimes through a live daemon, and
   * agent creation requires an existing runtime_id.
   */
  async seedAgent(
    name: string,
    opts: { description?: string; runtimeName?: string } = {},
  ): Promise<{ agentId: string; runtimeId: string }> {
    const ownerId = await this.lookupUserId();
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      const runtimeResult = await client.query<{ id: string }>(
        `INSERT INTO agent_runtime (
           workspace_id, daemon_id, name, runtime_mode, provider, status,
           device_info, metadata, last_seen_at
         )
         VALUES ($1, NULL, $2, 'cloud', 'r0_visual_baseline', 'online', $3, '{}'::jsonb, now())
         RETURNING id`,
        [this.workspace.id, opts.runtimeName ?? "R0 视觉基线机器", "R0 visual baseline runtime"],
      );
      const runtimeId = runtimeResult.rows[0]?.id;
      if (!runtimeId) {
        throw new Error("Failed to seed the visual baseline runtime");
      }
      const agentResult = await client.query<{ id: string }>(
        `INSERT INTO agent (
           workspace_id, name, description, runtime_mode, runtime_config,
           runtime_id, visibility, max_concurrent_tasks, owner_id
         )
         VALUES ($1, $2, $3, 'cloud', '{}'::jsonb, $4, 'workspace', 1, $5)
         RETURNING id`,
        [this.workspace.id, name, opts.description ?? "R0 视觉基线用智能体", runtimeId, ownerId],
      );
      const agentId = agentResult.rows[0]?.id;
      if (!agentId) {
        throw new Error("Failed to seed the visual baseline agent");
      }
      this.created.agentIds.push(agentId);
      this.created.runtimeIds.push(runtimeId);
      return { agentId, runtimeId };
    } finally {
      await client.end();
    }
  }

  /** Insert inbox rows for the logged-in user; returns the created item ids. */
  async seedInboxItems(items: CreatedInboxItem[]): Promise<string[]> {
    const recipientId = await this.lookupUserId();
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    const ids: string[] = [];
    try {
      for (const item of items) {
        const result = await client.query<{ id: string }>(
          `INSERT INTO inbox_item (
             workspace_id, recipient_type, recipient_id, type, severity,
             issue_id, title, body, read
           )
           VALUES ($1, 'member', $2, $3, $4, $5, $6, $7, $8)
           RETURNING id`,
          [
            this.workspace.id,
            recipientId,
            item.type ?? "issue_assigned",
            item.severity ?? "info",
            item.issueId ?? null,
            item.title,
            item.body ?? null,
            item.read ?? false,
          ],
        );
        const id = result.rows[0]?.id;
        if (id) ids.push(id);
      }
      this.created.inboxItemIds.push(...ids);
      return ids;
    } finally {
      await client.end();
    }
  }

  /**
   * Remove the visual-only rows. Order matters: autopilots and squads go
   * before the seeded agents (a squad's leader is a RESTRICT reference to an
   * agent), and the agents go before their runtimes. Call this before deleting
   * the workspace.
   */
  async cleanup(): Promise<void> {
    for (const id of this.created.autopilotIds) {
      await this.request(`/api/autopilots/${id}`, { method: "DELETE" }).catch(() => undefined);
    }
    this.created.autopilotIds = [];
    for (const id of this.created.squadIds) {
      await this.request(`/api/squads/${id}`, { method: "DELETE" }).catch(() => undefined);
    }
    this.created.squadIds = [];
    for (const id of this.created.skillIds) {
      await this.request(`/api/skills/${id}`, { method: "DELETE" }).catch(() => undefined);
    }
    this.created.skillIds = [];
    if (
      this.created.inboxItemIds.length === 0 &&
      this.created.agentIds.length === 0 &&
      this.created.runtimeIds.length === 0
    ) {
      return;
    }
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      if (this.created.inboxItemIds.length > 0) {
        await client.query(`DELETE FROM inbox_item WHERE id = ANY($1::uuid[])`, [
          this.created.inboxItemIds,
        ]);
      }
      if (this.created.agentIds.length > 0) {
        await client.query(`DELETE FROM agent WHERE id = ANY($1::uuid[])`, [this.created.agentIds]);
      }
      if (this.created.runtimeIds.length > 0) {
        await client.query(`DELETE FROM agent_runtime WHERE id = ANY($1::uuid[])`, [
          this.created.runtimeIds,
        ]);
      }
    } finally {
      await client.end();
    }
    this.created = {
      skillIds: [],
      squadIds: [],
      autopilotIds: [],
      agentIds: [],
      runtimeIds: [],
      inboxItemIds: [],
    };
  }

  private authHeaders(): Record<string, string> {
    const token = this.api.getToken();
    if (!token) {
      throw new Error("Visual baseline fixtures need a logged-in TestApiClient");
    }
    return {
      "Content-Type": "application/json",
      Authorization: `Bearer ${token}`,
      "X-Workspace-Slug": this.workspace.slug,
    };
  }

  private async request(path: string, init?: RequestInit): Promise<Response> {
    const headers = { ...this.authHeaders(), ...((init?.headers as Record<string, string>) ?? {}) };
    return fetch(`${API_BASE}${path}`, { ...init, headers });
  }

  private async lookupUserId(): Promise<string> {
    const client = new pg.Client(DATABASE_URL);
    await client.connect();
    try {
      const result = await client.query<{ id: string }>(
        `SELECT id FROM "user" WHERE email = $1`,
        [this.api.getEmail()],
      );
      const userId = result.rows[0]?.id;
      if (!userId) {
        throw new Error(`Cannot resolve the E2E user for ${this.api.getEmail()}`);
      }
      return userId;
    } finally {
      await client.end();
    }
  }
}
