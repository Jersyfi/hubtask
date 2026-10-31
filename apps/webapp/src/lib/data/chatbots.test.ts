// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { chatBotBody, chatBotMissing, chatBotsPath, chatIntakeAddress, emptyChatBotDraft, postsToIntake } from './chatbots.ts';

const draft = { ...emptyChatBotDraft('c'), homeserver: 'matrix.example.org', room: '!r:example.org', token: ' tok ', capture: false };

test('UC-INT-18: a bot is seated with an https homeserver, the room and the token trimmed', () => {
  assert.deepEqual(chatBotBody(draft), {
    platform: 'MATRIX', collection_id: 'c', homeserver: 'https://matrix.example.org', room: '!r:example.org', access_token: 'tok',
    unfurl: true, capture: false,
  });
  assert.equal(chatBotsPath('h1'), '/hubs/h1/chat-bots');
});

test('UC-INT-18: the form says what is missing before it is sent', () => {
  assert.equal(chatBotMissing(draft), undefined);
  assert.equal(chatBotMissing({ ...draft, room: '#team:example.org' }), 'webhooks.room_malformed');
  assert.equal(chatBotMissing({ ...draft, token: '' }), 'chatbot.token_required');
  assert.equal(chatBotMissing({ ...draft, unfurl: false }), 'chatbot.nothing_to_do');
});

test('UC-INT-19: each platform sends its own credentials alone, and no server where it has one', () => {
  const slack = { ...emptyChatBotDraft('c'), platform: 'SLACK' as const, homeserver: 'ignored', room: 'C0123ABCD', token: 'xoxb-1', signingSecret: 's', appPassword: 'stray' };
  assert.deepEqual(chatBotBody(slack), {
    platform: 'SLACK', collection_id: 'c', room: 'C0123ABCD', access_token: 'xoxb-1', signing_secret: 's', unfurl: true, capture: true,
  });
  assert.equal(chatBotMissing(slack), undefined);
  assert.equal(chatBotMissing({ ...slack, token: 'xoxp-user' }), 'chatbot.token_required');
  const teams = {
    ...emptyChatBotDraft('c'), platform: 'TEAMS' as const, room: '19:ops@thread.tacv2',
    appId: '00000000-0000-7000-8000-0000000000BB', appTenant: '00000000-0000-7000-8000-0000000000aa', appPassword: 'p',
  };
  assert.equal(chatBotMissing(teams), undefined);
  assert.equal(chatBotBody(teams).app_id, '00000000-0000-7000-8000-0000000000bb');
  assert.equal(chatBotMissing({ ...teams, appTenant: 'contoso' }), 'chatbot.app_required');
  const mattermost = { ...emptyChatBotDraft('c'), platform: 'MATTERMOST' as const, homeserver: 'chat.example.org', room: 'abcdefghijklmnopqrstuvwxyz', commandToken: 'c' };
  assert.equal(chatBotMissing(mattermost), 'chatbot.webhook_token_required');
  assert.deepEqual(chatBotBody({ ...mattermost, unfurl: false }), {
    platform: 'MATTERMOST', collection_id: 'c', room: 'abcdefghijklmnopqrstuvwxyz', homeserver: 'https://chat.example.org',
    command_token: 'c', unfurl: false, capture: true,
  });
});

test('UC-INT-19: a platform bot shows where its platform posts', () => {
  assert.equal(chatIntakeAddress('https://app.example/', 't-1', 'b-1'), 'https://app.example/api/v1/chat-intake/t-1/b-1');
  assert.equal(postsToIntake('SLACK'), true);
  assert.equal(postsToIntake('MATRIX'), false);
});
