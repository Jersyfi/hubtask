<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The notice that a private hub of the reader's was opened through the emergency access, once at
  // entry (ADR-0073 §4, UC-ID-16 check 11): who, which hub, when, until when and why, and a way to
  // confirm it. An alert dialog, because it interrupts on purpose and asks to be acknowledged; its
  // focus starts on the first answer of the foot, *Open the hub* for one notice, so a reflex Enter
  // goes somewhere rather than confirming unread. ✕ and Escape close it without confirming, a click
  // beside it does not (an alert dialog is answered); the notice then asks again at the next opening.
  //
  // Loaded when it is shown: the frame's first screen does not carry it. On a phone it stays the
  // centred card for several notices too, rather than an M's full screen.
  //
  // The reason is the opener's own text (user content, rule 10): drawn here and nowhere else - not
  // in the announcement, a log or an address.

  import { Badge, Button, Callout, Dialog, ProductTile } from '@hubtask/design-system/components';
  import type { EmergencyNotice, TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { emergencyNotices } from '../data/emergencynotices.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { isRunning } from '../data/privatehubs.ts';
  import { formatDateTime, formatTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { DEFAULT_TILE, chosenSymbol, markOf } from '../workspace/look.ts';
  import { FOLDED_REASON, LONG_REASON } from './emergencynotice.ts';
  import { viewport } from './viewport.svelte.ts';

  interface Props {
    /** The notices shown, as they were when it opened: the content does not move under the reader. */
    notices: readonly EmergencyNotice[];
    onnavigate: (path: string) => void;
    /** Ran when it closed, however it closed. */
    onclose: () => void;
  }

  const { notices, onnavigate, onclose }: Props = $props();

  let isOpen = $state(true);
  let isSending = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let unfolded = $state<ReadonlySet<string>>(new Set());

  const isMany = $derived(notices.length > 1);
  const isOffline = $derived(live.state === 'offline');

  $effect(() => {
    accounts.resolve(notices.map((each) => each.opened_by));
  });

  const hubOf = (notice: EmergencyNotice) => containers.hubs.find((hub) => hub.id === notice.hub_id);
  const nameOf = (id: string | null) => accounts.nameOf(id) ?? t('app.private_hubs.somebody');
  const openers = $derived([...new Set(notices.map((each) => each.opened_by))]);
  const at = (iso: string) => t('app.emergency_notice.at', { when: formatDateTime(iso, messages.locale) });
  const statusOf = (notice: EmergencyNotice) => isRunning(notice)
    ? t('app.emergency_notice.running_until', { time: formatTime(notice.ends_at, messages.locale) })
    : t('app.emergency_notice.ended_at', { time: formatTime(notice.ends_at, messages.locale) });

  /** A sentence with the opener's name in it, split so the name can stand out. */
  function around(code: string, name: string): readonly [string, string, string] {
    const text = t(code, { name });
    const index = text.indexOf(name);
    return index < 0 ? [text, '', ''] : [text.slice(0, index), name, text.slice(index + name.length)];
  }

  async function acknowledge() {
    if (isSending || isOffline) return;
    isSending = true;
    refusal = undefined;
    const refused = await emergencyNotices.acknowledge(notices.map((each) => each.id));
    isSending = false;
    if (refused !== undefined) {
      const reference = renderProblem(refused as TransportError, messages).reference;
      refusal = reference ? t('app.emergency_notice.refused_reference', { reference }) : t('app.emergency_notice.refused');
      return;
    }
    announcer.say(t('app.emergency_notice.acknowledged_said'));
    isOpen = false;
  }

  /** Goes to the hub, confirming its notice on the way; a refusal is then carried by the mark's dot. */
  function openHub(notice: EmergencyNotice) {
    if (!isOffline) void emergencyNotices.acknowledge([notice.id]);
    isOpen = false;
    onnavigate(`/hubs/${notice.hub_id}`);
  }
</script>

{#snippet hubName(notice: EmergencyNotice)}
  {@const hub = hubOf(notice)}
  {#if hub}
    <span class="hub">
      <ProductTile icon={hub.private && !chosenSymbol(hub) ? 'lock' : markOf(hub)} color={hub.color_token ?? DEFAULT_TILE} />
      <span class="hub-name">{hub.name}</span>
    </span>
  {:else}
    <!-- The reader left the hub since: the notice stays theirs, without a name or a way there. -->
    <span class="hub-name">{t('app.emergency_notice.a_private_hub')}</span>
  {/if}
{/snippet}

{#snippet status(notice: EmergencyNotice)}
  <Badge size="sm" tone={isRunning(notice) ? 'warning' : 'neutral'}>{statusOf(notice)}</Badge>
{/snippet}

<Dialog
  bind:isOpen
  isAlert
  size={isMany && !viewport.isCompact ? 'm' : 's'}
  title={isMany ? t('app.emergency_notice.title_many', { count: notices.length }) : t('app.emergency_notice.title_one')}
  dismissLabel={t('app.dismiss')}
  onClose={() => {
    // It opened over nothing: where the focus had nowhere to return, it goes to the page.
    setTimeout(() => {
      if (document.activeElement === document.body) document.getElementById('main')?.focus();
    }, 0);
    onclose();
  }}
  primary={{
    label: isMany ? t('app.emergency_notice.acknowledge_all') : t('app.emergency_notice.acknowledge'),
    onclick: () => void acknowledge(),
    isBusy: isSending,
    busyLabel: t('app.workspace.saving'),
    disabledReason: isOffline ? t('app.emergency_notice.offline_body') : undefined,
  }}
>
  <div class="notice" data-emergency-notice="entry">
    {#if isOffline}
      <Callout tone="warning" title={t('app.emergency_notice.offline_title')}>{t('app.emergency_notice.offline_body')}</Callout>
    {/if}
    {#if !isMany}
      {@const notice = notices[0]!}
      {@const [before, name, after] = around('app.emergency_notice.intro_one', nameOf(notice.opened_by))}
      <p class="text">{before}<b>{name}</b>{after}</p>
      <dl class="facts">
        <dt>{t('app.emergency_notice.fact_hub')}</dt>
        <dd>{@render hubName(notice)}</dd>
        <dt>{t('app.emergency_notice.fact_opened')}</dt>
        <dd>{at(notice.opened_at)}</dd>
        <dt>{t('app.emergency_notice.fact_access')}</dt>
        <dd>{@render status(notice)}</dd>
        <dt>{t('app.emergency_notice.fact_reason')}</dt>
        <dd><p class="reason">{notice.reason}</p></dd>
      </dl>
      <p class="footnote">
        {t('app.emergency_notice.stays_one')}
        {#if notice.reason.length > LONG_REASON}{t('app.emergency_notice.in_full', { name: nameOf(notice.opened_by) })}{/if}
      </p>
    {:else}
      {#if openers.length === 1}
        <p class="text">{t('app.emergency_notice.intro_many', { name: nameOf(openers[0] ?? null) })}</p>
      {:else}
        <p class="text">{t('app.emergency_notice.intro_many_owners')}</p>
      {/if}
      <ul class="list">
        {#each notices as notice (notice.id)}
          {@const isFolded = notice.reason.length > FOLDED_REASON && !unfolded.has(notice.id)}
          <li class="card">
            <div class="top">{@render hubName(notice)}{@render status(notice)}</div>
            <span class="who">{nameOf(notice.opened_by)} · {at(notice.opened_at)}</span>
            <p class="reason"><span class:folded={isFolded}>{notice.reason}</span></p>
            <div class="foot">
              {#if hubOf(notice)}
                <a href={`/hubs/${notice.hub_id}`} onclick={(event) => { event.preventDefault(); openHub(notice); }}>{t('app.private_hubs.notice_open_hub')}</a>
              {:else}
                <span></span>
              {/if}
              {#if notice.reason.length > FOLDED_REASON}
                <button type="button" class="more" aria-expanded={!isFolded}
                  onclick={() => (unfolded = isFolded ? new Set([...unfolded, notice.id]) : new Set([...unfolded].filter((id) => id !== notice.id)))}>
                  {isFolded ? t('app.emergency_notice.reason_more') : t('app.emergency_notice.reason_less')}
                </button>
              {/if}
            </div>
          </li>
        {/each}
      </ul>
      <p class="footnote">{t('app.emergency_notice.stays_many')}</p>
    {/if}
    {#if refusal}
      <p class="refusal" role="alert">{refusal}</p>
    {/if}
  </div>
  {#snippet actions()}
    {#if !isMany && hubOf(notices[0]!)}
      <Button tone="secondary" onclick={() => openHub(notices[0]!)}>{t('app.private_hubs.notice_open_hub')}</Button>
    {/if}
  {/snippet}
</Dialog>

<style>
  .notice { display: flex; flex-direction: column; gap: var(--sp-250); }

  .text { margin: 0; }

  .facts {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr);
    gap: var(--sp-100) var(--sp-200);
    margin: 0;
  }

  .facts dt { padding-block-start: var(--sp-025); color: var(--text-secondary); font-size: var(--fs-090); }

  .facts dd { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; margin: 0; }

  .facts .reason { inline-size: 100%; }

  .hub { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  /* The opener's own words, set apart from the product's. */
  .reason { margin: 0; padding: var(--sp-150); border-radius: var(--r-md); background: var(--bg-recess); overflow-wrap: anywhere; }

  .footnote { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .refusal { margin: 0; color: var(--text-danger); }

  .list { display: flex; flex-direction: column; gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }

  .card {
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .top { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .top .hub { flex: 1; }

  .hub-name { font-weight: var(--fw-medium); }

  .who { color: var(--text-secondary); font-size: var(--fs-090); }

  .folded { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }

  .foot { display: flex; justify-content: space-between; gap: var(--sp-150); font-size: var(--fs-090); }

  .foot a { color: var(--text-brand); text-decoration: underline; }

  .more { padding: 0; border: 0; background: none; color: var(--text-brand); font: inherit; text-decoration: underline; cursor: pointer; }

  /* Each a press of its own: at least 24 px everywhere (§10, 2.5.8), `layout.target` on a phone or
     a coarse pointer, where the target reaches into the card's padding and the gap above it so the
     card grows by little more than the text did. */
  .foot a, .more { display: inline-flex; align-items: center; min-block-size: var(--sp-300); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (pointer: coarse), (width < 600px) {
    .foot a, .more { min-block-size: var(--layout-target); margin-block: calc(-1 * var(--sp-100)); }
  }
</style>
