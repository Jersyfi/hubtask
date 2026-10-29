// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  forgetWorkspace,
  groupedByInstallation,
  keepWorkspace,
  owedRevocations,
  readWorkspace,
  readWorkspaces,
  settleOwed,
  writeWorkspace,
  type AppWorkspace,
  type Ending,
  type ForgetSteps,
  type HeldSession,
} from './appSession.ts';

function memoryStorage() {
  const items = new Map<string, string>();
  return {
    items,
    getItem: (key: string) => items.get(key) ?? null,
    setItem: (key: string, value: string) => void items.set(key, value),
    removeItem: (key: string) => void items.delete(key),
  };
}

const ACME: AppWorkspace = { server: 'https://tasks.acme.com', host: 'tasks.acme.com', name: 'Acme Ltd', installation: 'tasks.example.net' };
const MEIER: AppWorkspace = {
  server: 'https://familie-meier.tasks.example.net',
  host: 'familie-meier.tasks.example.net',
  name: 'Meier family',
  installation: 'tasks.example.net',
};
const VOGT: AppWorkspace = { server: 'https://hubtask.vogt.example', host: 'hubtask.vogt.example', name: 'Vogt household' };

/** The keystore's sessions and the shell's copies, standing in for the shell, with what was asked. */
function shell(ending: Ending = 'ended') {
  const sessions = new Map<string, HeldSession>();
  const copies = new Set<string>();
  const revoked: string[] = [];
  let stopped = 0;
  const hold = (workspace: AppWorkspace, account: string) => {
    sessions.set(workspace.server, { server: workspace.server, access: `a-${workspace.host}`, refresh: `r-${workspace.host}`, account });
    copies.add(`${workspace.server}|${account}`);
  };
  const storage = memoryStorage();
  const steps: ForgetSteps = {
    storage,
    readSession: async (server) => sessions.get(server),
    forgetSession: async (server) => void sessions.delete(server),
    keepSession: async (held) => void sessions.set(held.server, held),
    forgetCopy: async (server, account) => void copies.delete(`${server}|${account}`),
    revoke: async (held) => {
      revoked.push(held.refresh);
      return ending;
    },
    stopHere: async () => {
      stopped += 1;
    },
  };
  return { sessions, copies, revoked, storage, steps, hold, stopped: () => stopped };
}

// UC-SYN-07 check 4, UC-ID-25 check 7: the app holds workspaces of one installation and of another,
// each with its own session and copy, listed under each installation's host.
test('two workspaces of two installations hold separate sessions and copies', () => {
  const { storage, sessions, copies, hold } = shell();
  for (const [workspace, account] of [[ACME, 'account-1'], [VOGT, 'account-1']] as const) {
    hold(workspace, account);
    keepWorkspace(storage, workspace);
  }
  assert.deepEqual(readWorkspaces(storage).map((each) => each.host), ['tasks.acme.com', 'hubtask.vogt.example']);
  assert.notEqual(sessions.get(ACME.server)?.refresh, sessions.get(VOGT.server)?.refresh);
  assert.equal(copies.size, 2, 'a copy per workspace, though the account identifier is the same');
  // No credential in the list the webview keeps.
  for (const value of storage.items.values()) assert.doesNotMatch(value, /"(access|refresh)"/);

  keepWorkspace(storage, MEIER);
  assert.deepEqual(
    groupedByInstallation(readWorkspaces(storage)).map((group) => [group.installation, group.workspaces.map((each) => each.name)]),
    [['tasks.example.net', ['Acme Ltd', 'Meier family']], ['hubtask.vogt.example', ['Vogt household']]],
  );
  // Signing in again replaces the entry where it stood.
  keepWorkspace(storage, { ...ACME, name: 'Acme Limited' });
  assert.deepEqual(readWorkspaces(storage).map((each) => each.name), ['Acme Limited', 'Vogt household', 'Meier family']);
});

// UC-SYN-07 check 4: forgetting one revokes its grant and deletes its session and copy, and nothing else.
test('forgetting a workspace revokes its grant and deletes its copy and nothing else', async () => {
  const { storage, sessions, copies, revoked, steps, hold, stopped } = shell();
  hold(ACME, 'account-1');
  hold(VOGT, 'account-1');
  keepWorkspace(storage, ACME);
  keepWorkspace(storage, VOGT);
  writeWorkspace(storage, ACME);

  const answer = await forgetWorkspace(steps, VOGT.server);
  assert.deepEqual(answer, { wasOpen: false, next: undefined });
  assert.deepEqual(revoked, ['r-hubtask.vogt.example'], 'only its own grant');
  assert.deepEqual([...sessions.keys()], [ACME.server]);
  assert.deepEqual([...copies], [`${ACME.server}|account-1`]);
  assert.deepEqual(readWorkspaces(storage).map((each) => each.host), ['tasks.acme.com']);
  assert.deepEqual(readWorkspace(storage), ACME, 'the open workspace stays open');
  assert.equal(stopped(), 0, 'the window was not touched');
  assert.deepEqual(owedRevocations(storage), []);
});

test('forgetting the open workspace moves to the next one, or to Which workspace? when none is left', async () => {
  const { storage, sessions, steps, hold, stopped } = shell();
  for (const workspace of [ACME, MEIER, VOGT]) {
    hold(workspace, 'account-1');
    keepWorkspace(storage, workspace);
  }
  writeWorkspace(storage, MEIER);
  assert.deepEqual(await forgetWorkspace(steps, MEIER.server), { wasOpen: true, next: VOGT });
  assert.equal(stopped(), 1);
  assert.deepEqual(readWorkspace(storage), VOGT);

  assert.deepEqual(await forgetWorkspace(steps, VOGT.server), { wasOpen: true, next: ACME });
  assert.deepEqual(await forgetWorkspace(steps, ACME.server), { wasOpen: true, next: undefined });
  assert.equal(readWorkspace(storage), undefined);
  assert.deepEqual(readWorkspaces(storage), []);
  assert.equal(sessions.size, 0);
});

// The approved preview's choice: offline, the copy and its key go at once; the grant is withdrawn
// the next time the installation answers, and its session waits in the keystore until then.
test('forgetting offline deletes the copy at once and withdraws the grant later', async () => {
  const offline = shell('unreachable');
  offline.hold(VOGT, 'account-1');
  keepWorkspace(offline.storage, VOGT);
  writeWorkspace(offline.storage, VOGT);
  await forgetWorkspace(offline.steps, VOGT.server);
  assert.equal(offline.copies.size, 0, 'the copy is gone');
  assert.ok(offline.sessions.has(VOGT.server), 'the session waits for its withdrawal');
  assert.deepEqual(owedRevocations(offline.storage), [VOGT.server]);
  assert.deepEqual(readWorkspaces(offline.storage), []);

  // Still offline at the next start: owed still.
  await settleOwed(offline.steps);
  assert.deepEqual(owedRevocations(offline.storage), [VOGT.server]);

  // Online again: withdrawn, and the session goes.
  await settleOwed({ ...offline.steps, revoke: async () => 'ended' });
  assert.equal(offline.sessions.size, 0);
  assert.deepEqual(owedRevocations(offline.storage), []);
});

test('a workspace added again is not owed its old withdrawal', async () => {
  const { storage, sessions, steps, hold, revoked } = shell('unreachable');
  hold(VOGT, 'account-1');
  keepWorkspace(storage, VOGT);
  await forgetWorkspace(steps, VOGT.server);
  keepWorkspace(storage, VOGT);
  await settleOwed({ ...steps, revoke: async (held) => (revoked.push(held.refresh), 'ended') });
  assert.equal(revoked.length, 1, 'the new session was not ended');
  assert.ok(sessions.has(VOGT.server));
});

test('a window signed in before the app held several lists its workspace', () => {
  const storage = memoryStorage();
  writeWorkspace(storage, ACME);
  assert.deepEqual(readWorkspaces(storage), [ACME]);
  storage.setItem('hubtask.app.workspaces', 'garbage');
  assert.deepEqual(readWorkspaces(storage), [ACME]);
});
