// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A hub's chat bots (UC-INT-18, UC-INT-19): the paths, what the form sends and where a platform
// posts - pure, so that it is tested without a server.

export const chatBotsPath = (hubId: string) => `/hubs/${hubId}/chat-bots`;
export const chatBotPath = (botId: string) => `/chat-bots/${botId}`;
/** Where the caller links their own chat account to a bot (UC-SVC-09 check 6). */
export const chatLinkPath = (botId: string) => `/chat-bots/${botId}/link`;

export type ChatPlatform = 'MATRIX' | 'SLACK' | 'TEAMS' | 'MATTERMOST';

export const chatPlatforms: readonly ChatPlatform[] = ['MATRIX', 'SLACK', 'TEAMS', 'MATTERMOST'];

export interface ChatBotDraft {
  platform: ChatPlatform;
  collectionId: string;
  homeserver: string;
  room: string;
  /** The Matrix account's access token, or the Slack app's bot token. */
  token: string;
  signingSecret: string;
  appId: string;
  appTenant: string;
  appPassword: string;
  commandToken: string;
  webhookToken: string;
  /** A Mattermost incoming webhook's key, through which a linked person is told of an approval. */
  postHook: string;
  unfurl: boolean;
  capture: boolean;
}

export function emptyChatBotDraft(collectionId: string): ChatBotDraft {
  return {
    platform: 'MATRIX', collectionId, homeserver: '', room: '', token: '', signingSecret: '', appId: '', appTenant: '',
    appPassword: '', commandToken: '', webhookToken: '', postHook: '', unfurl: true, capture: true,
  };
}

/** Whether a platform needs its server typed: Slack and Teams have one address of their own. */
export const asksServer = (platform: ChatPlatform) => platform === 'MATRIX' || platform === 'MATTERMOST';

/** Whether a platform posts to the bot's intake rather than being read. */
export const postsToIntake = (platform: string | undefined) => platform === 'SLACK' || platform === 'TEAMS' || platform === 'MATTERMOST';

/** The address a platform posts to for a bot. */
export function chatIntakeAddress(origin: string, tenantId: string, botId: string): string {
  return `${origin.replace(/\/+$/u, '')}/api/v1/chat-intake/${tenantId}/${botId}`;
}

/** The body that seats a bot: each platform's own credentials alone, a server typed without its scheme an https one. */
export function chatBotBody(draft: ChatBotDraft): Record<string, unknown> {
  const host = draft.homeserver.trim();
  const body: Record<string, unknown> = {
    platform: draft.platform, collection_id: draft.collectionId, room: draft.room.trim(), unfurl: draft.unfurl, capture: draft.capture,
  };
  if (asksServer(draft.platform)) body.homeserver = host && !host.includes('://') ? `https://${host}` : host;
  switch (draft.platform) {
    case 'MATRIX':
      body.access_token = draft.token.trim();
      break;
    case 'SLACK':
      body.access_token = draft.token.trim();
      body.signing_secret = draft.signingSecret.trim();
      break;
    case 'TEAMS':
      body.app_id = draft.appId.trim().toLowerCase();
      body.app_tenant = draft.appTenant.trim().toLowerCase();
      body.app_password = draft.appPassword.trim();
      break;
    case 'MATTERMOST':
      if (draft.capture) body.command_token = draft.commandToken.trim();
      if (draft.unfurl) body.webhook_token = draft.webhookToken.trim();
      if (draft.postHook.trim()) body.post_hook = postHookKey(draft.postHook);
      break;
  }
  return body;
}

/** A Mattermost incoming webhook's key: pasted as its whole address, the last part of it. */
export function postHookKey(typed: string): string {
  const trimmed = typed.trim().replace(/\/+$/u, '');
  return trimmed.slice(trimmed.lastIndexOf('/') + 1);
}

const roomShape: Record<ChatPlatform, RegExp> = {
  MATRIX: /^![^:\s]+:\S+$/u,
  SLACK: /^[CGD][A-Z0-9]{8,20}$/u,
  MATTERMOST: /^[a-z0-9]{26}$/u,
  TEAMS: /^(19|a|8):[^\s;]{1,250}$/u,
};

const uuid = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/u;

/** Why the form cannot be sent yet, as a message code, or nothing. */
export function chatBotMissing(draft: ChatBotDraft): string | undefined {
  if (asksServer(draft.platform) && !draft.homeserver.trim()) return 'chatbot.homeserver_invalid';
  if (!roomShape[draft.platform].test(draft.room.trim())) return 'webhooks.room_malformed';
  if (!draft.unfurl && !draft.capture) return 'chatbot.nothing_to_do';
  switch (draft.platform) {
    case 'MATRIX':
      if (!draft.token.trim()) return 'chatbot.token_required';
      break;
    case 'SLACK':
      if (!draft.token.trim().startsWith('xoxb-')) return 'chatbot.token_required';
      if (!draft.signingSecret.trim()) return 'chatbot.signing_secret_required';
      break;
    case 'TEAMS':
      if (!uuid.test(draft.appId.trim().toLowerCase()) || !uuid.test(draft.appTenant.trim().toLowerCase()) || !draft.appPassword.trim()) {
        return 'chatbot.app_required';
      }
      break;
    case 'MATTERMOST':
      if (draft.unfurl && !draft.webhookToken.trim()) return 'chatbot.webhook_token_required';
      if (draft.capture && !draft.commandToken.trim()) return 'chatbot.command_token_required';
      break;
  }
  if (!draft.collectionId) return 'chatbot.collection_not_in_hub';
  return undefined;
}

/**
 * The address a Matrix room shows as a widget beside its timeline (UC-INT-18 check 5): the hub
 * itself, so the room's people work the hub without leaving it. The installation lets the client's
 * origin frame it (`HUBTASK_UI_FRAME_ANCESTORS`); nothing in the address is a credential - whoever
 * opens it signs in as themselves and sees what they may.
 */
export function roomWidgetAddress(origin: string, hubId: string): string {
  return `${origin.replace(/\/+$/u, '')}/hubs/${encodeURIComponent(hubId)}`;
}
