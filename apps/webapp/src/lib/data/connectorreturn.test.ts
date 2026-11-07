// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { connectorReturnOf } from './connectorreturn.ts';
import { forwardTo } from './oidc.ts';

const state = '01936f2a-7c1e-7000-8000-000000000001.aGFuZGxl-_';

test('each return page is located with its kind', () => {
  assert.deepEqual(connectorReturnOf('/administration/mail/oauth', `?code=c&state=${state}`), { state, kind: 'MAILBOX' });
  assert.deepEqual(connectorReturnOf('/profile/calendars', `?state=${state}&code=c`), { state, kind: 'CALENDAR_PUSH' });
  assert.deepEqual(connectorReturnOf('/profile/files/', `state=${state}&error=access_denied`), { state, kind: 'FILE_STORE' });
});

test('another page, no state or a state naming no workspace is no connector return', () => {
  assert.equal(connectorReturnOf('/profile', `?state=${state}`), undefined);
  assert.equal(connectorReturnOf('/profile/calendars', ''), undefined);
  assert.equal(connectorReturnOf('/profile/calendars', '?state=aGFuZGxl'), undefined);
  assert.equal(connectorReturnOf('/profile/calendars', '?state=anna.aGFuZGxl'), undefined);
  assert.equal(connectorReturnOf('/profile/calendars', `?state=${state}.extra`), undefined);
});

test('the return goes to the workspace host with its path and query unchanged', () => {
  const installation = { protocol: 'https:', hostname: 'hub.example', origin: 'https://hub.example' };
  const search = `?code=c&state=${state}`;
  assert.equal(
    forwardTo('https://anna.hub.example', installation, search, '/profile/calendars'),
    `https://anna.hub.example/profile/calendars${search}`,
  );
  // Here already, or a path that would leave the origin: finish here.
  assert.equal(forwardTo('https://hub.example', installation, search, '/profile/calendars'), undefined);
  assert.equal(forwardTo('https://anna.hub.example', installation, search, '//evil.example'), undefined);
});
