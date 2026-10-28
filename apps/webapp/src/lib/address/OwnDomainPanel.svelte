<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Use your own domain* on the workspace's *Address* screen (UC-INS-15 checks 1, 2, 3 and 10): the
  // field and its refusals, the two records with copy buttons while waiting for DNS, then verified,
  // active - with *Make it the main address* (check 6) - broken, with the proof record to put back,
  // or blocked, with the operator's reason word for word. Verification is the server's: nothing here
  // asks it to check.

  import { Badge, Banner, Button, Input, Stack } from '@hubtask/design-system/components';
  import type { OwnDomain } from '@hubtask/sync-engine';
  import { TransportError } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { formatLongDay, formatTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem, type RenderedProblem } from '../problem.ts';
  import { canBecomeMain, isRemovable, recordsShown, toneOf, zoneOf } from './owndomain.ts';

  interface Props {
    /** The workspace's own domain, absent where it has none. */
    own?: OwnDomain;
    /** The main address, which an active own domain does not replace. */
    main: string;
    /** Whether the reader may set one up or remove it: the workspace's owner. */
    isOwner: boolean;
    /** Whether the installation lets owners set one up now. */
    isAllowed: boolean;
    /** *Make it the main address*, offered on an active domain (UC-INS-15 check 6). */
    onmakemain?: () => void;
  }

  let { own, main, isOwner, isAllowed, onmakemain }: Props = $props();

  let domain = $state('');
  let working = $state(false);
  let failure = $state<RenderedProblem | undefined>(undefined);

  const day = (iso: string): string => formatLongDay(iso, messages.locale);
  const time = (iso: string): string => formatTime(iso, messages.locale);

  async function attempt(action: () => Promise<unknown>): Promise<void> {
    working = true;
    failure = undefined;
    try {
      await action();
      domain = '';
    } catch (error) {
      if (!(error instanceof TransportError)) throw error;
      failure = renderProblem(error, messages);
    } finally {
      working = false;
    }
  }

  async function copy(value: string): Promise<void> {
    try {
      await navigator.clipboard.writeText(value);
      announcer.say(t('app.address.own.copied'));
    } catch {
      announcer.say(t('app.address.own.copy_failed', { value }));
    }
  }

  const fieldError = $derived(failure?.fields.get('/domain'));
</script>

{#snippet record(each: OwnDomain['records'][number], isMissing: boolean)}
  <div class="rec" data-record={each.type}>
    <span class="type">{each.type}</span>
    <div class="kv">
      <span class="k">{t('app.address.own.name')}</span>
      <span class="v">{each.name}</span>
      <Button tone="subtle" size="sm" icon="copy" aria-label={t('app.address.own.copy_named', { value: each.name })} onclick={() => void copy(each.name)}>
        {t('app.address.own.copy')}
      </Button>
    </div>
    <div class="kv">
      <span class="k">{each.type === 'CNAME' ? t('app.address.own.points_to') : t('app.address.own.value')}</span>
      <span class="v">{each.value}</span>
      <Button tone="subtle" size="sm" icon="copy" aria-label={t('app.address.own.copy_named', { value: each.value })} onclick={() => void copy(each.value)}>
        {t('app.address.own.copy')}
      </Button>
    </div>
    {#if isMissing}
      <span class="found missing">{t('app.address.own.missing_since', { day: day(own?.since ?? '') })}</span>
    {:else}
      <span class="found" class:quiet={!each.found}>{each.found ? t('app.address.own.found') : t('app.address.own.not_found')}</span>
    {/if}
  </div>
{/snippet}

<section class="panel" aria-labelledby="own-domain" data-own-domain={own?.state ?? 'NONE'}>
  {#if !own}
    <Stack gap="200">
      <h2 id="own-domain">{t('app.address.own.title')}</h2>
      <p class="prose">{t('app.address.own.intro')}</p>
      {#if isOwner && isAllowed}
        {#if failure && !fieldError}
          <Banner tone="danger" title={failure.message} />
        {/if}
        <Input
          label={t('app.address.own.field')}
          hint={t('app.address.own.hint')}
          face="code"
          autocomplete="off"
          spellcheck={false}
          bind:value={domain}
          error={fieldError}
        />
        <div>
          <Button
            tone="primary"
            isBusy={working}
            busyLabel={t('app.address.own.working')}
            onclick={() => void attempt(() => workspace.setUpOwnDomain(domain))}
          >
            {t('app.address.own.show')}
          </Button>
        </div>
      {/if}
    </Stack>
  {:else}
    <Stack gap={own.state === 'PENDING' ? '200' : '100'}>
      <div class="row between">
        <h2 id="own-domain">{t('app.address.own.title')}</h2>
        <Badge tone={toneOf(own.state)}>{t(`app.address.own.state_${own.state.toLowerCase()}`)}</Badge>
      </div>
      <p class="fixed">{own.domain}</p>

      {#if own.state === 'PENDING'}
        <p class="prose">{t('app.address.own.records', { zone: zoneOf(own.domain) })}</p>
        <div class="records">
          {#each recordsShown(own) as each (each.type)}
            {@render record(each, false)}
          {/each}
        </div>
        <p class="quiet small">
          {own.checked_at ? t('app.address.own.checked', { time: time(own.checked_at) }) : t('app.address.own.unchecked')}
        </p>
      {:else if own.state === 'VERIFIED'}
        <p class="prose">{t('app.address.own.verified', { domain: own.domain })}</p>
        <div class="row"><Badge tone="neutral">{t('app.address.own.certificate_coming')}</Badge></div>
      {:else if own.state === 'ACTIVE'}
        <p class="prose">{t('app.address.own.active', { domain: own.domain, main })}</p>
      {:else if own.state === 'BROKEN'}
        <Banner tone="danger">{t('app.address.own.broken', { day: day(own.since), time: time(own.since), domain: own.domain })}</Banner>
        {#each recordsShown(own) as each (each.type)}
          {@render record(each, true)}
        {/each}
      {:else if own.state === 'BLOCKED'}
        <Banner tone="danger" title={t('app.address.own.blocked_title')}>
          {t('app.address.own.blocked', { day: day(own.since), reason: own.blocked_reason ?? '' })}
        </Banner>
      {/if}

      {#if failure}
        <Banner tone="danger" title={failure.message} />
      {/if}
      {#if isOwner && (isRemovable(own.state) || canBecomeMain(own))}
        <div class="row">
          {#if isAllowed && canBecomeMain(own) && onmakemain}
            <Button tone="primary" onclick={onmakemain}>{t('app.address.own.make_main')}</Button>
          {/if}
          {#if isRemovable(own.state)}
            <Button tone="subtle" isBusy={working} busyLabel={t('app.address.own.working')} onclick={() => void attempt(() => workspace.removeOwnDomain())}>
              {t('app.address.own.remove')}
            </Button>
          {/if}
        </div>
      {/if}
    </Stack>
  {/if}
</section>

<style>
  .panel {
    container-type: inline-size;
    max-inline-size: 72ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  /* The panel carries the measure, so the prose runs its width as the approved screen draws it. */
  .prose { margin: 0; max-inline-size: none; }

  .row { display: flex; align-items: center; gap: var(--sp-100); flex-wrap: wrap; }
  .between { justify-content: space-between; }

  .fixed {
    margin: 0;
    font-family: var(--font-mono);
    font-size: var(--fs-200);
    color: var(--text-primary);
    overflow-wrap: anywhere;
  }

  .quiet { color: var(--text-subtle); }
  .small { margin: 0; max-inline-size: none; font-size: var(--fs-075); }

  .records { display: flex; flex-direction: column; gap: var(--sp-100); }

  /* A record as a zone editor asks for it: its type, then each field with its own copy button. */
  .rec {
    display: grid;
    grid-template-columns: 9ch minmax(0, 1fr);
    gap: var(--sp-050) var(--sp-150);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface-hover);
  }

  .type {
    grid-row: span 2;
    align-self: start;
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--bg-surface-sunken);
    font-family: var(--font-mono);
    font-weight: var(--fw-medium);
    text-align: center;
  }

  .kv { display: flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; }
  .k { flex: none; inline-size: 9ch; font-size: var(--fs-075); color: var(--text-subtle); }
  .v { flex: 1; min-inline-size: 0; font-family: var(--font-mono); font-size: var(--fs-075); overflow-wrap: anywhere; }
  .found { grid-column: 2; font-size: var(--fs-075); }
  .missing { color: var(--text-danger); }

  /* A narrow container stacks the type above its fields rather than squeezing both. */
  @container (inline-size < 40ch) {
    .rec { grid-template-columns: minmax(0, 1fr); }
    .type { grid-row: auto; justify-self: start; }
    .found { grid-column: 1; }
  }
</style>
