<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New webhook* as a Dialog M (page-system §9.3): what the target receives, its address, and the
  // events by group - one box each choosing all of a group -; the events one by one, where it
  // listens and the condition under *Advanced*. The secret is answered once, to the caller.
  //
  // **The event types come from the manifest**, never from a list typed here: a type this build
  // does not emit is refused rather than stored. **The target URL is not validated here**: a
  // private range is refused by the guarded client (T-07), and the refusal is drawn at the field.
  // **The condition is written, not assembled**: the server compiles it and refuses naming where.
  import { Button, Checkbox, Dialog, Disclosure, Input, Select, Stack, Textarea } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { webhooks } from '../data/webhooks.svelte.ts';
  import { WEBHOOK_FORMATS, eventGroups, isChat, subscriptionBody, type WebhookFormat } from '../data/webhooks.ts';
  import { isComplete, type Missing } from '../forms/missing.ts';
  import { marked } from '../forms/press.ts';
  import FailureNote from '../frame/FailureNote.svelte';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import DiscardDialog from '../page/DiscardDialog.svelte';
  import { DiscardGuard } from '../page/discard.svelte.ts';
  import { renderProblem } from '../problem.ts';
  import { eventWord } from './words.svelte.ts';

  interface Props {
    isOpen: boolean;
    scopes: readonly { value: string; label: string }[];
    /** The new subscription's secret, to be shown once; nothing for a chat room, which signs nothing. */
    onCreated: (secret: string | undefined) => void;
  }

  let { isOpen = $bindable(false), scopes, onCreated }: Props = $props();

  let draftUrl = $state('');
  let draftTypes = $state<readonly string[]>([]);
  let draftCondition = $state('');
  let draftScope = $state('TENANT');
  let draftFormat = $state<WebhookFormat>('CLOUDEVENTS');
  let draftRoom = $state('');
  let draftCredential = $state('');
  let draftLocale = $state('');
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let missing = $state<Missing>({});
  let form = $state<HTMLFormElement | undefined>(undefined);

  const formats = $derived(WEBHOOK_FORMATS.map((format) => ({ value: format, label: t(`app.webhooks.format_${format.toLowerCase()}`) })));
  const eventTypes = $derived<readonly string[]>(manifest.value?.event_types ?? []);
  const groups = $derived(eventGroups(eventTypes));

  function reset(): void {
    draftUrl = '';
    draftTypes = [];
    draftCondition = '';
    draftScope = 'TENANT';
    draftFormat = 'CLOUDEVENTS';
    draftRoom = '';
    draftCredential = '';
    draftLocale = '';
    failure = undefined;
    missing = {};
  }

  function close(): void {
    isOpen = false;
  }

  const guard = new DiscardGuard(() => draftUrl.trim() !== '' || draftTypes.length > 0 || draftCondition.trim() !== '' || draftRoom !== '' || draftCredential !== '', close);

  function toggleGroup(types: readonly string[], wanted: boolean): void {
    draftTypes = wanted ? [...new Set([...draftTypes, ...types])] : draftTypes.filter((declared) => !types.includes(declared));
  }

  function toggleType(type: string, wanted: boolean): void {
    draftTypes = wanted ? [...draftTypes, type] : draftTypes.filter((declared) => declared !== type);
  }

  async function subscribe(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    missing = marked(event, [
      { kind: 'field', key: 'url', code: 'app.forms.required', isFilled: draftUrl.trim() !== '' },
      { kind: 'at-least-one', key: 'types', code: 'app.forms.at_least_one', chosen: draftTypes.length },
    ]);
    if (!isComplete(missing)) return;
    failure = undefined;
    isWorking = true;
    try {
      const created = await webhooks.subscribe(
        subscriptionBody({
          target_url: draftUrl,
          event_types: draftTypes,
          condition: draftCondition,
          scope: draftScope,
          format: draftFormat,
          room: draftRoom,
          credential: draftCredential,
          locale: draftLocale,
        }),
      );
      announcer.say(t('app.webhooks.subscribed_announced'));
      reset();
      close();
      // A chat room signs nothing, so there is no secret to show it once.
      onCreated(created.secret || undefined);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isWorking = false;
    }
  }
</script>

<Dialog
  size="m"
  title={t('app.webhooks.add')}
  bind:isOpen
  dismissLabel={t('app.webhooks.cancel')}
  onDismiss={guard.dismiss}
  onClose={reset}
  primary={{ label: t('app.webhooks.subscribe'), isBusy: isWorking, busyLabel: t('app.webhooks.subscribing'), onclick: () => form?.requestSubmit() }}
>
  <form bind:this={form} onsubmit={subscribe}>
    <Stack gap="150">
      {#if failure}
        <FailureNote problem={failure}>
          {#if failure.fields.size > 0}
            <Stack gap="050">{#each [...failure.fields] as [path, message] (path)}<span>{message}</span>{/each}</Stack>
          {/if}
        </FailureNote>
      {/if}
      <Select label={t('app.webhooks.format')} error={failure?.fields.get('/format')} bind:value={draftFormat} options={formats} />
      <Input
        label={t('app.webhooks.target_url')}
        hint={draftFormat === 'MATRIX'
          ? t('app.webhooks.target_url_hint_matrix')
          : isChat(draftFormat)
            ? t('app.webhooks.target_url_hint_room')
            : t('app.webhooks.target_url_hint')}
        bind:value={draftUrl}
        isRequired
        error={missing.url && !draftUrl.trim() ? t(missing.url) : failure?.fields.get('/target_url')}
      />
      {#if draftFormat === 'MATRIX'}
        <Input label={t('app.webhooks.room')} hint={t('app.webhooks.room_hint')} bind:value={draftRoom} error={failure?.fields.get('/room')} spellcheck="false" />
        <Input
          type="password"
          label={t('app.webhooks.credential')}
          hint={t('app.webhooks.credential_hint')}
          revealLabel={t('app.webhooks.reveal')}
          hideLabel={t('app.webhooks.hide')}
          bind:value={draftCredential}
          error={failure?.fields.get('/credential')}
          autocomplete="off"
        />
      {/if}
      {#if isChat(draftFormat)}
        <Input label={t('app.webhooks.locale')} hint={t('app.webhooks.locale_hint')} bind:value={draftLocale} error={failure?.fields.get('/locale')} spellcheck="false" />
      {/if}

      <fieldset
        class="types"
        data-invalid={missing.types && draftTypes.length === 0 ? '' : undefined}
        aria-describedby={missing.types && draftTypes.length === 0 ? 'webhook-types-error' : undefined}
      >
        <legend>{t('app.webhooks.event_types')}</legend>
        {#if missing.types && draftTypes.length === 0}<p class="failure" id="webhook-types-error">{t(missing.types)}</p>{/if}
        {#if eventTypes.length === 0}
          <!-- Nothing is knowable before the manifest is read. -->
          <p class="quiet small">{t('app.webhooks.types_unknown')}</p>
        {:else}
          <div class="groups">
            {#each groups as group (group.id)}
              {@const chosen = group.types.filter((type) => draftTypes.includes(type)).length}
              <Checkbox
                label={t('app.webhooks.group_all', { group: t(`app.webhooks.group.${group.id}`) })}
                hint={t('app.webhooks.group_chosen', { chosen: String(chosen), count: String(group.types.length) })}
                checked={chosen === group.types.length}
                isIndeterminate={chosen > 0 && chosen < group.types.length}
                onchange={(event) => toggleGroup(group.types, (event.currentTarget as HTMLInputElement).checked)}
              />
            {/each}
          </div>
        {/if}
      </fieldset>

      <Disclosure tone="quiet" label={t('app.webhooks.advanced')} description={t('app.webhooks.advanced_says')}>
        <Stack gap="150">
          {#each groups as group (group.id)}
            <fieldset class="types">
              <legend class="group-label">{t(`app.webhooks.group.${group.id}`)}</legend>
              <div class="type-grid">
                {#each group.types as type (type)}
                  <Checkbox label={eventWord(type)} checked={draftTypes.includes(type)} onchange={(event) => toggleType(type, (event.currentTarget as HTMLInputElement).checked)} />
                {/each}
              </div>
            </fieldset>
          {/each}
          <p class="quiet small">{t('app.webhooks.types_note')}</p>
          <Select
            label={t('app.webhooks.scope')}
            hint={t('app.webhooks.scope_hint')}
            error={failure?.fields.get('/scope') ?? failure?.fields.get('/scope/id')}
            bind:value={draftScope}
            options={scopes}
          />
          <Textarea label={t('app.webhooks.condition')} hint={t('app.webhooks.condition_hint')} error={failure?.fields.get('/filter')} bind:value={draftCondition} rows={2} spellcheck="false" />
        </Stack>
      </Disclosure>
      <p class="quiet small">{t('app.webhooks.egress_note')}</p>
    </Stack>
  </form>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => { if (guard.dismiss()) close(); }}>{t('app.webhooks.cancel')}</Button>
  {/snippet}
</Dialog>
<DiscardDialog {guard} />

<style>
  form { margin: 0; }
  .quiet { margin: 0; color: var(--text-secondary); }
  .small { font-size: var(--fs-075); }
  .failure { margin: 0; color: var(--text-danger); font-size: var(--fs-075); }
  .types { margin: 0; padding: 0; border: 0; display: grid; gap: var(--sp-100); min-inline-size: 0; }
  .types legend { padding: 0; margin-block-end: var(--sp-100); font-weight: var(--fw-medium); }
  .types .group-label { font-size: var(--fs-075); color: var(--text-secondary); }
  .groups { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 24ch), 1fr)); gap: var(--sp-100) var(--sp-200); }
  .type-grid { display: grid; grid-template-columns: repeat(auto-fit, minmax(min(100%, 22ch), 1fr)); gap: var(--sp-100) var(--sp-200); }
</style>
