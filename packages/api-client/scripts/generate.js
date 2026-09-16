// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates the client types from api/openapi.yaml.
//
// The same source the Go server types come from (ADR-0004): the specification is changed first,
// `make generate` and `make api-client` run, and both sides land in one pull request. That both
// halves of a contract change are reviewable together is the reason ADR-0027 keeps them in one
// repository at all.

import { execFileSync } from 'node:child_process';
import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const packageRoot = path.resolve(path.dirname(fileURLToPath(import.meta.url)), '..');
const specification = path.resolve(packageRoot, '..', '..', 'api', 'openapi.yaml');
const output = path.join(packageRoot, 'dist', 'schema.d.ts');

if (!fs.existsSync(specification)) {
  throw new Error(`${specification} is missing - the contract is the source (ADR-0004)`);
}

fs.mkdirSync(path.dirname(output), { recursive: true });
execFileSync(
  process.execPath,
  [
    path.join(packageRoot, 'node_modules', 'openapi-typescript', 'bin', 'cli.js'),
    specification,
    '--output', output,
    '--root-types',
  ],
  { stdio: 'inherit' },
);

const banner = `/**
 * Generated from api/openapi.yaml by \`make api-client\` - DO NOT EDIT.
 *
 * The specification is the source and this is the result (ADR-0004). Change api/openapi.yaml,
 * regenerate, and commit both in the same pull request.
 */
`;
fs.writeFileSync(output, banner + fs.readFileSync(output, 'utf8'));
console.log(`api-client: ${path.relative(process.cwd(), output)}`);

// The document itself, and the event schemas beside it (P-01). `api/openapi.json` is what
// `make generate` writes from the YAML (project-structure.md §6); it travels through this
// package because the website may reach the contract only through a workspace member, never by
// climbing out of its own directory (§2.1). The events are gathered into one object keyed by
// their type, which is the file name the schema carries.
const document = path.resolve(packageRoot, '..', '..', 'api', 'openapi.json');
if (!fs.existsSync(document)) {
  throw new Error(`${document} is missing - run make generate first (project-structure.md §6)`);
}
fs.copyFileSync(document, path.join(packageRoot, 'dist', 'openapi.json'));
console.log('api-client: dist/openapi.json');

const eventsDir = path.resolve(packageRoot, '..', '..', 'api', 'events');
const events = {};
for (const entry of fs.readdirSync(eventsDir).sort()) {
  if (!entry.endsWith('.json')) continue;
  events[entry.slice(0, -'.json'.length)] = JSON.parse(fs.readFileSync(path.join(eventsDir, entry), 'utf8'));
}
fs.writeFileSync(path.join(packageRoot, 'dist', 'events.json'), JSON.stringify(events, null, 2) + '\n');
console.log(`api-client: dist/events.json (${Object.keys(events).length} event schemas)`);
