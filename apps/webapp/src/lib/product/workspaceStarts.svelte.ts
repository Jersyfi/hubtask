// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A product's templates of the workspace alone, as its empty state draws them (user-experience.md
 * §4, configuration.md §6): goals' goal sets and time's kinds of work. Each card opens the
 * template's dialog - what it creates, then the plan - and the workspace takes it there; no hub is
 * made. Created while a component initialises, since it follows the catalogue while offered.
 */

import { untrack } from 'svelte';
import type { IconName } from '@hubtask/design-system/components';
import type { ConfigTemplate, ConfigTemplateList, ResourceState } from '@hubtask/sync-engine';

import { config } from '../data/config.svelte.ts';
import { messages, t } from '../i18n/i18n.svelte.ts';
import { markOf } from '../workspace/hubGallery.ts';
import { workspaceTemplates } from './templates.ts';

/** One card: its name, one line and mark, and what pressing it does. */
export interface StartCard {
  label: string;
  description?: string;
  icon: IconName;
  onclick: () => void;
}

export class WorkspaceStarts {
  readonly #module: string;
  #list = $state<ResourceState<ConfigTemplateList>>();
  /** The template whose dialog is open. */
  opened = $state<ConfigTemplate>();

  /** `isOffered` says while the cards are drawn: only then is the catalogue read. */
  constructor(module: string, isOffered: () => boolean) {
    this.#module = module;
    $effect(() => {
      if (!isOffered()) return;
      return untrack(() => config.templates((next) => (this.#list = next)));
    });
  }

  /**
   * The cards, Hubtask's first, three at most; none while the catalogue is read or refused, and
   * none once the workspace took one of them - its start is made, the empty state is a week's.
   */
  get cards(): StartCard[] {
    const list = this.#list;
    if (list?.status !== 'ready') return [];
    const offered = workspaceTemplates(list.data.data, this.#module);
    if (offered.some((each) => each.taken_version != null)) return [];
    return offered.map((each) => ({
      label: nameOf(each),
      description: summaryOf(each),
      icon: markOf(each),
      onclick: () => (this.opened = each),
    }));
  }
}

/** Hubtask's templates are named by their key, in the reader's language. */
function nameOf(template: ConfigTemplate): string {
  return template.origin === 'HUBTASK' ? t(`app.structure.preset.${template.key.toUpperCase()}`) : template.name;
}

function summaryOf(template: ConfigTemplate): string | undefined {
  const code = `app.templates.summary.${template.key.toUpperCase()}`;
  return template.origin === 'HUBTASK' && messages.has(code) ? t(code) : undefined;
}
