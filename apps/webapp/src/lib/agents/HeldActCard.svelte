<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One act an agent holds for a person's yes: whose agent, what it would do in words where the
  // act has them (its fields otherwise), how far it reaches, and *Approve* / *Decline* where the
  // reader may decide. The agents' requests and the one approvals inbox draw the same card.

  import { Badge, Button, Stack } from '@hubtask/design-system/components';
  import type { AgentRequest } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { heldActWordsOf, previewOf } from '../data/agents.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    request: AgentRequest;
    agentName: string;
    mayDecide: boolean;
    ondecide: (approve: boolean) => void;
  }

  const { request, agentName, mayDecide, ondecide }: Props = $props();

  const words = $derived(heldActWordsOf(request.use_case, request.input));
  const nameOf = (id: string | null | undefined) => (id ? (accounts.nameOf(id) ?? t('app.people.unnamed')) : '—');
  const at = (iso: string | null | undefined) => (iso ? formatDateTime(iso, messages.locale) : '—');
</script>

<li class="card" data-held-act={request.use_case} data-held-act-id={request.id}>
  <Stack gap="100">
    <div class="head">
      <strong class="name">{agentName}</strong>
      {#if words}
        <span class="act">{t(words.code, words.params)}</span>
      {:else}
        <code>{request.use_case}</code>
      {/if}
      <Badge size="sm" tone={request.class === 'IRREVERSIBLE' ? 'danger' : 'warning'}>{t(`app.agents.class.${request.class}`)}</Badge>
      <span class="meta">{at(request.created_at)}</span>
    </div>
    {#if words}
      <!-- What the agent wrote, as the decider would read it where it lands. -->
      {#if words.text}<blockquote class="said">{words.text}</blockquote>{/if}
      {#if words.href}<a class="meta" href={words.href}>{t('app.agents.held.open_entry')}</a>{/if}
    {:else}
      <dl class="preview">
        {#each previewOf(request.input) as line (line.key)}
          <dt>{line.key}</dt>
          <dd>{line.value}</dd>
        {/each}
      </dl>
    {/if}
    {#if request.status === 'PENDING'}
      {#if mayDecide}
        <div class="presses">
          <Button tone="primary" size="sm" icon="check" onclick={() => ondecide(true)}>{t('app.agents.approve_act')}</Button>
          <Button tone="secondary" size="sm" icon="x" onclick={() => ondecide(false)}>{t('app.agents.decline_act')}</Button>
        </div>
      {/if}
    {:else}
      <p class="meta">
        {t(`app.agents.request_status.${request.status}`)} · {nameOf(request.decided_by)} · {at(request.decided_at)}
        {#if request.code} · {messages.has(request.code) ? t(request.code) : request.code}{/if}
      </p>
    {/if}
  </Stack>
</li>

<style>
  .card {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    min-width: 0;
    list-style: none;
  }
  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .name { margin: 0; min-width: 0; overflow-wrap: anywhere; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }
  .act { font-weight: var(--fw-medium); }
  .meta { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .said {
    margin: 0;
    padding-inline-start: var(--sp-150);
    border-inline-start: var(--bw-thick) solid var(--border-default);
    color: var(--text-secondary);
    white-space: pre-line;
    overflow-wrap: anywhere;
  }
  .preview { margin: 0; display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-025) var(--sp-150); }
  .preview dt { color: var(--text-subtle); font-size: var(--fs-075); font-family: var(--font-mono); }
  .preview dd { margin: 0; overflow-wrap: anywhere; }
  .presses { display: flex; flex-wrap: wrap; gap: var(--sp-100); }
</style>
