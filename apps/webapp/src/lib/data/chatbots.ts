// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's Matrix bots (UC-INT-18): the paths and what the form sends - pure, so that it is tested
// without a server.

export const chatBotsPath = (hubId: string) => `/hubs/${hubId}/chat-bots`;
export const chatBotPath = (botId: string) => `/chat-bots/${botId}`;

export interface ChatBotDraft {
  collectionId: string;
  homeserver: string;
  room: string;
  token: string;
  unfurl: boolean;
  capture: boolean;
}

/** The body that seats a bot: a homeserver typed without its scheme is an https one. */
export function chatBotBody(draft: ChatBotDraft): Record<string, unknown> {
  const host = draft.homeserver.trim();
  return {
    collection_id: draft.collectionId,
    homeserver: host && !host.includes('://') ? `https://${host}` : host,
    room: draft.room.trim(),
    access_token: draft.token.trim(),
    unfurl: draft.unfurl,
    capture: draft.capture,
  };
}

/** Why the form cannot be sent yet, as a message code, or nothing. */
export function chatBotMissing(draft: ChatBotDraft): string | undefined {
  if (!draft.homeserver.trim()) return 'chatbot.homeserver_invalid';
  if (!draft.room.trim().startsWith('!')) return 'webhooks.room_malformed';
  if (!draft.token.trim()) return 'chatbot.token_required';
  if (!draft.collectionId) return 'chatbot.collection_not_in_hub';
  if (!draft.unfurl && !draft.capture) return 'chatbot.nothing_to_do';
  return undefined;
}
