// Synthetic reader benchmark only: no host, credentials, RPC, or model calls.
// Usage: node server/internal/dshweb/bridge.bench.mjs [baseline-git-ref]
import fs from 'node:fs';
import os from 'node:os';
import path from 'node:path';
import { execFileSync } from 'node:child_process';
import { performance } from 'node:perf_hooks';
import { zstdCompressSync } from 'node:zlib';
import { createLogReader } from './bridge.mjs';

const baseline = process.argv[2] ?? 'eecb33a23';
const source = execFileSync('git', ['show', `${baseline}:server/internal/dshweb/bridge.mjs`], { encoding: 'utf8' });
const { readRows: oldRead } = await import(`data:text/javascript;base64,${Buffer.from(source).toString('base64')}`);
const root = fs.mkdtempSync(path.join(os.tmpdir(), 'multica-reader-bench-'));
const count = 50000, warmup = 5, samples = 40;
const frame = rows => zstdCompressSync(Buffer.from(rows.map(row => JSON.stringify(row) + '\n').join('')));
const frames = [];
for (let start = 0; start < count; start += 100) {
  frames.push(frame(Array.from({ length: 100 }, (_, index) => ({
    type: 'assistant/message', data: { turn: start + index, message: { content: [{ type: 'text', text: `Synthetic row ${start + index}: 中文测试内容 ${'payload '.repeat(16)}` }] } },
  }))));
}
const bytes = Buffer.concat(frames), extra = frame(Array.from({ length: 10 }, (_, i) => ({ type: 'bench', i })));
function measure(fn, setup = () => {}) {
  const times = [];
  for (let i = 0; i < warmup + samples; i++) {
    setup();
    const start = performance.now();
    fn();
    if (i >= warmup) times.push(performance.now() - start);
  }
  times.sort((a, b) => a - b);
  return { p50_ms: times[Math.ceil(times.length * .5) - 1], p95_ms: times[Math.ceil(times.length * .95) - 1] };
}
try {
  const file = path.join(root, 'session.zstd');
  fs.writeFileSync(file, bytes);
  const reader = createLogReader(file);
  reader.readNew();
  const empty = { before: measure(() => oldRead(file)), after: measure(() => reader.readNew()) };
  const appended = { before: measure(() => oldRead(file), () => fs.appendFileSync(file, extra)) };
  fs.writeFileSync(file, bytes);
  const appendReader = createLogReader(file);
  appendReader.readNew();
  appended.after = measure(() => appendReader.readNew(), () => fs.appendFileSync(file, extra));
  fs.writeFileSync(file, bytes);
  const resumeScan = {
    before: measure(() => { oldRead(file); oldRead(file); }),
    after: measure(() => { const scan = createLogReader(file); scan.readNew(); scan.readNew(); }),
  };
  process.stdout.write(JSON.stringify({ baseline, node: process.version, platform: process.platform,
    arch: process.arch, cpu: os.cpus()[0].model, rows: count, frames: frames.length,
    compressedBytes: bytes.length, addedRows: 10, warmup, samples, empty, appended, resumeScan }, null, 2) + '\n');
} finally { fs.rmSync(root, { recursive: true, force: true }); }
