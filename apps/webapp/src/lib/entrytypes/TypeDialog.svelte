<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A type's own fields as a Dialog L (page-system §5.1): *The type* - name, description, icon and
  // colour -, *What it does* - a record, completed, dates, a workflow, estimates and what may sit
  // underneath (UC-FLW-01 check 9) -, and its words in other languages folded. Only what changed is
  // sent.
  //
  // A type's description and its texts per language are what a portal reads (UC-SVC-05 check 7):
  // one row per language of the installation the workspace writes, the default text for the rest.
  import { Button, CheckboxGroup, Dialog, Disclosure, Icon, IconButton, Inline, Input, LabelChip, Select, Stack, Switch, Textarea } from '@hubtask/design-system/components';
  import { labelTokens } from '@hubtask/design-system';
  import type { EntryType } from '@hubtask/sync-engine';

  import { manifest } from '../data/capabilities.svelte.ts';
  import { localesOf } from '../data/preferences.ts';
  import { TYPE_ICONS, childTypes, entryTypes, iconOf } from '../data/entrytypes.svelte.ts';
  import { type TranslationRow, languagesLeft, sameTranslations, translationRowsOf, translationsOf } from '../data/entrytypes.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** The type being changed; closed while undefined. */
    type: EntryType | undefined;
    onClose: () => void;
  }

  const { type, onClose }: Props = $props();

  interface Draft {
    name: string;
    description: string;
    translations: TranslationRow[];
    icon: string;
    color: string;
    completable: boolean;
    workflow: boolean;
    record: boolean;
    schedulable: boolean;
    estimable: boolean;
    /** The keys of the level below that may sit underneath; none allows every one (check 9). */
    allowed: string[];
  }

  const draftOf = (each: EntryType): Draft => ({
    name: each.name ?? '', description: each.description ?? '', translations: translationRowsOf(each), icon: each.icon,
    color: each.color_token, completable: each.completable, workflow: each.workflow, record: each.record,
    schedulable: each.schedulable, estimable: each.estimable, allowed: [...each.allowed_children],
  });

  let draft = $state<Draft | undefined>(undefined);
  let opened: EntryType | undefined;
  $effect(() => {
    if (type === opened) return;
    opened = type;
    draft = type ? draftOf(type) : undefined;
    refusal = undefined;
    adding = '';
  });

  let isSaving = $state(false);
  let refusal = $state<string | undefined>(undefined);
  let adding = $state('');

  const levelName = (level: string) => t('app.entry_types.level', { level });
  const nameOf = (each: EntryType) => each.name ?? levelName(each.level);
  const guard = new DiscardGuard(() => type !== undefined && draft !== undefined && JSON.stringify(draft) !== JSON.stringify(draftOf(type)), () => onClose());

  const sameKeys = (a: readonly string[], b: readonly string[]) => a.length === b.length && a.every((key) => b.includes(key));

  async function save() {
    if (!type || !draft) return;
    const d = draft;
    const before = type;
    const behaviours = {
      completable: d.record ? false : d.completable, workflow: d.workflow, record: d.record, schedulable: d.schedulable, estimable: d.estimable,
    };
    isSaving = true;
    refusal = undefined;
    try {
      const change = {
        ...(d.name.trim() !== (before.name ?? '') && d.name.trim() !== '' ? { name: d.name.trim() } : {}),
        ...(d.description.trim() !== (before.description ?? '') ? { description: d.description.trim() } : {}),
        ...(sameTranslations(translationsOf(d.translations), before.translations) ? {} : { translations: translationsOf(d.translations) }),
        ...(d.icon !== before.icon ? { icon: d.icon } : {}),
        ...(d.color !== before.color_token ? { color_token: d.color } : {}),
        ...(Object.fromEntries(
          Object.entries(behaviours).filter(([name, on]) => before[name as keyof typeof behaviours] !== on),
        ) as Partial<typeof behaviours>),
        ...(sameKeys(d.allowed, before.allowed_children) ? {} : { allowed_children: d.allowed }),
      };
      if (Object.keys(change).length > 0) await entryTypes.change(before, change);
      toast.show({ text: t('app.entry_types.saved', { name: d.name.trim() || levelName(before.level) }), tone: 'success' });
      onClose();
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const childOptions = $derived(
    draft && type ? childTypes(entryTypes.all, type.level, draft.allowed).map((each) => ({ value: each.key, label: nameOf(each) })) : [],
  );

  /** A language's own name in the reader's language. */
  function languageName(language: string): string {
    try {
      return new Intl.DisplayNames([messages.locale], { type: 'language' }).of(language) ?? language;
    } catch {
      return language;
    }
  }

  const supported = $derived(localesOf(manifest.value).map((each) => each.locale));
  const addable = $derived(draft ? languagesLeft(supported, draft.translations) : []);

  function addLanguage() {
    const language = adding || addable[0];
    if (!draft || !language) return;
    draft.translations = [...draft.translations, { language, name: '', description: '' }];
    adding = '';
  }

  function changeRow(language: string, change: Partial<TranslationRow>) {
    if (!draft) return;
    draft.translations = draft.translations.map((row) => (row.language === language ? { ...row, ...change } : row));
  }

  const iconOptions = TYPE_ICONS.map((icon) => ({ value: icon, label: icon }));
  const colourOptions = $derived(labelTokens.map((token) => ({ value: token, label: t(`app.colour.${token}`) })));
</script>

<Dialog
  size="l"
  isOpen={type !== undefined}
  title={type ? t('app.entry_types.edit_title', { name: nameOf(type) }) : ''}
  dismissLabel={t('app.entry_types.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={{ label: t('app.entry_types.save'), isBusy: isSaving, onclick: () => void save() }}
>
  {#if type && draft}
    <Stack gap="300">
      {#if refusal}<FailureNote problem={refusal} />{/if}
      <section class="group">
        <h3 class="heading">{t('app.entry_types.group_type')}</h3>
        <Input
          label={t('app.entry_types.name')}
          bind:value={draft.name}
          isRequired={!type.system}
          placeholder={type.system ? levelName(type.level) : undefined}
        />
        <Textarea label={t('app.entry_types.description')} hint={t('app.entry_types.description_hint')} rows={2} maxlength={500} bind:value={draft.description} />
        <Inline gap="200" align="start">
          <Select label={t('app.entry_types.icon')} options={iconOptions} bind:value={draft.icon} />
          <Select label={t('app.entry_types.colour')} options={colourOptions} bind:value={draft.color} />
          <span class="preview" aria-hidden="true">
            <Icon name={iconOf({ icon: draft.icon })} size="sm" />
            <LabelChip name={draft.name || levelName(type.level)} colorToken={draft.color} />
          </span>
        </Inline>
      </section>
      <section class="group">
        <h3 class="heading">{t('app.entry_types.group_behaviour')}</h3>
        {#if !type.system}
          <Switch label={t('app.entry_types.behaviour_record')} hint={t('app.entry_types.behaviour_record_hint')} bind:checked={draft.record} />
        {/if}
        <Switch
          label={t('app.entry_types.behaviour_completable')}
          bind:checked={draft.completable}
          disabledReason={draft.record ? t('app.entry_types.record_not_completable') : undefined}
        />
        <Switch label={t('app.entry_types.behaviour_schedulable')} hint={t('app.entry_types.behaviour_schedulable_hint')} bind:checked={draft.schedulable} />
        <Switch label={t('app.entry_types.behaviour_workflow')} bind:checked={draft.workflow} />
        <Switch label={t('app.entry_types.behaviour_estimable')} bind:checked={draft.estimable} />
        {#if childOptions.length > 0}
          <CheckboxGroup
            label={t('app.entry_types.allowed_children')}
            hint={t('app.entry_types.allowed_children_hint')}
            options={childOptions}
            selected={draft.allowed}
            onChange={(next) => { if (draft) draft.allowed = [...next]; }}
          />
        {/if}
      </section>
      <Disclosure label={t('app.entry_types.translations')} tone="quiet" description={t('app.entry_types.translations_count', { count: String(draft.translations.length) })}>
        <Stack gap="150">
          <p class="quiet">{t('app.entry_types.translations_hint')}</p>
          {#each draft.translations as row (row.language)}
            <div class="language" data-language={row.language}>
              <Inline gap="100" align="end">
                <Input
                  label={t('app.entry_types.translation_name', { language: languageName(row.language) })}
                  value={row.name}
                  maxlength={120}
                  oninput={(event) => changeRow(row.language, { name: (event.currentTarget as HTMLInputElement).value })}
                />
                <IconButton icon="trash" label={t('app.entry_types.translation_remove', { language: languageName(row.language) })}
                  onclick={() => { if (draft) draft.translations = draft.translations.filter((each) => each.language !== row.language); }} />
              </Inline>
              <Textarea
                label={t('app.entry_types.translation_description', { language: languageName(row.language) })}
                rows={2}
                maxlength={500}
                value={row.description}
                oninput={(event) => changeRow(row.language, { description: (event.currentTarget as HTMLTextAreaElement).value })}
              />
            </div>
          {/each}
          {#if addable.length > 0}
            <Inline gap="100" align="end">
              <Select label={t('app.entry_types.translation_language')}
                options={addable.map((language) => ({ value: language, label: languageName(language) }))}
                value={adding || addable[0]}
                onchange={(event: Event) => (adding = (event.currentTarget as HTMLSelectElement).value)} />
              <Button tone="secondary" icon="plus" onclick={addLanguage}>{t('app.entry_types.translation_add')}</Button>
            </Inline>
          {/if}
        </Stack>
      </Disclosure>
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) onClose(); }}>{t('app.entry_types.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
  .group { display: grid; gap: var(--sp-200); }
  .heading { margin: 0; font-size: var(--fs-100); font-weight: var(--fw-semibold); }
  .language { display: grid; gap: var(--sp-100); }
  .preview { display: inline-flex; align-items: center; gap: var(--sp-050); align-self: end; padding-block-end: var(--sp-100); }
</style>
