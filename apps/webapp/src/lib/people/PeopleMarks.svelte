<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who is on an entry, in the space of a row: the assignee as a face, the others as a stack.
  //
  // **Names come from the cache, not from the list.** `expand=assignee` is what a list would ask
  // for, and this server serves `labels` and refuses every other relation by name — so asking for
  // it would fail the whole request rather than degrade. The accounts cache gets the same result
  // without a request per row: twenty entries by three people is three reads, once per session.
  //
  // **The assignee and the members are drawn apart**, because they are two different facts and one
  // stack of faces would say neither. The assignee is who the entry belongs to; the members are
  // who else can see it.

  import { Avatar, AvatarGroup } from '@hubtask/design-system/components';

  import { accounts } from '../data/accounts.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  const {
    assigneeId,
    memberIds = [],
  }: { assigneeId?: string | null; memberIds?: readonly string[] } = $props();

  const nameOf = (id: string) => accounts.nameOf(id) ?? t('app.people.unnamed');

  /** The members other than the assignee: the same face twice would be saying it twice. */
  const others = $derived(memberIds.filter((id) => id !== assigneeId));

  $effect(() => {
    accounts.resolve([assigneeId, ...memberIds]);
  });
</script>

{#if assigneeId || others.length > 0}
  <span class="marks">
    {#if assigneeId}
      <Avatar name={nameOf(assigneeId)} size="sm" />
    {/if}
    {#if others.length > 0}
      <AvatarGroup
        people={others.map((id) => ({ name: nameOf(id) }))}
        size="sm"
        max={3}
        overflowLabel={t('app.people.more_members', { count: String(Math.max(others.length - 3, 0)) })}
      />
    {/if}
  </span>
{/if}

<style>
  .marks { display: inline-flex; align-items: center; gap: var(--sp-050); }
</style>
