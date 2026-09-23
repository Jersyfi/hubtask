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
  // **And two named parts, because that is two questions** (ADR-0063 decision 10). The panel drew
  // the two pickers with nothing between them, so the difference the model makes was invisible:
  // the reader saw one list of people and then another. Each part now carries its name and the one
  // sentence that distinguishes it from the other — who the entry is *theirs*, and who is along.
  //
  // **The manifest decides which of them exist.** An activity carries an assignee and no member
  // list, and this component learns that from `/meta/capabilities` rather than from a list of
  // types written here. A type that carries neither shows the reason, because a control that is
  // missing tells the reader nothing.
  //
  // **The picker is a courtesy.** It offers the accounts that hold a membership along the path;
  // the server refuses one that cannot see the entry, and that refusal reaches the reader as a
  // sentence rather than being pre-empted badly.
  //
  // **Auto-assign exists where a policy does, and nowhere else.** A disabled button carrying its
  // reason was the shape issue 917 was closed in; decision 10 replaces it, because the reason is
  // not a refusal of something this reader may do - it is a setting that has not been made. So the
  // part says where a policy is set and leads there, and a reader who holds `STRUCTURE` on the
  // collection gets the link; a reader who does not gets the sentence, which is what tells them
  // whom to ask.

  import { untrack } from 'svelte';

  import { AssigneeControl, Button, CapabilityGate, Stack } from '@hubtask/design-system/components';
  import type { AutoAssignOutcome, WorkItem } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { holds, supports } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { people, type Path } from '../data/people.svelte.ts';
  import { byName } from '../i18n/collation.ts';
  import { announcer } from '../announce.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const { item, path }: { item: WorkItem; path: Path } = $props();

  const assignment = $derived(supports(item.type, 'ASSIGNMENT'));
  const membership = $derived(supports(item.type, 'MEMBERS'));

  /**
   * Whether anything would choose, if asked.
   *
   * The answer is on the collection the client already holds - `policies.auto_assign`, with its
   * `enabled` - rather than a round trip that ends in "nothing happened" (issue 917).
   */
  $effect(() => {
    const wanted = item.collection_id;
    if (!wanted) return;
    return untrack(() => containers.openSingle(wanted));
  });
  const policy = $derived(containers.find(item.collection_id)?.policies?.auto_assign ?? undefined);
  const chooses = $derived(policy !== undefined && policy.enabled !== false);

  /**
   * Whether this reader may set one, from any membership they hold along the path.
   *
   * Any, not the first: `along` composes from the workspace downwards, and somebody who is a
   * member of the workspace and an owner of this collection holds `STRUCTURE` here. Setting a
   * policy is `STRUCTURE` on the server (`UpdateContainerPolicies`), so that is what is asked.
   */
  const maySetPolicy = $derived(
    people
      .along(path)
      .filter((membership) => membership.account_id === actor.account?.id)
      .some((membership) => holds(membership.role as string, 'STRUCTURE').status === 'permitted'),
  );

  /** Where a policy is set: the collection's own screen, which carries it in its menu. */
  const collectionHref = $derived(item.collection_id ? `/collections/${item.collection_id}` : undefined);

  const candidateIds = $derived(people.candidates(path));
  // Gathered from four memberships, so the list has no order until this gives it one - the
  // reader's, under the collator for the resolved locale (F5-09, i18n-l10n.md §6 line 3).
  const candidates = $derived(
    candidateIds
      .map((id) => ({ id, name: accounts.nameOf(id) ?? t('app.people.unnamed') }))
      .sort((left, right) => byName(messages.locale)(left.name, right.name)),
  );

  const memberIds = $derived((item.member_ids ?? []) as readonly string[]);

  let failure = $state<string | undefined>(undefined);
  let outcome = $state<AutoAssignOutcome | undefined>(undefined);
  let isAutoAssigning = $state(false);

  /** One place where a write becomes a sentence, so no branch below reads a code itself. */
  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      // The same cast every write in this application makes: the engine promises a
      // `TransportError` and a `catch` binding is `unknown` whatever it promises.
      failure = renderProblem(error as never, messages).message;
    }
  }

  function setAssignee(ids: readonly string[]) {
    const next = ids[0];
    void attempt(
      () =>
        next === undefined
          ? items.unassign(item.id, item.version, crypto.randomUUID())
          : items.assign(item.id, next, item.version, crypto.randomUUID()),
      next === undefined ? t('app.people.unassigned_announced') : t('app.people.assigned_announced'),
    );
  }

  function setMembers(ids: readonly string[]) {
    // One call per change, because the set is an OR-set: the difference between what was and what
    // is names exactly one account, and sending the whole list would be a replacement.
    const added = ids.find((id) => !memberIds.includes(id));
    const removed = memberIds.find((id) => !ids.includes(id));
    if (added) void attempt(() => items.addMember(item.id, added, crypto.randomUUID()), t('app.people.member_added_announced'));
    else if (removed) void attempt(() => items.removeMember(item.id, removed), t('app.people.member_removed_announced'));
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

<Stack gap="250">
  <!-- Responsible: one person, the entry is theirs. -->
  <section class="part">
    <h3 class="name">{t('app.people.responsible')}</h3>
    <p class="says">{t('app.people.responsible_hint')}</p>
    <CapabilityGate
      status={assignment.status}
      reason={assignment.status === 'refused' ? t(assignment.code, assignment.params) : undefined}
      pendingLabel={t('app.people.deciding')}
    >
      <AssigneeControl
        label={t('app.people.responsible')}
        {candidates}
        selection="single"
        selected={item.assignee_id ? [item.assignee_id] : []}
        filterLabel={t('app.people.filter')}
        locale={messages.locale}
        emptyLabel={t('app.people.no_candidates')}
        noMatchLabel={t('app.people.no_match')}
        chosenLabel={t('app.people.assigned_to')}
        unassignedLabel={t('app.people.unassigned')}
        onSelect={setAssignee}
      />

      {#if chooses}
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
            <!-- A result, not a failure. "Nobody was eligible" is the policy having run and found
                 no one, and rendering it as an error would say something broke. -->
            <p class="outcome">
              {outcome.assigned
                ? t('app.people.auto_assigned', { strategy: outcome.strategy })
                : t(outcome.code ?? 'app.people.auto_assign_none', { strategy: outcome.strategy })}
            </p>
          {/if}
        </div>
      {:else}
        <!-- No button, because there is nothing to press: what is missing is a setting, not a
             permission (decision 10). The sentence says where it is made, and the way there is
             offered to a reader who may make it. -->
        <p class="says">
          {t('app.people.auto_assign_unset')}
          {#if maySetPolicy && collectionHref}
            <a href={collectionHref}>{t('app.people.auto_assign_open')}</a>
          {/if}
        </p>
      {/if}
    </CapabilityGate>
  </section>

  <!-- Also on it: several people, who follow the entry rather than own it. -->
  <section class="part">
    <h3 class="name">{t('app.people.also_on_it')}</h3>
    <p class="says">{t('app.people.also_on_it_hint')}</p>
    <CapabilityGate
      status={membership.status}
      reason={membership.status === 'refused' ? t(membership.code, membership.params) : undefined}
      pendingLabel={t('app.people.deciding')}
    >
      <AssigneeControl
        label={t('app.people.also_on_it')}
        {candidates}
        selection="multiple"
        selected={memberIds}
        filterLabel={t('app.people.filter')}
        locale={messages.locale}
        emptyLabel={t('app.people.no_candidates')}
        noMatchLabel={t('app.people.no_match')}
        chosenLabel={t('app.people.members')}
        unassignedLabel={t('app.people.no_members')}
        onSelect={setMembers}
      />
    </CapabilityGate>
  </section>

  {#if failure}
    <p class="failure" role="alert">{failure}</p>
  {/if}
</Stack>

<style>
  .part { display: flex; flex-direction: column; gap: var(--sp-100); }

  /* The part's name. The `label` row of design-system.md §3, which is what every other field
     name in this panel's neighbours is drawn as. */
  .name {
    margin: 0;
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    color: var(--text-subtle);
  }

  /* The sentence that distinguishes one part from the other. */
  .says { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); max-width: 48ch; }

  .auto { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .outcome { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
