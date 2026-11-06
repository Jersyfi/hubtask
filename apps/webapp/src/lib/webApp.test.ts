// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { webAppAddress, webAppHost } from './webApp.ts';

const multi = { workspace: 'https://tasks.acme.com', installation: 'https://tasks.example.net' };
const single = { workspace: 'https://hubtask.vogt.example', installation: null };

test('the workspace\'s administration is at the workspace, the installation at the installation (UC-SYN-07 check 7)', () => {
  assert.equal(webAppHost('administration', multi), 'tasks.acme.com');
  assert.equal(webAppHost('instance', multi), 'tasks.example.net');
  assert.equal(webAppHost('instance', single), 'hubtask.vogt.example', 'single mode: the one workspace is the installation');
  assert.equal(webAppHost('instance', { ...multi, installation: 'not an address' }), 'tasks.acme.com');
});

test('a link into an area opens the same path in the web app, query included', () => {
  assert.equal(webAppAddress('administration', '/administration/people?tab=groups', multi).href, 'https://tasks.acme.com/administration/people?tab=groups');
  assert.equal(webAppAddress('instance', '/instance/workspaces', multi).href, 'https://tasks.example.net/instance/workspaces');
  assert.equal(webAppAddress('administration', '//evil.example/x', multi).host, 'tasks.acme.com', 'only the path is taken from the address');
});
