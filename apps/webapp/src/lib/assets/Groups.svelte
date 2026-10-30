<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A hub's CI groups and its service map (UC-AST-11 check 5): each group with its filter, how many
  // CIs it holds now and the service it stands for, one opened with its members; a group defined
  // by its key or changed; and the map of the hub, or of one service, drawn in its layers.
  //
  // **A group is a filter, never a list of CIs** (UC-AST-11, *Where it ends*): its members are what
  // the filter matches when it is read. Defining needs WRITE_ITEMS and is not drawn without it.

  import { untrack } from 'svelte';
  import { Banner, Button, CheckboxGroup, Dialog, IconButton, Input, Select, Spinner, Stack, Textarea, type SelectOption } from '@hubtask/design-system/components';
  import type { CiClass, CiGroup, CiGroupList, CiGroupMembers, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import {
    CI_STAGES, cisPath, classWord, cmdb, groupDefine, groupDraft, groupMembersPath, groupMissing, groupsPath, instantiable, type GroupDraft, type Words,
  } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import ServiceMap from './ServiceMap.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
  }

  const { hubId, classes, words, mayWrite }: Props = $props();

  let groups = $state<readonly CiGroup[] | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let hubCis = $state<readonly ConfigurationItem[]>([]);
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<CiGroupList>({ path: groupsPath(at) }, (state) => {
          if (state.status === 'ready') groups = state.data.data;
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
          if (state.status === 'ready') hubCis = state.data.data;
        }),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  // The services a group may stand for and the map may be narrowed to: the hub's service CIs.
  const isService = (ci: ConfigurationItem) => classes.find((each) => each.key === ci.class)?.lineage.includes('service') ?? false;
  const services = $derived(hubCis.filter((each) => !each.merged_into && isService(each)));
  const serviceOptions = $derived<SelectOption[]>(services.map((each) => ({ value: each.id, label: each.name })));
  const serviceName = (id: string | null) => (id ? (hubCis.find((each) => each.id === id)?.name ?? t('app.assets.unknown_ci')) : undefined);

  let openId = $state<string | undefined>(undefined);
  let members = $state<CiGroupMembers | undefined>(undefined);
  $effect(() => {
    const id = openId;
    members = undefined;
    if (!id) return;
    return untrack(() =>
      engine.subscribe<CiGroupMembers>({ path: groupMembersPath(id) }, (state) => {
        if (state.status === 'ready') members = state.data;
        if (state.status === 'failed') toast.show({ text: renderProblem(state.error, messages).message, tone: 'danger' });
      }),
    );
  });

  const filterText = (group: CiGroup) => {
    const parts: string[] = [];
    if (group.filter.classes?.length) parts.push(group.filter.classes.map((each) => classWord(each, classes, words)).join(', '));
    if (group.filter.stages?.length) parts.push(group.filter.stages.map((each) => t(`app.assets.stage.${each}`)).join(', '));
    if (group.filter.name_has) parts.push(t('app.ci_groups.name_has_value', { text: group.filter.name_has }));
    for (const [key, value] of Object.entries(group.filter.attributes ?? {})) parts.push(`${key} = ${value}`);
    return parts.join(' · ');
  };

  // Defining one.
  let isOpen = $state(false);
  let editing = $state<CiGroup | undefined>(undefined);
  let draft = $state<GroupDraft>(groupDraft());
  let isSaving = $state(false);
  const missing = $derived(groupMissing(draft, !editing));
  const classChecks = $derived(instantiable(classes).map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));
  const stageChecks = $derived(CI_STAGES.map((each) => ({ value: each, label: t(`app.assets.stage.${each}`) })));

  function openDefine(of?: CiGroup) {
    editing = of;
    draft = groupDraft(of);
    isOpen = true;
  }

  async function save() {
    isSaving = true;
    try {
      const saved = await cmdb.defineGroup(hubId, editing?.key ?? draft.key, groupDefine(draft), editing?.version);
      toast.show({ text: t('app.ci_groups.saved'), tone: 'success' });
      isOpen = false;
      openId = saved.id;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isSaving = false;
    }
  }

  let mapService = $state('');
</script>

{#if refusal}
  <Banner tone="danger">{refusal}</Banner>
{:else if !groups}
  <Spinner label={t('app.assets.loading')} />
{:else}
  <Stack gap="200">
    <section class="part" aria-labelledby="groups-heading">
      <h2 id="groups-heading" class="heading">{t('app.ci_groups.title')}</h2>
      <p class="meta">{t('app.ci_groups.hint')}</p>
      {#if mayWrite}<div><Button icon="plus" data-new-group onclick={() => openDefine()}>{t('app.ci_groups.new')}</Button></div>{/if}
      {#if groups.length === 0}
        <p class="meta">{t('app.ci_groups.none')}</p>
      {:else}
        <ul class="rows">
          {#each groups as group (group.id)}
            <li class="row" data-group={group.key}>
              <button type="button" class="pick" aria-expanded={openId === group.id} onclick={() => (openId = openId === group.id ? undefined : group.id)}>
                <strong>{group.name}</strong>
                <span class="meta">
                  {group.key}{#if group.members !== undefined} · {t('app.ci_groups.members_count', { count: group.members })}{/if}
                  {#if group.service_id} · {t('app.ci_groups.stands_for', { name: serviceName(group.service_id) ?? '' })}{/if}
                </span>
                <span class="meta">{filterText(group)}</span>
              </button>
              {#if mayWrite}
                <IconButton icon="pencil" size="sm" tone="subtle" label={t('app.cmdb_admin.edit_named', { name: group.name })} onclick={() => openDefine(group)} />
              {/if}
            </li>
            {#if openId === group.id}
              <li class="members" data-group-members={group.key}>
                {#if !members}
                  <Spinner label={t('app.assets.loading')} />
                {:else if members.data.length === 0}
                  <p class="meta">{t('app.ci_groups.members_none')}</p>
                {:else}
                  <ul class="rows">
                    {#each members.data as member (member.ci_id)}
                      <li class="member">
                        <a href={`/cis/${member.ci_id}`}>{member.name}</a>
                        <span class="meta">{classWord(member.class, classes, words)} · {t(`app.assets.stage.${member.stage}`)}</span>
                      </li>
                    {/each}
                  </ul>
                {/if}
              </li>
            {/if}
          {/each}
        </ul>
      {/if}
    </section>

    <section class="part" aria-labelledby="map-heading">
      <div class="part-head">
        <h2 id="map-heading" class="heading">{t('app.service_map.title')}</h2>
        {#if serviceOptions.length > 0}
          <Select label={t('app.service_map.service')} size="sm" options={[{ value: '', label: t('app.service_map.whole_hub') }, ...serviceOptions]} bind:value={mapService} />
        {/if}
      </div>
      {#key mapService}
        <ServiceMap {hubId} serviceId={mapService || undefined} {classes} {words} />
      {/key}
    </section>
  </Stack>
{/if}

{#if isOpen}
  <Dialog bind:isOpen title={editing ? t('app.cmdb_admin.edit_named', { name: editing.name }) : t('app.ci_groups.new')} dismissLabel={t('app.assets.cancel')} isWide>
    <Stack gap="150">
      {#if !editing}<Input label={t('app.cmdb_admin.key')} hint={t('app.cmdb_admin.key_hint')} bind:value={draft.key} maxlength={63} isRequired />{/if}
      <Input label={t('app.ci_groups.name')} bind:value={draft.name} maxlength={200} isRequired />
      <CheckboxGroup label={t('app.ci_groups.classes')} hint={t('app.ci_groups.classes_hint')} options={classChecks} selected={draft.classes} onChange={(next) => (draft.classes = [...next])} />
      <CheckboxGroup label={t('app.ci_groups.stages')} options={stageChecks} selected={draft.stages} onChange={(next) => (draft.stages = [...next] as GroupDraft['stages'])} />
      <Input label={t('app.ci_groups.name_has')} bind:value={draft.nameHas} maxlength={200} />
      <Textarea label={t('app.ci_groups.conditions')} hint={t('app.ci_groups.conditions_hint')} rows={3} bind:value={draft.conditions} />
      <Select label={t('app.ci_groups.service')} options={[{ value: '', label: t('app.ci_groups.no_service') }, ...serviceOptions]} bind:value={draft.service} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.assets.cancel')}</Button>
      <Button tone="primary" isBusy={isSaving} disabledReason={missing[0] ? t(missing[0]) : undefined} onclick={() => void save()}>{t('app.assets.save')}</Button>
    {/snippet}
  </Dialog>
{/if}

<style>
  .part { display: grid; gap: var(--sp-100); }
  .part-head { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: center; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .pick { display: grid; gap: var(--sp-025); padding: 0; border: none; background: none; font: inherit; color: var(--text-primary); text-align: start; cursor: pointer; min-inline-size: 0; }
  .pick:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .members { padding-inline-start: var(--sp-200); }
  .member { display: flex; flex-wrap: wrap; align-items: baseline; gap: var(--sp-100); }
  .member a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
