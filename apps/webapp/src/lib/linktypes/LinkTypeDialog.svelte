<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A new link type, or a change to one, as a Dialog M (page-system §5.1, §9.1): the direction, its
  // two names and which end waits; the key, the limit and loops under *Advanced*. What a type means
  // is fixed once it is made, so a change offers its names only, and a type that comes with Hubtask
  // none at all.
  //
  // **The draft follows the server's rules**: a symmetric type has no waiting end and no limit, and
  // a waiting type forbids loops, so the dialog never offers a combination the server refuses.
  import { Dialog, Disclosure, Input, Select, Stack, Switch, Button } from '@hubtask/design-system/components';
  import type { LinkType } from '@hubtask/sync-engine';

  import { keyFrom } from '../data/entrytypes.svelte.ts';
  import { linkTypes, sideName } from '../data/links.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** Open for a new type (`'new'`) or for this one; closed while undefined. */
    subject: LinkType | 'new' | undefined;
    onClose: () => void;
  }

  const { subject, onClose }: Props = $props();

  interface Draft {
    name: string;
    inverseName: string;
    key: string;
    isKeyTouched: boolean;
    // Strings, as a select list binds them; narrowed when the type is sent.
    direction: string;
    waits: string;
    isOne: boolean;
    areLoopsAllowed: boolean;
  }

  const blank = (): Draft => ({
    name: '', inverseName: '', key: '', isKeyTouched: false, direction: 'DIRECTED', waits: 'NONE', isOne: false, areLoopsAllowed: true,
  });
  const draftOf = (type: LinkType): Draft => ({
    name: type.name ?? '', inverseName: type.inverse_name ?? '', key: type.key, isKeyTouched: true,
    direction: type.direction, waits: type.waits, isOne: type.cardinality === 'ONE', areLoopsAllowed: type.cycles === 'ALLOWED',
  });

  const editing = $derived(subject !== undefined && subject !== 'new' ? subject : undefined);
  let draft = $state<Draft>(blank());
  let opened = $state<LinkType | 'new' | undefined>(undefined);
  $effect(() => {
    if (subject === opened) return;
    opened = subject;
    draft = subject === undefined || subject === 'new' ? blank() : draftOf(subject);
    refusal = undefined;
  });

  let isSaving = $state(false);
  let refusal = $state<string | undefined>(undefined);

  const nameOf = (type: LinkType) => sideName(type, type.key, true, t);
  const isSymmetric = $derived(draft.direction === 'SYMMETRIC');
  const isWaiting = $derived(draft.waits !== 'NONE');
  const isFixed = $derived(editing?.system === true);

  const before = $derived(editing ? draftOf(editing) : blank());
  const guard = new DiscardGuard(() => JSON.stringify(draft) !== JSON.stringify(before), () => onClose());

  function rename(value: string) {
    draft.name = value;
    if (!editing && !draft.isKeyTouched) draft.key = keyFrom(value);
  }

  async function save() {
    const d = draft;
    isSaving = true;
    refusal = undefined;
    try {
      if (editing) {
        const change = {
          ...(d.name.trim() !== (editing.name ?? '') ? { name: d.name.trim() } : {}),
          ...(!isSymmetric && d.inverseName.trim() !== (editing.inverse_name ?? '') ? { inverse_name: d.inverseName.trim() } : {}),
        };
        if (Object.keys(change).length > 0) await linkTypes.change(editing, change);
      } else {
        const position = Math.max(-1, ...linkTypes.all.map((each) => each.position)) + 1;
        await linkTypes.create({
          key: d.key,
          name: d.name.trim(),
          ...(isSymmetric ? {} : { inverse_name: d.inverseName.trim() }),
          direction: d.direction as LinkType['direction'],
          waits: isSymmetric ? 'NONE' : (d.waits as LinkType['waits']),
          cardinality: !isSymmetric && d.isOne ? 'ONE' : 'MANY',
          cycles: isWaiting || !d.areLoopsAllowed ? 'FORBIDDEN' : 'ALLOWED',
          position,
        });
      }
      toast.show({ text: t('app.link_types.saved', { name: d.name.trim() || (editing ? nameOf(editing) : d.key) }), tone: 'success' });
      onClose();
    } catch (error) {
      refusal = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }

  const directionOptions = $derived([
    { value: 'DIRECTED', label: t('app.link_types.direction_directed') },
    { value: 'SYMMETRIC', label: t('app.link_types.direction_symmetric') },
  ]);
  const waitsOptions = $derived([
    { value: 'NONE', label: t('app.link_types.waits_none') },
    { value: 'SOURCE', label: t('app.link_types.waits_source_option') },
    { value: 'TARGET', label: t('app.link_types.waits_target_option') },
  ]);
</script>

<Dialog
  size="m"
  isOpen={subject !== undefined}
  title={editing ? t('app.link_types.edit_title', { name: nameOf(editing) }) : t('app.link_types.new_title')}
  dismissLabel={t('app.link_types.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={isFixed ? undefined : { label: editing ? t('app.link_types.save') : t('app.link_types.create'), isBusy: isSaving, onclick: () => void save() }}
>
  <Stack gap="200">
    {#if refusal}<FailureNote problem={refusal} />{/if}
    {#if isFixed}
      <p class="quiet" data-link-type-fixed>{t('app.link_types.system_fixed')}</p>
    {:else}
      {#if !editing}
        <Select label={t('app.link_types.direction')} options={directionOptions} bind:value={draft.direction} />
      {/if}
      <Input label={t('app.link_types.name')} value={draft.name} isRequired oninput={(event) => rename((event.currentTarget as HTMLInputElement).value)} />
      {#if !isSymmetric}
        <Input label={t('app.link_types.inverse_name')} bind:value={draft.inverseName} isRequired />
      {/if}
      {#if !editing}
        {#if !isSymmetric}
          <Select label={t('app.link_types.waits')} options={waitsOptions} bind:value={draft.waits} />
        {/if}
        <Disclosure label={t('app.link_types.advanced')} tone="quiet">
          <Stack gap="200">
            <Input
              label={t('app.link_types.key')}
              hint={t('app.link_types.key_hint')}
              value={draft.key}
              isRequired
              oninput={(event) => { draft.key = (event.currentTarget as HTMLInputElement).value; draft.isKeyTouched = true; }}
            />
            {#if !isSymmetric}
              <Switch label={t('app.link_types.one_label')} bind:checked={draft.isOne} />
            {/if}
            <Switch
              label={t('app.link_types.loops_label')}
              checked={!isWaiting && draft.areLoopsAllowed}
              onchange={(event: Event) => (draft.areLoopsAllowed = (event.currentTarget as HTMLInputElement).checked)}
              disabledReason={isWaiting ? t('app.link_types.loops_waiting') : undefined}
            />
          </Stack>
        </Disclosure>
        <p class="quiet">{t('app.link_types.fixed_note')}</p>
      {/if}
    {/if}
  </Stack>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) onClose(); }}>{isFixed ? t('app.page.close') : t('app.link_types.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  .quiet { margin: 0; color: var(--text-secondary); }
</style>
