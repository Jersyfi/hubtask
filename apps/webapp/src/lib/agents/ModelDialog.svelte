<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Registering or changing a model agents may use (UC-AGT-03): up to eight fields in one group with
  // the consent said first, so a Dialog L (page-system §5.1). A refusal is said inside.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, Select, Stack, Switch, type SelectOption } from '@hubtask/design-system/components';
  import type { AgentModel, AiSource } from '@hubtask/sync-engine';

  import { agents } from '../data/agents.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { resourceWhen } from '../data/resource.svelte.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  interface Props {
    /** What is open: a model to change, `null` for a new one. */
    opened: AgentModel | null | undefined;
    onclose: () => void;
  }

  const { opened, onclose }: Props = $props();

  interface ModelDraft {
    key: string;
    name: string;
    source: 'OWN' | 'OFFERED';
    offered: string;
    model: string;
    region: AgentModel['region'];
    retention: string;
    retired: boolean;
  }

  const editing = $derived(opened ?? undefined);
  let draft = $state<ModelDraft | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() => {
    const now = opened;
    untrack(() => {
      failure = undefined;
      if (now === undefined) return (draft = undefined);
      draft = {
        key: now?.key ?? '', name: now?.name ?? '', source: now?.source ?? 'OWN', offered: now?.offered_id ?? '',
        model: now?.model ?? '', region: now?.region ?? 'SELF_HOSTED',
        retention: now?.retention_days === null || now?.retention_days === undefined ? '' : String(now.retention_days), retired: now?.retired ?? false,
      };
    });
  });

  const sourceRead = resourceWhen<AiSource>(() => (holdsHere('READ_CONFIGURATION') === true ? '/ai-source' : undefined));
  const offered = $derived(sourceRead.state.status === 'ready' ? sourceRead.state.data.offered : []);
  const sourceOptions = $derived<SelectOption[]>((['OWN', 'OFFERED'] as const).map((each) => ({ value: each, label: t(`app.agents.source.${each}`) })));
  const offeredOptions = $derived<SelectOption[]>(offered.map((each) => ({ value: each.id, label: each.display_name })));
  const regionOptions = $derived<SelectOption[]>((['SELF_HOSTED', 'EEA', 'ADEQUACY', 'ANY'] as const).map((each) => ({ value: each, label: t(`app.agents.region.${each}`) })));

  const whole = (text: string) => (/^\d+$/.test(text.trim()) ? Number.parseInt(text, 10) : Number.NaN);
  const reason = $derived(
    !draft ? undefined
    : !editing && !/^[a-z][a-z0-9_-]{0,63}$/.test(draft.key) ? t('app.agents.model_key_hint')
    : !draft.name.trim() || !draft.model.trim() ? t('app.agents.model_needs_name')
    : draft.source === 'OFFERED' && !draft.offered ? t('app.agents.model_needs_offered')
    : draft.retention.trim() !== '' && Number.isNaN(whole(draft.retention)) ? t('app.agents.retention_hint')
    : undefined,
  );

  async function save() {
    const d = draft;
    if (!d) return;
    const common = {
      name: d.name.trim(), source: d.source, model: d.model.trim(), region: d.region,
      ...(d.source === 'OFFERED' && d.offered ? { offered_id: d.offered } : {}),
      ...(d.retention.trim() !== '' ? { retention_days: whole(d.retention) } : {}),
    };
    isWorking = true;
    failure = undefined;
    try {
      if (editing) {
        await agents.changeModel(editing, { ...common, retired: d.retired });
        toast.show({ text: t('app.agents.model_saved'), tone: 'success' });
      } else {
        await agents.registerModel({ key: d.key, ...common });
        toast.show({ text: t('app.agents.model_registered'), tone: 'success' });
      }
      onclose();
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="l"
  isOpen={opened !== undefined && draft !== undefined}
  title={editing ? t('app.agents.edit_named', { name: editing.name }) : t('app.agents.new_model')}
  dismissLabel={t('app.agents.cancel')}
  onClose={onclose}
>
  {#if draft}
    <Stack gap="150">
      <Banner tone="info">{t('app.agents.model_consent')}</Banner>
      {#if !editing}<Input label={t('app.agents.model_key')} hint={t('app.agents.model_key_hint')} bind:value={draft.key} maxlength={64} />{/if}
      <Input label={t('app.agents.name')} bind:value={draft.name} maxlength={120} />
      <Select label={t('app.agents.col_source')} options={sourceOptions} bind:value={draft.source} />
      {#if draft.source === 'OFFERED'}
        <Select label={t('app.agents.offered')} options={offeredOptions} bind:value={draft.offered} />
      {/if}
      <Input label={t('app.agents.model_name')} bind:value={draft.model} maxlength={200} />
      <Select label={t('app.agents.col_region')} options={regionOptions} bind:value={draft.region} />
      <Input label={t('app.agents.col_retention')} hint={t('app.agents.retention_hint')} inputmode="numeric" bind:value={draft.retention} />
      {#if editing}<Switch label={t('app.agents.retire_model')} bind:checked={draft.retired} />{/if}
      {#if failure}<FailureNote problem={failure} />{/if}
    </Stack>
  {/if}
  {#snippet actions()}
    <Button tone="secondary" onclick={onclose}>{t('app.agents.cancel')}</Button>
    <Button tone="primary" isBusy={isWorking} busyLabel={t('app.agents.working')} disabledReason={reason} onclick={() => void save()}>{t('app.agents.save')}</Button>
  {/snippet}
</Dialog>
