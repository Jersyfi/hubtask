<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who reads a page beyond its hub (UC-DOC-07, UC-DOC-08): its restriction to fewer of the hub's
  // readers, one above it, and the people it is shared with read-only. A writer restricts and
  // lifts; somebody who manages the hub's members shares (`may_share`). What the reader may not do
  // is not drawn (P-05). A public link, where the workspace allows one, is shown once when made:
  // only its hash is kept.

  import { untrack } from 'svelte';
  import { Badge, Button, CheckboxGroup, Inline, Input, Select, Stack } from '@hubtask/design-system/components';
  import type { PageAccess, PageRead } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { accessPath, pages, publicLinkAddress } from '../data/pages.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    page: PageRead;
    hubId: string;
  }

  const { page, hubId }: Props = $props();

  const access = untrack(() => resource<PageAccess>(accessPath(page.id)));
  const info = $derived(access.state.status === 'ready' ? access.state.data : undefined);
  const nameOf = (id: string | null) => (id ? accounts.nameOf(id) : undefined) ?? t('app.people.unnamed');

  $effect(() => {
    if (info) accounts.resolve([...info.restricted_to, ...(info.readers ?? []), ...info.shares.map((share) => share.account_id)]);
  });

  let refusal = $state<string | undefined>(undefined);
  async function act(run: () => Promise<unknown>, done: string) {
    refusal = undefined;
    try {
      await run();
      toast.show({ text: done, tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  // --- Restricting -------------------------------------------------------------------------------

  let isChoosing = $state(false);
  let chosen = $state<readonly string[]>([]);
  const self = $derived(actor.account?.id ?? '');
  const hubReaders = $derived(people.candidates({ hubId }));
  const readerOptions = $derived(
    [...new Set([self, ...hubReaders])].filter(Boolean).map((id) => ({
      value: id,
      label: id === self ? t('app.pages.access_you', { name: nameOf(id) }) : nameOf(id),
      ...(id === self ? { disabledReason: t('app.pages.access_you_stay') } : {}),
    })),
  );

  function startRestrict() {
    chosen = info && info.restricted ? [...new Set([self, ...info.restricted_to])] : [self];
    isChoosing = true;
  }

  async function restrict() {
    await act(() => pages.restrict(page, [...new Set([self, ...chosen])]), t('app.pages.access_restricted'));
    isChoosing = false;
  }

  // --- Sharing -----------------------------------------------------------------------------------

  let shareWith = $state('');
  const shareOptions = $derived([
    { value: '', label: t('app.pages.access_share_choose') },
    ...people
      .known()
      .filter((id) => id !== self && !(info?.shares ?? []).some((share) => share.account_id === id))
      .map((id) => ({ value: id, label: nameOf(id) })),
  ]);

  async function share() {
    if (!shareWith) return;
    const whom = shareWith;
    await act(() => pages.share(page.id, whom), t('app.pages.access_shared', { name: nameOf(whom) }));
    shareWith = '';
  }

  // --- A public link -----------------------------------------------------------------------------

  let madeAddress = $state<string | undefined>(undefined);
  const linkable = $derived(info?.may_share === true && info.public_links_allowed && !info.restricted && !info.restricted_above);

  async function makeLink() {
    refusal = undefined;
    try {
      const made = await pages.link(page.id);
      madeAddress = publicLinkAddress(globalThis.location.origin, made.token);
      void navigator.clipboard?.writeText(madeAddress).catch(() => undefined);
      toast.show({ text: t('app.pages.access_link_made'), tone: 'success' });
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    }
  }

  async function removeLink() {
    madeAddress = undefined;
    await act(() => pages.unlink(page.id), t('app.pages.access_link_removed'));
  }
</script>

<section aria-labelledby="page-access-heading">
  <Stack gap="100">
    <h3 class="heading" id="page-access-heading">{t('app.pages.access')}</h3>
    {#if refusal}<p class="refusal" role="alert">{refusal}</p>{/if}
    {#if access.state.status === 'failed'}
      <p class="refusal">{renderProblem(access.state.error, messages).message}</p>
    {:else if info}
      {#if info.restricted}
        <p class="meta">
          <Badge size="sm" tone="warning">{t('app.pages.access_restricted_badge')}</Badge>
          {t('app.pages.access_restricted_to', { names: info.restricted_to.map(nameOf).join(', ') })}
        </p>
      {:else if info.restricted_above}
        <p class="meta">{t('app.pages.access_restricted_above', { names: (info.readers ?? []).map(nameOf).join(', ') })}</p>
      {:else}
        <p class="quiet">{t('app.pages.access_hub')}</p>
      {/if}

      {#if page.writable}
        {#if isChoosing}
          <CheckboxGroup label={t('app.pages.access_who')} options={readerOptions} selected={chosen} onChange={(next) => (chosen = next)} />
          <Inline gap="100">
            <Button size="sm" tone="primary" onclick={restrict}>{t('app.pages.access_restrict_save')}</Button>
            <Button size="sm" tone="subtle" onclick={() => (isChoosing = false)}>{t('app.pages.cancel')}</Button>
          </Inline>
        {:else}
          <Inline gap="100" isWrapping>
            <Button size="sm" tone="secondary" icon="lock" onclick={startRestrict}>
              {info.restricted ? t('app.pages.access_restrict_change') : t('app.pages.access_restrict')}
            </Button>
            {#if info.restricted}
              <Button size="sm" tone="subtle" onclick={() => act(() => pages.restrict(page, undefined), t('app.pages.access_lifted'))}>
                {t('app.pages.access_lift')}
              </Button>
            {/if}
          </Inline>
        {/if}
      {/if}

      <h4 class="sub">{t('app.pages.access_shares')}</h4>
      <ul class="shares">
        {#each info.shares as held (held.account_id)}
          <li>
            <span>{nameOf(held.account_id)}</span>
            {#if info.may_share}
              <Button size="sm" tone="subtle" onclick={() => act(() => pages.unshare(page.id, held.account_id), t('app.pages.access_unshared'))}>
                {t('app.pages.access_unshare')}
              </Button>
            {/if}
          </li>
        {:else}
          <li class="quiet">{t('app.pages.access_no_shares')}</li>
        {/each}
      </ul>
      {#if info.may_share}
        <Inline gap="100" align="end" isWrapping>
          <Select label={t('app.pages.access_share_with')} size="sm" options={shareOptions} bind:value={shareWith} />
          <Button size="sm" tone="secondary" onclick={share}>{t('app.pages.access_share')}</Button>
        </Inline>
        <p class="quiet">{t('app.pages.access_share_hint')}</p>
      {/if}

      {#if info.public_link || linkable}
        <h4 class="sub">{t('app.pages.access_link')}</h4>
        {#if info.public_link}
          <p class="meta">
            {info.public_links_allowed
              ? t('app.pages.access_link_open', { name: nameOf(info.public_link.created_by) })
              : t('app.pages.access_link_paused')}
          </p>
        {:else}
          <p class="quiet">{t('app.pages.access_link_none')}</p>
        {/if}
        {#if madeAddress}
          <Input label={t('app.pages.access_link_address')} value={madeAddress} readonly />
          <p class="quiet">{t('app.pages.access_link_once')}</p>
        {/if}
        {#if info.may_share}
          <Inline gap="100" isWrapping>
            {#if linkable}
              <Button size="sm" tone="secondary" icon="link" onclick={makeLink}>
                {info.public_link ? t('app.pages.access_link_renew') : t('app.pages.access_link_make')}
              </Button>
            {/if}
            {#if info.public_link}
              <Button size="sm" tone="subtle" onclick={removeLink}>{t('app.pages.access_link_remove')}</Button>
            {/if}
          </Inline>
          <p class="quiet">{t('app.pages.access_link_hint')}</p>
        {/if}
      {/if}
    {/if}
  </Stack>
</section>

<style>
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .sub { margin: 0; font-size: var(--fs-075); font-weight: var(--fw-semibold); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .refusal { margin: 0; color: var(--text-danger); }
  .shares { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .shares li { display: flex; justify-content: space-between; align-items: center; gap: var(--sp-100); }
</style>
