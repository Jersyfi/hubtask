<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One audit entry as a Dialog M (page-system §7.6): when, who, what in words, what about, how it
  // ended and what changed; the action's code, the chain's number, the request and the target's
  // identifier under *Details for support* (PS-15).
  import { Badge, Button, Dialog, Disclosure, Stack } from '@hubtask/design-system/components';

  import type { Entry } from '../data/audit.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { actionWords, thingWord } from './words.ts';

  interface Props {
    /** The entry opened; closed while undefined. */
    entry: Entry | undefined;
    actorOf: (entry: Entry) => string;
    onClose: () => void;
  }

  const { entry, actorOf, onClose }: Props = $props();

  const words = { t, has: (code: string) => messages.has(code) };
  const tone = (each: Entry) => (each.outcome === 'SUCCESS' ? 'success' : each.outcome === 'DENIED' ? 'warning' : 'danger');
  const severityTone = (each: Entry) => (each.severity === 'CRITICAL' ? 'danger' : each.severity === 'WARNING' ? 'warning' : 'neutral');
</script>

<Dialog
  size="m"
  isOpen={entry !== undefined}
  title={entry ? actionWords(entry.action, entry.target?.type, words) : ''}
  dismissLabel={t('app.page.close')}
  {onClose}
>
  {#if entry}
    <Stack gap="200">
      <dl class="facts" data-audit-record={entry.id}>
        <div class="fact"><dt>{t('app.audit.when')}</dt><dd>{formatDateTime(entry.occurred_at, messages.locale)}</dd></div>
        <div class="fact"><dt>{t('app.audit.actor')}</dt><dd>{actorOf(entry)}</dd></div>
        {#if entry.target?.type || entry.target?.label}
          <div class="fact">
            <dt>{t('app.audit.about_column')}</dt>
            <dd>{entry.target?.label ? t('app.audit.about_named', { thing: thingWord(entry.target?.type, entry.action, words), name: entry.target.label }) : thingWord(entry.target?.type, entry.action, words)}</dd>
          </div>
        {/if}
        <div class="fact">
          <dt>{t('app.audit.outcome')}</dt>
          <dd>
            <Badge tone={tone(entry)}>{t(`app.audit.outcome_${entry.outcome.toLowerCase()}`)}</Badge>
            {#if entry.severity && entry.severity !== 'INFO'}
              <Badge tone={severityTone(entry)}>{t(`app.audit.severity_${entry.severity.toLowerCase()}`)}</Badge>
            {/if}
          </dd>
        </div>
        {#if entry.context?.channel}<div class="fact"><dt>{t('app.audit.channel')}</dt><dd>{entry.context.channel}</dd></div>{/if}
      </dl>

      {#if (entry.changes ?? []).length > 0}
        <ul class="changes">
          {#each entry.changes ?? [] as change (change.field)}
            <li>
              {#if change.changed === true || change.from_hash != null || change.to_hash != null}
                <!-- A sensitive field: that it changed, never what it became; a field set for the
                     first time has only the hash of what it became. -->
                {t('app.audit.changed_sensitive', { field: change.field ?? '' })}
              {:else}
                {t('app.audit.changed', { field: change.field ?? '', to: String(change.to ?? '') })}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      <Disclosure label={t('app.audit.support')} tone="quiet">
        <dl class="support">
          <dt>{t('app.audit.support_action')}</dt><dd class="mono">{entry.action}</dd>
          {#if entry.seq !== undefined}<dt>{t('app.audit.support_seq')}</dt><dd class="mono">{entry.seq}</dd>{/if}
          {#if entry.context?.request_id}<dt>{t('app.audit.request_id')}</dt><dd class="mono">{entry.context.request_id}</dd>{/if}
          {#if entry.target?.type}<dt>{t('app.audit.target_type')}</dt><dd class="mono">{entry.target.type}</dd>{/if}
          {#if entry.target?.id}<dt>{t('app.audit.target_id')}</dt><dd class="mono">{entry.target.id}</dd>{/if}
          {#if entry.legal_basis}<dt>{t('app.audit.support_basis')}</dt><dd class="mono">{entry.legal_basis}</dd>{/if}
        </dl>
      </Disclosure>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={onClose}>{t('app.page.close')}</Button>
  {/snippet}
</Dialog>

<style>
  .facts { margin: 0; display: grid; gap: var(--sp-100); }
  .fact { display: grid; grid-template-columns: minmax(0, 12ch) minmax(0, 1fr); gap: var(--sp-150); align-items: center; }
  .fact dt { color: var(--text-secondary); }
  .fact dd { margin: 0; display: flex; flex-wrap: wrap; gap: var(--sp-050); }

  .changes { margin: 0; padding-inline-start: var(--sp-250); display: grid; gap: var(--sp-050); color: var(--text-secondary); }

  .support { margin: 0; display: grid; grid-template-columns: max-content minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); font-size: var(--fs-075); }
  .support dt { color: var(--text-secondary); }
  .support dd { margin: 0; }
  .mono { font-family: var(--font-mono); overflow-wrap: anywhere; }
</style>
