// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// `pnpm --filter @hubtask/sync-engine conformance --base-url <url> --token <token> [--report <file>]`
//
// The engine's conformance run against a real instance (F6-08): the report on standard output,
// one row per requirement on standard error as it is decided, and a non-zero exit where any check
// failed. The credential may come as `HUBTASK_TOKEN` instead of `--token`, which keeps it out of
// the process list on a shared machine.

import { parseArgs } from 'node:util';
import { writeFile } from 'node:fs/promises';

import { FetchTransport } from '../src/FetchTransport.ts';
import { runConformance } from './run.ts';

const { values } = parseArgs({
  options: {
    'base-url': { type: 'string' },
    token: { type: 'string' },
    report: { type: 'string' },
    keep: { type: 'boolean', default: false },
  },
  strict: true,
});

const baseUrl = values['base-url'];
const token = values.token ?? process.env.HUBTASK_TOKEN;
if (!baseUrl || !token) {
  process.stderr.write('usage: conformance --base-url <url> --token <token> [--report <file>] [--keep]\n');
  process.exit(2);
}

const run = await runConformance({
  baseUrl,
  token,
  transportFor: () => new FetchTransport({ baseUrl }),
  log: (line) => process.stderr.write(`${line}\n`),
  keep: values.keep,
});

process.stdout.write(run.report);
if (values.report) await writeFile(values.report, run.report, { mode: 0o600 });
if (run.failed > 0) {
  process.stderr.write(`${run.failed} of the checks failed\n`);
  process.exit(1);
}
