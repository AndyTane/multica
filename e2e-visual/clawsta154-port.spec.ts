import { test, expect } from "@playwright/test";
import pg from "pg";
import { mkdirSync, writeFileSync } from "node:fs";
import { join } from "node:path";
import { TestApiClient } from "../e2e/fixtures";

test("restored Multica review cards: list, themes, board, feedback and atomic undo", async ({ browser }) => {
  if (!process.env.DATABASE_URL?.includes(":15434/multica_clawsta154_port")) throw new Error("Use the isolated restored database");
  const db = new pg.Client(process.env.DATABASE_URL);
  await db.connect();
  const output = join(process.cwd(), "evidence/clawsta-151/port/screenshots");
  mkdirSync(output, { recursive: true });
  const api = new TestApiClient();
  const workspaceId = "8daf5cf8-6287-44d5-84cd-6e7d72e80f35";
  try {
    const ws = (await db.query("SELECT id,slug,name FROM workspace WHERE id=$1", [workspaceId])).rows[0];
    const user=(await db.query(`UPDATE "user" SET onboarded_at=now(),language='zh-Hans' WHERE id='47e5e4f1-5146-49d0-978e-59d16663e46c' RETURNING email,name`)).rows[0];
    const login = await api.login(user.email, user.name);
    api.setWorkspaceId(ws.id); api.setWorkspaceSlug(ws.slug);
    const cards = (await db.query(`SELECT id,number,title,assignee_id,assignee_type,created_at,updated_at,metadata FROM issue WHERE workspace_id=$1 AND status='in_review' ORDER BY position,created_at DESC`, [ws.id])).rows;
    const marked = cards.filter((card) => [143,149,148].includes(card.number));
    expect(marked).toHaveLength(3);
    const actions: Record<number, string> = {
      143: "在公司 G1024152 装好 v0.2.16，跑一次四十多分钟的编译，中途没被停掉就关",
      149: "看完本次汇报，没问题就关",
    };
    const ids = new Map<number,string>(marked.map((card) => [card.number,card.id]));
    for (const card of marked) {
      const metadata = card.number === 148
        ? { review_by: "specialist", review_agent: card.assignee_id }
        : { review_by: "user", review_user: login.user.id, review_action: actions[card.number] };
      // Legacy restored cards deliberately keep no review_since: exercise updated_at fallback.
      await db.query("UPDATE issue SET metadata=metadata || $2::jsonb WHERE id=$1", [card.id, JSON.stringify(metadata)]);
    }
    const request = async (id: string, body: unknown) => {
      const response = await fetch(`http://127.0.0.1:18084/api/issues/${id}`, {
        method: "PUT", headers: { Authorization: `Bearer ${api.getToken()}`, "X-Workspace-ID": ws.id, "Content-Type": "application/json" }, body: JSON.stringify(body),
      });
      expect(response.status).toBe(200);
      return response.json();
    };
    for (const theme of ["light", "dark"] as const) {
      const context = await browser.newContext({ viewport: { width: 1280, height: 900 }, colorScheme: theme });
      await context.addCookies([{ name: "multica-locale", value: "zh-Hans", domain: "localhost", path: "/" }]);
      await context.addInitScript(({ theme, token, slug }) => {
        localStorage.setItem("theme",theme); localStorage.setItem("multica_token",token);
        localStorage.setItem("multica-locale","zh-Hans"); localStorage.setItem("multica:chat:isOpen","false");
        localStorage.setItem(`multica_issue_surface_views:${slug}`, JSON.stringify({state:{surfaces:{"workspace:all":{state:{viewMode:"board"},updatedAt:new Date().toISOString()}}},version:1}));
        document.cookie="sidebar_state=true; path=/";
      }, { theme, token: api.getToken()!, slug: ws.slug });
      const page = await context.newPage();
      const errors: string[] = []; page.on("pageerror", (err) => errors.push(err.message));
      try {
        await page.goto(`/${ws.slug}/awaiting`, {waitUntil:"domcontentloaded"});
        const list=page.getByRole("list",{name:"等我处理"});
        await expect(list.getByRole("listitem")).toHaveCount(2);
        const expectedOrder=marked.filter((card)=>card.number!==148).sort((a,b)=>new Date(a.updated_at).valueOf()-new Date(b.updated_at).valueOf() || a.id.localeCompare(b.id));
        await expect(list.getByRole("listitem").first()).toContainText(`CLAWSTA-${expectedOrder[0].number}`);
        await expect(page.locator(`a[href="/${ws.slug}/awaiting"]`)).toContainText("2");
        await page.screenshot({path:join(output,`awaiting-1280-${theme}.png`),animations:"disabled"});
        await page.setViewportSize({width:1024,height:900});
        await page.screenshot({path:join(output,`awaiting-1024-${theme}.png`),animations:"disabled"});
        await page.setViewportSize({width:1280,height:900});
        await list.getByRole("button",{name:"审批意见",exact:true}).first().click();
        await page.getByRole("textbox",{name:"审批意见"}).fill("编译结束后补一张产物截图。");
        await page.screenshot({path:join(output,`send-back-${theme}.png`),animations:"disabled"});
        await page.keyboard.press("Escape");
        const approvedNumber=expectedOrder[0].number;
        await list.getByRole("button",{name:"通过",exact:true}).first().click();
        const toast=page.locator("[data-sonner-toast]").filter({hasText:`已通过 CLAWSTA-${approvedNumber}`});
        await expect(toast).toBeVisible();
        await expect(list.getByRole("listitem")).toHaveCount(1);
        await page.screenshot({path:join(output,`approved-${theme}.png`),animations:"disabled"});
        await toast.getByRole("button",{name:"撤销"}).click();
        await expect(list.getByRole("listitem")).toHaveCount(2);
        // Reset only the isolated legacy fixture after the real atomic undo.
        for (const card of marked) await db.query("UPDATE issue SET metadata=(metadata-'review_since'),updated_at=$2 WHERE id=$1",[card.id,card.updated_at]);
        await page.goto(`/${ws.slug}/issues`,{waitUntil:"domcontentloaded"});
        const mine=page.locator(`[data-peek-target="${ids.get(143)}"]`).first();
        await expect(mine).toBeVisible(); await mine.scrollIntoViewIfNeeded();
        await expect(mine).toContainText("等你");
        await expect(page.locator(`[data-peek-target="${ids.get(148)}"]`).first()).toContainText("等验收");
        await page.screenshot({path:join(output,`board-${theme}.png`),animations:"disabled"});
        expect(errors).toEqual([]);
      } finally {await context.close();}
    }
    // Actually send back once in the restored copy, then restore its review snapshot.
    const context=await browser.newContext();
    await context.addCookies([{name:"multica-locale",value:"zh-Hans",domain:"localhost",path:"/"}]);
    await context.addInitScript((token)=>{localStorage.setItem("multica_token",token);localStorage.setItem("multica-locale","zh-Hans");},api.getToken()!);
    const page=await context.newPage();
    try {
      await page.goto(`/${ws.slug}/awaiting`);
      const list=page.getByRole("list",{name:"等我处理"});
      await expect(list.getByRole("listitem")).toHaveCount(2);
      const row=list.getByRole("listitem").filter({hasText:"CLAWSTA-149"});
      await row.getByRole("button",{name:"审批意见",exact:true}).click();
      await page.getByRole("textbox",{name:"审批意见"}).fill("隔离验收：请补充汇报中的实际升级结果。");
      await page.getByRole("button",{name:"发送",exact:true}).last().click();
      await expect(list.getByRole("listitem")).toHaveCount(1);
      const sent=(await db.query("SELECT status,revision,metadata FROM issue WHERE id=$1",[ids.get(149)])).rows[0];
      expect(sent.status).toBe("in_progress");
      expect(Object.keys(sent.metadata).filter((key)=>key.startsWith("review_"))).toEqual([]);
      await request(ids.get(149)!,{status:"in_review",expected_revision:Number(sent.revision),review_metadata:{review_by:"user",review_user:login.user.id,review_action:actions[149]}});
      writeFileSync(join(output,"../demo-result.json"),JSON.stringify({restoredCards:cards.length,marked:[143,149,148],legacyTimeSource:"updated_at",actions,themes:["light","dark"],actualApproveUndo:true,actualSendBack:true,liveCardsModified:false},null,2));
    } finally {await context.close();}
  } finally {await db.end();}
});
