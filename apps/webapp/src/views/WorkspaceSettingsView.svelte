<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the workspace is set up: its name and what its members fall back to. How they sign
  // in is the sign-in screen's alone (UC-ID-12 check 1).
  //
  // **The three defaults are stated as what they are.** The locale and the zone here are not "the
  // workspace's language" — they are the third link of `i18n-l10n.md` §2's chain: what a member
  // who has set nothing falls back to. Somebody who reads them as "everybody's language" changes
  // them and is surprised that most people see nothing different.
  //
  // **The installation's default is said beside each of them** (UC-INS-09 check 5): what a new
  // workspace started with, and for the week start what applies while this workspace decides
  // none. Only where the installation decided one - a private installation is not a column here.
  //
  // The address is not here: it has a screen of its own, *Address*, with who may change it.

  import { tick, untrack } from 'svelte';

  import { Banner, Input, PageHeader, SaveBar, Select, Spinner, Stack } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { WEEK_STARTS, localesOf, zoneOptions } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { languageName } from '../lib/i18n/locale.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { enterSubmits } from '../lib/forms/enter.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import WriteOnly from '../lib/workspace/WriteOnly.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

  let displayName = $state('');
  let locale = $state('');
  let zone = $state('');
  let weekStart = $state('');
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let form = $state<HTMLFormElement | undefined>(undefined);
  let missing = $state<Missing>({});
  /** Filled from the read once, so that typing is not overwritten by the re-read that follows a save. */
  let filled = false;

  $effect(() => untrack(() => workspace.open()));

  const reading = $derived(workspace.state);
  const current = $derived(workspace.workspace);
  const locales = $derived(localesOf(manifest.value));
  // The workspace's own zone has to be among them, or the control shows nothing selected and
  // the screen says "no zone set" about a workspace that has one.
  const zones = $derived(zoneOptions(current?.default_time_zone));

  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  $effect(() => {
    const held = current;
    if (!held || filled) return;
    filled = true;
    displayName = held.display_name;
    locale = held.default_locale;
    zone = held.default_time_zone;
    weekStart = held.default_week_start ?? '';
  });

  const above = $derived(current?.installation_defaults ?? {});
  /** The day the week starts on while this workspace decides none, in words. */
  const weekStartAbove = $derived(
    above.week_start
      ? t('app.workspace.week_start_installation', { day: t(`app.profile.week_${above.week_start}`) })
      : t('app.workspace.week_start_language'),
  );

  /** Only what moved. Merge-patch means an absent key changes nothing, so absence is the default. */
  function movement() {
    const held = current;
    if (!held) return {};
    return {
      ...(displayName.trim() !== held.display_name ? { display_name: displayName.trim() } : {}),
      ...(locale !== held.default_locale ? { default_locale: locale } : {}),
      ...(zone !== held.default_time_zone ? { default_time_zone: zone } : {}),
      ...(weekStart !== (held.default_week_start ?? '') ? { default_week_start: weekStart } : {}),
    };
  }

  /** How many values moved: the save bar's count, and whether it is there at all. */
  const changedCount = $derived(Object.keys(movement()).length);

  /** Everything back as the workspace holds it. */
  function discard(): void {
    const held = current;
    if (!held) return;
    failure = undefined;
    missing = {};
    displayName = held.display_name;
    locale = held.default_locale;
    zone = held.default_time_zone;
    weekStart = held.default_week_start ?? '';
  }

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const body = movement();
    // A save with nothing in it would be a request that writes nothing and a version bump that
    // means nothing to whoever reads the trail.
    if (Object.keys(body).length === 0) return;
    missing = check([{ kind: 'field', key: 'name', code: 'app.workspace.name_needed', isFilled: displayName.trim() !== '' }]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(form));
      return;
    }

    isWorking = true;
    failure = undefined;
    try {
      await workspace.change(body);
      toast.show({ text: t('app.workspace.saved'), tone: 'success' });
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.workspace.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.workspace.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'workspace', label: t('app.workspace.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

    <Stack gap="300">

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="quiet">
        <Spinner label={t('app.workspace.reading')} />
        <span>{t('app.workspace.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else if current}
      {#if failure}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {/if}

      <WorkspaceGate permission="STRUCTURE">
        <form class="panel" bind:this={form} onsubmit={save} use:enterSubmits>
          <Stack gap="200">
            <Input
              label={t('app.workspace.name')}
              hint={t('app.workspace.name_hint')}
              bind:value={displayName}
              error={(missing.name && !displayName.trim() ? t(missing.name) : undefined) ?? failure?.fields.get('display_name')}
              isRequired
            />


            {#if locales.length > 0}
              <Select
                label={t('app.workspace.locale')}
                hint={above.locale
                  ? `${t('app.workspace.locale_hint')} ${t('app.workspace.installation_default', { value: languageName(above.locale, messages.locale) })}`
                  : t('app.workspace.locale_hint')}
                bind:value={locale}
                options={locales.map((each) => ({ value: each.locale, label: languageName(each.locale, messages.locale) }))}
              />
            {:else}
              <!-- An installation that declares no locales has none to choose between, and the
                   server still refuses one it cannot resolve. -->
              <Input
                label={t('app.workspace.locale')}
                hint={t('app.workspace.locale_free')}
                bind:value={locale}
                error={failure?.fields.get('default_locale')}
              />
            {/if}

            {#if zones.length > 0}
              <Select
                label={t('app.workspace.zone')}
                hint={above.time_zone
                  ? `${t('app.workspace.zone_hint')} ${t('app.workspace.installation_default', { value: above.time_zone })}`
                  : t('app.workspace.zone_hint')}
                bind:value={zone}
                options={zones.map((each) => ({ value: each, label: each }))}
              />
            {:else}
              <Input
                label={t('app.workspace.zone')}
                hint={t('app.workspace.zone_free')}
                bind:value={zone}
                error={failure?.fields.get('default_time_zone')}
              />
            {/if}

            <Select
              label={t('app.workspace.week_start')}
              hint={t('app.workspace.week_start_hint')}
              bind:value={weekStart}
              options={[
                { value: '', label: weekStartAbove },
                ...WEEK_STARTS.map((each) => ({ value: each, label: t(`app.profile.week_${each}`) })),
              ]}
              error={failure?.fields.get('/default_week_start')}
            />
            {#if weekStart === '' && above.week_start}
              <!-- Following the installation: the chip says whose day it is (UC-WRK-32 check 5). A
                   week that follows each person's language is this workspace's own choice. -->
              <p class="origin"><ValueSource source="INSTANCE" value={t(`app.profile.week_${above.week_start}`)} /></p>
            {/if}

            <!-- No second-factor switch here: who needs one is the sign-in rule's, set on the sign-in
                 screen with its step-up, its lock and its direction. A switch here was a second place
                 for the same rule, stored apart from it, and the two disagreed (UC-ID-12 check 1). -->

          </Stack>
          <WriteOnly>
            <SaveBar
              label={t('app.save_bar.label')}
              changesLabel={changedCount > 0 ? t('app.save_bar.changes', { count: String(changedCount) }) : undefined}
              discardLabel={t('app.save_bar.discard')}
              saveLabel={t('app.workspace.save')}
              isSaving={isWorking}
              savingLabel={t('app.workspace.saving')}
              onDiscard={discard}
              onSave={() => form?.requestSubmit()}
            />
          </WriteOnly>
        </form>
      </WorkspaceGate>
    {/if}
    </Stack>
</Stack>

<style>
  .origin { margin: 0; }

  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have `.fields`, and a table or a list has none
     (ADR-0065 decision 2). A 60ch column around the whole of it would be a document's measure
     around a screen that is not a document. */

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }


  /* A form is neither prose nor a table, and it is the third case of ADR-0065 decision 2: an
     input as wide as the region is a target nobody aims at, so the fields carry a measure of
     their own while the lists and tables beside them take the width. */
  form { margin: 0; max-inline-size: 52ch; }

  /* The surface a form stands on, as the other screens of the section draw one: a standalone
     element in the sense of design-system.md rule 1, on the frame's canvas. */
  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
  }
</style>
