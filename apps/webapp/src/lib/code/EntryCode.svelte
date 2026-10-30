<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's code (UC-INT-13 check 3): the pull requests, branches and commits that named its key
  // in a forge hooked to its hub, each with its state as the forge last told it. Nothing is drawn
  // where none did - most entries have no code, and a heading over nothing would say so on all of
  // them. A link opens the forge's own page in a new tab; nothing here writes.

  import { untrack } from 'svelte';
  import { Badge, Icon } from '@hubtask/design-system/components';
  import type { CodeLinkList } from '@hubtask/sync-engine';

  import { codeGroups, codeHref, codeLinksPath, codeRef, codeTone } from '../data/code.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    itemId: string;
  }

  const { itemId }: Props = $props();

  // Keyed on the entry by its caller, so the path is fixed for its life.
  const read = resource<CodeLinkList>(untrack(() => codeLinksPath(itemId)));
  const links = $derived(read.state.status === 'ready' ? read.state.data.data : []);
  const groups = $derived(codeGroups(links));
</script>

{#if links.length > 0}
  <section class="code" aria-labelledby="entry-code-head" data-entry-code>
    <h3 id="entry-code-head" class="head"><Icon name="git-branch" size="sm" />{t('app.code.title')}</h3>
    {#each groups as group (group.kind)}
      <ul class="rows" aria-label={t(`app.code.kind.${group.kind}`)}>
        {#each group.links as link (link.id)}
          {@const href = codeHref(link.url)}
          <li class="row" data-code-link={link.kind}>
            <span class="what">
              {#if href}
                <a {href} target="_blank" rel="noopener noreferrer">{link.title}</a>
              {:else}
                <span>{link.title}</span>
              {/if}
              <span class="meta">
                {t(`app.code.kind.${link.kind}`)} · <span class="mono">{link.repository} {codeRef(link)}</span>
                {#if link.author} · {link.author}{/if}
                · {formatDateTime(link.happened_at, messages.locale)}
              </span>
            </span>
            <Badge size="sm" tone={codeTone(link.state)}>{t(`app.code.state.${link.state}`)}</Badge>
          </li>
        {/each}
      </ul>
    {/each}
  </section>
{/if}

<style>
  .code { display: grid; gap: var(--sp-100); }
  .head { display: inline-flex; align-items: center; gap: var(--sp-050); margin: 0; font-size: var(--fs-100); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row { display: flex; align-items: baseline; justify-content: space-between; gap: var(--sp-100); }
  .what { display: grid; gap: var(--sp-025); min-width: 0; overflow-wrap: anywhere; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); }
  .mono { font-family: var(--font-mono); }
</style>
