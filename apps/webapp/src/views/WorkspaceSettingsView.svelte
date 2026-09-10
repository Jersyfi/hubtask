<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How the workspace is set up: its name, what its members fall back to, and the one switch that
  // changes how they sign in (F4-01, F4-08).
  //
  // **The three defaults are stated as what they are.** The locale and the zone here are not "the
  // workspace's language" — they are the third link of `i18n-l10n.md` §2's chain: what a member
  // who has set nothing falls back to. Somebody who reads them as "everybody's language" changes
  // them and is surprised that most people see nothing different.
  //
  // **The slug is shown and not editable, with the reason.** It is the subdomain the workspace is
  // reached by; changing it would break every link anybody has bookmarked or pasted, and the
  // contract does not offer it. Showing it without the reason would look like an oversight.
  //
  // **The enforcement switch says what switching it on costs.** It demands a second factor of
  // every `OWNER` and `ADMIN` at their next sign-in — including the person pressing it, who may be
  // the only administrator. That is not a warning invented here: it is what `security.md` §5 says
  // the switch does, and the sentence is in the catalogue.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { localesOf, zoneOptions } from '../lib/data/preferences.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  let displayName = $state('');
  let locale = $state('');
  let zone = $state('');
  let requireAdminTotp = $state(false);
  let isWorking = $state(false);
  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let saved = $state(false);
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
    requireAdminTotp = held.require_admin_totp;
  });

  /** Only what moved. Merge-patch means an absent key changes nothing, so absence is the default. */
  function movement() {
    const held = current;
    if (!held) return {};
    return {
      ...(displayName.trim() !== held.display_name ? { display_name: displayName.trim() } : {}),
      ...(locale !== held.default_locale ? { default_locale: locale } : {}),
      ...(zone !== held.default_time_zone ? { default_time_zone: zone } : {}),
      ...(requireAdminTotp !== held.require_admin_totp
        ? { require_admin_totp: requireAdminTotp }
        : {}),
    };
  }

  const hasChanges = $derived(Object.keys(movement()).length > 0);

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const body = movement();
    // A save with nothing in it would be a request that writes nothing and a version bump that
    // means nothing to whoever reads the trail.
    if (Object.keys(body).length === 0) return;

    isWorking = true;
    failure = undefined;
    saved = false;
    try {
      await workspace.change(body);
      saved = true;
    } catch (cause) {
      failure =
        cause instanceof TransportError
          ? renderProblem(cause, messages)
          : { message: messages.t('errors.internal', {}), fields: new Map(), isServerFault: true };
    } finally {
      isWorking = false;
    }
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.workspace.title')}</h1>

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
      {:else if saved}
        <Banner tone="success">{t('app.workspace.saved')}</Banner>
      {/if}

      <form onsubmit={save}>
        <Stack gap="200">
          <Input
            label={t('app.workspace.name')}
            hint={t('app.workspace.name_hint')}
            bind:value={displayName}
            error={failure?.fields.get('display_name')}
            isRequired
          />

          <!-- Read-only and said so, rather than absent: the address people reach this workspace
               at is a thing they look for on this screen, and leaving it out reads as a gap. -->
          <Stack gap="050">
            <span class="label">{t('app.workspace.slug')}</span>
            <p class="fixed">{current.slug}</p>
            <p class="quiet small">{t('app.workspace.slug_fixed')}</p>
          </Stack>

          {#if locales.length > 0}
            <Select
              label={t('app.workspace.locale')}
              hint={t('app.workspace.locale_hint')}
              bind:value={locale}
              options={locales.map((each) => ({ value: each.locale, label: each.locale }))}
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
              hint={t('app.workspace.zone_hint')}
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

          <Switch
            label={t('app.workspace.require_totp')}
            hint={t('app.workspace.require_totp_hint')}
            bind:checked={requireAdminTotp}
          />

          <div>
            <Button
              type="submit"
              tone="primary"
              isBusy={isWorking}
              busyLabel={t('app.workspace.saving')}
              disabledReason={hasChanges ? undefined : t('app.workspace.nothing_changed')}
            >
              {t('app.workspace.save')}
            </Button>
          </div>
        </Stack>
      </form>
    {/if}
  </Stack>
</div>

<style>
  /* Rule 4: a column that grows with its text and stops before it becomes a line nobody can read. */
  .screen { max-width: 60ch; }

  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .quiet {
    margin: 0;
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    color: var(--text-secondary);
  }

  .small { font-size: var(--fs-075); }

  .label { color: var(--text-primary); font-size: var(--fs-075); font-weight: var(--fw-medium); }

  .fixed { margin: 0; font-family: var(--font-mono); color: var(--text-primary); }

  form { margin: 0; }
</style>
