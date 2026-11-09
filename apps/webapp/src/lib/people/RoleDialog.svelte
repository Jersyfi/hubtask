<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A role of the workspace's own, defined or changed (UC-ID-26 check 6): its name, its key under
  // *Advanced*, and the permissions it carries, by area - a Dialog L, since one list of boxes to
  // pick from is the large rung (page-system §5.1).
  //
  // **Only what the reader holds is offered.** The server refuses a role carrying a permission its
  // definer lacks (`access.permission_above_own`), so the boxes offered are what the reader's own
  // explanation at the workspace says they hold. A permission the role already carries beyond that
  // is shown ticked and locked, with its reason, and kept on save.
  //
  // **A role people hold changes through its draft** (ui.md A4): changing one is a draft published
  // with its reach, never a direct write.

  import { tick, untrack } from 'svelte';

  import { Button, CheckboxGroup, Dialog, Disclosure, Input, Stack } from '@hubtask/design-system/components';
  import type { AccessExplanation, CustomRole } from '@hubtask/sync-engine';

  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import DraftPublish from '../settings/DraftPublish.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { announcer } from '../announce.svelte.ts';
  import { customFields } from '../data/customfields.svelte.ts';
  import { chosen, keyFrom, offers, ROLE_KEY, roles, type Offer } from '../data/roles.svelte.ts';
  import { workflows } from '../data/workflows.svelte.ts';
  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { permissionLabel } from './rolewords.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** Open: `new` for a new role, or the role being changed. */
    role: 'new' | CustomRole | undefined;
    onClose: () => void;
  }

  const { role, onClose }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;

  $effect(() =>
    untrack(() => {
      const closers = [roles.open(), customFields.open(), workflows.open()];
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
  const objects = $derived({
    restrictedFields: customFields.of().filter((field) => field.restricted === true).map((field) => field.key),
    hiddenFields: customFields.of().filter((field) => field.read_restricted === true).map((field) => field.key),
    steps: (workflows.list?.data ?? []).flatMap((workflow) =>
      (workflow.published.steps ?? []).map((step) => ({ workflow: workflow.key, step: step.key })),
    ),
  });

  let isOpen = $state(false);
  let name = $state('');
  let key = $state('');
  let isKeyTouched = $state(false);
  let isKeyOpen = $state(false);
  let checked = $state<readonly string[]>([]);
  let missing = $state<Missing>({});
  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  const isNew = $derived(role === 'new');
  const changing = $derived(role !== undefined && role !== 'new' ? role : undefined);

  $effect(() => {
    if (role === undefined) return;
    untrack(() => {
      name = changing?.name ?? '';
      key = changing?.key ?? '';
      checked = changing ? [...changing.permissions] : [];
      isKeyTouched = false;
      isKeyOpen = false;
      missing = {};
      failure = undefined;
      isOpen = true;
    });
  });

  $effect(() => {
    if (isNew && !isKeyTouched) key = keyFrom(name);
  });
  // *Advanced* with the key opens by itself where the name yields no valid key.
  $effect(() => {
    if (isNew && (name.trim() !== '' || missing.key) && !ROLE_KEY.test(key)) isKeyOpen = true;
  });

  const carried = $derived(changing?.permissions ?? []);
  const offered = $derived<Offer[]>(catalogue ? offers(catalogue.catalogue, objects, mine, carried) : []);

  /** The boxes, by the area each permission belongs to. */
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

  const hasChanges = (): boolean => {
    if (role === undefined) return false;
    if (!changing) return name.trim() !== '' || checked.length > 0;
    return name !== changing.name || checked.length !== changing.permissions.length || checked.some((each) => !changing.permissions.includes(each));
  };
  const guard = new DiscardGuard(hasChanges, () => (isOpen = false));

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    // A role people hold changes through its draft below; Enter in its name submits nothing.
    if (!isNew) return;
    const permissions = chosen(offered, new Set(checked), carried);
    missing = check([
      { kind: 'field', key: 'name', code: 'app.forms.required', isFilled: name.trim() !== '' },
      { kind: 'field', key: 'key', code: 'app.roles.key_invalid', isFilled: ROLE_KEY.test(key) },
      { kind: 'field', key: 'permissions', code: 'app.roles.permissions_empty', isFilled: permissions.length > 0 },
    ]);
    if (!isComplete(missing)) {
      await tick();
      focusFirst(document.querySelector('dialog[open]') ?? document);
      return;
    }
    failure = undefined;
    isWriting = true;
    try {
      await roles.create(key, name.trim(), permissions);
      announcer.say(t('app.roles.created_announced'));
      toast.show({ text: t('app.roles.created_toast', { name: name.trim() }), tone: 'success' });
      isOpen = false;
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }
</script>

<Dialog
  size="l"
  title={isNew ? t('app.roles.new_title') : t('app.roles.change_title', { name: changing?.name ?? '' })}
  bind:isOpen
  dismissLabel={t('app.roles.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={isNew ? { label: t('app.roles.create'), isBusy: isWriting, busyLabel: t('app.roles.working'), onclick: () => form?.requestSubmit() } : undefined}
>
  <!-- Drawn while open only: what is offered is worked out from the reader's own access, asked for
       when the dialog opens rather than for every visit of the list. -->
  {#if isOpen}
  <form bind:this={form} onsubmit={save}>
    <Stack gap="150">
      {#if failure}<FailureNote problem={failure} />{/if}
      <Input label={t('app.roles.name')} bind:value={name} isRequired error={missing.name && !name.trim() ? t(missing.name) : undefined} />
      {#if isNew}
        <!-- The key is made from the name (UX-5); it opens by itself where that yields none. -->
        <Disclosure label={t('app.roles.advanced')} tone="quiet" bind:isOpen={isKeyOpen}>
          <Input label={t('app.roles.key')} hint={t('app.roles.key_hint')} bind:value={key} isRequired
            oninput={() => (isKeyTouched = true)}
            error={missing.key && !ROLE_KEY.test(key) ? t(missing.key) : undefined} />
        </Disclosure>
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
      {#if changing}
        {#key changing.key}
          <DraftPublish section="roles" key={changing.key}
            blockedReason={name.trim() === '' || chosen(offered, new Set(checked), carried).length === 0 ? t('app.roles.draft_incomplete') : undefined}
            block={() => ({ name: name.trim(), permissions: chosen(offered, new Set(checked), carried) })}
            onLoad={(saved) => {
              if (typeof saved.name === 'string') name = saved.name;
              if (Array.isArray(saved.permissions)) checked = saved.permissions.map(String);
            }}
            onPublished={() => (isOpen = false)} />
        {/key}
      {/if}
    </Stack>
  </form>
  {/if}
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) isOpen = false; }}>{t('app.roles.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .error { margin: 0; color: var(--text-danger); }
</style>
