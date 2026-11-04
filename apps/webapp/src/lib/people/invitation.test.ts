// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { deliveryLine, handedOverLink, inviteWay, collectionChoice, customRoleOf, invitePlaceOf, inviteRoleChoices } from './invitation.ts';

test('the form promises a link exactly where the installation sends no mail (UC-ID-14 check 5)', () => {
  assert.equal(inviteWay({ mail: false }), 'link');
  assert.equal(inviteWay({ mail: true }), 'mail');
});

test('before the manifest is read, or from a server that does not say, the form promises neither', () => {
  assert.equal(inviteWay(undefined), 'unknown');
  assert.equal(inviteWay({ storage: true }), 'unknown');
});

test('a link is shown only when the answer carries one', () => {
  assert.equal(handedOverLink({ invitation_link: 'https://home.example/redeem#token=x' }), 'https://home.example/redeem#token=x');
  assert.equal(handedOverLink({}), undefined);
  assert.equal(handedOverLink({ invitation_link: null }), undefined);
  assert.equal(handedOverLink({ invitation_link: '' }), undefined);
  // A replay of the same request answers the status without the body: no link, and no guess at one.
  assert.equal(handedOverLink(undefined), undefined);
});

// A requester is invited with the workspace's requester role at the desk's collection alone, not
// with a shipped role at a whole hub first.
test('an invitation offers the workspace\'s own roles beside the shipped ones, and a collection as its place', () => {
  const shipped = [{ value: 'GUEST', label: 'Guest' }];
  const choices = inviteRoleChoices(shipped, [{ key: 'requester', name: 'Requester' }]);
  assert.deepEqual(choices.map((each) => each.label), ['Guest', 'Requester']);
  assert.equal(customRoleOf(choices[1]?.value ?? ''), 'requester');
  assert.equal(customRoleOf('GUEST'), undefined);
  assert.deepEqual(inviteRoleChoices([], [{ key: 'requester', name: 'Requester' }]), [], 'who may give nothing there is offered nothing');
  assert.deepEqual(invitePlaceOf(''), { scopeType: 'TENANT' });
  assert.deepEqual(invitePlaceOf('h1'), { scopeType: 'HUB', scopeId: 'h1' });
  assert.deepEqual(invitePlaceOf(collectionChoice('c1')), { scopeType: 'COLLECTION', scopeId: 'c1' });
});

// user-experience.md §8.3: an invited person's row says how the last mail went, in every state the
// server answers, and a failure's own reason beside it.
test('each invitation delivery state has its sentence, and a failure its reason', () => {
  const catalogue = JSON.parse(readFileSync(new URL('../../../../../locales/en.json', import.meta.url), 'utf8')) as Record<string, string>;
  for (const state of ['QUEUED', 'SENDING', 'SENT', 'FAILED', 'CANCELLED'] as const) {
    const line = deliveryLine({ state, at: '2026-11-04T09:00:00Z', code: null });
    assert.ok(catalogue[line.code], `${state} has no words`);
    assert.equal(line.reason, undefined);
  }
  const failed = deliveryLine({ state: 'FAILED', at: '2026-11-04T09:00:00Z', code: 'mail.recipient_rejected', params: { status: '550' } });
  assert.deepEqual(failed.reason, { code: 'mail.recipient_rejected', params: { status: '550' } });
});
