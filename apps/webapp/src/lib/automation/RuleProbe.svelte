<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The probe (F8-06, decision 9): the sample event a dry run takes - the event type, a real entry
  // as its subject found through the client's own search, and the body an inbound delivery would
  // have carried - and the button that runs the canvas's definition through `rules:test`.
  //
  // It draws nothing itself: the result goes back to the view, which lays it onto the canvas
  // frame by frame. What this panel owns is the sample, and the honest sentence that nothing is
  // written.
  //
  // Above the button stands what the draft is missing (F8-26, decision 29): the review's notes,
  // each pressing through to the card it is about, and the sample's own mismatch where it is not
  // the event the rule starts on. Nothing here refuses the run - a probe of a rule with something
  // missing is exactly how one finds out what it does.

  import { Button, Callout, RunStatusBadge, SearchField, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import type { Outcome } from './probe.ts';
  import { search } from '../data/search.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { eventGroups } from './words.ts';

  interface Props {
    /** The event types this installation publishes, and the trigger's own where it has one. */
    eventTypes: readonly string[];
    defaultType: string;
    /** What the draft is missing (decision 29), in the order the canvas draws the cards. */
    notes?: readonly { level: 'broken' | 'attention'; card: string; text: string }[];
    /** The card a note is about, pressed. */
    onpick?: (card: string) => void;
    /** Whether the rule is an inbound one, which takes a payload. */
    takesPayload: boolean;
    isRunning: boolean;
    outcome?: Outcome;
    onrun: (sample: { type: string; subject?: string; payload?: Record<string, unknown> }) => void;
    onclear: () => void;
  }

  const { eventTypes, defaultType, notes = [], onpick, takesPayload, isRunning, outcome, onrun, onclear }: Props = $props();

  const words = { t, has: (code: string) => messages.has(code) };

  let type = $state('');
  $effect(() => {
    if (type === '') type = defaultType || eventTypes[0] || '';
  });
  let term = $state('');
  let chosen = $state<{ id: string; title: string } | undefined>(undefined);
  let payloadText = $state('{}');
  let payloadError = $state(false);

  $effect(() => {
    const question = term.trim();
    if (question === '') {
      search.reset();
      return;
    }
    const timer = setTimeout(() => void search.run({ q: question }), 250);
    return () => clearTimeout(timer);
  });

  function run(): void {
    let payload: Record<string, unknown> | undefined;
    if (takesPayload) {
      try {
        payload = JSON.parse(payloadText || '{}') as Record<string, unknown>;
        payloadError = false;
      } catch {
        payloadError = true;
        return;
      }
    }
    onrun({ type, ...(chosen ? { subject: `item/${chosen.id}` } : {}), ...(payload ? { payload } : {}) });
  }
</script>

<div class="panel">
  <h3>{t('app.flow.tab_probe')}</h3>
  <p class="quiet">{t('app.flow.probe_intro')}</p>

  <!-- What is missing, before the button (decision 29): the rule's own reading of the draft,
       each line the card it is about. -->
  {#if notes.length > 0}
    <Callout tone={notes.some((note) => note.level === 'broken') ? 'warning' : 'info'} title={t('app.flow.review_title')}>
      <ul class="notes">
        {#each notes as note, index (index)}
          <li>
            <button class="note" type="button" onclick={() => onpick?.(note.card)}>{note.text}</button>
          </li>
        {/each}
      </ul>
    </Callout>
  {:else}
    <p class="ready">{t('app.flow.review_none')}</p>
  {/if}

  <Select label={t('app.flow.probe_event')} hint={type ? t('app.flow.event_wire_name', { type }) : undefined} bind:value={type} options={[]} groups={eventGroups(words, eventTypes)} />

  <Stack gap="050">
    <SearchField label={t('app.flow.probe_subject')} clearLabel={t('app.search.clear')} bind:value={term} onclear={() => { chosen = undefined; search.reset(); }} />
    <span class="hint">{t('app.flow.probe_subject_hint')}</span>
    {#if chosen}
      <span class="chosen">{t('app.flow.probe_subject_chosen', { title: chosen.title })}</span>
    {/if}
    {#if term.trim() !== '' && search.status === 'done'}
      {#if search.hits.length === 0}
        <span class="hint">{t('app.flow.probe_subject_none')}</span>
      {:else}
        <ul class="hits">
          {#each search.hits.slice(0, 8) as hit (hit.id)}
            <li>
              <button class="hit" type="button" aria-pressed={chosen?.id === hit.id} onclick={() => (chosen = { id: hit.id, title: hit.title })}>{hit.title}</button>
            </li>
          {/each}
        </ul>
      {/if}
    {/if}
  </Stack>

  {#if takesPayload}
    <Textarea label={t('app.flow.probe_payload')} hint={t('app.flow.probe_payload_hint')} error={payloadError ? t('app.rules.action_params_hint') : undefined} rows={4} spellcheck={false} bind:value={payloadText} />
  {/if}

  {#if defaultType && type && type !== defaultType}
    <!-- A sample of another event runs to the trigger and stops there; said before the run rather
         than read out of the frames afterwards. -->
    <Callout tone="info">{t('app.flow.probe_event_mismatch')}</Callout>
  {/if}

  <div>
    <Button tone="primary" icon="play" isBusy={isRunning} busyLabel={t('app.flow.probe_running')} onclick={run}>{t('app.flow.probe_run')}</Button>
  </div>

  {#if outcome}
    <Callout title={t('app.flow.probe_result')}>
      <Stack gap="100">
        <div><RunStatusBadge status={outcome.status as never} label={t(`app.runs.status_${outcome.status.toLowerCase()}`)} isDryRun dryRunLabel={t('app.runs.dry_run')} /></div>
        <span>{t(outcome.code, outcome.params)}</span>
        <div><Button size="sm" tone="subtle" onclick={onclear}>{t('app.flow.probe_clear')}</Button></div>
      </Stack>
    </Callout>
  {/if}
</div>

<style>
  .panel { display: flex; flex-direction: column; gap: var(--sp-200); padding: var(--sp-200); }

  h3 { margin: 0; font-family: var(--font-ui); font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; font-size: var(--fs-075); color: var(--text-secondary); }

  .hint { font-size: var(--fs-075); color: var(--text-subtle); }

  .chosen { font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-primary); }

  .hits { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-025); }

  .hit { width: 100%; text-align: start; padding: var(--sp-050) var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-sm); background: var(--bg-surface); color: var(--text-primary); font-size: var(--fs-075); }

  .hit:hover { background: var(--bg-surface-hover); }

  .hit[aria-pressed='true'] { border-color: var(--accent-primary); background: var(--accent-primary-subtle); }

  .hit:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .notes { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }

  .note { width: 100%; padding: 0; border: 0; background: none; color: inherit; font-size: var(--fs-075); text-align: start; text-decoration: underline; text-underline-offset: var(--sp-025); cursor: pointer; }

  .note:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }

  .ready { margin: 0; font-size: var(--fs-075); color: var(--text-success); }
</style>
