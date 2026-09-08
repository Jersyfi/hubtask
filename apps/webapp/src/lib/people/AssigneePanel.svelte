<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who the entry belongs to, and who else is on it.
  //
  // **Two controls, because the model has two operations.** C-01 split the assignee from the
  // member list because they merge differently — last-write-wins against an OR-set — and a single
  // control that wrote both would be a client deciding which merge rule applies. So one picker
  // sets the assignee and another adds and removes members, one call at a time.
  //
  // **The manifest decides which of them exist.** An activity carries an assignee and no member
  // list, and this component learns that from `/meta/capabilities` rather than from a list of
  // types written here. A type that carries neither shows the reason, because a control that is
  // missing tells the reader nothing.
  //
  // **The picker is a courtesy.** It offers the accounts that hold a membership along the path;
  // the server refuses one that cannot see the entry, and that refusal reaches the reader as a
  // sentence rather than being pre-empted badly.

  import { AssigneeControl, Button, CapabilityGate, Stack } from '@hubtask/design-system/components';
  import type { AutoAssignOutcome, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { supports } from '../data/capability.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item, path }: { item: WorkItem; path: Path } = $props();

  const assignment = $derived(supports(item.type, 'ASSIGNMENT'));
  const membership = $derived(supports(item.type, 'MEMBERS'));

  const candidateIds = $derived(people.candidates(path));
  const candidates = $derived(
    candidateIds.map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  const memberIds = $derived((item.member_ids ?? []) as readonly string[]);

  let failure = $state<string | undefined>(undefined);
  let outcome = $state<AutoAssignOutcome | undefined>(undefined);
  let isAutoAssigning = $state(false);

  /** One place where a write becomes a sentence, so no branch below reads a code itself. */
  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    try {
      await work();
    } catch (error) {
      // The same cast every write in this application makes: the engine promises a
      // `TransportError` and a `catch` binding is `unknown` whatever it promises.
      failure = renderProblem(error as never, messages).message;
    }
  }

  function setAssignee(ids: readonly string[]) {
    const next = ids[0];
    void attempt(() =>
      next === undefined
        ? items.unassign(item.id, item.version, crypto.randomUUID())
        : items.assign(item.id, next, item.version, crypto.randomUUID()),
    );
  }

  function setMembers(ids: readonly string[]) {
    // One call per change, because the set is an OR-set: the difference between what was and what
    // is names exactly one account, and sending the whole list would be a replacement.
    const added = ids.find((id) => !memberIds.includes(id));
    const removed = memberIds.find((id) => !ids.includes(id));
    if (added) void attempt(() => items.addMember(item.id, added, crypto.randomUUID()));
    else if (removed) void attempt(() => items.removeMember(item.id, removed));
  }

  async function runAutoAssign() {
    isAutoAssigning = true;
    outcome = undefined;
    await attempt(async () => {
      outcome = await items.autoAssign(item.id, crypto.randomUUID());
    });
    isAutoAssigning = false;
  }
</script>

<Stack gap="200">
  <CapabilityGate
    status={assignment.status}
    reason={assignment.status === 'refused' ? t(assignment.code, assignment.params) : undefined}
    pendingLabel={t('app.people.deciding')}
  >
    <AssigneeControl
      label={t('app.people.assignee')}
      {candidates}
      selection="single"
      selected={item.assignee_id ? [item.assignee_id] : []}
      filterLabel={t('app.people.filter')}
      emptyLabel={t('app.people.no_candidates')}
      noMatchLabel={t('app.people.no_match')}
      chosenLabel={t('app.people.assigned_to')}
      unassignedLabel={t('app.people.unassigned')}
      onSelect={setAssignee}
    />

    <div class="auto">
      <Button
        size="sm"
        tone="secondary"
        isBusy={isAutoAssigning}
        busyLabel={t('app.people.auto_assigning')}
        onclick={runAutoAssign}
      >
        {t('app.people.auto_assign')}
      </Button>
      {#if outcome}
        <!-- A result, not a failure. "Nobody was eligible" is the policy having run and found no
             one, and rendering it as an error would say something broke. -->
        <p class="outcome">
          {outcome.assigned
            ? t('app.people.auto_assigned', { strategy: outcome.strategy })
            : t(outcome.code ?? 'app.people.auto_assign_none', { strategy: outcome.strategy })}
        </p>
      {/if}
    </div>
  </CapabilityGate>

  <CapabilityGate
    status={membership.status}
    reason={membership.status === 'refused' ? t(membership.code, membership.params) : undefined}
    pendingLabel={t('app.people.deciding')}
  >
    <AssigneeControl
      label={t('app.people.members')}
      {candidates}
      selection="multiple"
      selected={memberIds}
      filterLabel={t('app.people.filter')}
      emptyLabel={t('app.people.no_candidates')}
      noMatchLabel={t('app.people.no_match')}
      chosenLabel={t('app.people.members')}
      unassignedLabel={t('app.people.no_members')}
      onSelect={setMembers}
    />
  </CapabilityGate>

  {#if failure}
    <p class="failure">{failure}</p>
  {/if}
</Stack>

<style>
  .auto { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .outcome { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
