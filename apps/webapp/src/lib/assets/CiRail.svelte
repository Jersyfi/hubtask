<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An object's property rail (ET-1, IN-2, scene `ci`), the entry's rail for the inventory:
  // *Responsibility* - person, group, location, next review; *Protection need* per dimension with
  // its own value or whom it inherits from, and the reason a fixed one gives; *Origin* - the
  // sources, when it was last seen, how many; *Groups* - the saved groups holding it, each opening
  // the stock with its line. A writer changes the first two in a dialog, through the same compliance
  // write the page always used; nobody else sees a way to (P-05). On a phone the rail is a sheet
  // behind three facts.
  import { Checkbox, Dialog, Input, Property, PropertyRail, Select, Stack, Textarea, type RailSection, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, CiCompliance, CiGroup, CiImpact, ConfigurationItem } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { accounts } from '../data/accounts.svelte.ts';
  import { cmdb, DIMENSIONS, PROTECTION_LEVELS, protectionRows, type Dimension, type Level } from '../data/cmdb.svelte.ts';
  import { groups as peopleGroups } from '../data/groups.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDueShort, formatPlainDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { inheritedFrom } from './objectpage.ts';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    known: ReadonlyMap<string, ConfigurationItem>;
    hubCis: readonly ConfigurationItem[];
    /** What suffers when it fails: whom a raised need came from. */
    downstream: CiImpact | undefined;
    /** The saved groups holding it. */
    holding: readonly CiGroup[];
    mayWrite: boolean;
  }

  const { ci, classes, known, hubCis, downstream, holding, mayWrite }: Props = $props();

  const nameOf = (id: string) => known.get(id)?.name ?? t('app.assets.unknown_ci');
  const day = (date: string | null | undefined) => (date ? formatPlainDay(date, messages.locale) : undefined);
  const canWrite = $derived(mayWrite && !ci.merged_into);

  // Responsibility, the stage aside, in one dialog; status and quantity ride with it as before.
  const groupOptions = $derived<SelectOption[]>(peopleGroups.all.map((each) => ({ value: each.id, label: each.name })));
  const accountOptions = $derived<SelectOption[]>(
    people.candidates({ hubId: ci.hub_id }).map((id) => ({ value: id, label: accounts.nameOf(id) ?? id })),
  );
  const locations = $derived<SelectOption[]>(
    hubCis
      .filter((each) => classes.find((one) => one.key === each.class)?.lineage.includes('location'))
      .map((each) => ({ value: each.id, label: each.name })),
  );

  let isOwnerOpen = $state(false);
  let isProtectionOpen = $state(false);
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);
  let status = $state<NonNullable<CiCompliance['status']>>('OK');
  let ownerAccount = $state('');
  let ownerGroup = $state('');
  let location = $state('');
  let reviewDue = $state('');
  let quantity = $state('1');
  let levels = $state<Record<Dimension, string>>({ c: '', i: '', a: '' });
  let fixed = $state<Record<Dimension, boolean>>({ c: false, i: false, a: false });
  let reason = $state('');
  const anyFixed = $derived(DIMENSIONS.some((each) => fixed[each]));

  function openOwner() {
    status = ci.status === 'MERGED' ? 'OK' : ci.status;
    ownerAccount = ci.owner_account_id ?? '';
    ownerGroup = ci.owner_group_id ?? '';
    location = ci.location_id ?? '';
    reviewDue = ci.review_due ?? '';
    quantity = String(ci.quantity);
    failure = undefined;
    isOwnerOpen = true;
  }
  function openProtection() {
    levels = { c: ci.protection.c ?? '', i: ci.protection.i ?? '', a: ci.protection.a ?? '' };
    fixed = { c: false, i: false, a: false };
    for (const each of ci.protection.fixed ?? []) fixed[each] = true;
    reason = ci.protection.reason ?? '';
    failure = undefined;
    isProtectionOpen = true;
  }

  async function save(body: CiCompliance) {
    isSaving = true;
    try {
      await cmdb.setCompliance(ci, body);
      toast.show({ text: t('app.assets.saved'), tone: 'success' });
      isOwnerOpen = false;
      isProtectionOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
  const saveOwner = () => save({
    status,
    owner_account_id: ownerAccount || null,
    owner_group_id: ownerGroup || null,
    location_id: location || null,
    review_due: reviewDue || null,
    quantity: Math.max(1, Number.parseInt(quantity, 10) || 1),
  });
  const level = (dimension: Dimension) => (levels[dimension] ? (levels[dimension] as Level) : null);
  const saveProtection = () => save({
    protection: { c: level('c'), i: level('i'), a: level('a'), fixed: DIMENSIONS.filter((each) => fixed[each]), ...(anyFixed ? { reason: reason.trim() } : {}) },
  });
  const levelOptions = $derived<SelectOption[]>([{ value: '', label: t('app.assets.level.none') }, ...PROTECTION_LEVELS.map((each) => ({ value: each, label: t(`app.assets.level.${each}`) }))]);

  // The need per dimension: the effective level, and where it comes from.
  const protection = $derived(protectionRows(ci.protection, ci.effective_protection));
  function noteOf(row: (typeof protection)[number]): string | undefined {
    if (row.raised) {
      const from = inheritedFrom(row.dimension, row.effective, downstream?.reached ?? [], known);
      return from ? t('app.object.inherited_from', { name: nameOf(from) }) : t('app.object.inherited');
    }
    if (!row.own) return undefined;
    return row.fixed && ci.protection.reason ? t('app.object.own_reason', { reason: ci.protection.reason }) : t('app.object.own');
  }

  // Where its values came from: every source that wrote an attribute or a relation.
  const sources = $derived([...new Set([...Object.values(ci.provenance).map((each) => each.source), ...(ci.relations ?? []).map((each) => each.data_source)])]);
  const sourceWord = (source: string) => (source === 'manual' ? t('app.object.origin_manual') : source);

  const owner = $derived(ci.owner_account_id ? (accounts.nameOf(ci.owner_account_id) ?? t('app.people.unnamed')) : undefined);
  const group = $derived(ci.owner_group_id ? (peopleGroups.nameOf(ci.owner_group_id) ?? undefined) : undefined);

  const sections = $derived<RailSection[]>([
    { id: 'owner', title: t('app.object.rail_owner') },
    { id: 'protection', title: t('app.assets.protection') },
    { id: 'origin', title: t('app.object.rail_origin') },
    { id: 'groups', title: t('app.object.rail_groups'), count: String(holding.length), isFolded: holding.length === 0 },
  ]);
</script>

<div class="rail" data-ci-rail>
  <PropertyRail
    label={t('app.object.rail', { name: ci.name })}
    {sections}
    {section}
    {summary}
    foldLabel={(title) => t('app.entry.fold', { title })}
    unfoldLabel={(title) => t('app.entry.unfold', { title })}
    allLabel={t('app.entry.all_properties', { count: String(sections.length) })}
    sheetTitle={t('app.entry.all_properties_title')}
    closeLabel={t('app.workflows.close')}
  />
</div>

{#snippet summary()}
  <Property label={t('app.object.person')} value={owner} emptyLabel={t('app.object.nobody')} />
  <Property label={t('app.assets.location')} value={ci.location_id ? nameOf(ci.location_id) : undefined} emptyLabel="–" />
  <Property label={t('app.assets.review_due')} value={day(ci.review_due)} emptyLabel="–" />
{/snippet}

{#snippet section(item: RailSection)}
  {#if item.id === 'owner'}
    <div class="rows" data-rail-owner>
      <!-- A choice drawn as a button holds no other control: with nobody named, *set* is the way in. -->
      <Property label={t('app.object.person')} onedit={canWrite && owner ? openOwner : undefined} editLabel={t('app.object.edit_owner')}>
        <span class="line">{#if owner}{owner}{:else}<span class="nobody">{t('app.object.nobody')}</span>{#if canWrite}{' · '}<button type="button" class="set" onclick={openOwner}>{t('app.object.set_owner')}</button>{/if}{/if}</span>
      </Property>
      <Property label={t('app.object.group')} value={group} emptyLabel="–" onedit={canWrite ? openOwner : undefined} editLabel={t('app.object.edit_owner')} />
      {#if canWrite}
        <Property label={t('app.assets.location')} value={ci.location_id ? nameOf(ci.location_id) : undefined} emptyLabel="–" onedit={openOwner} editLabel={t('app.object.edit_owner')} />
      {:else}
        <Property label={t('app.assets.location')}>{#if ci.location_id}<a href={`/cis/${ci.location_id}`}>{nameOf(ci.location_id)}</a>{:else}–{/if}</Property>
      {/if}
      <Property label={t('app.assets.review_due')} value={day(ci.review_due)} emptyLabel="–" onedit={canWrite ? openOwner : undefined} editLabel={t('app.object.edit_owner')} />
    </div>
  {:else if item.id === 'protection'}
    <div class="rows" data-rail-protection>
      {#each protection as row (row.dimension)}
        <Property label={t(`app.assets.dimension.${row.dimension}`)} value={t(`app.assets.level.${row.effective ?? 'none'}`)} note={noteOf(row)}
          onedit={canWrite ? openProtection : undefined} editLabel={t('app.object.edit_protection')} />
      {/each}
    </div>
  {:else if item.id === 'origin'}
    <div class="rows" data-rail-origin>
      <Property label={t('app.object.sources')} value={sources.length > 0 ? sources.map(sourceWord).join(', ') : undefined} emptyLabel="–" />
      <Property label={t('app.object.last_seen')} value={ci.last_seen_at ? formatDueShort(ci.last_seen_at, messages.locale, actor.zone) : undefined} emptyLabel={t('app.object.never')} />
      <Property label={t('app.assets.quantity')} value={String(ci.quantity)} />
    </div>
  {:else if item.id === 'groups'}
    {#if holding.length === 0}
      <p class="none">{t('app.object.groups_none')}</p>
    {:else}
      <ul class="groups" data-rail-groups>
        {#each holding as each (each.id)}<li><a href={`/hubs/${ci.hub_id}/inventory?group=${encodeURIComponent(each.id)}`}>{each.name}</a></li>{/each}
      </ul>
    {/if}
  {/if}
{/snippet}

{#if isOwnerOpen}
  <Dialog size="m" bind:isOpen={isOwnerOpen} title={t('app.object.edit_owner')} dismissLabel={t('app.assets.cancel')}
    primary={{ label: t('app.assets.save'), isBusy: isSaving, onclick: () => void saveOwner() }}>
    <Stack gap="150">
      <Select label={t('app.assets.owner_person')} options={[{ value: '', label: t('app.assets.nobody') }, ...accountOptions]} bind:value={ownerAccount} />
      <Select label={t('app.assets.owner_group')} options={[{ value: '', label: t('app.assets.nobody') }, ...groupOptions]} bind:value={ownerGroup} />
      <Select label={t('app.assets.location')} options={[{ value: '', label: t('app.assets.no_location') }, ...locations]} bind:value={location} />
      <Input label={t('app.assets.review_due')} type="date" bind:value={reviewDue} />
      <Select label={t('app.assets.status')} options={(['OK', 'PENDING', 'BLOCKED'] as const).map((each) => ({ value: each, label: t(`app.assets.status.${each}`) }))} bind:value={status} />
      <Input label={t('app.assets.quantity')} inputmode="numeric" bind:value={quantity} />
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  </Dialog>
{/if}

{#if isProtectionOpen}
  <Dialog size="m" bind:isOpen={isProtectionOpen} title={t('app.object.edit_protection')} dismissLabel={t('app.assets.cancel')}
    primary={{ label: t('app.assets.save'), isBusy: isSaving, disabledReason: anyFixed && !reason.trim() ? t('app.assets.fix_reason_required') : undefined, onclick: () => void saveProtection() }}>
    <Stack gap="150">
      {#each DIMENSIONS as dimension (dimension)}
        <div class="dimension">
          <Select label={t(`app.assets.dimension.${dimension}`)} options={levelOptions} bind:value={levels[dimension]} />
          <Checkbox label={t('app.assets.fixed_dimensions')} bind:checked={fixed[dimension]} />
        </div>
      {/each}
      {#if anyFixed}
        <Textarea label={t('app.assets.fix_reason')} bind:value={reason} maxlength={2000} isRequired />
      {/if}
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  </Dialog>
{/if}

<style>
  .rail { min-inline-size: 0; }
  .rows { display: flex; flex-direction: column; }
  .line { display: inline; }
  .nobody { color: var(--text-warning); }
  .set {
    padding: 0; border: 0; background: none; color: var(--text-brand); font: inherit; font-weight: var(--fw-semibold); cursor: pointer;
  }
  .set:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-sm); }
  .none { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .groups { margin: 0; padding: 0; list-style: none; display: flex; flex-direction: column; gap: var(--sp-050); }
  .groups a { color: var(--text-primary); }
  .rows :global(a) { color: var(--text-primary); }
  .dimension { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
</style>
