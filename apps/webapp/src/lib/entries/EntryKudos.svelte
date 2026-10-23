<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's kudos, in its head (UC-WRK-29 checks 5 to 7). Mounted by the entry's page only
  // while `kudosOffer` says there is something to offer - so with celebrations off nothing is read
  // or drawn. The person who completed the entry reads who gave kudos, by name and never as a
  // number; everybody else has one toggle, their own kudos to the person who completed it.

  import { untrack } from 'svelte';
  import type { WorkItem } from '@hubtask/sync-engine';
  import { Button, Icon } from '@hubtask/design-system/components';

  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { celebration } from '../celebration.svelte.ts';
  import { live } from '../data/live.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { kudos, kudosPath, type Kudos } from '../data/kudos.svelte.ts';
  import { kudosOffer, namesOf } from '../data/kudos.ts';
  import { renderProblem } from '../problem.ts';
  import { toast } from '../frame/toasts.svelte.ts';

  interface Props {
    item: WorkItem;
  }

  const { item }: Props = $props();

  const view = resource<Kudos>(untrack(() => kudosPath(item.id)));
  const offer = $derived(kudosOffer(item, actor.account?.id, celebration.isOn));
  const current = $derived(view.state.status === 'ready' ? view.state.data : undefined);
  const completer = $derived(accounts.nameOf(item.completion?.completed_by) ?? t('app.people.unnamed'));
  const givers = $derived((current?.from ?? []).map((each) => accounts.nameOf(each.account_id) ?? t('app.people.unnamed')));
  // The names are read here, not borrowed from the head's people marks: a collection whose
  // entries carry no person draws none.
  $effect(() => {
    const ids = [item.completion?.completed_by, ...(current?.from ?? []).map((each) => each.account_id)].filter((id): id is string => Boolean(id));
    if (ids.length > 0) accounts.resolve(ids);
  });

  let isBusy = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);

  async function toggle() {
    if (!current || isBusy) return;
    isBusy = true;
    failure = undefined;
    const isTakingBack = current.given;
    try {
      await (isTakingBack ? kudos.takeBack(item.id) : kudos.give(item.id));
      toast.show({
        text: isTakingBack ? t('app.kudos.taken_back_announced') : t('app.kudos.given_announced', { name: completer }),
        tone: 'success',
      });
    } catch (error) {
      failure = renderProblem(error as never, messages);
    } finally {
      isBusy = false;
    }
  }
</script>

{#if offer === 'received' && givers.length > 0}
  <p class="kudos" data-kudos="received"><Icon name="hand" size="sm" />{t('app.kudos.from', { names: namesOf(givers, messages.locale) })}</p>
{:else if offer === 'give' && current}
  <!-- One toggle with one name: what it does to whom, pressed while the kudos stand. Offline it
       stays in place and says why (design-system.md §4). -->
  <span class="kudos" data-kudos="give">
    <Button
      size="sm"
      tone={current.given ? 'primary' : 'secondary'}
      icon="hand"
      aria-pressed={current.given}
      aria-label={t('app.kudos.give_to', { name: completer })}
      {isBusy}
      busyLabel={t('app.kudos.give')}
      disabledReason={live.state === 'offline' ? t('app.kudos.offline') : undefined}
      onclick={() => void toggle()}
    >
      {t('app.kudos.give')}
    </Button>
    {#if failure}<span class="failure" role="alert">{failure.message}</span>{/if}
  </span>
{/if}

<style>
  .kudos {
    display: inline-flex;
    align-items: center;
    gap: var(--sp-100);
    margin: 0;
    color: var(--text-secondary);
  }

  .failure { color: var(--text-danger); font-size: var(--fs-075); }
</style>
