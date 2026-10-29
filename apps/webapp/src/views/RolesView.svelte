<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The shipped roles and the workspace's own (UC-ID-26 check 6), drawn as *People*'s *Roles* tab.
  //
  // **Only what the reader holds is offered.** The server refuses a role carrying a permission its
  // definer lacks (`access.permission_above_own`), so the editor offers what the reader's own
  // explanation at the workspace says they hold. A permission the role already carries beyond that
  // is shown ticked and locked, with its reason, and kept on save: the server refuses removing it
  // as it refuses adding it.
  //
  // **Deleting a role takes its grants with it.** Said before the button, as on the groups tab.

  import { tick, untrack } from 'svelte';

  import { Banner, Button, CheckboxGroup, Input, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import type { AccessExplanation, CustomRole } from '@hubtask/sync-engine';

  import { announcer } from '../lib/announce.svelte.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { customFields } from '../lib/data/customfields.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { chosen, grantsOf, keyFrom, offers, ROLE_KEY, roles, type Offer } from '../lib/data/roles.svelte.ts';
  import { workflows } from '../lib/data/workflows.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { permissionLabel, roleLabel } from '../lib/people/rolewords.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';

  const TENANT = { scopeType: 'TENANT' } as const;
  const WORKSPACE_PLACE = '';

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), groups.open(), people.openScope(TENANT), containers.start(), customFields.open(), workflows.open()];
      return () => closers.forEach((close) => close());
    }),
  );

  /** What the reader holds at the workspace: what the editor may offer. */
  let mine = $state<AccessExplanation | undefined>(undefined);
  $effect(() => {
    untrack(() => {
      void roles.explain(TENANT).then((answer) => (mine = answer)).catch(() => (mine = undefined));
    });
  });

  const catalogue = $derived(roles.catalogue);
  const custom = $derived(catalogue?.custom ?? []);

  /** The objects a field or step permission may name. */
  const objects = $derived({
    restrictedFields: customFields.of().filter((field) => field.restricted === true).map((field) => field.key),
    hiddenFields: customFields.of().filter((field) => field.read_restricted === true).map((field) => field.key),
    steps: (workflows.list?.data ?? []).flatMap((workflow) =>
      (workflow.published.steps ?? []).map((step) => ({ workflow: workflow.key, step: step.key })),
    ),
  });

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let missing = $state<Missing>({});

  /** The role being edited: `new` for a new one, a key for one of the workspace's own. */
  let editing = $state<string | undefined>(undefined);
  let name = $state('');
  let key = $state('');
  let isKeyTouched = $state(false);
  let checked = $state<readonly string[]>([]);
  let removing = $state<string | undefined>(undefined);

  const carried = $derived(editing && editing !== 'new' ? (custom.find((role) => role.key === editing)?.permissions ?? []) : []);
  const offered = $derived<Offer[]>(catalogue ? offers(catalogue.catalogue, objects, mine, carried) : []);

  /** The editor's boxes, by the area each permission belongs to. */
  const areas = $derived.by(() => {
    const out: { group: string; offers: Offer[] }[] = [];
    for (const offer of offered) {
      const area = out.find((each) => each.group === offer.group);
      if (area) area.offers.push(offer);
      else out.push({ group: offer.group, offers: [offer] });
    }
    return out;
  });

  function areaLabel(group: string): string {
    const known = ['content', 'how_we_work', 'people', 'evidence', 'portal', 'fields', 'workflows'];
    return known.includes(group) ? t(`app.roles.area.${group}`) : group;
  }

  function startNew(): void {
    editing = 'new';
    name = '';
    key = '';
    isKeyTouched = false;
    checked = [];
    missing = {};
  }

  function startChange(role: CustomRole): void {
    editing = role.key;
    name = role.name;
    key = role.key;
    checked = [...role.permissions];
    missing = {};
  }

  $effect(() => {
    if (editing === 'new' && !isKeyTouched) key = keyFrom(name);
  });

  async function attempt(work: () => Promise<unknown>, said: string): Promise<boolean> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
      announcer.say(said);
      return true;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isWriting = false;
    }
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const permissions = chosen(offered, new Set(checked), carried);
    missing = check([
      { kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name.trim() !== '' },
      { kind: 'field', key: 'key', code: 'app.roles.key_invalid', isFilled: ROLE_KEY.test(key) },
      { kind: 'field', key: 'permissions', code: 'app.roles.permissions_empty', isFilled: permissions.length > 0 },
    ]);
    if (!isComplete(missing)) {
      await tick();
      focusFirst(document);
      return;
    }
    const role = custom.find((each) => each.key === editing);
    const done = role
      ? await attempt(() => roles.change(role, name.trim(), permissions), t('app.roles.changed_announced'))
      : await attempt(() => roles.create(key, name.trim(), permissions), t('app.roles.created_announced'));
    if (done) editing = undefined;
  }

  // Giving a role of the workspace's own to a person or a group, at the workspace or a hub.
  let grantingRole = $state<string | undefined>(undefined);
  let grantSubject = $state('');
  let grantPlace = $state(WORKSPACE_PLACE);

  const subjects = $derived([
    ...people.candidates({}).map((id) => ({ value: `account:${id}`, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
    ...groups.all.map((group) => ({ value: `group:${group.id}`, label: t('app.people.group_named', { name: group.name }) })),
  ]);
  const places = $derived([
    { value: WORKSPACE_PLACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.map((hub) => ({ value: hub.id, label: hub.name })),
  ]);

  async function grant(roleKey: string): Promise<void> {
    missing = check([{ kind: 'field', key: 'subject', code: 'app.forms.choose', isFilled: grantSubject !== '' }]);
    if (!isComplete(missing)) return;
    const [kind, id] = grantSubject.split(':');
    const done = await attempt(
      () =>
        roles.grant({
          role: roleKey,
          scope_type: grantPlace === WORKSPACE_PLACE ? 'TENANT' : 'HUB',
          ...(grantPlace === WORKSPACE_PLACE ? {} : { scope_id: grantPlace }),
          ...(kind === 'group' ? { group_id: id } : { account_id: id }),
        }),
      t('app.roles.granted_announced'),
    );
    if (done) {
      grantingRole = undefined;
      grantSubject = '';
      grantPlace = WORKSPACE_PLACE;
    }
  }

  function holderName(grant: { account_id?: string; group_id?: string }): string {
    if (grant.group_id) return t('app.people.group_named', { name: groups.nameOf(grant.group_id) ?? t('app.people.unnamed_group') });
    return accounts.nameOf(grant.account_id ?? '') ?? t('app.people.unnamed');
  }

  function placeName(grant: { scope_type: string; scope_id?: string }): string {
    if (grant.scope_type === 'TENANT' || !grant.scope_id) return t('app.people.managed_where_workspace');
    return containers.find(grant.scope_id)?.name ?? t('app.roles.place_unknown');
  }
</script>

<Stack gap="300">
  <p class="quiet">{t('app.roles.intro')}</p>
  <div class="file"><ViewSectionsAsFile sections={['roles']} /></div>

  {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

  {#if !catalogue}
    {#if roles.state?.status === 'failed'}
      <Banner tone="warning">{renderProblem(roles.state.error as never, messages).message}</Banner>
    {:else}
      <p class="waiting"><Spinner label={t('app.roles.reading')} /> <span>{t('app.roles.reading')}</span></p>
    {/if}
  {:else}
    <section>
      <Stack gap="150">
        <h2 class="section">{t('app.roles.shipped_title')}</h2>
        <p class="quiet small">{t('app.roles.shipped_hint')}</p>
        <ul class="roles">
          {#each catalogue.shipped as role (role.role)}
            <li class="role">
              <h3 class="name">{roleLabel(role.role)}</h3>
              <p class="quiet small">
                {catalogue.catalogue.filter((entry) => entry.shipped.includes(role.role)).map((entry) => permissionLabel(entry.permission)).join(' · ') || t('app.roles.carries_nothing')}
              </p>
            </li>
          {/each}
        </ul>
      </Stack>
    </section>

    <section>
      <Stack gap="150">
        <h2 class="section">{t('app.roles.custom_title')}</h2>
        {#each custom as role (role.key)}
          <article class="role">
            <Stack gap="100">
              <div class="head">
                <h3 class="name">{role.name}</h3>
                <code class="key">{role.key}</code>
              </div>
              <p class="quiet small">{role.permissions.map(permissionLabel).join(' · ')}</p>
              <ul class="grants">
                {#each grantsOf(catalogue, role.key) as held (held.id)}
                  <li>
                    <span>{t('app.roles.held_by', { who: holderName(held), place: placeName(held) })}</span>
                    <WorkspaceGate permission="MANAGE_MEMBERS">
                      {#snippet actions()}
                        <Button size="sm" tone="subtle" isBusy={isWriting} busyLabel={t('app.roles.working')}
                          onclick={() => void attempt(() => roles.revoke(held.id), t('app.roles.revoked_announced'))}>
                          {t('app.roles.revoke')}
                        </Button>
                      {/snippet}
                    </WorkspaceGate>
                  </li>
                {:else}
                  <li class="quiet small">{t('app.roles.held_by_nobody')}</li>
                {/each}
              </ul>
              <WorkspaceGate permission="MANAGE_MEMBERS">
                {#snippet actions()}
                  {#if grantingRole === role.key}
                    <div class="add">
                      <Select label={t('app.people.who')} bind:value={grantSubject} placeholder={t('app.people.choose_person')} options={subjects}
                        error={missing.subject && !grantSubject ? t(missing.subject) : undefined} />
                      <Select label={t('app.roles.where')} bind:value={grantPlace} options={places} />
                      <Button tone="primary" isBusy={isWriting} busyLabel={t('app.roles.working')} onclick={() => void grant(role.key)}>
                        {t('app.roles.grant')}
                      </Button>
                      <Button tone="subtle" onclick={() => (grantingRole = undefined)}>{t('app.roles.cancel')}</Button>
                    </div>
                  {:else if removing === role.key}
                    <Banner tone="warning">{t('app.roles.delete_cost')}</Banner>
                    <div class="row">
                      <Button tone="danger" isBusy={isWriting} busyLabel={t('app.roles.working')}
                        onclick={() => void attempt(() => roles.remove(role.key), t('app.roles.removed_announced')).then(() => (removing = undefined))}>
                        {t('app.roles.delete_now')}
                      </Button>
                      <Button tone="subtle" onclick={() => (removing = undefined)}>{t('app.roles.keep')}</Button>
                    </div>
                  {:else}
                    <div class="row">
                      <Button size="sm" onclick={() => (grantingRole = role.key)}>{t('app.roles.give')}</Button>
                      <Button size="sm" tone="secondary" onclick={() => startChange(role)}>{t('app.roles.change')}</Button>
                      <Button size="sm" tone="secondary" onclick={() => (removing = role.key)}>{t('app.roles.delete')}</Button>
                    </div>
                  {/if}
                {/snippet}
              </WorkspaceGate>
            </Stack>
          </article>
        {:else}
          <p class="quiet">{t('app.roles.none')}</p>
        {/each}
      </Stack>
    </section>

    <WorkspaceGate permission="MANAGE_MEMBERS">
      {#snippet actions()}
        {#if editing}
          <form class="panel" onsubmit={save}>
            <Stack gap="150">
              <h2 class="section">{editing === 'new' ? t('app.roles.new_title') : t('app.roles.change_title', { name })}</h2>
              <Input label={t('app.roles.name')} bind:value={name} isRequired
                error={missing.name && !name.trim() ? t(missing.name) : undefined} />
              {#if editing === 'new'}
                <Input label={t('app.roles.key')} hint={t('app.roles.key_hint')} bind:value={key} isRequired
                  oninput={() => (isKeyTouched = true)}
                  error={missing.key && !ROLE_KEY.test(key) ? t(missing.key) : undefined} />
              {/if}
              <p class="quiet small">{t('app.roles.offered_hint')}</p>
              {#each areas as area (area.group)}
                <CheckboxGroup
                  label={areaLabel(area.group)}
                  options={area.offers.map((offer) => ({
                    value: offer.permission,
                    label: permissionLabel(offer.permission),
                    ...(offer.isHeld ? {} : { disabledReason: t('app.roles.not_yours') }),
                  }))}
                  selected={checked.filter((permission) => area.offers.some((offer) => offer.permission === permission))}
                  onChange={(next) => {
                    const others = checked.filter((permission) => !area.offers.some((offer) => offer.permission === permission));
                    checked = [...others, ...next];
                  }}
                />
              {/each}
              {#if missing.permissions && chosen(offered, new Set(checked), carried).length === 0}
                <p class="error small">{t(missing.permissions)}</p>
              {/if}
              <div class="row">
                <Button type="submit" tone="primary" isBusy={isWriting} busyLabel={t('app.roles.working')}>
                  {editing === 'new' ? t('app.roles.create') : t('app.roles.save')}
                </Button>
                <Button tone="subtle" onclick={() => (editing = undefined)}>{t('app.roles.cancel')}</Button>
              </div>
            </Stack>
          </form>
        {:else}
          <div><Button tone="primary" onclick={startNew}>{t('app.roles.new')}</Button></div>
        {/if}
      {/snippet}
    </WorkspaceGate>
  {/if}
</Stack>

<style>
  .section,
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .name { font-size: var(--fs-200); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .error { margin: 0; color: var(--text-danger); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .roles,
  .grants { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  .grants li { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .role {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .head { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }

  .key { font-size: var(--fs-075); color: var(--text-secondary); }

  .add,
  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  form { margin: 0; max-inline-size: 52ch; }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
  .file { display: flex; justify-content: flex-end; }
</style>
