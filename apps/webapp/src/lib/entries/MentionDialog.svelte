<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Mentioning an entry in a note or a comment (UC-PLN-01 check 7): one chosen from anywhere the
  // writer can see, written into the text as its address - inline, or as a card. **No title goes
  // into the text** (`links.ts`): every reader of the text resolves the address for themselves.
  // Where the assets module is on, a configuration item is mentioned the same way (UC-AST-01
  // check 11).

  import { Button, Dialog, Radio, Stack, Switch } from '@hubtask/design-system/components';
  import type { ConfigurationItem, WorkItem } from '@hubtask/sync-engine';

  import EntryPicker from './EntryPicker.svelte';
  import CiPicker from '../assets/CiPicker.svelte';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { ciReferenceText } from '../data/cmdb.ts';
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
  let chosenCi = $state<ConfigurationItem | undefined>(undefined);
  let isCard = $state(false);
  let kind = $state<'entry' | 'ci'>('entry');

  const offersCis = $derived((manifest.value?.modules_on ?? []).includes('ASSETS'));
  const ready = $derived(kind === 'ci' ? chosenCi !== undefined : chosen !== undefined);

  function insert() {
    const strength = isCard ? 'CARD' : 'MENTION';
    if (kind === 'ci' && chosenCi) oninsert(ciReferenceText(chosenCi.id, strength));
    else if (kind === 'entry' && chosen) oninsert(referenceText(chosen.id, strength));
    else return;
    isOpen = false;
  }
</script>

{#if isOpen}
  <Dialog
    bind:isOpen
    title={t('app.mentions.picker_title')}
    dismissLabel={t('app.links.cancel')}
    onClose={() => { chosen = undefined; chosenCi = undefined; isCard = false; kind = 'entry'; }}
  >
    <Stack gap="200">
      {#if offersCis}
        <Radio
          label={t('app.mentions.kind')}
          options={[{ value: 'entry', label: t('app.mentions.kind_entry') }, { value: 'ci', label: t('app.mentions.kind_ci') }]}
          bind:value={kind}
        />
      {/if}
      {#if kind === 'ci' && offersCis}
        <CiPicker chosen={chosenCi} onchoose={(ci) => (chosenCi = ci)} />
      {:else}
        <EntryPicker excluded={new Set([selfId])} {chosen} onchoose={(item) => (chosen = item)} />
      {/if}
      <Switch label={t('app.mentions.as_card')} hint={t('app.mentions.as_card_hint')} bind:checked={isCard} />
    </Stack>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.links.cancel')}</Button>
      <Button
        tone="primary"
        data-insert-mention
        disabledReason={ready ? undefined : t('app.links.choose_first')}
        onclick={insert}
      >{t('app.mentions.confirm')}</Button>
    {/snippet}
  </Dialog>
{/if}
