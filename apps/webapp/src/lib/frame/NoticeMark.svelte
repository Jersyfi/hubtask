<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What the application has to say **about itself**, as one mark in the bar (ADR-0065 decision 4).
  //
  // Three things qualify, and the rule that lets them in is that they would say the same thing on
  // every screen: the health report, where the reader may read one and it says something is wrong;
  // **the manifest this client could not read**, with the way to ask again; and **the reader's own
  // notices that a private hub of theirs was opened through the emergency access** (ADR-0073 §4),
  // for thirty days after the dialog that told them once. The maturity stage is not among them: it
  // is stated in the release notes only (ADR-0107).
  //
  // The notices are every member's own, the health report only for who may read it: two
  // conditions, kept apart. While only confirmed notices are there the mark is drawn quiet - the
  // shield, no dot, no tint; an unconfirmed one raises it as a fault does.
  //
  // The manifest is here rather than in `SyncLine` because **`SyncLine` is drawn only with a
  // session, this mark always**, and `/meta/capabilities` is read before anybody signs in. A
  // retry behind a mark that is not on the screen is no retry.
  //
  // It is the pattern the connection's mark already is (ADR-0063 decision 5): unpressed it says
  // only that there is something; pressed it says all of it.
  //
  // A notice about **the page** - a refused write, a check's findings - stays where `PageHeader`
  // draws it. This is for what is true wherever the reader stands.

  import { untrack } from 'svelte';

  import { featureNameCode } from '../instance/health.ts';
  import { Button, Drawer, Icon, IconButton, Popover, Stack } from '@hubtask/design-system/components';
  import type { EmergencyNotice, TransportError } from '@hubtask/sync-engine';

  import { viewport } from './viewport.svelte.ts';

  import { accounts } from '../data/accounts.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { emergencyNotices } from '../data/emergencynotices.svelte.ts';
  import { isRunning } from '../data/privatehubs.ts';
  import { reachesAdministrationArea } from '../data/capability.svelte.ts';
  import { health } from '../data/health.svelte.ts';
  import { instance } from '../data/instance.svelte.ts';
  import { formatDateTime, formatTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { session } from '../session.svelte.ts';
  import { noticeStateOf } from './emergencynotice.ts';

  interface Props {
    /** Where a notice's hub is opened. */
    onnavigate: (path: string) => void;
  }

  const { onnavigate }: Props = $props();

  /**
   * Whether this reader may read the report at all: `STRUCTURE` or `READ_CONFIGURATION` on the
   * workspace, or a session raised to the installation's scope - the two conditions
   * `GetHealthReport` answers. Anybody else would be refused, and a refusal is recorded; asking on
   * every page is what filled a member's trail. Derived, so the elevation's clock ticking does not
   * start the read again every second.
   */
  const mayReadHealth = $derived(reachesAdministrationArea() === true || instance.isElevated);

  /**
   * The health report, subscribed for as long as this mark is drawn and read again when the
   * session changes: without a bearer there is nothing to read, and the subscription taken before
   * somebody signed in is one the sign-out already dropped.
   *
   * **The manifest deliberately has no effect here.** Both are read again when the actor changes,
   * by two mechanisms, because they are two kinds of read: a *subscription* is started and stopped
   * by whoever draws it, which is this component; a *one-shot* read is refreshed by whoever changed
   * the actor, which is `session.svelte.ts` - it calls `manifest.refresh()` at each of its four
   * transitions. A second refresh here would be this component asking again for a
   * read it does not own.
   */
  $effect(() => {
    void session.status;
    return health.start(mayReadHealth);
  });

  /** Whether the report says something is wrong. Nothing at all without a report or a right to it. */
  const isUnwell = $derived(health.isTroubled);
  /**
   * The manifest, where it could not be read. Absent in the ordinary case, which is every case but
   * one - and the one is the whole application running on nothing it was told.
   *
   * **`failure`, not `state`**: the engine re-reads an unanswered resource on every reconnect and
   * publishes `loading` over the error, so a mark that read the live state would flicker between
   * the sentence and nothing - and the retry would go out from under the finger pressing it.
   */
  const unread = $derived.by(() => {
    if (!manifest.failure) return undefined;
    const problem = renderProblem(manifest.failure, messages);
    return { reason: problem.message, reference: problem.reference };
  });
  /** Anything wrong: the report, or an installation this client never read. */
  const isFaulty = $derived(isUnwell || unread !== undefined);

  // The reader's own notices, read while signed in; the dialog that tells them once reads the same.
  $effect(() => {
    if (!session.isSignedIn) return;
    return untrack(() => emergencyNotices.open());
  });
  const notices = $derived(emergencyNotices.notices);
  const noticeState = $derived(noticeStateOf(notices));
  const unconfirmed = $derived(notices.filter((each) => each.acknowledged_at === null).length);
  $effect(() => {
    accounts.resolve(notices.map((each) => each.opened_by));
  });

  /** Raised - the tone and the dot - for a fault or an unconfirmed notice; quiet for confirmed ones. */
  const isTroubled = $derived(isFaulty || noticeState === 'unconfirmed');
  const isDrawn = $derived(isFaulty || noticeState !== 'none');

  /**
   * The mark, and what it says without being pressed: the state is the mark, the dot *and* the
   * accessible name, never the colour alone (rule 3). A fault keeps the triangle; the notices alone
   * draw the shield.
   */
  const noticesLabel = $derived(
    unconfirmed > 0 ? t('app.emergency_notice.mark_unconfirmed', { count: unconfirmed }) : t('app.emergency_notice.mark_quiet'),
  );
  const label = $derived(
    isFaulty
      ? `${unread ? t('app.installation.unread') : t('app.health.title')}${notices.length > 0 ? t('app.emergency_notice.mark_also') : ''}`
      : noticesLabel,
  );
  /** What the open panel is called: the reader's own notices make it theirs. */
  const title = $derived(notices.length === 0 ? label : unconfirmed > 0 ? t('app.emergency_notice.panel_yours') : t('app.emergency_notice.panel'));

  const hubOf = (notice: EmergencyNotice) => containers.hubs.find((hub) => hub.id === notice.hub_id);
  const nameOf = (id: string | null) => accounts.nameOf(id) ?? t('app.private_hubs.somebody');

  /** The notice being confirmed from here, and the one whose confirmation was refused. */
  let sending = $state<string | undefined>(undefined);
  let refused = $state<{ id: string; text: string } | undefined>(undefined);
  async function acknowledge(notice: EmergencyNotice) {
    sending = notice.id;
    refused = undefined;
    const refusal = await emergencyNotices.acknowledge([notice.id]);
    sending = undefined;
    if (refusal === undefined) return;
    const reference = renderProblem(refusal as TransportError, messages).reference;
    refused = { id: notice.id, text: reference ? t('app.emergency_notice.refused_reference', { reference }) : t('app.emergency_notice.refused') };
  }

  let isOpen = $state(false);
  let isPopoverOpen = $state(false);
  const instanceId = $props.id();
  const headingId = `notices-${instanceId}`;

  function openHub(notice: EmergencyNotice) {
    isOpen = false;
    isPopoverOpen = false;
    onnavigate(`/hubs/${notice.hub_id}`);
  }
</script>

{#snippet inside()}
  <div class="panel">
    <Stack gap="200">
      <!-- The reader's own first: what happened to their private hubs, for thirty days. -->
      {#if notices.length > 0}
        <section class="notices" aria-labelledby={headingId} data-emergency-notice="mark">
          <h3 class="heading" id={headingId}>{t('app.emergency_notice.section')}</h3>
          {#each notices as notice (notice.id)}
            {@const hub = hubOf(notice)}
            <div class="item" data-tone={notice.acknowledged_at === null ? 'warning' : undefined}>
              <div class="first">
                {#if hub}
                  <a class="hub" href={`/hubs/${notice.hub_id}`} onclick={(event) => { event.preventDefault(); openHub(notice); }}>{hub.name}</a>
                {:else}
                  <span class="hub">{t('app.emergency_notice.a_private_hub')}</span>
                {/if}
                {#if notice.acknowledged_at === null}
                  <Button size="sm" tone="secondary" isBusy={sending === notice.id} busyLabel={t('app.workspace.saving')} onclick={() => void acknowledge(notice)}>
                    {t('app.emergency_notice.acknowledge')}
                  </Button>
                {:else}
                  <span class="confirmed"><Icon name="check" size="sm" />{t('app.emergency_notice.acknowledge')}</span>
                {/if}
              </div>
              <span class="line">{t(isRunning(notice) ? 'app.emergency_notice.mark_line_running' : 'app.emergency_notice.mark_line', {
                name: nameOf(notice.opened_by), when: formatDateTime(notice.opened_at, messages.locale), until: formatTime(notice.ends_at, messages.locale),
              })}</span>
              <span class="line folded">{t('app.emergency_notice.mark_reason', { reason: notice.reason })}</span>
              {#if refused?.id === notice.id}<span class="line refusal" role="alert">{refused.text}</span>{/if}
            </div>
          {/each}
          <p class="footnote">{t('app.emergency_notice.mark_stays')}</p>
        </section>
      {/if}
      <!-- What this client could not read about the installation, first: a client that has not read
           the manifest knows no type, no role and no limit, so everything else it says is said on
           nothing. The ask-again is here because this mark is on every screen, signed in or not. -->
      {#if unread}
        <section class="notice" data-tone="danger" aria-label={t('app.installation.unread')}>
          <Stack gap="050">
            <h3 class="heading">{t('app.installation.unread')}</h3>
            {#if unread.reason}<p class="line">{unread.reason}</p>{/if}
            {#if unread.reference}
              <p class="line reference">{t('app.reference')} <code>{unread.reference}</code></p>
            {/if}
            <div>
              <Button size="sm" tone="secondary" icon="repeat" onclick={() => void manifest.refresh()}>
                {t('app.retry')}
              </Button>
            </div>
          </Stack>
        </section>
      {/if}
      <!-- Then the report. -->
      {#if isUnwell}
        <section class="notice" data-tone={health.isDown ? 'danger' : 'warning'} aria-label={t('app.health.title')}>
          <Stack gap="050">
            <h3 class="heading">{t('app.health.title')}</h3>
            {#each health.degradations as degradation (degradation.feature)}
              <p class="line">{t('app.health.feature', { feature: featureNameCode(degradation.feature) ? t(featureNameCode(degradation.feature)!) : degradation.feature, reason: t(degradation.reasonCode) })}</p>
            {/each}
          </Stack>
        </section>
      {/if}
    </Stack>
  </div>
{/snippet}

{#snippet mark(props: Record<string, unknown>)}
  <span class="mark" data-troubled={isTroubled ? '' : undefined}>
    <IconButton icon={isFaulty ? 'triangle-alert' : 'shield'} {label} size="sm" {...props} />
    {#if isTroubled}<span class="dot" aria-hidden="true"></span>{/if}
  </span>
{/snippet}

{#if isDrawn}
  <div class="notice-mark">
    {#if viewport.isCompact}
      <!-- The same choice `SyncStatus` and the account group make: on a phone there is no room
           beside a bar's control, so what it opens comes from the edge. -->
      {@render mark({ onclick: () => (isOpen = true), 'aria-expanded': isOpen, 'aria-haspopup': 'dialog' })}
      <Drawer bind:isOpen edge="block-end" {title} dismissLabel={t('app.dismiss')}>
        {@render inside()}
      </Drawer>
    {:else}
      <Popover isAnchored label={title} bind:isOpen={isPopoverOpen}>
        {#snippet trigger(props)}{@render mark(props)}{/snippet}
        {@render inside()}
      </Popover>
    {/if}
  </div>
{/if}

<style>
  .notice-mark { display: inline-flex; align-items: center; }

  /* A floor on the width, so the surface beside a bar's control is not squeezed into a column of
     two words by the edge it is anchored against. */
  .panel { min-inline-size: 28ch; max-inline-size: 44ch; }

  .mark { position: relative; display: inline-flex; }

  /* Rule 3 again: the state is the mark, the dot *and* the accessible name, never the colour
     alone - the mark turns into the warning triangle and the name becomes the report's title. */
  .mark[data-troubled] { border-radius: var(--r-sm); background: var(--status-warning-surface); color: var(--status-warning-text); }

  .dot {
    position: absolute;
    inset-block-start: 0;
    inset-inline-end: 0;
    inline-size: var(--sp-100);
    block-size: var(--sp-100);
    border-radius: var(--r-full);
    background: var(--status-warning-accent);
  }

  .notices { display: flex; flex-direction: column; gap: var(--sp-100); }

  .item { display: flex; flex-direction: column; gap: var(--sp-050); padding: var(--sp-100); border-radius: var(--r-md); }

  .item + .item:not([data-tone]) { border-block-start: var(--bw-hairline) solid var(--border-subtle); border-radius: 0; }

  /* Unconfirmed: on the warning surface, the button that confirms it beside its hub. */
  .item[data-tone='warning'] { background: var(--status-warning-surface); }

  .first { display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100); }

  .hub { font-weight: var(--fw-medium); }

  a.hub { color: var(--text-brand); text-decoration: underline; }

  .confirmed { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); white-space: nowrap; }

  .folded { display: -webkit-box; overflow: hidden; -webkit-box-orient: vertical; -webkit-line-clamp: 2; line-clamp: 2; }

  .refusal { color: var(--text-danger); }

  .footnote { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); color: var(--text-primary); }

  .line { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }

  /* A notice with a tone is a status: its surface under the heading and the line (§6 rule 3). */
  .notice[data-tone] { padding: var(--sp-100); border-radius: var(--r-md); }

  .notice[data-tone='danger'] { background: var(--status-danger-surface); }

  .notice[data-tone='danger'] .heading { color: var(--status-danger-text); }

  .notice[data-tone='warning'] { background: var(--status-warning-surface); }

  .notice[data-tone='warning'] .heading { color: var(--status-warning-text); }

  /* The correlation id, in the data style: it is quoted into a support thread, not read. */
  .reference { color: var(--text-subtle); font-family: var(--font-mono); }
</style>
