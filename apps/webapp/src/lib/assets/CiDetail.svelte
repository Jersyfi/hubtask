<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One configuration item (UC-AST-01 check 8, UC-AST-05 check 5): its attributes with the source
  // that last wrote each, its relations both ways, its lifecycle and compliance fields, its own
  // protection need beside the effective one, and its impact grouped by depth. A member who may
  // write changes it as the source `manual`; a value a higher-ranked source holds is kept, and the
  // answer's decision names it.
  //
  // **What the reader may never do is not drawn** (P-05): without WRITE_ITEMS there is no edit,
  // no relate and no remove.

  import { untrack } from 'svelte';
  import { Badge, Banner, Button, Checkbox, Input, Select, Spinner, Stack, Textarea, type SelectOption } from '@hubtask/design-system/components';
  import type { CiBacklinkList, CiClass, CiCompliance, CiImpact, CiRelationType, ConfigurationItem, ConfigurationItemList } from '@hubtask/sync-engine';

  import { accounts } from '../data/accounts.svelte.ts';
  import {
    attributeText, changedAttributes, ciReferenceText, cisPath, classWord, cmdb, DIMENSIONS, impactByDepth, PROTECTION_LEVELS, protectionRows, refinements,
    relationRows, relationWord, typeAllows, type Dimension, type Level, type Words,
  } from '../data/cmdb.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { groups } from '../data/groups.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { backlinkHref } from '../data/whiteboards.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import AttributeField from './AttributeField.svelte';
  import CiAssetPart from './CiAssetPart.svelte';

  interface Props {
    ci: ConfigurationItem;
    classes: readonly CiClass[];
    types: readonly CiRelationType[];
    words: Words;
    mayWrite: boolean;
    onnavigate: (path: string) => void;
  }

  const { ci, classes, types, words, mayWrite, onnavigate }: Props = $props();

  const klass = $derived(classes.find((each) => each.key === ci.class));
  const attributes = $derived(klass?.all_attributes ?? []);
  const when = (at: string) => formatDateTime(at, messages.locale);
  const word = (key: string) => (messages.has(`app.assets.attribute.${key}`) ? t(`app.assets.attribute.${key}`) : key);

  async function act<T>(run: () => Promise<T>, done?: string): Promise<T | undefined> {
    try {
      const out = await run();
      if (done) toast.show({ text: done, tone: 'success' });
      return out;
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
      return undefined;
    }
  }

  // The hub's CIs: the names a relation's other end and a location are shown by, and the CIs a
  // relation may be made to. The first page is enough to pick from; an end beyond it is read alone.
  let hubCis = $state<readonly ConfigurationItem[]>([]);
  let alone = $state<Record<string, ConfigurationItem>>({});
  $effect(() => {
    const at = ci.hub_id;
    return untrack(() => {
      const stops = [
        engine.subscribe<ConfigurationItemList>({ path: cisPath(at) }, (state) => {
          if (state.status === 'ready') hubCis = state.data.data;
        }),
        people.open({ hubId: at }),
        groups.open(),
      ];
      return () => stops.forEach((stop) => stop());
    });
  });
  const known = $derived(new Map([...Object.values(alone), ...hubCis].map((each) => [each.id, each])));
  const nameOf = (id: string) => known.get(id)?.name ?? t('app.assets.unknown_ci');

  const rows = $derived(relationRows(ci.id, ci.relations ?? []));
  $effect(() => {
    const unknown = rows.map((row) => row.other).filter((id) => !known.has(id));
    if (unknown.length === 0) return;
    untrack(() => {
      for (const id of new Set(unknown)) {
        void cmdb.read(id).then((read) => {
          if (read) alone = { ...alone, [id]: read };
        });
      }
    });
  });

  const groupOptions = $derived<SelectOption[]>(groups.all.map((each) => ({ value: each.id, label: each.name })));
  const accountOptions = $derived<SelectOption[]>(
    people.candidates({ hubId: ci.hub_id }).map((id) => ({ value: id, label: accounts.nameOf(id) ?? id })),
  );

  // Attributes, the name and a more specific class, changed by hand.
  let isEditing = $state(false);
  let drafts = $state<Record<string, string>>({});
  let draftName = $state('');
  let draftClass = $state('');
  const refined = $derived(refinements(classes, ci.class));

  function startEditing() {
    drafts = Object.fromEntries(attributes.map((each) => [each.key, attributeText(ci.attributes[each.key])]));
    draftName = ci.name;
    draftClass = '';
    isEditing = true;
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
    const answer = await act(() => cmdb.update(ci, body));
    if (!answer) return;
    isEditing = false;
    const kept = answer.decision?.kept ?? [];
    toast.show({
      text: kept.length > 0 ? t('app.assets.kept', { keys: kept.map((each) => t('app.assets.kept_one', { attribute: word(each.attribute), by: each.by })).join(', ') }) : t('app.assets.saved'),
      tone: kept.length > 0 ? 'warning' : 'success',
    });
  }

  // A relation made from this CI's end: the type and the way it reads, offered only where its
  // class pairs join the two (`cmdb.relation_not_allowed` stays the server's answer).
  let relateTarget = $state('');
  let relateChoice = $state('');
  const target = $derived(known.get(relateTarget));
  const targetClass = $derived(target ? classes.find((each) => each.key === target.class) : undefined);
  const relateOptions = $derived<SelectOption[]>(
    !target
      ? []
      : types.flatMap((type) => [
          ...(typeAllows(type, klass, targetClass) ? [{ value: `${type.key}:out`, label: relationWord(type, type.key, true, words) }] : []),
          ...(typeAllows(type, targetClass, klass) ? [{ value: `${type.key}:in`, label: relationWord(type, type.key, false, words) }] : []),
        ]),
  );
  const targetOptions = $derived<SelectOption[]>(hubCis.filter((each) => each.id !== ci.id && each.status !== 'MERGED').map((each) => ({ value: each.id, label: `${each.name} · ${classWord(each.class, classes, words)}` })));

  async function relate() {
    const [type, way] = relateChoice.split(':');
    if (!type || !relateTarget) return;
    const done = await act(
      () => (way === 'out' ? cmdb.relate(ci.hub_id, ci.id, relateTarget, type) : cmdb.relate(ci.hub_id, relateTarget, ci.id, type)),
      t('app.assets.related'),
    );
    if (done) {
      relateTarget = '';
      relateChoice = '';
    }
  }

  // Lifecycle and compliance, with the protection need per dimension.
  let isEditingCompliance = $state(false);
  let stage = $state<NonNullable<CiCompliance['stage']>>('IN_USE');
  let status = $state<NonNullable<CiCompliance['status']>>('OK');
  let ownerAccount = $state('');
  let ownerGroup = $state('');
  let location = $state('');
  let reviewDue = $state('');
  let quantity = $state('1');
  let levels = $state<Record<Dimension, string>>({ c: '', i: '', a: '' });
  let fixed = $state<Record<Dimension, boolean>>({ c: false, i: false, a: false });
  let reason = $state('');
  const locations = $derived<SelectOption[]>(
    hubCis
      .filter((each) => classes.find((one) => one.key === each.class)?.lineage.includes('location'))
      .map((each) => ({ value: each.id, label: each.name })),
  );
  const anyFixed = $derived(DIMENSIONS.some((each) => fixed[each]));

  function startCompliance() {
    stage = ci.stage;
    status = ci.status === 'MERGED' ? 'OK' : ci.status;
    ownerAccount = ci.owner_account_id ?? '';
    ownerGroup = ci.owner_group_id ?? '';
    location = ci.location_id ?? '';
    reviewDue = ci.review_due ?? '';
    quantity = String(ci.quantity);
    levels = { c: ci.protection.c ?? '', i: ci.protection.i ?? '', a: ci.protection.a ?? '' };
    fixed = { c: false, i: false, a: false };
    for (const each of ci.protection.fixed ?? []) fixed[each] = true;
    reason = ci.protection.reason ?? '';
    isEditingCompliance = true;
  }

  async function saveCompliance() {
    const level = (dimension: Dimension) => (levels[dimension] ? (levels[dimension] as Level) : null);
    const body: CiCompliance = {
      stage,
      status,
      owner_account_id: ownerAccount || null,
      owner_group_id: ownerGroup || null,
      location_id: location || null,
      review_due: reviewDue || null,
      quantity: Math.max(1, Number.parseInt(quantity, 10) || 1),
      protection: {
        c: level('c'), i: level('i'), a: level('a'),
        fixed: DIMENSIONS.filter((each) => fixed[each]),
        ...(anyFixed ? { reason: reason.trim() } : {}),
      },
    };
    const done = await act(() => cmdb.setCompliance(ci, body), t('app.assets.saved'));
    if (done) isEditingCompliance = false;
  }

  const levelOptions = $derived<SelectOption[]>([{ value: '', label: t('app.assets.level.none') }, ...PROTECTION_LEVELS.map((each) => ({ value: each, label: t(`app.assets.level.${each}`) }))]);
  const protection = $derived(protectionRows(ci.protection, ci.effective_protection));
  const levelWord = (level: Level | null) => t(`app.assets.level.${level ?? 'none'}`);

  // The impact, read again when the direction changes or the CI does.
  let direction = $state<'DOWNSTREAM' | 'UPSTREAM'>('DOWNSTREAM');
  let impact = $state<CiImpact | undefined>(undefined);
  let impactRefusal = $state<string | undefined>(undefined);
  $effect(() => {
    const id = ci.id;
    const way = direction;
    void ci.version;
    untrack(() => {
      impact = undefined;
      impactRefusal = undefined;
      cmdb.impact(id, way).then(
        (read) => (impact = read),
        (error) => (impactRefusal = renderProblem(error as never, messages).message),
      );
    });
  });
  // What points at the CI, read when asked: a reference is the reader's, judged per source.
  let backlinks = $state<CiBacklinkList['data'] | undefined>(undefined);
  async function showBacklinks() {
    const read = await act(() => cmdb.backlinks(ci.id));
    if (read) backlinks = read;
  }

  async function copyReference() {
    try {
      await navigator.clipboard.writeText(ciReferenceText(ci.id));
      toast.show({ text: t('app.assets.reference_copied'), tone: 'success' });
    } catch {
      // A clipboard the browser refuses says nothing: the CI's address is on screen.
    }
  }

  const depths = $derived(impact ? impactByDepth(impact.reached) : []);
  const services = $derived(new Set(impact?.services ?? []));
</script>

<Stack gap="200">
  <header class="head">
    <span class="meta">{classWord(ci.class, classes, words)}</span>
    <h1 class="title">{ci.name}</h1>
    <span class="facts">
      <Badge size="sm">{t(`app.assets.stage.${ci.stage}`)}</Badge>
      <Badge size="sm" tone={ci.status === 'OK' ? 'neutral' : 'warning'}>{t(`app.assets.status.${ci.status}`)}</Badge>
      <span class="meta">{ci.last_seen_at ? t('app.assets.last_seen', { when: when(ci.last_seen_at) }) : t('app.assets.never_seen')}</span>
    </span>
    <div><Button size="sm" tone="subtle" icon="link" onclick={() => void copyReference()}>{t('app.assets.copy_reference')}</Button></div>
  </header>

  {#if ci.merged_into}
    <Banner tone="info">
      {t('app.assets.merged_into')}
      <Button size="sm" tone="subtle" onclick={() => onnavigate(`/cis/${ci.merged_into}`)}>{t('app.assets.open_survivor')}</Button>
    </Banner>
  {/if}

  <section class="part" aria-labelledby="ci-attributes">
    <div class="part-head">
      <h2 id="ci-attributes" class="heading">{t('app.assets.attributes')}</h2>
      {#if mayWrite && !isEditing && !ci.merged_into}<Button size="sm" tone="secondary" icon="pencil" onclick={startEditing}>{t('app.assets.edit')}</Button>{/if}
    </div>
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
          <AttributeField {attribute} bind:value={drafts[attribute.key]} groups={groupOptions} accounts={accountOptions} />
        {/each}
        <div class="buttons">
          <Button tone="secondary" onclick={() => (isEditing = false)}>{t('app.assets.cancel')}</Button>
          <Button tone="primary" onclick={() => void saveAttributes()}>{t('app.assets.save')}</Button>
        </div>
      </Stack>
    {:else if attributes.length === 0}
      <p class="meta">{t('app.assets.no_attributes')}</p>
    {:else}
      <dl class="pairs">
        {#each attributes as attribute (attribute.key)}
          <dt>{word(attribute.key)}</dt>
          <dd>
            {attributeText(ci.attributes[attribute.key]) || '—'}
            {#if ci.provenance[attribute.key]}<span class="meta">{t('app.assets.from_source', { source: ci.provenance[attribute.key]!.source })}</span>{/if}
          </dd>
        {/each}
      </dl>
    {/if}
  </section>

  <section class="part" aria-labelledby="ci-relations">
    <h2 id="ci-relations" class="heading">{t('app.assets.relations')}</h2>
    {#if rows.length === 0}
      <p class="meta">{t('app.assets.no_relations')}</p>
    {:else}
      <ul class="rows">
        {#each rows as row (row.id)}
          <li class="row">
            <span>
              {relationWord(types.find((each) => each.key === row.type), row.type, row.outgoing, words)}
              <a href={`/cis/${row.other}`}>{nameOf(row.other)}</a>
              <span class="meta">{t('app.assets.from_source', { source: row.source })}</span>
            </span>
            {#if mayWrite}
              {@const relation = ci.relations?.find((each) => each.id === row.id)}
              {#if relation}<Button size="sm" tone="subtle" icon="x" onclick={() => void act(() => cmdb.unrelate(ci.hub_id, relation), t('app.assets.unrelated'))}>{t('app.assets.unrelate')}</Button>{/if}
            {/if}
          </li>
        {/each}
      </ul>
    {/if}
    {#if mayWrite && !ci.merged_into}
      <div class="relate">
        <Select label={t('app.assets.relation_target')} size="sm" options={targetOptions} placeholder={t('app.assets.relation_pick')} bind:value={relateTarget} />
        {#if target}
          {#if relateOptions.length === 0}
            <p class="meta">{t('app.assets.relation_none_fits')}</p>
          {:else}
            <Select label={t('app.assets.relation_type')} size="sm" options={relateOptions} placeholder={t('app.assets.relation_type')} bind:value={relateChoice} />
            <Button size="sm" icon="link" disabledReason={relateChoice ? undefined : t('app.assets.relation_type')} onclick={() => void relate()}>{t('app.assets.relate')}</Button>
          {/if}
        {/if}
      </div>
    {/if}
  </section>

  <section class="part" aria-labelledby="ci-compliance">
    <div class="part-head">
      <h2 id="ci-compliance" class="heading">{t('app.assets.compliance')}</h2>
      {#if mayWrite && !isEditingCompliance && !ci.merged_into}<Button size="sm" tone="secondary" icon="pencil" onclick={startCompliance}>{t('app.assets.edit')}</Button>{/if}
    </div>
    {#if isEditingCompliance}
      <Stack gap="100">
        <Select label={t('app.assets.stage')} options={(['PLANNED', 'ORDERED', 'IN_STOCK', 'IN_USE', 'MAINTENANCE', 'RETIRED', 'DISPOSED'] as const).map((each) => ({ value: each, label: t(`app.assets.stage.${each}`) }))} bind:value={stage} />
        <Select label={t('app.assets.status')} options={(['OK', 'PENDING', 'BLOCKED'] as const).map((each) => ({ value: each, label: t(`app.assets.status.${each}`) }))} bind:value={status} />
        <Select label={t('app.assets.owner_person')} options={[{ value: '', label: t('app.assets.nobody') }, ...accountOptions]} bind:value={ownerAccount} />
        <Select label={t('app.assets.owner_group')} options={[{ value: '', label: t('app.assets.nobody') }, ...groupOptions]} bind:value={ownerGroup} />
        <Select label={t('app.assets.location')} options={[{ value: '', label: t('app.assets.no_location') }, ...locations]} bind:value={location} />
        <Input label={t('app.assets.review_due')} type="date" bind:value={reviewDue} />
        <Input label={t('app.assets.quantity')} inputmode="numeric" bind:value={quantity} />
        <fieldset class="protection">
          <legend>{t('app.assets.protection')}</legend>
          {#each DIMENSIONS as dimension (dimension)}
            <div class="dimension">
              <Select label={t(`app.assets.dimension.${dimension}`)} options={levelOptions} bind:value={levels[dimension]} />
              <Checkbox label={t('app.assets.fixed_dimensions')} bind:checked={fixed[dimension]} />
            </div>
          {/each}
          {#if anyFixed}
            <Textarea label={t('app.assets.fix_reason')} bind:value={reason} maxlength={2000} isRequired />
          {/if}
        </fieldset>
        <div class="buttons">
          <Button tone="secondary" onclick={() => (isEditingCompliance = false)}>{t('app.assets.cancel')}</Button>
          <Button tone="primary" disabledReason={anyFixed && !reason.trim() ? t('app.assets.fix_reason_required') : undefined} onclick={() => void saveCompliance()}>{t('app.assets.save')}</Button>
        </div>
      </Stack>
    {:else}
      <dl class="pairs">
        <dt>{t('app.assets.owner_person')}</dt><dd>{ci.owner_account_id ? (accounts.nameOf(ci.owner_account_id) ?? '—') : '—'}</dd>
        <dt>{t('app.assets.owner_group')}</dt><dd>{ci.owner_group_id ? (groups.nameOf(ci.owner_group_id) ?? '—') : '—'}</dd>
        <dt>{t('app.assets.location')}</dt><dd>{#if ci.location_id}<a href={`/cis/${ci.location_id}`}>{nameOf(ci.location_id)}</a>{:else}—{/if}</dd>
        <dt>{t('app.assets.review_due')}</dt><dd>{ci.review_due ?? '—'}</dd>
        <dt>{t('app.assets.quantity')}</dt><dd>{ci.quantity}</dd>
      </dl>
    {/if}
    <table class="needs">
      <caption>{t('app.assets.protection')}</caption>
      <thead><tr><th scope="col"></th><th scope="col">{t('app.assets.protection_own')}</th><th scope="col">{t('app.assets.protection_effective')}</th></tr></thead>
      <tbody>
        {#each protection as row (row.dimension)}
          <tr>
            <th scope="row">{t(`app.assets.dimension.${row.dimension}`)}</th>
            <td>{levelWord(row.own)}</td>
            <td>
              {levelWord(row.effective)}
              {#if row.raised}<span class="meta">{t('app.assets.raised')}</span>{/if}
              {#if row.fixed}<span class="meta">{t('app.assets.fixed', { reason: ci.protection.reason ?? '' })}</span>{/if}
            </td>
          </tr>
        {/each}
      </tbody>
    </table>
  </section>

  <CiAssetPart {ci} {classes} {hubCis} {mayWrite} />

  <section class="part" aria-labelledby="ci-impact">
    <div class="part-head">
      <h2 id="ci-impact" class="heading">{t('app.assets.impact')}</h2>
      <Select
        label={t('app.assets.impact')}
        size="sm"
        options={[{ value: 'DOWNSTREAM', label: t('app.assets.impact_downstream') }, { value: 'UPSTREAM', label: t('app.assets.impact_upstream') }]}
        bind:value={direction}
      />
    </div>
    {#if impactRefusal}
      <Banner tone="danger">{impactRefusal}</Banner>
    {:else if !impact}
      <Spinner label={t('app.assets.loading')} />
    {:else if depths.length === 0}
      <p class="meta">{t('app.assets.impact_none')}</p>
    {:else}
      {#if direction === 'DOWNSTREAM'}<p class="meta">{t('app.assets.impact_services', { count: impact.services.length })}</p>{/if}
      {#each depths as group (group.depth)}
        <h3 class="depth">{t('app.assets.impact_depth', { depth: group.depth })}</h3>
        <ul class="rows">
          {#each group.items as reached (reached.ci_id)}
            <li class="row">
              <a href={`/cis/${reached.ci_id}`}>{reached.name}</a>
              <span class="facts">
                <span class="meta">{classWord(reached.class, classes, words)}</span>
                {#if services.has(reached.ci_id)}<Badge size="sm" tone="info">{t('app.assets.impact_service')}</Badge>{/if}
              </span>
            </li>
          {/each}
        </ul>
      {/each}
    {/if}
  </section>

  <section class="part" aria-labelledby="ci-backlinks">
    <h2 id="ci-backlinks" class="heading">{t('app.assets.backlinks')}</h2>
    {#if !backlinks}
      <div><Button size="sm" tone="secondary" onclick={() => void showBacklinks()}>{t('app.assets.backlinks_show')}</Button></div>
    {:else if backlinks.length === 0}
      <p class="meta">{t('app.assets.backlinks_none')}</p>
    {:else}
      <ul class="rows">
        {#each backlinks as link (`${link.source_kind}-${link.source_id}`)}
          <li class="row">
            <a href={backlinkHref(link)}>{link.title}</a>
            <span class="meta">{t(`app.assets.backlink_kind.${link.source_kind}`)}</span>
          </li>
        {/each}
      </ul>
    {/if}
  </section>
</Stack>

<style>
  .head { display: grid; gap: var(--sp-050); }
  .title { margin: 0; font-size: var(--fs-400); font-weight: var(--fw-semibold); }
  .part { display: grid; gap: var(--sp-100); }
  .part-head { display: flex; flex-wrap: wrap; align-items: end; justify-content: space-between; gap: var(--sp-100); }
  .heading { margin: 0; font-size: var(--fs-200); font-weight: var(--fw-semibold); }
  .depth { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-medium); color: var(--text-secondary); }
  .pairs { display: grid; grid-template-columns: minmax(0, max-content) minmax(0, 1fr); gap: var(--sp-050) var(--sp-150); margin: 0; }
  .pairs dt { color: var(--text-secondary); }
  .pairs dd { margin: 0; display: flex; flex-wrap: wrap; gap: var(--sp-050); align-items: baseline; overflow-wrap: anywhere; }
  .rows { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-050); }
  .row {
    display: flex; flex-wrap: wrap; align-items: baseline; justify-content: space-between; gap: var(--sp-100);
    padding: var(--sp-100) var(--sp-150); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md);
    background: var(--bg-surface);
  }
  .row a { color: var(--text-primary); font-weight: var(--fw-medium); }
  .facts { display: inline-flex; flex-wrap: wrap; gap: var(--sp-050); align-items: baseline; }
  .relate, .buttons { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .buttons { justify-content: flex-end; }
  .protection { display: grid; gap: var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-md); padding: var(--sp-100) var(--sp-150); margin: 0; }
  .dimension { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: end; }
  .needs { border-collapse: collapse; inline-size: 100%; }
  .needs caption { text-align: start; color: var(--text-secondary); font-size: var(--fs-075); padding-block-end: var(--sp-050); }
  .needs th, .needs td { text-align: start; padding: var(--sp-050) var(--sp-100); border-block-end: var(--bw-hairline) solid var(--border-subtle); }
  .needs td { display: table-cell; }
  .meta { color: var(--text-subtle); font-size: var(--fs-075); margin: 0; }
</style>
