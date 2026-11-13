import { test, expect } from "@playwright/test";
import { mkdirSync } from "node:fs";
import { join } from "node:path";
import pg from "pg";
import { TestApiClient } from "./fixtures";

// This regression writes only to an explicitly provisioned isolated database.
test.skip(process.env.E2E_HANDBACK_ISOLATED !== "1", "Requires an isolated handback environment");
test.use({ viewport: { width: 1440, height: 1000 }, launchOptions: { executablePath: process.env.PLAYWRIGHT_EXECUTABLE_PATH } });

for (const theme of ["light", "dark"]) {
  test(`receiver sidebar comment returns control and preserves the card (${theme})`, async ({ page }, testInfo) => {
    test.setTimeout(180_000);
    const database = new URL(process.env.DATABASE_URL!);
    expect(database.hostname).toBe("127.0.0.1");
    const db = new pg.Client(process.env.DATABASE_URL);
    await db.connect();
    const api = new TestApiClient();
    let workspaceId: string | undefined;
    try {
      const suffix = `${theme}-${Date.now()}`;
      await api.login(`handback-${suffix}@example.test`, "Tane");
      const workspace = await api.ensureWorkspace("Comment handback", `handback-${suffix}`);
      workspaceId = workspace.id;
      await api.markUserOnboarded();
      const user = await db.query(`SELECT id FROM "user" WHERE email=$1`, [api.getEmail()]);
      const userId = user.rows[0].id;
      // A fixture runtime with no connected worker: it can queue, never execute a model.
      const runtime = await db.query(`INSERT INTO agent_runtime(workspace_id,name,runtime_mode,provider,status,owner_id,last_seen_at)
        VALUES ($1,'Fixture runtime','local','e2e_handback','online',$2,now()) RETURNING id`, [workspace.id, userId]);
      const agent = await db.query(`INSERT INTO agent(workspace_id,name,owner_id,runtime_mode,runtime_id,visibility,permission_mode)
        VALUES ($1,'Developer',$2,'local',$3,'private','private') RETURNING id`, [workspace.id, userId, runtime.rows[0].id]);
      const cards = [];
      for (const title of ["Review the sidebar reply", "Check the remaining task"]) {
        const card = await api.createIssue(title, { status: "in_review" });
        await db.query(`UPDATE issue SET assignee_type='agent',assignee_id=$2,metadata=$3 WHERE id=$1`,
          [card.id, agent.rows[0].id, { review_by: "user", review_kind: "approve", review_action: title, review_user: userId }]);
        cards.push(card);
      }
      const card = cards[0]!;
      await page.addInitScript(({ token, selectedTheme }) => {
        localStorage.setItem("multica_token", token!);
        localStorage.setItem("multica:chat:isOpen", "false");
        localStorage.setItem("theme", selectedTheme);
      }, { token: api.getToken(), selectedTheme: theme });
      await page.goto(`/${workspace.slug}/awaiting`, { waitUntil: "domcontentloaded" });
      await page.locator(`[data-peek-target="${card.id}"]`).getByRole("button").first().click();
      const peek = page.locator('aside[data-issue-peek]');
      await expect(peek).toContainText("1 / 2");
      await peek.getByTestId("comment-composer-shell").click();
      const editor = peek.locator('.ProseMirror[contenteditable="true"]').last();
      await editor.fill("Please continue with the agreed changes.");
      const posted = page.waitForResponse((response) => response.request().method() === "POST" && response.url().endsWith(`/api/issues/${card.id}/comments`));
      await editor.press("ControlOrMeta+Enter");
      const response = await posted;
      expect(response.status()).toBe(201);
      const comment = await response.json();
      expect(comment.issue_status).toBe("in_progress");
      await expect(page.locator(`[data-peek-target="${card.id}"]`)).toHaveCount(0);
      await expect(page.locator(`[data-peek-target="${cards[1]!.id}"]`)).toBeVisible();
      await expect(peek).toContainText(card.title);
      await expect(peek).toContainText("0 / 1");
      await expect(peek).toContainText("Please continue with the agreed changes.");
      const current = await db.query(`SELECT status,revision,metadata FROM issue WHERE id=$1`, [card.id]);
      expect(current.rows[0].status).toBe("in_progress");
      expect(Number(current.rows[0].revision)).toBe(comment.issue_revision);
      expect(current.rows[0].metadata.review_by).toBeUndefined();
      const activity = await db.query(`SELECT details FROM activity_log WHERE issue_id=$1 AND action='status_changed'`, [card.id]);
      expect(activity.rows).toHaveLength(1);
      expect(activity.rows[0].details).toMatchObject({ from: "in_review", to: "in_progress" });
      const tasks = await db.query(`SELECT trigger_comment_id FROM agent_task_queue WHERE issue_id=$1`, [card.id]);
      expect(tasks.rows).toEqual([{ trigger_comment_id: comment.id }]);
      const evidence = process.env.E2E_HANDBACK_EVIDENCE_DIR ?? testInfo.outputPath("screenshots");
      mkdirSync(evidence, { recursive: true });
      await page.screenshot({ path: join(evidence, `comment-handback-${theme}.png`), fullPage: true, animations: "disabled" });
    } finally {
      await api.cleanup();
      if (workspaceId) await api.deleteWorkspace();
      await db.end();
    }
  });
}
