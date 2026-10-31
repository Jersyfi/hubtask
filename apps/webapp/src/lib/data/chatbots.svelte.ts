// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes of a hub's chat bots (UC-INT-18, UC-INT-19).

import type { ChatBot } from '@hubtask/sync-engine';

import { chatBotBody, chatBotPath, chatBotsPath, type ChatBotDraft } from './chatbots.ts';
import { engine } from './engine.ts';

export const chatBots = {
  seat(hubId: string, draft: ChatBotDraft): Promise<ChatBot> {
    return engine.mutate<ChatBot>('POST', chatBotsPath(hubId), chatBotBody(draft), { invalidates: [chatBotsPath(hubId)] });
  },
  change(bot: ChatBot, change: Partial<Pick<ChatBot, 'enabled' | 'unfurl' | 'capture'>>): Promise<ChatBot> {
    return engine.mutate<ChatBot>('PATCH', chatBotPath(bot.id), change, { invalidates: [chatBotsPath(bot.hub_id)] });
  },
  remove(bot: ChatBot): Promise<void> {
    return engine.mutate<void>('DELETE', chatBotPath(bot.id), undefined, { invalidates: [chatBotsPath(bot.hub_id)] });
  },
};
