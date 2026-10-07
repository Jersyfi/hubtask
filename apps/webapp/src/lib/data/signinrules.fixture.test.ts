// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shared corpus, read here and by the Go domain that decides it.
//
// Two implementations of one rule cannot be avoided - the field has to predict at every keystroke
// what the server decides on send, and a screen that predicted wrong would be a screen that lies.
// What can be avoided is drift, and `api/fixtures/password-rules.json` is how: one table, two
// readers, and a case added on either side turns both red or neither.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

import { evaluate, type PasswordContext, type PasswordRules } from './signinrules.ts';

interface FixtureCase {
  readonly name: string;
  readonly rules: PasswordRules;
  readonly context: {
    readonly email?: string;
    readonly display_name?: string;
    readonly workspace_name?: string;
    readonly workspace_host?: string;
  };
  readonly password: string;
  readonly local_violations: readonly string[];
  readonly server_lines: readonly string[];
}

const fixture = JSON.parse(
  readFileSync(fileURLToPath(new URL('../../../../../api/fixtures/password-rules.json', import.meta.url)), 'utf8'),
) as { readonly cases: readonly FixtureCase[] };

const contextOf = (entry: FixtureCase): PasswordContext => ({
  email: entry.context.email,
  displayName: entry.context.display_name,
  workspaceName: entry.context.workspace_name,
  workspaceHost: entry.context.workspace_host,
});

test('the fixture holds cases at all', () => {
  assert.ok(fixture.cases.length > 0, 'the corpus is empty - the path is wrong');
});

for (const entry of fixture.cases) {
  test(`fixture: ${entry.name}`, () => {
    const lines = evaluate(entry.rules, entry.password, contextOf(entry));

    const unmet = lines.filter((line) => !line.isServerSide && line.state !== 'met').map((line) => line.id);
    assert.deepEqual(unmet, [...entry.local_violations], 'the local rules this password breaks');

    const server = lines.filter((line) => line.isServerSide).map((line) => line.id);
    assert.deepEqual(server, [...entry.server_lines], 'the lines the server answers');
  });
}
