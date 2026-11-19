// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The reply's snippets and macros as the composer's two menus (ET-7, UC-SVC-06): a snippet is
// rendered for this ticket and inserted; a macro is an automation with the trigger *by hand*, and
// its line says what it does - its actions in words (rule 8: the words are the catalogue's).

import { untrack } from 'svelte';
import type { MenuItem } from '@hubtask/design-system/components';
import type { ReplyTemplateList } from '@hubtask/sync-engine';
import { kindWord } from '../automation/words.ts';
import { desk, repliesPath } from '../data/desk.svelte.ts';
import { resource } from '../data/resource.svelte.ts';
import { rules } from '../data/rules.svelte.ts';
import { toast } from '../frame/toasts.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { renderProblem } from '../problem.ts';

export class DeskTools {
  #replies = untrack(() => resource<ReplyTemplateList>(repliesPath));
  refusal = $state<string | undefined>(undefined);
  isBusy = $state(false);

  readonly snippets = $derived<readonly MenuItem[]>(
    this.#replies.state.status === 'ready' ? this.#replies.state.data.data.map((reply) => ({ id: reply.key, label: reply.name, icon: 'message-square' })) : [],
  );

  readonly macros = $derived<readonly MenuItem[]>(
    rules.all
      .filter((rule) => rule.enabled && rule.trigger.kind === 'MANUAL')
      .map((rule) => ({
        id: rule.id,
        label: rule.name,
        icon: 'zap',
        description: t('app.desk.macro_does', { actions: rule.actions.map((action) => kindWord(messages, action.kind)).join(' · ') }),
      })),
  );

  constructor(readonly itemId: string) {
    untrack(() => rules.open());
  }

  async insert(key: string, oninsert: (text: string) => void): Promise<void> {
    this.refusal = undefined;
    this.isBusy = true;
    try {
      oninsert((await desk.render(this.itemId, key)).body);
    } catch (error) {
      this.refusal = renderProblem(error as never, messages).message;
    } finally {
      this.isBusy = false;
    }
  }

  async press(ruleId: string): Promise<void> {
    this.refusal = undefined;
    this.isBusy = true;
    try {
      await desk.press(ruleId, this.itemId);
      toast.show({ text: t('app.desk.macro_pressed'), tone: 'success' });
    } catch (error) {
      this.refusal = renderProblem(error as never, messages).message;
    } finally {
      this.isBusy = false;
    }
  }
}
