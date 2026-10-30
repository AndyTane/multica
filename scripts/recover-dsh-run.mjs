// Recover a finished request via public APIs; posting never changes the historical task status.
import fs from 'node:fs';
import path from 'node:path';
import { randomUUID } from 'node:crypto';
import { spawnSync } from 'node:child_process';
import { connectWeb, loadConfig, recoverRun } from '../server/internal/dshweb/bridge.mjs';

function multica(command) {
  const result = spawnSync('multica', command, { encoding: 'utf8', windowsHide: true });
  if (result.error || result.status !== 0) throw new Error('Multica CLI command failed; no result was confirmed');
  return result.stdout;
}
export function postRecoveredResult(run, result, invoke = multica, directory = process.cwd()) {
  // Plain names avoid dispatching new agents when replaying an old final response.
  const output = result.output.replace(/\[([^\]]+)\]\(mention:\/\/(?:agent|member)\/[^)]+\)/g, '$1');
  const body = `Recovered DSH result for run \`${run.id}\`: **${result.status}**.\n\n${output}\n`;
  const file = path.join(directory, `.dsh-recovery-${randomUUID()}.md`);
  fs.writeFileSync(file, body, { encoding: 'utf8', flag: 'wx', mode: 0o600 });
  let receipt;
  try { receipt = invoke(['issue', 'comment', 'add', run.issue_id, '--content-file', file, '--output', 'table']); }
  catch { throw new Error(`Multica did not confirm posting; body preserved in ${path.basename(file)}`); }
  fs.unlinkSync(file);
  return receipt;
}

export async function main(args = process.argv.slice(2)) {
  const option = name => args.includes(name) ? args[args.indexOf(name) + 1] : undefined;
  const issue = option('--issue'), taskId = option('--run');
  if (!issue || !taskId || !/^[A-Za-z0-9-]+$/.test(issue) || !/^[a-f0-9-]{36}$/i.test(taskId)) {
    throw new Error('Usage: node scripts/recover-dsh-run.mjs --issue CLAWSTA-61 --run <task-uuid> [--post]');
  }
  const runs = JSON.parse(multica(['issue', 'runs', issue, '--output', 'json']));
  const run = runs.find(row => row.id === taskId);
  if (!run?.session_id || !run.work_dir) throw new Error('Run has no recoverable DSH session in this issue');
  if (!['failed', 'cancelled', 'completed'].includes(run.status)) throw new Error('Run is still active in Multica');
  const config = loadConfig();
  const result = await recoverRun({ request_id: taskId, session_id: run.session_id, cwd: run.work_dir }, config, await connectWeb(config));
  if (!args.includes('--post')) process.stdout.write(JSON.stringify(result, null, 2) + '\n');
  else process.stdout.write(postRecoveredResult(run, result));
}
if (import.meta.main) main().catch(error => {
  process.stderr.write(error.message + '\n');
  process.exitCode = 1;
});
