<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A sender rule - a sender or a domain and the collection its mail goes to - as a Dialog M
  // (page-system §9.3): new from the section's *Add*, the same dialog for one pressed in the table.
  // The rule is written by this dialog's press; the caller writes the rules whole.
  import { tick } from 'svelte';

  import { Button, Dialog, Input, Select, Stack } from '@hubtask/design-system/components';

  import { check, focusFirst, isComplete, type Missing } from '../forms/missing.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import type { SenderRule } from './mailboxes.ts';
  import { DiscardGuard } from '../page/discard.svelte.ts';

  interface Props {
    /** The rule pressed, `null` for a new one, `undefined` while closed. */
    rule: SenderRule | null | undefined;
    collections: readonly { value: string; label: string }[];
    /** Where a new rule's mail goes until somebody chooses: the intake's default. */
    defaultPlace: string;
    isWorking: boolean;
    failure?: string;
    onClose: () => void;
    onsave: (next: SenderRule) => void;
  }

  const { rule, collections, defaultPlace, isWorking, failure, onClose, onsave }: Props = $props();

  let match = $state('');
  let collection = $state('');
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  // Filled each time the dialog opens, from the rule pressed or the defaults of a new one.
  $effect(() => {
    if (rule === undefined) return;
    match = rule?.match ?? '';
    collection = rule?.collection ?? (defaultPlace || (collections[0]?.value ?? ''));
    missing = {};
  });

  const isNew = $derived(rule === null);
  const guard = new DiscardGuard(() => match.trim() !== (rule?.match ?? '') || (rule !== null && rule !== undefined && collection !== rule.collection), () => onClose());

  function submit(event: SubmitEvent): void {
    event.preventDefault();
    missing = check([{ kind: 'field', key: 'match', code: 'app.mail.route_needed', isFilled: match.trim() !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }
    onsave({ match: match.trim(), collection });
  }
</script>

<Dialog
  size="m"
  isOpen={rule !== undefined}
  title={isNew ? t('app.mail.route_new') : t('app.mail.route_edit')}
  dismissLabel={t('app.workspace.cancel')}
  onDismiss={guard.dismiss}
  {onClose}
  primary={{ label: isNew ? t('app.mail.route_add') : t('app.mail.route_save'), isBusy: isWorking, busyLabel: t('app.mail.saving'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={submit} data-mail-route>
    <Stack gap="150">
      <p class="quiet">{t('app.mail.routes_hint')}</p>
      {#if failure}<FailureNote problem={failure} />{/if}
      <Input label={t('app.mail.route_match')} bind:value={match} error={missing.match && !match.trim() ? t(missing.match) : undefined} isRequired />
      <Select label={t('app.mail.route_place')} options={collections} bind:value={collection} />
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) onClose(); }}>{t('app.workspace.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); }
</style>
