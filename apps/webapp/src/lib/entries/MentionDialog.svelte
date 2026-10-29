<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Mentioning an entry in a note or a comment (UC-PLN-01 check 7): one chosen from anywhere the
  // writer can see, written into the text as its address - inline, or as a card. **No title goes
  // into the text** (`links.ts`): every reader of the text resolves the address for themselves.

  import { Button, Dialog, Stack, Switch } from '@hubtask/design-system/components';
  import type { WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from './EntryPicker.svelte';
  import { referenceText } from '../data/links.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    isOpen: boolean;
    /** The entry the text belongs to, which is not offered. */
    selfId: string;
    /** The text to put where the caret was. */
    oninsert: (text: string) => void;
  }

  let { isOpen = $bindable(false), selfId, oninsert }: Props = $props();

  let chosen = $state<WorkItem | undefined>(undefined);
  let isCard = $state(false);

  function insert() {
    if (!chosen) return;
    oninsert(referenceText(chosen.id, isCard ? 'CARD' : 'MENTION'));
    isOpen = false;
  }
</script>

{#if isOpen}
  <Dialog
    bind:isOpen
    title={t('app.mentions.picker_title')}
    dismissLabel={t('app.links.cancel')}
    onClose={() => { chosen = undefined; isCard = false; }}
  >
    <Stack gap="200">
      <EntryPicker excluded={new Set([selfId])} {chosen} onchoose={(item) => (chosen = item)} />
      <Switch label={t('app.mentions.as_card')} hint={t('app.mentions.as_card_hint')} bind:checked={isCard} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.links.cancel')}</Button>
      <Button
        tone="primary"
        data-insert-mention
        disabledReason={chosen ? undefined : t('app.links.choose_first')}
        onclick={insert}
      >{t('app.mentions.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}
