<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How people in this workspace prove who they are.
  //
  // **Eighteen switches, not one.** Every organisation draws the line somewhere else: one wants
  // NIST's answer - length, a blocklist and nothing more - and the next has to satisfy a rule that
  // demands classes, an expiry and a history. Both are configurations of one product rather than
  // two products, so each switch is its own, each is off by default where NIST says it should be,
  // and each carries the sentence that says what it costs.
  //
  // **What the installation decided is shown, not hidden.** A locked switch keeps its value, its
  // control is switched off with the reason (`disabledReason`, which is how this design system
  // says "not for you"), and the reason names *who* locked it. A screen that simply omitted the
  // row would produce a support ticket asking where the setting went.
  //
  // **Asking everybody for a new password is a button, not a field.** It sets a moment, and every
  // password older than it meets the change step at the next sign-in. It is red, it is behind a
  // confirmation, and its sentence says what it does to sessions.

  import { untrack } from 'svelte';

  import { Banner, Button, Dialog, Input, PageHeader, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';

  import { signInPolicy, type LockOrigin, type Setting } from '../lib/data/signinpolicy.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  $effect(() => untrack(() => signInPolicy.open()));

  const policy = $derived(signInPolicy.policy);

  /** What is being edited, filled from the read once so a save's re-read does not fight typing. */
  let draft = $state<Record<string, unknown>>({});
  let filled = false;
  let confirming = $state(false);

  $effect(() => {
    if (!policy || filled) return;
    filled = true;
    draft = {
      min_length: policy.password.min_length.value,
      min_lowercase: policy.password.min_lowercase.value,
      min_uppercase: policy.password.min_uppercase.value,
      min_digits: policy.password.min_digits.value,
      min_symbols: policy.password.min_symbols.value,
      min_classes: policy.password.min_classes.value,
      max_repeat: policy.password.max_repeat.value,
      common_passwords: policy.password.common_passwords.value,
      context_words: policy.password.context_words.value,
      breach_check: policy.password.breach_check.value,
      max_age_days: policy.password.max_age_days.value,
      history_count: policy.password.history_count.value,
      min_age_hours: policy.password.min_age_hours.value,
      mfa_required_for: policy.mfa_required_for.value,
      session_max_days: policy.session.max_days.value,
      session_idle_minutes: policy.session.idle_minutes.value,
      imprint_url: policy.legal.imprint_url.value,
      privacy_url: policy.legal.privacy_url.value,
      terms_url: policy.legal.terms_url.value,
    };
  });

  /** Why a switch cannot be touched here, or nothing where it can. */
  function lockedBecause(lock: LockOrigin): string | undefined {
    if (lock === 'INSTANCE') return t('app.signin_settings.locked_installation');
    if (lock === 'PLAN') return t('app.signin_settings.locked_plan');
    return undefined;
  }

  /** What the level above set, said beside the control that may tighten it. */
  function defaultOf<T>(setting: Setting<T>, off: string): string {
    const shown = setting.installation === null || setting.installation === false ? off : String(setting.installation);
    return t('app.signin_settings.installation_default', { value: shown });
  }

  const numberOf = (value: unknown): number => Number(value ?? 0);

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (await signInPolicy.save(draft)) announcer.say(t('app.signin_settings.saved'));
  }

  async function rotate(): Promise<void> {
    confirming = false;
    if (await signInPolicy.requireNewPasswords()) announcer.say(t('app.signin_settings.rotated'));
  }

  $effect(() => page.entitle(t('app.signin_settings.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.signin_settings.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'sign-in', label: t('app.signin_settings.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

  {#if !policy}
    <p class="quiet">
      <Spinner label={t('app.workspace.reading')} />
      <span>{t('app.workspace.reading')}</span>
    </p>
  {:else}
    <Stack gap="300">
      <p class="lead">{t('app.signin_settings.lead')}</p>

      {#if signInPolicy.problem}
        <Banner tone="danger" title={signInPolicy.problem.message}>
          {#if signInPolicy.problem.reference}{signInPolicy.problem.reference}{/if}
        </Banner>
      {:else if signInPolicy.wasSaved}
        <Banner tone="success">{t('app.signin_settings.saved')}</Banner>
      {/if}

      <form onsubmit={save}>
        <Stack gap="300">
          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.passwords')}</h2>

              <Input
                label={t('app.signin_settings.min_length')}
                hint={defaultOf(policy.password.min_length, '0')}
                type="number"
                value={String(draft['min_length'] ?? '')}
                oninput={(event) => (draft['min_length'] = numberOf((event.currentTarget as HTMLInputElement).value))}
                disabledReason={lockedBecause(policy.password.min_length.lock)}
                error={signInPolicy.problem?.fields.get('min_length')}
              />

              <fieldset class="classes">
                <legend>{t('app.signin_settings.classes')}</legend>
                <p class="quiet small">{t('app.signin_settings.classes_hint')}</p>
                <div class="grid">
                  {#each [['min_lowercase', policy.password.min_lowercase], ['min_uppercase', policy.password.min_uppercase], ['min_digits', policy.password.min_digits], ['min_symbols', policy.password.min_symbols]] as const as [key, setting] (key)}
                    <Input
                      label={t(`app.signin_settings.${key}`)}
                      type="number"
                      size="sm"
                      value={String(draft[key] ?? '')}
                      oninput={(event) => (draft[key] = numberOf((event.currentTarget as HTMLInputElement).value))}
                      disabledReason={lockedBecause(setting.lock)}
                    />
                  {/each}
                </div>
              </fieldset>

              <Input
                label={t('app.signin_settings.min_classes')}
                hint={defaultOf(policy.password.min_classes, '0')}
                type="number"
                value={String(draft['min_classes'] ?? '')}
                oninput={(event) => (draft['min_classes'] = numberOf((event.currentTarget as HTMLInputElement).value))}
                disabledReason={lockedBecause(policy.password.min_classes.lock)}
              />

              <Input
                label={t('app.signin_settings.max_repeat')}
                hint={t('app.signin_settings.max_repeat_hint')}
                type="number"
                value={draft['max_repeat'] === null ? '' : String(draft['max_repeat'] ?? '')}
                oninput={(event) => {
                  const raw = (event.currentTarget as HTMLInputElement).value;
                  draft['max_repeat'] = raw === '' ? null : Number(raw);
                }}
                disabledReason={lockedBecause(policy.password.max_repeat.lock)}
              />

              <Switch
                label={t('app.signin_settings.common_passwords')}
                hint={t('app.signin_settings.adds_a_line')}
                checked={Boolean(draft['common_passwords'])}
                onchange={(event) => (draft['common_passwords'] = (event.currentTarget as HTMLInputElement).checked)}
                disabledReason={lockedBecause(policy.password.common_passwords.lock)}
              />
              <Switch
                label={t('app.signin_settings.context_words')}
                hint={t('app.signin_settings.adds_a_line')}
                checked={Boolean(draft['context_words'])}
                onchange={(event) => (draft['context_words'] = (event.currentTarget as HTMLInputElement).checked)}
                disabledReason={lockedBecause(policy.password.context_words.lock)}
              />
              <Switch
                label={t('app.signin_settings.breach_check')}
                hint={t('app.signin_settings.breach_hint')}
                checked={Boolean(draft['breach_check'])}
                onchange={(event) => (draft['breach_check'] = (event.currentTarget as HTMLInputElement).checked)}
                disabledReason={lockedBecause(policy.password.breach_check.lock)}
              />

              <Input
                label={t('app.signin_settings.max_age_days')}
                hint={t('app.signin_settings.max_age_hint')}
                type="number"
                value={draft['max_age_days'] === null ? '' : String(draft['max_age_days'] ?? '')}
                oninput={(event) => {
                  const raw = (event.currentTarget as HTMLInputElement).value;
                  draft['max_age_days'] = raw === '' ? null : Number(raw);
                }}
                disabledReason={lockedBecause(policy.password.max_age_days.lock)}
              />
              <Input
                label={t('app.signin_settings.history_count')}
                hint={t('app.signin_settings.history_hint')}
                type="number"
                value={String(draft['history_count'] ?? '')}
                oninput={(event) => (draft['history_count'] = numberOf((event.currentTarget as HTMLInputElement).value))}
                disabledReason={lockedBecause(policy.password.history_count.lock)}
              />
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.second_factor')}</h2>
              <Select
                label={t('app.signin_settings.mfa_required_for')}
                hint={t('app.signin_settings.mfa_hint')}
                value={String(draft['mfa_required_for'] ?? 'NONE')}
                onchange={(event) => (draft['mfa_required_for'] = (event.currentTarget as HTMLSelectElement).value)}
                options={[
                  { value: 'NONE', label: t('app.signin_settings.mfa_none') },
                  { value: 'ADMINS', label: t('app.signin_settings.mfa_admins') },
                  { value: 'EVERYONE', label: t('app.signin_settings.mfa_everyone') },
                ]}
                disabledReason={lockedBecause(policy.mfa_required_for.lock)}
              />
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.sessions')}</h2>
              <Input
                label={t('app.signin_settings.session_max_days')}
                hint={defaultOf(policy.session.max_days, '0')}
                type="number"
                value={String(draft['session_max_days'] ?? '')}
                oninput={(event) => (draft['session_max_days'] = numberOf((event.currentTarget as HTMLInputElement).value))}
                disabledReason={lockedBecause(policy.session.max_days.lock)}
              />
              <Input
                label={t('app.signin_settings.session_idle')}
                hint={t('app.signin_settings.session_idle_hint')}
                type="number"
                value={draft['session_idle_minutes'] === null ? '' : String(draft['session_idle_minutes'] ?? '')}
                oninput={(event) => {
                  const raw = (event.currentTarget as HTMLInputElement).value;
                  draft['session_idle_minutes'] = raw === '' ? null : Number(raw);
                }}
                disabledReason={lockedBecause(policy.session.idle_minutes.lock)}
              />
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.legal')}</h2>
              <p class="quiet small">{t('app.signin_settings.legal_hint')}</p>
              <Input
                label={t('app.legal.imprint')}
                value={String(draft['imprint_url'] ?? '')}
                oninput={(event) => (draft['imprint_url'] = (event.currentTarget as HTMLInputElement).value)}
                disabledReason={lockedBecause(policy.legal.imprint_url.lock)}
              />
              <Input
                label={t('app.legal.privacy')}
                value={String(draft['privacy_url'] ?? '')}
                oninput={(event) => (draft['privacy_url'] = (event.currentTarget as HTMLInputElement).value)}
                disabledReason={lockedBecause(policy.legal.privacy_url.lock)}
              />
              <Input
                label={t('app.legal.terms')}
                value={String(draft['terms_url'] ?? '')}
                oninput={(event) => (draft['terms_url'] = (event.currentTarget as HTMLInputElement).value)}
                disabledReason={lockedBecause(policy.legal.terms_url.lock)}
              />
            </Stack>
          </section>

          <div>
            <Button type="submit" tone="primary" isBusy={signInPolicy.isWorking} busyLabel={t('app.workspace.saving')}>
              {t('app.workspace.save')}
            </Button>
          </div>
        </Stack>
      </form>

      <section class="panel">
        <Stack gap="150">
          <h2>{t('app.signin_settings.rotate_title')}</h2>
          <p class="quiet">{t('app.signin_settings.rotate_body')}</p>
          <div>
            <Button tone="danger" onclick={() => (confirming = true)}>
              {t('app.signin_settings.rotate')}
            </Button>
          </div>
        </Stack>
      </section>
    </Stack>
  {/if}
</Stack>

<Dialog
  bind:isOpen={confirming}
  title={t('app.signin_settings.rotate_title')}
  dismissLabel={t('app.dismiss')}
  onClose={() => (confirming = false)}
>
  {#snippet actions()}
    <Button tone="subtle" onclick={() => (confirming = false)}>{t('app.dismiss')}</Button>
    <Button tone="danger" onclick={() => void rotate()}>{t('app.signin_settings.rotate')}</Button>
  {/snippet}
  <p>{t('app.signin_settings.rotate_confirm')}</p>
</Dialog>

<style>
  .panel {
    padding: var(--sp-250);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  form { margin: 0; }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .lead { margin: 0; color: var(--text-secondary); }

  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .classes { margin: 0; padding: 0; border: 0; }

  .classes legend {
    padding: 0;
    color: var(--text-primary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(10ch, 1fr));
    gap: var(--sp-150);
    margin-block-start: var(--sp-100);
  }

</style>
