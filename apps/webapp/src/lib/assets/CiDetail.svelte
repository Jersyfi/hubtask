<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An inventory object's page (UC-AST-01 check 8, UC-AST-05 check 5, UC-AST-09, UC-AST-12; IN-2,
  // IN-3, scene `ci`): the entry's shape for an object - one head over the full width with the
  // stage as a step bar, jump marks with counts, the main column and the property rail. The main
  // column holds what grows: the attributes with the source that wrote each, the relations as a
  // picture or a list, the impact both ways, the work on it and what mentions it, the register
  // record with the device's life, its files and its history (UC-AST-01 checks 12 and 13). The rail holds responsibility, protection need, origin and the
  // saved groups (`CiRail`).
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS there is no edit, no
  // relate, no stage and no *report an incident*; *Confirm* is the owning person's or a writer's. A phone reads (UC-AST-01 check 10): the marks are
  // the switch between the sections, and nothing writes there; the stage and the reports also wait
  // for the network.
  import { placeMark } from '../workspace/PlaceMark.svelte';

  import { untrack } from 'svelte';
  import {
    Banner, Breadcrumb, Button, DetailPage, FoldingSection, Input, JumpMarks, Menu, ObjectHead, Select, Stack, StateLozenge, StepBar, type SelectOption,
  } from '@hubtask/design-system/components';
  import type { CiAsset, CiClass, MediaPage, CiGroup, CiGroupList, CiImpact, CiRelationType, CiWork, ConfigurationItem, ConfigurationItemList, HubQueueList } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import {
    attributeText, changedAttributes, ciAssetPath, ciReferenceText, cisPath, classWord, classesWord, cmdb, groupsPath, objectWorkPath, refinements, type Words,
  } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { withKey } from '../data/inventoryquery.ts';
  import { live } from '../data/live.svelte.ts';
  import { modules, modulesHere } from '../data/modules.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { resource, resourceWhen } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { formatDateTime, formatPlainDay } from '../i18n/datetime.ts';
  import { todayIn } from '../i18n/zone.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import AttributeField from './AttributeField.svelte';
  import CiAssetPart from './CiAssetPart.svelte';
  import CiHistoryPart from './CiHistoryPart.svelte';
  import AttachmentPanel from '../media/AttachmentPanel.svelte';
  import { ciFilesPath } from '../data/media.svelte.ts';
  import CiImpactPart from './CiImpactPart.svelte';
  import CiRail from './CiRail.svelte';
  import CiRelations from './CiRelations.svelte';
  import CiWorkPart from './CiWorkPart.svelte';
  import { groupsHolding, missingRecommended, openKeysByObject, STAGE_PATH, type Stage } from './objectpage.ts';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    types: readonly CiRelationType[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
    /** The hub the object is in, for the trail back to its inventory. */
    hub?: { readonly id: string; readonly name: string };
  }

  const { ci, classes, types, words, mayWrite, onnavigate, hub }: Props = $props();

  const klass = $derived(classes.find((each) => each.key === ci.class));
  const attributes = $derived(klass?.all_attributes ?? []);
  const typeWord = $derived(classWord(ci.class, classes, words));
  const word = (key: string) => (messages.has(`app.assets.attribute.${key}`) ? t(`app.assets.attribute.${key}`) : key);
  const sourceWord = (source: string) => (source === 'manual' ? t('app.object.origin_manual') : t('app.object.origin_source', { source }));

  /** A writer on a screen that writes; the stage and the reports also need the network. */
  const mayEdit = $derived(mayWrite && !viewport.isCompact && !ci.merged_into);
  const mayAct = $derived(mayEdit && live.state !== 'offline');
  /** *Confirm*: the owning person reads and confirms their own object, a writer any (UC-AST-10). */
  const isOwner = $derived(Boolean(ci.owner_account_id) && ci.owner_account_id === actor.account?.id);
  const mayConfirm = $derived((mayWrite || isOwner) && !viewport.isCompact && !ci.merged_into && live.state !== 'offline');

  // The hub's objects: the names a relation's other end and a location are shown by, the owners and
  // needs an impact row says, the objects a relation may be made to. An end beyond the first page is
  // read alone.
  let hubCis = $state<readonly ConfigurationItem[]>([]);
  let alone = $state<Record<string, ConfigurationItem>>({});
  let saved = $state<readonly CiGroup[]>([]);
  let placeModules = $state<readonly string[] | undefined>(undefined);
  let queues = $state<HubQueueList | undefined>(undefined);
  $effect(() => {
    const at = ci.hub_id;
    return untrack(() => {
      const stops = [
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
          if (state.status === 'ready') hubCis = state.data.data;
        }),
        engine.subscribe<CiGroupList>({ path: groupsPath(at) }, (state) => {
          if (state.status === 'ready') saved = state.data.data;
        }),
        modules.here(at, (next) => {
          if (next.status === 'ready') placeModules = next.data.modules_on;
        }),
        people.open({ hubId: at }),
        groups.open(),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  // The desk's collections, only where the desk is on in this hub (P-17): *Report an incident* goes there.
  const modulesOn = $derived(modulesHere(manifest.value?.modules_on, placeModules));
  const isDeskOn = $derived(modulesOn.includes('SERVICE_DESK'));
  $effect(() => {
    if (!isDeskOn) return;
    const at = ci.hub_id;
    return untrack(() => engine.subscribe<HubQueueList>({ path: `/hubs/${at}/queues` }, (state) => {
      if (state.status === 'ready') queues = state.data;
    }));
  });
  const desks = $derived(isDeskOn ? [...(queues?.desks ?? [])] : []);

  const known = $derived(new Map([...Object.values(alone), ...hubCis].map((each) => [each.id, each])));
  $effect(() => {
    const ends = [...(ci.relations ?? []).flatMap((each) => [each.source_ci_id, each.target_ci_id]), ...(ci.location_id ? [ci.location_id] : [])];
    const unknown = ends.filter((id) => id !== ci.id && !known.has(id));
    if (unknown.length === 0) return;
    untrack(() => {
      for (const id of new Set(unknown)) {
        void cmdb.read(id).then((read) => {
          if (read) alone = { ...alone, [id]: read };
        });
      }
    });
  });

  // The names the page says: owners here and on the objects an impact row names.
  $effect(() => {
    const ids = [ci.owner_account_id, ...hubCis.map((each) => each.owner_account_id)].filter((each): each is string => Boolean(each));
    if (ids.length > 0) untrack(() => accounts.resolve(ids));
  });

  // The work on it (IN-3), what suffers when it fails, and the register's number the groups read.
  const work = resource<CiWork>(untrack(() => objectWorkPath(ci.id)));
  const openKeys = $derived(work.state.status === 'ready' ? openKeysByObject(work.state.data.data) : new Map<string, string>());
  let downstream = $state<CiImpact | undefined>(undefined);
  let downstreamRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const id = ci.id;
    void ci.version;
    untrack(() => {
      cmdb.impact(id, 'DOWNSTREAM').then(
        (read) => (downstream = read),
        (error) => (downstreamRefusal = renderProblem(error as never, messages).message),
      );
    });
  });
  const assetRead = resourceWhen<CiAsset>(() => (ci.has_asset === false ? undefined : ciAssetPath(ci.id)));
  const tags = $derived(new Map(assetRead.state.status === 'ready' && assetRead.state.data.asset_tag ? [[ci.id, assetRead.state.data.asset_tag]] : []));
  const holding = $derived(groupsHolding(ci, saved, { classes, me: actor.account?.id, tags }));

  // The head: confirmed when and kept by whom, the stage as a path, the stage changed by a writer.
  const confirmedWhen = $derived(ci.confirmed_at ?? ci.last_seen_at);
  const sources = $derived([...new Set(Object.values(ci.provenance).map((each) => each.source))].filter((each) => each !== 'manual'));
  const meta = $derived(
    [
      confirmedWhen ? t('app.object.confirmed', { when: formatDateTime(confirmedWhen, messages.locale) }) : t('app.object.never_confirmed'),
      sources.length > 0 ? t('app.object.kept_from', { sources: sources.join(', ') }) : t('app.object.kept_by_hand'),
    ].join(' · '),
  );
  const stageSteps = $derived(STAGE_PATH.map((stage) => ({ id: stage, label: t(`app.assets.stage.${stage}`) })));
  let isStaging = $state(false);
  async function changeStage(stage: string) {
    if (stage === ci.stage || !(STAGE_PATH as readonly string[]).includes(stage)) return;
    isStaging = true;
    try {
      await cmdb.setCompliance(ci, { stage: stage as Stage });
      toast.show({ text: t('app.object.stage_changed', { stage: t(`app.assets.stage.${stage}`) }), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isStaging = false;
    }
  }

  // *Confirm* (scene `grammar`): the object is as recorded, its next review moves a year on.
  let isConfirming = $state(false);
  async function confirm() {
    if (isConfirming) return;
    isConfirming = true;
    try {
      const answer = await cmdb.confirm(ci);
      toast.show({ text: t('app.object.confirmed_toast', { day: formatPlainDay(answer.review_due, messages.locale) }), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    } finally {
      isConfirming = false;
    }
  }
  const reviewChip = $derived(
    ci.review_due
      ? { label: t('app.object.review_chip', { day: formatPlainDay(ci.review_due, messages.locale) }), tone: ci.review_due < todayIn(actor.zone) ? ('warning' as const) : ('neutral' as const) }
      : undefined,
  );

  async function copyReference() {
    try {
      await navigator.clipboard.writeText(ciReferenceText(ci.id));
      toast.show({ text: t('app.assets.reference_copied'), tone: 'success' });
    } catch {
      // A clipboard the browser refuses says nothing: the object's address is on screen.
    }
  }

  // The attributes: those holding a value, each with the source that wrote it; the recommended
  // ones nobody filled counted on one line that opens the edit.
  const filled = $derived(attributes.filter((each) => attributeText(ci.attributes[each.key]) !== ''));
  const missing = $derived(missingRecommended(attributes, ci.attributes));
  let isEditing = $state(false);
  let drafts = $state<Record<string, string>>({});
  let draftName = $state('');
  let draftClass = $state('');
  const refined = $derived(refinements(classes, ci.class));
  const groupOptions = $derived<SelectOption[]>(groups.all.map((each) => ({ value: each.id, label: each.name })));
  const accountOptions = $derived<SelectOption[]>(
    people.candidates({ hubId: ci.hub_id }).map((id) => ({ value: id, label: accounts.nameOf(id) ?? id })),
  );

  function startEditing() {
    drafts = Object.fromEntries(attributes.map((each) => [each.key, attributeText(ci.attributes[each.key])]));
    draftName = ci.name;
    draftClass = '';
    isEditing = true;
    mark = 'ci-attributes';
  }

  async function saveAttributes() {
    const target = draftClass ? classes.find((each) => each.key === draftClass) : klass;
    const { changes, invalid } = changedAttributes(target?.all_attributes ?? attributes, ci.attributes, drafts);
    if (invalid.length > 0) {
      toast.show({ text: t('app.assets.attribute_invalid', { keys: invalid.map(word).join(', ') }), tone: 'danger' });
      return;
    }
    const name = draftName.trim();
    const body = {
      ...(name && name !== ci.name ? { name } : {}),
      ...(draftClass ? { class: draftClass } : {}),
      ...(Object.keys(changes).length > 0 ? { attributes: changes } : {}),
    };
    if (Object.keys(body).length === 0) {
      isEditing = false;
      return;
    }
    try {
      const answer = await cmdb.update(ci, body);
      isEditing = false;
      const kept = answer.decision?.kept ?? [];
      toast.show({
        text: kept.length > 0 ? t('app.assets.kept', { keys: kept.map((each) => t('app.assets.kept_one', { attribute: word(each.attribute), by: each.by })).join(', ') }) : t('app.assets.saved'),
        tone: kept.length > 0 ? 'warning' : 'success',
      });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }

  // The head's ⋯, for a writer: what the sections offer, from one place.
  let isRelateAsked = $state(false);
  const headMenu = $derived(
    mayEdit
      ? {
          label: t('app.workspace.actions', { name: ci.name }),
          items: [
            { id: 'attributes', label: t('app.object.edit_attributes'), icon: 'pencil' as const },
            { id: 'relate', label: t('app.object.relate_title', { name: ci.name }), icon: 'link' as const },
          ],
          onselect: (id: string) => {
            if (id === 'attributes') startEditing();
            if (id === 'relate') isRelateAsked = true;
          },
        }
      : undefined,
  );

  // The object's files (UC-AST-01 check 12; ET-4, F-1): a grid with previews, dropped anywhere on
  // the section by a writer on a screen that writes and with the network; read on a phone.
  const filesRead = resource<MediaPage>(untrack(() => ciFilesPath(ci.id)));
  const fileRows = $derived(filesRead.state.status === 'ready' ? filesRead.state.data.data : []);
  let filesPanel = $state<ReturnType<typeof AttachmentPanel> | undefined>(undefined);
  let isDropping = $state(false);
  function dragOver(event: DragEvent) {
    if (!mayAct || !event.dataTransfer?.types.includes('Files')) return;
    event.preventDefault();
    isDropping = true;
  }
  function dropFiles(event: DragEvent) {
    isDropping = false;
    if (!mayAct || !event.dataTransfer?.files.length) return;
    event.preventDefault();
    void filesPanel?.attach([...event.dataTransfer.files]);
  }

  // Jump marks with counts (ET-2); on a phone the switch between the sections, the relations first.
  const relationCount = $derived((ci.relations ?? []).length);
  const workCount = $derived(work.state.status === 'ready' ? work.state.data.data.length : undefined);
  const marks = $derived.by(() => {
    const count = (n: number | undefined) => (n ? String(n) : undefined);
    const all = [
      { id: 'ci-attributes', label: t('app.object.mark_attributes'), count: count(filled.length) },
      { id: 'ci-relations', label: t('app.object.mark_relations'), count: count(relationCount) },
      { id: 'ci-impact', label: t('app.object.mark_impact'), count: count(downstream?.reached.length) },
      { id: 'ci-work', label: t('app.object.mark_work'), count: count(workCount) },
      { id: 'ci-register', label: t('app.object.mark_register') },
      { id: 'ci-files', label: t('app.object.mark_files'), count: count(fileRows.length) },
      { id: 'ci-history', label: t('app.object.mark_history') },
    ];
    return viewport.isCompact ? [all[1]!, all[3]!, all[0]!, all[2]!, all[4]!, all[5]!, all[6]!] : all;
  });
  let mark = $state<string>(untrack(() => (viewport.isCompact ? 'ci-relations' : 'ci-attributes')));

  const stockOf = (line: string) => `/hubs/${ci.hub_id}/inventory?line=${encodeURIComponent(line)}`;
</script>

<DetailPage details={rail}>
  {#snippet top()}
    <!-- One head for every object (WB-1, IN-2): the object type with the inventory's tile, the
         stage, when it was confirmed and by whom, the name, and the stage as its path. -->
    <ObjectHead hasActsOnPhone
      objectKey={{ label: typeWord, icon: 'shield', product: 'assets' }}
      type={{ label: t(`app.assets.stage.${ci.stage}`) }}
      {meta}
      title={ci.name}
      titleLabel={typeWord}
      trail={hub && !viewport.isCompact ? ciTrail : undefined}
      tools={headTools}
      menu={headMenu}
      next={mayConfirm ? { label: t('app.object.confirm'), icon: 'check', onclick: () => void confirm() } : undefined}
      due={reviewChip}
      facts={ci.status !== 'OK' ? statusFact : undefined}
      end={mayAct ? stageMenu : undefined}
    >
      {#snippet status()}
        <span class="path" data-stage-bar>
          <StepBar label={t('app.object.stage_label', { name: ci.name })} steps={stageSteps} current={ci.stage} doneLabel={t('app.object.step_done')} />
        </span>
      {/snippet}
    </ObjectHead>
  {/snippet}

  {#if ci.merged_into}
    <Banner tone="info">
      {t('app.assets.merged_into')}
      <Button size="sm" tone="subtle" onclick={() => onnavigate(`/cis/${ci.merged_into}`)}>{t('app.assets.open_survivor')}</Button>
    </Banner>
  {/if}

  <div class="ci-main" data-show={mark}>
    <JumpMarks label={t('app.object.marks', { name: ci.name })} {marks} bind:current={mark} />

    <FoldingSection id="ci-attributes" title={t('app.object.mark_attributes')} count={filled.length > 0 ? String(filled.length) : undefined} actions={mayEdit && !isEditing ? attributeActions : undefined}>
      {#snippet children()}
        {#if isEditing}
          <Stack gap="100">
            <Input label={t('app.assets.name_label')} bind:value={draftName} maxlength={200} />
            {#if refined.length > 0}
              <Select
                label={t('app.assets.refine')}
                options={[{ value: '', label: t('app.assets.keep_class') }, ...refined.map((each) => ({ value: each.key, label: classWord(each.key, classes, words) }))]}
                bind:value={draftClass}
              />
            {/if}
            {#each (draftClass ? classes.find((each) => each.key === draftClass)?.all_attributes ?? attributes : attributes) as attribute (attribute.key)}
              <AttributeField {attribute} bind:value={() => drafts[attribute.key] ?? '', (typed: string) => (drafts[attribute.key] = typed)} groups={groupOptions} accounts={accountOptions} />
            {/each}
            <div class="buttons">
              <Button tone="secondary" onclick={() => (isEditing = false)}>{t('app.assets.cancel')}</Button>
              <Button tone="primary" onclick={() => void saveAttributes()}>{t('app.assets.save')}</Button>
            </div>
          </Stack>
        {:else}
          {#if attributes.length === 0}
            <p class="none">{t('app.assets.no_attributes')}</p>
          {:else if filled.length > 0}
            <dl class="attributes" data-ci-attributes>
              {#each filled as attribute (attribute.key)}
                {@const origin = ci.provenance[attribute.key]}
                <div class="attribute">
                  <dt>{word(attribute.key)}{#if origin}{' · '}<span class="origin">{sourceWord(origin.source)}</span>{/if}</dt>
                  <dd>{attributeText(ci.attributes[attribute.key])}</dd>
                </div>
              {/each}
            </dl>
          {/if}
          {#if missing.length > 0}
            {#if mayEdit}
              <button type="button" class="missing" data-missing-attributes onclick={startEditing}>{t('app.object.missing_recommended', { count: missing.length })}</button>
            {:else}
              <p class="none" data-missing-attributes>{t('app.object.missing_recommended_read', { count: missing.length })}</p>
            {/if}
          {/if}
        {/if}
      {/snippet}
    </FoldingSection>

    <CiRelations {ci} {classes} {types} {words} {known} {hubCis} {openKeys} mayWrite={mayEdit} reader={actor.account?.id}
      {isRelateAsked} onrelateasked={() => (isRelateAsked = false)} />

    <CiImpactPart {ci} {classes} {words} {known} {downstream} {downstreamRefusal} />

    <CiWorkPart {ci} work={work.state} {desks} {mayAct} />

    <CiAssetPart {ci} {classes} {hubCis} mayWrite={mayEdit} />

    <FoldingSection id="ci-files" title={t('app.object.mark_files')} count={fileRows.length > 0 ? String(fileRows.length) : undefined}>
      {#snippet children()}
        <div class="files" class:dropping={isDropping} role="presentation" data-ci-files ondragover={dragOver} ondragleave={() => (isDropping = false)} ondrop={dropFiles}>
          <AttachmentPanel bind:this={filesPanel} ci={{ id: ci.id, name: ci.name }} isChangeable={mayAct} view="grid" ownerKey={typeWord} />
        </div>
      {/snippet}
    </FoldingSection>

    <CiHistoryPart {ci} {known} files={fileRows} {word} />
  </div>
</DetailPage>

{#snippet rail()}
  <CiRail {ci} {classes} {known} {hubCis} {downstream} {holding} mayWrite={mayEdit} />
{/snippet}

{#snippet attributeActions()}
  <Button size="sm" tone="subtle" icon="pencil" data-edit-attributes onclick={startEditing}>{t('app.assets.edit')}</Button>
{/snippet}

{#snippet headTools()}
  <Button size="sm" tone="subtle" icon="link" data-copy-reference onclick={() => void copyReference()}>{t('app.assets.copy_reference')}</Button>
{/snippet}

{#snippet statusFact()}
  <StateLozenge tone="warning" label={t(`app.assets.status.${ci.status}`)} />
{/snippet}

{#snippet stageMenu()}
  <Menu label={t('app.object.change_stage')} placement={{ side: 'block-end', align: 'end' }} onselect={(id) => void changeStage(id)}
    items={STAGE_PATH.map((stage) => ({ id: stage, label: t(`app.assets.stage.${stage}`), icon: stage === ci.stage ? 'check' : undefined, disabledReason: stage === ci.stage ? t('app.object.stage_current') : undefined }))}>
    {#snippet trigger(props)}
      <Button size="sm" tone="secondary" icon="chevron-down" isBusy={isStaging} data-change-stage {...props}>{t('app.object.change_stage')}</Button>
    {/snippet}
  </Menu>
{/snippet}

{#snippet ciTrail()}
  {#if hub}
    {@const at = hub}
    <Breadcrumb
      label={t('app.assets.trail')}
      trail={[
        { id: at.id, label: at.name, href: `/hubs/${at.id}` },
        { id: 'inventory', label: t('app.assets.title'), href: `/hubs/${at.id}/inventory` },
        { id: 'type', label: classesWord(ci.class, classes, words), href: stockOf(withKey('', 'asset.class', ci.class)) },
        { id: 'ci', label: ci.name },
      ]}
      mark={placeMark}
      expandLabel={t('app.admin.expand_trail')}
      onnavigate={(id) => onnavigate(id === at.id ? `/hubs/${at.id}` : id === 'type' ? stockOf(withKey('', 'asset.class', ci.class)) : `/hubs/${at.id}/inventory`)}
    />
  {/if}
{/snippet}

<style>
  .path { display: flex; flex: 1 1 auto; min-inline-size: 0; }
  .ci-main { display: flex; flex-direction: column; min-inline-size: 0; }
  .none { margin: 0; color: var(--text-subtle); font-size: var(--fs-075); }
  .attributes { display: grid; grid-template-columns: minmax(0, 1fr); gap: 0 var(--sp-300); margin: 0; }
  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px); a media query cannot read a custom property. */
  @media (width >= 905px) { .attributes { grid-template-columns: repeat(2, minmax(0, 1fr)); } }
  .attribute { display: flex; flex-direction: column; gap: var(--sp-025); padding-block: var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); min-inline-size: 0; }
  .attribute dt { color: var(--text-secondary); font-size: var(--fs-075); }
  .origin { color: var(--text-subtle); }
  .attribute dd { margin: 0; overflow-wrap: anywhere; }
  .missing {
    align-self: center; margin-block-start: var(--sp-150); padding: var(--sp-050) var(--sp-100); border: 0; border-radius: var(--r-sm);
    background: none; color: var(--text-brand); font: inherit; font-size: var(--fs-075); font-weight: var(--fw-semibold); cursor: pointer;
  }
  .missing:hover { background: var(--bg-surface-hover); }
  .missing:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
  .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); justify-content: flex-end; }
  .files { border-radius: var(--r-md); }
  .files.dropping { outline: var(--bw-thick) dashed var(--accent-primary); outline-offset: var(--sp-050); background: var(--accent-primary-subtle); }

  /* On a phone the marks are the switch: one section shows (§7). */
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .ci-main[data-show] > :global(section[id^='ci-']) { display: none; }
    .ci-main[data-show='ci-attributes'] > :global(#ci-attributes),
    .ci-main[data-show='ci-relations'] > :global(#ci-relations),
    .ci-main[data-show='ci-impact'] > :global(#ci-impact),
    .ci-main[data-show='ci-work'] > :global(#ci-work),
    .ci-main[data-show='ci-register'] > :global(#ci-register),
    .ci-main[data-show='ci-files'] > :global(#ci-files),
    .ci-main[data-show='ci-history'] > :global(#ci-history) { display: flex; }
  }
</style>
