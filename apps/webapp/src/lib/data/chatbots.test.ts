// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { chatBotBody, chatBotMissing, chatBotsPath } from './chatbots.ts';

const draft = { collectionId: 'c', homeserver: 'matrix.example.org', room: '!r:example.org', token: ' tok ', unfurl: true, capture: false };

test('UC-INT-18: a bot is seated with an https homeserver, the room and the token trimmed', () => {
  assert.deepEqual(chatBotBody(draft), {
    collection_id: 'c', homeserver: 'https://matrix.example.org', room: '!r:example.org', access_token: 'tok', unfurl: true, capture: false,
  });
  assert.equal(chatBotsPath('h1'), '/hubs/h1/chat-bots');
});

test('UC-INT-18: the form says what is missing before it is sent', () => {
  assert.equal(chatBotMissing(draft), undefined);
  assert.equal(chatBotMissing({ ...draft, room: '#team:example.org' }), 'webhooks.room_malformed');
  assert.equal(chatBotMissing({ ...draft, token: '' }), 'chatbot.token_required');
  assert.equal(chatBotMissing({ ...draft, unfurl: false }), 'chatbot.nothing_to_do');
});
