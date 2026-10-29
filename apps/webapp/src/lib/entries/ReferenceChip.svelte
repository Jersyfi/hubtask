<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A reference as the reader sees it (UC-PLN-01 checks 4 and 7): a mention is the entry's key and
  // title, a card adds its state and due date. **What the reader may not see is "not available to
  // you" and nothing else** - the resolver answered no title for it, and none is invented here.

  import { Icon } from '@hubtask/design-system/components';
  import type { ReferenceStrength, ReferenceTarget } from '@hubtask/sync-engine';

  import { targetLabel } from '../data/links.ts';
  import { formatDue } from '../i18n/datetime.ts';
  import { actor } from '../data/account.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    target: ReferenceTarget | undefined;
    strength?: ReferenceStrength;
  }

  const { target, strength = 'MENTION' }: Props = $props();

  const label = $derived(targetLabel(target));
  const href = $derived(
    !target?.available
      ? undefined
      : target.kind === 'ENTRY'
        ? `/items/${target.id}`
        : target.kind === 'COLLECTION'
          ? `/collections/${target.id}`
          : target.kind === 'PAGE'
            ? `/pages/${target.id}`
            : target.kind === 'HUB'
              ? `/hubs/${target.id}`
              : target.kind === 'CI'
                ? `/cis/${target.id}`
                : undefined,
  );
</script>

{#if label === undefined}
  <span class="reference" data-unavailable data-strength={strength}>
    <Icon name="lock" size="sm" />
    <span>{t('app.links.unavailable')}</span>
  </span>
{:else if strength === 'MENTION'}
  {#if href}<a class="reference" {href} data-strength="MENTION">{label}</a>{:else}<span class="reference">{label}</span>{/if}
{:else}
  <span class="reference card" data-strength={strength} data-completed={target?.completed ? '' : undefined}>
    {#if href}<a {href}>{label}</a>{:else}<span>{label}</span>{/if}
    {#if target?.kind === 'ENTRY'}
      <span class="meta">{t('app.mentions.card_state', { completed: String(target.completed === true) })}</span>
      {#if target.due_at}<span class="meta">{formatDue(target.due_at, messages.locale, actor.zone)}</span>{/if}
    {/if}
  </span>
{/if}

<style>
  .reference {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
  }

  .reference[data-unavailable] { color: var(--text-subtle); }

  .card {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: var(--sp-100);
    padding-block: var(--sp-050);
    padding-inline: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface);
  }

  .card[data-completed] a { text-decoration: line-through; }

  .meta { color: var(--text-secondary); font-size: var(--fs-075); }
</style>
