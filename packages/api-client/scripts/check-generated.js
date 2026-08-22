// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when anything under src/ describes the API instead of re-exporting the generated types.
//
// This package exists to hold generated output and nothing else, so that extracting it into a
// separately licensed repository before 1.0.0 stays a move rather than a rewrite (ADR-0027). A
// hand-written interface here is the first step towards a second description of the contract.

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const source = path.join(packageRoot, 'src');

const FORBIDDEN = [
  { what: 'an interface', pattern: /^\s*(export\s+)?interface\s+\w/m },
  { what: 'a type alias built by hand', pattern: /^\s*export\s+type\s+\w+\s*=\s*\{/m },
  { what: 'a runtime value', pattern: /^\s*export\s+(const|function|class)\s/m },
];

const problems = [];
for (const entry of fs.readdirSync(source)) {
  if (!entry.endsWith('.ts')) continue;
  const content = fs.readFileSync(path.join(source, entry), 'utf8');
  for (const rule of FORBIDDEN) {
    if (rule.pattern.test(content)) {
      problems.push(`src/${entry}: ${rule.what} - this package re-exports generated types only`);
    }
  }
}

if (problems.length > 0) {
  for (const problem of problems) console.error(problem);
  console.error('\nDescribe it in api/openapi.yaml and regenerate (ADR-0004, ADR-0027).');
  process.exit(1);
}
console.log('api-client: src/ re-exports the generated types and describes nothing itself');
