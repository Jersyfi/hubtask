<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Stock*'s objects (work-surface §6.10, scene invlist 2-4): the query line with the `asset.`
  // keys, the object-type tree with counts and the saved groups beside it, and the objects as one
  // calm table - name, type, stage, owner, location, protection, open entries (UC-AST-01 check 8).
  //
  // **Narrowed on the device.** The hub's objects are in the device's copy (UC-AST-01 check 10);
  // the line narrows what is loaded with the server's reading of the same keys
  // (`data/inventoryquery.ts`), so reading and narrowing work offline. *Load more* fetches the next
  // part of the hub. A saved group puts its filter in the line; a line becomes a group.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS no create, no import,
  // no *Save as group* and no group's ⋯ - nor on a phone, where the stock is read only.
  import { untrack } from 'svelte';
  import {
    Badge, Button, DataTable, Icon, IconButton, Menu, QueryLine, Select, Skeleton, StateLozenge, ViewSwitcher, lineText,
    type DataColumn, type QueryToken, type SelectOption,
  } from '@hubtask/design-system/components';
  import { createCatalogue, createLanguage, relabelled, type CountedValue, type QueryKey } from '@hubtask/query';
  import FailureNote from '../frame/FailureNote.svelte';
  import type { CiClass, CiGroup, CiGroupList, CiWorkCounts, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import { actor } from '../data/account.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { ciWorkPath, cisPath, classWord, cmdb, groupsPath, instantiable, missingRequired, parseAttribute, type Words } from '../data/cmdb.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { groups as peopleGroups } from '../data/groups.svelte.ts';
  import {
    ASSET_KEYS, DEFAULT_LINE, classTree, groupOfLine, highestProtection, holdsAttributes, lineOfGroup, narrowed, readLine, soleValue, valueCounts, withKey,
    type AssetKey, type InventoryFilter,
  } from '../data/inventoryquery.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import CreateDialog from '../product/CreateDialog.svelte';
  import ProductEmpty from '../product/ProductEmpty.svelte';
  import { createPlaces } from '../product/places.svelte.ts';
  import AttributeField from './AttributeField.svelte';
  import GroupDialog from './GroupDialog.svelte';

  interface Props {
    hubId: string;
    classes: readonly CiClass[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
    /** The objects health finds wanting or an alert fires on: marked in the name column. */
    wanting?: ReadonlySet<string>;
    /** The register's inventory numbers by object, which `asset.tag` reads. */
    tags?: ReadonlyMap<string, string>;
    /** The address was the former groups page: the saved groups are scrolled to. */
    isSavedShown?: boolean;
    /** The head asked for a new object or a new group; the dialog opens once and `onasked` gives the ask back. */
    asked?: 'ci' | 'group';
    onasked?: () => void;
    /** Where an empty inventory's *Import a file* goes. */
    onimport?: () => void;
    /** The line it opens with instead of the default (an object page's trail: its object type). */
    openLine?: string;
    /** The saved group it opens with, once the groups are read (an object page's rail). */
    openGroup?: string;
  }

  const { hubId, classes, words, mayWrite, onnavigate, wanting = new Set(), tags = new Map(), isSavedShown = false, asked, onasked, onimport, openLine, openGroup }: Props = $props();

  // The hub's objects: as many at once as the list gives, the rest on *Load more*.
  const BATCH = 500;
  let first = $state<ConfigurationItemList | undefined>(undefined);
  let more = $state<readonly ConfigurationItem[]>([]);
  let next = $state<string | null | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let isLoadingMore = $state(false);
  let moreFailed = $state<string | undefined>(undefined);
  let saved = $state<readonly CiGroup[]>([]);
  let work = $state<ReadonlyMap<string, number>>(new Map());
  $effect(() => {
    const at = hubId;
    return untrack(() => {
      const stops = [
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at, { limit: BATCH }) }, (state) => {
          if (state.status === 'ready') {
            first = state.data;
            more = [];
            next = state.data.next;
          }
          if (state.status === 'failed') refusal = renderProblem(state.error, messages).message;
        }),
        engine.subscribe<CiGroupList>({ path: groupsPath(at) }, (state) => {
          if (state.status === 'ready') saved = state.data.data;
        }),
        // The open entries per object; a refusal leaves the column empty rather than the list.
        engine.subscribe<CiWorkCounts>({ path: ciWorkPath(at) }, (state) => {
          if (state.status === 'ready') work = new Map(state.data.data.map((each) => [each.ci_id, each.open]));
        }),
        peopleGroups.open(),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });

  const all = $derived([...(first?.data ?? []), ...more].filter((each) => !each.merged_into));
  // On a phone the stock is read, searched and opened; writing waits for a larger screen (§6.10, UC-AST-01 check 10).
  const drawsWrites = $derived(mayWrite && !viewport.isCompact);
  const byId = $derived(new Map(all.map((each) => [each.id, each])));

  async function loadMore() {
    if (!next) return;
    isLoadingMore = true;
    moreFailed = undefined;
    try {
      const page = await cmdb.page(hubId, next);
      more = [...more, ...page.data];
      next = page.next;
    } catch (error) {
      moreFailed = renderProblem(error as never, messages).message;
    } finally {
      isLoadingMore = false;
    }
  }

  // The line (§6.11): chips and words, the keys the server serves for the inventory.
  let line = $state(untrack(() => openLine) ?? DEFAULT_LINE);
  let tokens = $state<readonly QueryToken[]>([]);
  let text = $state('');
  const assetKeys = $derived<readonly QueryKey[]>(
    ((manifest.value as { query_keys?: readonly QueryKey[] } | undefined)?.query_keys ?? [])
      .filter((key) => key.product === 'asset')
      // The values come counted from the objects, `me` and `none` among them.
      .map((key) => ({ ...key, values: [] })),
  );
  const me = $derived(actor.account?.id);
  const context = $derived({ classes, me, tags });

  function ownerName(id: string): string {
    return accounts.nameOf(id) ?? peopleGroups.nameOf(id) ?? (id === me ? actor.account?.display_name : undefined) ?? t('app.people.unnamed');
  }
  function valueWord(key: string, value: string): string {
    if (value === 'none') return key === 'asset.owner' ? t('app.inventory.nobody') : t('app.inventory.value_none');
    if (key === 'asset.owner') return value === 'me' ? t('app.inventory.me') : ownerName(value);
    if (key === 'asset.class') return classWord(value, classes, words);
    if (key === 'asset.stage') return messages.has(`app.assets.stage.${value.toUpperCase()}`) ? t(`app.assets.stage.${value.toUpperCase()}`) : value;
    if (key === 'asset.location') return byId.get(value)?.name ?? value;
    return value;
  }
  const named = (code: string, fallback: string, params?: Record<string, string>) => (messages.has(code) ? t(code, params) : fallback);
  const wording = $derived({
    key: (key: string) => named(`app.qkey.${key.replaceAll('.', '_')}`, key),
    value: valueWord,
    problem: (error: { code: string; params: Readonly<Record<string, string>> }) => named(error.code, error.code, error.params as Record<string, string>),
  });
  const language = $derived(createLanguage(wording));

  let spelled: string | undefined;
  $effect(() => {
    const wanted = line;
    const fresh = language.parse(wanted ? `${wanted} ` : '').tokens;
    if (wanted === spelled) {
      const renamed = relabelled(untrack(() => tokens), fresh);
      if (renamed) tokens = renamed;
      return;
    }
    tokens = fresh;
    text = '';
    spelled = wanted;
  });
  function changed(chips: readonly QueryToken[]) {
    spelled = lineText(language, chips, '');
    line = spelled;
  }

  // What is typed counts at once (Q-4): the line with the words not yet a chip, else the line.
  const read = $derived.by(() => {
    const typed = readLine(lineText(language, tokens, text));
    return typed.ok ? typed : readLine(line);
  });
  const filter = $derived<InventoryFilter>(read.ok ? read.filter : { conditions: [], terms: [] });
  const lineProblem = $derived(!read.ok ? named(read.error.code, read.error.code, read.error.params as Record<string, string>) : undefined);

  const counted = (key: string, prefix: string): readonly CountedValue[] => {
    if (!(ASSET_KEYS as readonly string[]).includes(key)) return [];
    const typed = prefix.toLowerCase();
    const values = valueCounts(all, filter, key as AssetKey, context).map((each) => ({
      value: each.value === 'none' ? 'none' : key === 'asset.owner' && each.value === me ? 'me' : each.value,
      label: valueWord(key, each.value === me && key === 'asset.owner' ? 'me' : each.value),
      countLabel: String(each.count),
    }));
    const offered = key === 'asset.owner' && !values.some((each) => each.value === 'me') ? [...values, { value: 'me', label: t('app.inventory.me') }] : values;
    return offered.filter((each) => !typed || each.value.toLowerCase().includes(typed) || each.label.toLowerCase().includes(typed));
  };
  const catalogue = $derived(createCatalogue(assetKeys, wording, counted));

  // A saved group in the line: its attribute values, which have no key, narrow beside it while the
  // line is the group's.
  let chosenGroup = $state<{ id: string; line: string; attributes: Readonly<Record<string, string>> } | undefined>(undefined);
  const groupActive = $derived(chosenGroup && chosenGroup.line === line ? chosenGroup : undefined);
  function clearLine() {
    chosenGroup = undefined;
    line = '';
    text = '';
  }
  function chooseGroup(group: CiGroup) {
    const of = lineOfGroup(group.filter);
    chosenGroup = { id: group.id, line: of.line, attributes: of.attributes };
    line = of.line;
  }
  // The group the address named, chosen once its filter is read.
  let isGroupOpened = false;
  $effect(() => {
    if (isGroupOpened || !openGroup) return;
    const found = saved.find((each) => each.id === openGroup);
    if (!found) return;
    isGroupOpened = true;
    untrack(() => chooseGroup(found));
  });

  const rows = $derived(
    narrowed(all, filter, context).filter((each) => !groupActive || holdsAttributes(each, groupActive.attributes)),
  );

  // The tree counts within the rest of the line, its own object type aside.
  const tree = $derived(classTree(narrowed(all, { ...filter, conditions: filter.conditions.filter((each) => each.key !== 'asset.class') }, context), classes));
  const treeTotal = $derived(narrowed(all, { ...filter, conditions: filter.conditions.filter((each) => each.key !== 'asset.class') }, context).length);
  const chosenClass = $derived(soleValue(line, 'asset.class'));
  const classChosen = $derived(filter.conditions.some((each) => each.key === 'asset.class'));
  const chooseClass = (key: string | undefined) => {
    chosenGroup = undefined;
    line = withKey(line, 'asset.class', key);
  };

  // The phone folds the tree into one choice.
  const treeChoice = $derived(groupActive ? `group:${groupActive.id}` : chosenClass ? `class:${chosenClass}` : '');
  const treeOptions = $derived<SelectOption[]>([
    { value: '', label: t('app.inventory.with_count', { label: t('app.inventory.all'), count: String(treeTotal) }) },
    ...tree.map((row) => ({ value: `class:${row.key}`, label: t('app.inventory.with_count', { label: `${'· '.repeat(row.depth)}${classWord(row.key, classes, words)}`, count: String(row.count) }) })),
    ...saved.map((group) => ({ value: `group:${group.id}`, label: group.members === undefined ? group.name : t('app.inventory.with_count', { label: group.name, count: String(group.members) }) })),
  ]);
  function chooseFromTree(value: string) {
    if (value.startsWith('group:')) {
      const group = saved.find((each) => `group:${each.id}` === value);
      if (group) chooseGroup(group);
    } else chooseClass(value.startsWith('class:') ? value.slice('class:'.length) : undefined);
  }

  // How the table is drawn, kept on this device (WB-4): the columns shown and the arrangement.
  const COLUMNS_KEY = 'hubtask.inventory.columns';
  const LAYOUT_KEY = 'hubtask.inventory.layout';
  const HIDEABLE = ['class', 'stage', 'owner', 'location', 'protection', 'open'] as const;
  function remembered(key: string): string | undefined {
    try {
      return localStorage.getItem(key) ?? undefined;
    } catch {
      return undefined;
    }
  }
  function remember(key: string, value: string) {
    try {
      localStorage.setItem(key, value);
    } catch {
      // A browser that keeps nothing still draws the choice for this visit.
    }
  }
  let hidden = $state<string[]>(
    (remembered(COLUMNS_KEY) ?? '').split(',').filter((each) => (HIDEABLE as readonly string[]).includes(each)),
  );
  let layout = $state<'list' | 'types'>(remembered(LAYOUT_KEY) === 'types' ? 'types' : 'list');
  // Seven columns need more than the stock's box beside the tree on a laptop: until the reader
  // chooses columns, the two the object's page also shows - location and protection - step aside
  // while the row would run past the box, and come back once it is wide enough again.
  let isChosen = $state(remembered(COLUMNS_KEY) !== undefined);
  let box = $state<HTMLElement | undefined>(undefined);
  let boxWidth = $state(0);
  let isCrowded = $state(false);
  let neededWidth = 0;
  $effect(() => {
    const width = boxWidth;
    void rows.length;
    const table = box?.querySelector('table');
    if (isChosen || !table || width === 0) return;
    if (!isCrowded && table.getBoundingClientRect().width > width + 1) {
      neededWidth = table.getBoundingClientRect().width;
      isCrowded = true;
    } else if (isCrowded && width >= neededWidth) isCrowded = false;
  });
  const CROWDED = ['location', 'protection'] as const;
  const shownHidden = $derived(isCrowded ? [...new Set([...hidden, ...CROWDED])] : hidden);
  function toggleColumn(id: string) {
    if (!isChosen) {
      hidden = shownHidden;
      isChosen = true;
    }
    hidden = hidden.includes(id) ? hidden.filter((each) => each !== id) : [...hidden, id];
    remember(COLUMNS_KEY, hidden.join(','));
  }
  function chooseLayout(id: string) {
    layout = id === 'types' ? 'types' : 'list';
    remember(LAYOUT_KEY, layout);
  }

  const columns = $derived<DataColumn[]>([
    { id: 'name', label: t('app.inventory.col.name'), priority: 1 },
    { id: 'class', label: t('app.inventory.col.class'), priority: 2, isHideable: true },
    { id: 'stage', label: t('app.inventory.col.stage'), priority: 2, isHideable: true, isStatus: true },
    { id: 'owner', label: t('app.inventory.col.owner'), priority: 3, isHideable: true },
    { id: 'location', label: t('app.inventory.col.location'), priority: 3, isHideable: true },
    { id: 'protection', label: t('app.inventory.col.protection'), priority: 3, isHideable: true },
    { id: 'open', label: t('app.inventory.col.open'), priority: 3, isHideable: true, align: 'end' },
  ]);
  const columnItems = $derived(
    columns.filter((each) => each.isHideable).map((each) => ({ id: each.id, label: each.label, ...(shownHidden.includes(each.id) ? {} : { icon: 'check' as const }) })),
  );

  const STAGE_TONE: Record<ConfigurationItem['stage'], 'neutral' | 'info' | 'success' | 'warning' | 'danger'> = {
    PLANNED: 'neutral', ORDERED: 'info', IN_STOCK: 'info', IN_USE: 'success', MAINTENANCE: 'warning', RETIRED: 'neutral', DISPOSED: 'neutral',
  };
  const ownerOf = (ci: ConfigurationItem) => (ci.owner_account_id ? ownerName(ci.owner_account_id) : ci.owner_group_id ? ownerName(ci.owner_group_id) : undefined);
  const locationOf = (ci: ConfigurationItem) => (ci.location_id ? byId.get(ci.location_id)?.name : undefined);
  const typeOf = (ci: ConfigurationItem) => classWord(ci.class, classes, words);

  // Saving the line as a group, or changing a saved one.
  let isGroupOpen = $state(false);
  let editing = $state<CiGroup | undefined>(undefined);
  let prefill = $state<ReturnType<typeof groupOfLine> | undefined>(undefined);
  const services = $derived<SelectOption[]>(
    all.filter((each) => classes.find((one) => one.key === each.class)?.lineage.includes('service')).map((each) => ({ value: each.id, label: each.name })),
  );
  function keptWords(of: { owners?: string[]; no_owner?: boolean; locations?: string[]; no_location?: boolean; tags?: string[]; no_tag?: boolean } | undefined): string | undefined {
    if (!of) return undefined;
    const part = (key: AssetKey, values: readonly string[] | undefined, none: boolean | undefined) => {
      const all = [...(values ?? []).map((value) => valueWord(key, value)), ...(none ? [valueWord(key, 'none')] : [])];
      return all.length > 0 ? `${wording.key(key)}: ${all.join(', ')}` : '';
    };
    const text = [part('asset.owner', of.owners, of.no_owner), part('asset.location', of.locations, of.no_location), part('asset.tag', of.tags, of.no_tag)].filter(Boolean).join(' · ');
    return text || undefined;
  }
  function saveLine() {
    editing = undefined;
    prefill = groupOfLine(line, me);
    isGroupOpen = true;
  }
  function editGroup(group: CiGroup) {
    editing = group;
    prefill = undefined;
    isGroupOpen = true;
  }
  const groupNote = $derived(prefill && !prefill.ok ? t(messages.has(prefill.code) ? prefill.code : 'app.inventory.group_unheld') : undefined);
  const attributesNote = $derived(
    groupActive && Object.keys(groupActive.attributes).length > 0
      ? t('app.inventory.group_attributes', { conditions: Object.entries(groupActive.attributes).map(([key, value]) => `${key} = ${value}`).join(', ') })
      : undefined,
  );

  // A new object, by hand (UC-AST-01): through the same reconciliation every source writes through.
  const offered = $derived(instantiable(classes));
  const classOptions = $derived(offered.map((each) => ({ value: each.key, label: classWord(each.key, classes, words) })));
  let isCreating = $state(false);
  let createFailure = $state<string | undefined>(undefined);
  let draftClass = $state('');
  let draftName = $state('');
  let drafts = $state<Record<string, string>>({});
  /** Where the object is made (§5.2): this hub, or another where the inventory is on; the types are the workspace's. */
  let draftHub = $state('');
  const placesToMake = createPlaces('ASSETS', 'HUB', () => isCreating);
  const chosen = $derived(classes.find((each) => each.key === draftClass));
  const missing = $derived(chosen ? missingRequired(chosen.all_attributes, drafts) : []);

  function startCreating() {
    draftClass = chosenClass && offered.some((each) => each.key === chosenClass) ? chosenClass : '';
    draftName = '';
    drafts = {};
    draftHub = hubId;
    createFailure = undefined;
    isCreating = true;
  }

  $effect(() => {
    if (!asked || !mayWrite) return;
    const what = asked;
    untrack(() => {
      if (what === 'ci') startCreating();
      else {
        editing = undefined;
        prefill = undefined;
        isGroupOpen = true;
      }
      onasked?.();
    });
  });

  let savedPart = $state<HTMLElement | null>(null);
  $effect(() => {
    if (isSavedShown && savedPart) untrack(() => savedPart?.scrollIntoView({ block: 'nearest' }));
  });

  async function create() {
    if (!chosen) return;
    const attributes: Record<string, unknown> = {};
    const invalid: string[] = [];
    for (const attribute of chosen.all_attributes) {
      const parsed = parseAttribute(attribute, drafts[attribute.key] ?? '');
      if (!parsed.ok) invalid.push(attribute.key);
      else if (parsed.value !== null) attributes[attribute.key] = parsed.value;
    }
    if (invalid.length > 0) {
      createFailure = t('app.assets.attribute_invalid', { keys: invalid.join(', ') });
      return;
    }
    try {
      const made = await cmdb.create(draftHub || hubId, { class: chosen.key, name: draftName.trim(), attributes });
      isCreating = false;
      onnavigate(`/cis/${made.id}`);
    } catch (error) {
      // Said inside the dialog, which keeps what was typed (user-experience.md §5.10).
      createFailure = renderProblem(error as never, messages).message;
    }
  }

  const createReason = $derived(
    !draftClass ? t('app.assets.needs_class')
    : !draftName.trim() ? t('app.assets.needs_name')
    : missing.length > 0 ? t('app.assets.required_missing', { keys: missing.join(', ') })
    : undefined,
  );
</script>

{#snippet treeColumn()}
  <nav class="tree" aria-label={t('app.inventory.types')} data-inventory-tree>
    <h2 class="tree-heading">{t('app.inventory.types')}</h2>
    <ul class="tree-rows">
      <li>
        <button type="button" class="tree-row" aria-current={!classChosen && !groupActive ? 'true' : undefined} data-tree-class="" onclick={() => chooseClass(undefined)}>
          <span class="tree-name">{t('app.inventory.all')}</span><span class="tree-count">{treeTotal}</span>
        </button>
      </li>
      {#each tree as row (row.key)}
        <li>
          <button type="button" class="tree-row" data-depth={row.depth} aria-current={chosenClass === row.key && !groupActive ? 'true' : undefined} data-tree-class={row.key} onclick={() => chooseClass(row.key)}>
            <span class="tree-name">{classWord(row.key, classes, words)}</span><span class="tree-count">{row.count}</span>
          </button>
        </li>
      {/each}
    </ul>
    <h2 class="tree-heading" bind:this={savedPart} data-inventory-saved>{t('app.inventory.saved')}</h2>
    <ul class="tree-rows">
      {#each saved as group (group.id)}
        <li class="saved-row" data-group={group.key}>
          <button type="button" class="tree-row" aria-current={groupActive?.id === group.id ? 'true' : undefined} onclick={() => chooseGroup(group)}>
            <Icon name="star" size="sm" />
            <span class="tree-name">{group.name}</span>{#if group.members !== undefined}<span class="tree-count">{group.members}</span>{/if}
          </button>
          {#if drawsWrites}
            <Menu label={t('app.inventory.group_more', { name: group.name })} items={[{ id: 'edit', label: t('app.inventory.group_edit'), icon: 'pencil' }]} onselect={() => editGroup(group)}>
              {#snippet trigger(props)}
                <IconButton icon="ellipsis" size="sm" tone="subtle" label={t('app.inventory.group_more', { name: group.name })} {...props} />
              {/snippet}
            </Menu>
          {/if}
        </li>
      {:else}
        <li class="tree-empty">{t('app.ci_groups.none')}</li>
      {/each}
    </ul>
    {#if drawsWrites}
      <div><Button tone="subtle" size="sm" icon="plus" data-save-group onclick={saveLine}>{t('app.inventory.save_group')}</Button></div>
    {/if}
  </nav>
{/snippet}

{#if refusal}
  <FailureNote problem={refusal} />
{:else if !first}
  <div aria-busy="true"><Skeleton lines={4} /></div>
{:else if all.length === 0}
  <ProductEmpty product="ASSETS" place={{ type: 'HUB', id: hubId }}
    kind="unused"
    icon="shield"
    title={t('app.assets.empty')}
    description={mayWrite ? t('app.assets.empty_hint') : t('app.assets.empty_reader')}
    create={drawsWrites ? { label: t('app.assets.new'), onclick: startCreating } : undefined}
    templates={drawsWrites && onimport ? [{ label: t('app.assets.create_import'), onclick: onimport }] : []}
  />
{:else}
  <div class="stock" data-inventory-stock>
    <div class="line-row">
      <div class="line" data-inventory-line>
        <QueryLine
          label={t('app.inventory.line')}
          {language}
          {catalogue}
          bind:tokens
          bind:text
          placeholder={t('app.inventory.line_placeholder')}
          suggestionsLabel={t('app.viewcfg.suggestions')}
          fieldsHeading={(typed) => t('app.viewcfg.fields_heading', { typed })}
          valuesHeading={(field) => t('app.viewcfg.values_heading', { field })}
          editLabel={(spelling) => t('app.viewcfg.edit_chip', { spelling })}
          removeLabel={(spelling) => t('app.viewcfg.remove_chip', { spelling })}
          onchange={changed}
          count={countSnippet}
        />
      </div>
      <div class="line-tools">
        <ViewSwitcher label={t('app.inventory.layout')} views={[{ id: 'list', label: t('app.inventory.layout_list'), icon: 'list' }, { id: 'types', label: t('app.inventory.layout_types'), icon: 'layout-grid' }]} selected={layout} onselect={chooseLayout} />
        {#if !viewport.isCompact}
          <Menu label={t('app.inventory.columns')} items={columnItems} onselect={toggleColumn}>
            {#snippet trigger(props)}
              <Button tone="subtle" size="sm" icon="columns-3" data-inventory-columns {...props}>{t('app.inventory.columns')}</Button>
            {/snippet}
          </Menu>
        {/if}
      </div>
    </div>
    {#if lineProblem}<p class="note" role="status" data-line-problem>{lineProblem}</p>{/if}
    {#if attributesNote}<p class="note" data-group-attributes>{attributesNote}</p>{/if}

    <div class="body">
      {#if viewport.isCompact}
        <Select label={t('app.inventory.type_choice')} size="sm" options={treeOptions} value={treeChoice} onchange={(event: Event) => chooseFromTree((event.currentTarget as HTMLSelectElement).value)} />
      {:else}
        {@render treeColumn()}
      {/if}
      <div class="table" bind:this={box} bind:clientWidth={boxWidth}>
        <DataTable label={t('app.inventory.table')} kind="work" {columns} {rows} key={(ci: ConfigurationItem) => ci.id}
          hiddenColumns={shownHidden}
          group={layout === 'types' ? typeOf : undefined}
          foldGroupLabel={(name) => t('app.list.fold_group', { group: name })} unfoldGroupLabel={(name) => t('app.list.unfold_group', { group: name })}
          rowLabel={(ci) => ci.name} titleText={(ci) => ci.name} href={(ci) => `/cis/${ci.id}`}
          sheetDismissLabel={t('app.page.sheet_close')} sheetGrabLabel={t('app.page.sheet_grab')} actionsLabel={t('app.list.acts')}
          more={next || more.length > 0 ? {
            label: t('app.assets.load_more'), busyLabel: t('app.assets.loading_more'), hasMore: Boolean(next), isBusy: isLoadingMore, endLabel: t('app.assets.end'),
            ...(moreFailed ? { failedLabel: moreFailed } : {}), onLoadMore: () => void loadMore(),
          } : undefined}>
          {#snippet cell(ci: ConfigurationItem, column: DataColumn)}
            {#if column.id === 'name'}
              <span class="name" data-ci-row={ci.id}>
                {#if wanting.has(ci.id)}
                  <!-- The mark repeats what *Health* and *Alerts* say in words; the link keeps the object's name alone. -->
                  <span class="mark warn" aria-hidden="true" title={t('app.inventory.wanting')} data-wanting><Icon name="triangle-alert" size="sm" /></span>
                {:else}
                  <span class="mark" aria-hidden="true"><Icon name="shield" size="sm" /></span>
                {/if}
                <span class="title">{ci.name}</span>
              </span>
            {:else if column.id === 'class'}<span class="quiet">{typeOf(ci)}</span>
            {:else if column.id === 'stage'}<StateLozenge label={t(`app.assets.stage.${ci.stage}`)} tone={STAGE_TONE[ci.stage]} />
            {:else if column.id === 'owner'}
              {@const owner = ownerOf(ci)}
              {#if owner}<span class="who">{owner}</span>{:else}<span class="nobody" data-nobody>{t('app.inventory.nobody')}</span>{/if}
            {:else if column.id === 'location'}<span class="quiet">{locationOf(ci) ?? '–'}</span>
            {:else if column.id === 'protection'}
              {@const level = highestProtection(ci.protection)}
              <span class="quiet">{level ? t(`app.inventory.protection.${level}`) : '–'}</span>
            {:else if column.id === 'open'}
              {@const open = work.get(ci.id) ?? 0}
              {#if open > 0}<span data-open-count={open}><Badge size="sm">{open}</Badge></span>{:else}<span class="quiet">–</span>{/if}
            {/if}
          {/snippet}
          {#snippet stack(ci: ConfigurationItem)}
            {@const open = work.get(ci.id) ?? 0}
            <span class="meta" data-ci-meta>
              {[typeOf(ci), t(`app.assets.stage.${ci.stage}`), ownerOf(ci) ?? t('app.inventory.nobody'), ...(open > 0 ? [t('app.inventory.open_short', { count: String(open) })] : [])].join(' · ')}
            </span>
          {/snippet}
          {#snippet empty()}
            <ProductEmpty kind="filtered" icon="funnel" title={t('app.inventory.none_match')} clear={{ label: t('app.inventory.clear_line'), onclick: clearLine }} />
          {/snippet}
        </DataTable>
      </div>
    </div>
  </div>
{/if}

{#snippet countSnippet()}<span data-inventory-count>{t('app.assets.count', { count: rows.length })}</span>{/snippet}

<GroupDialog bind:isOpen={isGroupOpen} {hubId} {classes} {words} {services} {editing}
  prefill={prefill?.ok ? prefill.filter : undefined}
  keptText={keptWords(editing?.filter ?? (prefill?.ok ? prefill.filter : undefined))}
  note={groupNote}
  onsaved={(group) => chooseGroup(group)} />

{#if isCreating}
  <CreateDialog
    bind:isOpen={isCreating}
    title={t('app.assets.new')}
    cancelLabel={t('app.assets.cancel')}
    nameLabel={t('app.assets.name_label')}
    bind:name={draftName}
    maxlength={200}
    place={{ current: draftHub, fallback: containers.find(hubId)?.name, places: placesToMake.list, onchoose: (id) => (draftHub = id) }}
    failure={createFailure}
    primary={{ label: t('app.assets.create'), disabledReason: createReason, onclick: () => void create() }}
  >
    <Select label={t('app.assets.class_label')} options={classOptions} placeholder={t('app.assets.needs_class')} bind:value={draftClass} />
    {#if chosen}
      {#each chosen.all_attributes as attribute (attribute.key)}
        <AttributeField {attribute} bind:value={() => drafts[attribute.key] ?? '', (typed: string) => (drafts[attribute.key] = typed)} />
      {/each}
    {/if}
  </CreateDialog>
{/if}

<style>
  .stock { display: grid; grid-template-columns: minmax(0, 1fr); gap: var(--sp-150); }
  .line-row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .line { flex: 1 1 var(--layout-field-md); min-inline-size: 0; }
  .line-tools { display: flex; align-items: center; gap: var(--sp-100); }
  .note { margin: 0; color: var(--text-secondary); font-size: var(--fs-090); }
  .body { display: grid; grid-template-columns: var(--layout-settings-nav) minmax(0, 1fr); align-items: start; gap: var(--sp-200); }
  .table { min-inline-size: 0; }
  .tree { display: grid; gap: var(--sp-050); }
  .tree-heading { margin: 0; padding-block: var(--sp-050); font-size: var(--fs-090); font-weight: var(--fw-semibold); color: var(--text-secondary); }
  .tree-heading + .tree-rows { margin-block-end: var(--sp-150); }
  .tree-rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-025); }
  .tree-row {
    display: flex; align-items: center; gap: var(--sp-050); inline-size: 100%; min-block-size: var(--layout-nav-row);
    padding-block: 0; padding-inline: var(--sp-100); border: none; border-radius: var(--r-md); background: none;
    font: inherit; font-size: var(--fs-090); color: var(--text-primary); text-align: start; cursor: pointer;
  }
  .tree-row:hover { background: var(--bg-surface-hover); }
  .tree-row[aria-current='true'] { background: var(--accent-primary-subtle); font-weight: var(--fw-semibold); }
  .tree-row:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--bw-ring)); }
  .tree-row[data-depth='1'] { padding-inline-start: var(--sp-300); }
  .tree-row[data-depth='2'] { padding-inline-start: var(--sp-500); }
  .tree-row[data-depth='3'] { padding-inline-start: var(--sp-600); }
  .tree-name { flex: 1 1 auto; min-inline-size: 0; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
  .tree-count { color: var(--text-subtle); font-variant-numeric: tabular-nums; }
  .tree-empty { padding-inline: var(--sp-100); color: var(--text-subtle); font-size: var(--fs-075); }
  .saved-row { display: flex; align-items: center; gap: var(--sp-025); }
  .name { display: inline-flex; align-items: center; gap: var(--sp-100); min-inline-size: 0; max-inline-size: 100%; }
  .mark { display: inline-flex; color: var(--text-subtle); }
  .mark.warn { color: var(--text-warning); }
  .title { font-weight: var(--fw-medium); color: var(--text-primary); }
  /* The title column takes the rest of the row and nothing when the other columns fill it: a
     field's width keeps the name readable, and the person's name wraps to make room. */
  @media (width >= 600px) {
    .table :global(td[data-role='title']) { min-inline-size: var(--layout-field-sm); }
  }
  .quiet { color: var(--text-secondary); }
  .who { white-space: normal; }
  .nobody { color: var(--text-warning); white-space: nowrap; }
  .meta { color: var(--text-secondary); font-size: var(--fs-075); white-space: normal; overflow-wrap: anywhere; }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px), where the tree folds into a choice; a media query cannot read a custom property. */
  @media (width < 600px) {
    .body { grid-template-columns: minmax(0, 1fr); }
    .title { white-space: normal; }
  }
</style>
