<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *New access token*: a create page at its own address (page-system §5.1, §7.7, §9.5), because
  // more than twelve choices in several groups is past a dialog. *About it* and *What it may do* are
  // regions of setting rows; the scopes are grouped by area with a box per verb (`scopeareas.ts`),
  // the configuration's folded, and no scope's key is read outside *Details* (PS-15), where each
  // scope can also be chosen on its own. The three rules of a token's making are `TokenDraft`'s.
  //
  // **The credential appears once, in `OneTimeSecret`, and this page keeps no copy.** It is held in
  // one `$state` while the page is open and dropped when it closes; nothing writes it to storage, to
  // the address, or to a log.

  import {
    Checkbox,
    CheckboxGroup,
    CreatePage,
    FormLayout,
    FormSection,
    Input,
    OneTimeSecret,
    Radio,
    SettingRow,
    SettingsSection,
    Stack,
  } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import type { MintedToken } from '../lib/data/tokens.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';
  import { focusFirst, isComplete, without } from '../lib/forms/missing.ts';
  import { profileTrail } from '../lib/profile/profilehead.ts';
  import { NO_INSTALLATION, TokenDraft } from '../lib/tokens/draft.svelte.ts';
  import { areaRows, configurationParts, toggleVerb, verbState, type AreaRow } from '../lib/tokens/scopeareas.ts';
  import { scopeWord } from '../lib/tokens/tokenwords.ts';

  const { onnavigate }: { onnavigate?: (path: string) => void } = $props();

  const title = $derived(t('app.tokens.mint_title'));
  $effect(() => page.entitle(title));

  const draft = new TokenDraft();
  let form = $state<HTMLFormElement | null>(null);
  let minted = $state<MintedToken | undefined>(undefined);
  // The credential dies with the page.
  $effect(() => () => (minted = undefined));
  $effect(() => leaveGuard.hold(() => (draft.hasChanges ? 1 : 0)));

  const rows = $derived(areaRows(draft.scopes));
  const plain = $derived(rows.filter((row) => row.area !== 'configuration'));
  const configuration = $derived(rows.find((row) => row.area === 'configuration'));
  const parts = $derived(configurationParts(draft.scopes));

  const verbWord = (verb: string) => t(`app.tokens.verb_${verb}`);

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const made = await draft.mint();
    if (made) minted = made;
    else if (!isComplete(draft.missing)) queueMicrotask(() => focusFirst(form));
  }

  const toList = () => onnavigate?.('/profile/tokens');

  function choose(scopes: readonly string[], isOn: boolean): void {
    draft.chosen = toggleVerb(draft.chosen, scopes, isOn);
    draft.missing = without(draft.missing, 'scopes');
  }
</script>

{#snippet areaRow(row: AreaRow, groupLabel?: string)}
  <SettingRow label={t(`app.tokens.area_${row.area}`)} help={t(`app.tokens.area_${row.area}_help`)} {groupLabel} fieldWidth="md">
    {#snippet control(nameId)}
      <div class="verbs" role="group" aria-labelledby={nameId} data-area={row.area}>
        {#each row.verbs as each (each.verb)}
          {@const state = verbState(draft.chosen, each.scopes)}
          <Checkbox
            label={verbWord(each.verb)}
            checked={state === 'all'}
            isIndeterminate={state === 'some'}
            onchange={(event) => choose(each.scopes, (event.currentTarget as HTMLInputElement).checked)}
          />
        {/each}
      </div>
    {/snippet}
  </SettingRow>
{/snippet}

<CreatePage
  head={{
    title,
    subtitle: t('app.tokens.new_lead'),
    breadcrumb: profileTrail('tokens', t('app.tokens.new_crumb')),
    primary: minted ? undefined : { label: t('app.tokens.mint'), opener: 'create-token', onclick: () => form?.requestSubmit() },
    secondary: minted ? [] : [{ label: t('app.tokens.cancel'), isQuiet: true, onclick: toList }],
  }}
  close={{ label: t('app.tokens.cancel'), onclose: toList }}
>
  {#if minted}
    <Stack gap="200">
      <!-- The only time this value exists anywhere but the server's hash. The acknowledgement is
           required, because a token scrolled past is a token nobody copied. -->
      <OneTimeSecret
        value={minted.token}
        label={t('app.tokens.minted_title')}
        hint={t('app.tokens.minted_hint')}
        revealLabel={t('app.tokens.reveal')}
        hideLabel={t('app.tokens.hide')}
        copyLabel={t('app.tokens.copy')}
        copiedLabel={t('app.tokens.copied')}
        acknowledgementLabel={t('app.tokens.kept')}
        notAcknowledgedReason={t('app.tokens.keep_first')}
        dismissLabel={t('app.tokens.done')}
        onDismiss={() => {
          const id = minted?.id;
          minted = undefined;
          onnavigate?.(id ? `/profile/tokens/${id}` : '/profile/tokens');
        }}
      />
      <!-- The one other thing a token is for, said here because no other screen says it: what a
           calendar client asks for as the password. -->
      <p class="quiet">{t('app.tokens.caldav_note')}</p>
    </Stack>
  {:else}
    <!-- `novalidate`: what is missing is said by the page itself, in its own words, when *Create*
         is pressed - not by the browser's bubble (design-system.md §4, case 3). -->
    <form bind:this={form} onsubmit={create} novalidate>
      <FormLayout place="page">
        {#if draft.failure}<FailureNote problem={draft.failure} />{/if}

        <FormSection title={t('app.tokens.about_title')}>
          <SettingRow label={t('app.tokens.name')} help={t('app.tokens.name_hint')} fieldWidth="md" isRequired>
            {#snippet control(nameId)}
              <Input
                label={t('app.tokens.name')}
                isLabelHidden
                aria-labelledby={nameId}
                bind:value={draft.name}
                isRequired
                maxlength={200}
                oninput={() => (draft.missing = without(draft.missing, 'name'))}
                error={draft.missing.name ? t(draft.missing.name) : undefined}
              />
            {/snippet}
          </SettingRow>
          <!-- `max` as well as `required`: the server refuses more than a year, and being told
               before the round trip is the same number said earlier rather than a second policy. -->
          <SettingRow label={t('app.tokens.expires')} help={t('app.tokens.expires_hint')} fieldWidth="sm" isRequired>
            {#snippet control(nameId)}
              <Input
                label={t('app.tokens.expires')}
                isLabelHidden
                aria-labelledby={nameId}
                bind:value={draft.expiresAt}
                type="date"
                min={draft.today}
                max={draft.latestAllowed}
                isRequired
                oninput={() => (draft.missing = without(draft.missing, 'expires'))}
                error={draft.missing.expires ? t(draft.missing.expires) : undefined}
              />
            {/snippet}
          </SettingRow>
        </FormSection>

        {#if draft.scopes.length === 0}
          <!-- The manifest has not arrived. Offering nothing is honest; offering a list written
               here would be wrong on somebody's installation. -->
          <p class="quiet">{t('app.tokens.scopes_unknown')}</p>
        {:else}
          <SettingsSection
            title={t('app.tokens.may_do_title')}
            description={t('app.tokens.scopes_hint')}
            status={t('app.tokens.chosen_count', { count: String(draft.requested.length) })}
            advanced={configuration || parts.length > 0
              ? { label: t('app.tokens.configuration_title'), description: t('app.tokens.configuration_hint', { count: String(parts.length + (configuration?.verbs.length ?? 0)) }), rows: configurationRows }
              : undefined}
          >
            {#if draft.missing.scopes}<p class="failure" role="alert" data-scopes-missing>{t(draft.missing.scopes)}</p>{/if}
            {#each plain as row (row.area)}{@render areaRow(row)}{/each}
            {#if draft.installationOptions.length > 0}
              <!-- Absent, not disabled, for somebody the register does not name (P-05). -->
              <SettingRow label={t('app.tokens.installation')} fieldWidth="lg">
                {#snippet control()}
                  <Radio
                    label={t('app.tokens.installation')}
                    options={[
                      { value: NO_INSTALLATION, label: t('app.tokens.installation_none'), hint: t('app.tokens.installation_none_hint') },
                      ...draft.installationOptions,
                    ]}
                    bind:value={draft.installation}
                  />
                {/snippet}
              </SettingRow>
            {/if}
          </SettingsSection>

          <!-- Each scope on its own, with its key: what a script's author checks against the
               reference, and the one place a key is read (PS-15). -->
          <FormSection title={t('app.tokens.details_title')} description={t('app.tokens.details_hint')} isFolded>
            <div class="details">
              <CheckboxGroup
                label={t('app.tokens.scopes')}
                options={draft.scopes.map((scope) => ({ value: scope, label: scopeWord(scope), hint: scopeWord(scope) === scope ? undefined : scope }))}
                selected={draft.chosen}
                onChange={(next) => {
                  draft.chosen = [...next];
                  draft.missing = without(draft.missing, 'scopes');
                }}
              />
            </div>
          </FormSection>
        {/if}
      </FormLayout>
    </form>
  {/if}
</CreatePage>

{#snippet configurationRows()}
  {#if configuration}{@render areaRow(configuration)}{/if}
  {#if parts.length > 0}
    <SettingRow label={t('app.tokens.configuration_parts')} help={t('app.tokens.configuration_parts_help')} fieldWidth="lg">
      {#snippet control(nameId)}
        <div class="parts" role="group" aria-labelledby={nameId}>
          {#each parts as part (part)}
            <Checkbox label={scopeWord(part)} checked={draft.chosen.includes(part)} onchange={(event) => choose([part], (event.currentTarget as HTMLInputElement).checked)} />
          {/each}
        </div>
      {/snippet}
    </SettingRow>
  {/if}
{/snippet}

<style>
  form { margin: 0; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .failure { margin: 0; padding: var(--sp-150) var(--sp-250) 0; color: var(--text-danger); }

  .verbs { display: flex; flex-wrap: wrap; gap: var(--sp-100) var(--sp-250); }

  .parts { display: grid; gap: var(--sp-100); }

  .details { padding-block: var(--sp-150); }
</style>
