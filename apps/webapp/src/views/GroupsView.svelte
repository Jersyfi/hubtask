<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workspace's groups: a name, the people in it, and what happens when it goes.
  //
  // **A group is a way to grant a role once instead of eleven times.** That is also why deleting
  // one is not a tidy-up: what the group granted, nobody holds afterwards. The screen says that
  // before it offers the button rather than in the dialog after it — somebody deciding whether to
  // press is the person who needs to know.
  //
  // **Members are set whole.** `PATCH /groups/{id}` takes "the complete membership after the
  // change", so adding one person means sending the list with them in it. The screen composes it;
  // a caller that sent one identifier would empty the group.
  //
  // **Any member may read this.** The contract's reasoning, not a relaxation: a membership granted
  // to a group is unreadable until the group can be shown as the people it reaches. What it
  // discloses is a name and identifiers that resolve through the same minimal read as everywhere
  // else.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, Select, Spinner, Stack } from '@hubtask/design-system/components';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const TENANT = { scopeType: 'TENANT' } as const;

  let newName = $state('');
  let renaming = $state<string | undefined>(undefined);
  let renamed = $state('');
  let removing = $state<string | undefined>(undefined);
  let adding = $state<Record<string, string>>({});
  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);

  $effect(() =>
    untrack(() => {
      // The workspace's memberships are what says who could join a group at all.
      people.openScope(TENANT);
      return groups.open();
    }),
  );

  const reading = $derived(groups.state);
  const all = $derived(groups.all);

  /** Everybody this workspace knows about, for the "add somebody" chooser. */
  const candidates = $derived(people.candidates({}));

  $effect(() => {
    // One read per group, for the members it holds. Asked for here rather than in the store,
    // because it is this screen that wants them listed.
    for (const group of all) if (!groups.detail(group.id)) void groups.read(group.id);
  });

  function membersOf(groupId: string): readonly string[] {
    return groups.detail(groupId)?.members ?? [];
  }

  function nameFor(accountId: string): string {
    return accounts.nameOf(accountId) ?? t('app.people.unnamed');
  }

  function choicesFor(groupId: string) {
    const held = new Set(membersOf(groupId));
    return candidates
      .filter((id) => !held.has(id))
      .map((id) => ({ value: id, label: nameFor(id) }));
  }

  async function attempt(work: () => Promise<unknown>): Promise<void> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const name = newName.trim();
    if (!name) return;
    await attempt(() => groups.create(name));
    newName = '';
  }

  async function rename(groupId: string): Promise<void> {
    const name = renamed.trim();
    if (!name) return;
    await attempt(() => groups.update(groupId, { name }));
    renaming = undefined;
    renamed = '';
  }

  /** Adding and removing are the same call: the list as it should be afterwards. */
  async function setMembers(groupId: string, members: readonly string[]): Promise<void> {
    await attempt(() => groups.update(groupId, { members }));
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.groups.title')}</h1>
    <p class="quiet">{t('app.groups.intro')}</p>

    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting"><Spinner label={t('app.groups.reading')} /> <span>{t('app.groups.reading')}</span></p>
    {:else}
      {#each all as group (group.id)}
        <section class="group">
          <Stack gap="150">
            <div class="head">
              {#if renaming === group.id}
                <Input label={t('app.groups.name')} bind:value={renamed} isRequired />
                <Button
                  tone="primary"
                  isBusy={isWriting}
                  busyLabel={t('app.groups.working')}
                  onclick={() => void rename(group.id)}
                >
                  {t('app.groups.save_name')}
                </Button>
                <Button tone="subtle" onclick={() => (renaming = undefined)}>{t('app.groups.cancel')}</Button>
              {:else}
                <h2 class="name">{group.name}</h2>
                <Button
                  size="sm"
                  tone="subtle"
                  onclick={() => {
                    renaming = group.id;
                    renamed = group.name;
                  }}
                >
                  {t('app.groups.rename')}
                </Button>
              {/if}
            </div>

            {#if group.description}<p class="quiet small">{group.description}</p>{/if}

            <ul class="members">
              {#each membersOf(group.id) as member (member)}
                <li>
                  <span>{nameFor(member)}</span>
                  <Button
                    size="sm"
                    tone="subtle"
                    isBusy={isWriting}
                    busyLabel={t('app.groups.working')}
                    onclick={() =>
                      void setMembers(group.id, membersOf(group.id).filter((id) => id !== member))}
                  >
                    {t('app.groups.remove_member')}
                  </Button>
                </li>
              {:else}
                <li class="quiet small">{t('app.groups.empty')}</li>
              {/each}
            </ul>

            <div class="add">
              <Select
                label={t('app.groups.add_member')}
                bind:value={adding[group.id]}
                placeholder={t('app.people.choose_person')}
                options={choicesFor(group.id)}
              />
              <Button
                isBusy={isWriting}
                busyLabel={t('app.groups.working')}
                onclick={() => {
                  const chosen = adding[group.id];
                  if (!chosen) return;
                  void setMembers(group.id, [...membersOf(group.id), chosen]);
                  adding = { ...adding, [group.id]: '' };
                }}
              >
                {t('app.groups.add')}
              </Button>
            </div>

            {#if removing === group.id}
              <!-- Said before the button, not after it. -->
              <Banner tone="warning">{t('app.groups.delete_cost')}</Banner>
              <div class="row">
                <Button
                  tone="danger"
                  isBusy={isWriting}
                  busyLabel={t('app.groups.working')}
                  onclick={() =>
                    void attempt(async () => {
                      await groups.remove(group.id);
                      removing = undefined;
                    })}
                >
                  {t('app.groups.delete_now')}
                </Button>
                <Button tone="subtle" onclick={() => (removing = undefined)}>{t('app.groups.keep')}</Button>
              </div>
            {:else}
              <div>
                <Button size="sm" tone="secondary" onclick={() => (removing = group.id)}>
                  {t('app.groups.delete')}
                </Button>
              </div>
            {/if}
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.groups.none')}</p>
      {/each}
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.groups.new_title')}</h2>
      <form onsubmit={create}>
        <Stack gap="150">
          <Input label={t('app.groups.name')} hint={t('app.groups.name_hint')} bind:value={newName} isRequired />
          <div>
            <Button type="submit" tone="primary" isBusy={isWriting} busyLabel={t('app.groups.working')}>
              {t('app.groups.create')}
            </Button>
          </div>
        </Stack>
      </form>
    </Stack>
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section,
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .group {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .head { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .members { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }

  .members li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .add { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .row { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  form { margin: 0; }
</style>
