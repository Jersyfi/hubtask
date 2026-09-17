// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Generates the types the client is written against, from api/openapi.yaml.
//
// The client itself (src/client.gen.ts) is written by tools/sdkgen on `make generate`; what it
// imports is `operations` from dist/schema.d.ts, and this script is what writes that file. It is
// the SDK's own generation rather than an import of @hubtask/api-client's, on purpose: the SDK is
// Apache-2.0 and the first-party types are not (ADR-0059 §6), so the SDK regenerates them from
// the same Apache-2.0 document instead of depending on them - the "regeneration rather than a
// copy" ADR-0057 foresaw for an extraction, done in place.

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
 * Generated from api/openapi.yaml by \`pnpm build\` - DO NOT EDIT.
 *
 * SPDX-License-Identifier: Apache-2.0
 *
 * The specification is the source and this is the result (ADR-0004); the client in
 * src/client.gen.ts is typed against these operations.
 */
`;
fs.writeFileSync(output, banner + fs.readFileSync(output, 'utf8'));
console.log(`sdk-typescript: ${path.relative(process.cwd(), output)}`);
