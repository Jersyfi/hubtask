<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // An entry's amount on its page (UC-WRK-25 check 2): the details row's editor, offered where the
  // collection's structure in force carries AMOUNT for the type - the server's own condition, so
  // the row is absent exactly where a write would be refused (P-05). Empty clears it.

  import { Button, Input, Stack } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import { announcer } from '../announce.svelte.ts';
  import { items } from '../data/items.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    item: WorkItem;
    /** Why the entry may not be changed now, or nothing. */
    disabledReason?: string;
  }

  const { item, disabledReason }: Props = $props();

  let draft = $state('');
  let isSaving = $state(false);
  let failure = $state<string | undefined>(undefined);

  // The field follows the entry until the person types: a re-read after somebody else's change
  // shows their amount.
  $effect(() => {
    draft = item.amount ?? '';
  });

  async function save() {
    isSaving = true;
    failure = undefined;
    try {
      await items.update(item.id, { amount: draft.trim() }, item.version);
      announcer.say(t('app.entries.amount_saved_announced', { title: item.title }));
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
  }
</script>

<Stack gap="150">
  <Input label={t('app.entries.amount_label')} bind:value={draft} maxlength={40} error={failure} {disabledReason} />
  <div>
    <Button size="sm" isBusy={isSaving} busyLabel={t('app.workspace.saving')} {disabledReason} onclick={save}>
      {t('app.entries.save')}
    </Button>
  </div>
</Stack>
