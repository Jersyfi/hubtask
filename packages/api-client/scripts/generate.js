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
