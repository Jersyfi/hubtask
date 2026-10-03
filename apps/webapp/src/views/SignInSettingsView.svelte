<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How people in this workspace prove who they are - the one place every sign-in rule is set
  // (UC-ID-12).
  //
  // **Eighteen switches, not one.** Every organisation draws the line somewhere else: one wants
  // NIST's answer - length, a blocklist and nothing more - and the next has to satisfy a rule that
  // demands classes, an expiry and a history. Both are configurations of one product rather than
  // two products, so each switch is its own, each is off by default where NIST says it should be,
  // and each carries the sentence that says what it costs.
  //
  // **Every rule says where its value came from** (check 4, P-06): set here, set by the
  // installation, or Hubtask's own default - and the installation's value beside it only where the
  // installation decided one. A private installation that decided nothing is not a column on this
  // screen (P-10). A locked rule keeps its value, its control is switched off with the reason
  // (`disabledReason`, which is how this design system says "not for you"), and the reason names
  // *who* locked it.
  //
  // **What the installation forbids is not offered** (check 5): a select does not list a choice
  // looser than the installation's, a number field does not go below it, and a switch the
  // installation requires is shown on with the reason. The server refuses the same things; the
  // screen just does not make anybody ask.
  //
  // **A refusal lands at its rule** (check 8): the server names the switch in the field's path, and
  // the sentence is drawn under that control rather than in a banner at the top of eighteen rows.
  //
  // **Every way in is one row with one switch** (check 6, UC-ID-11 check 8): the password, the
  // workspace's own providers and the ones the installation offers, in one list. A switch there acts
  // at once, behind the step-up - the password's through `methods`, a provider's through `:offer`,
  // the one verb for both kinds - and the provider screen switches nothing any more. The last way in
  // that is on cannot be switched off, here and at the server.
  //
  // **A withdrawal is said where the switch is** (ADR-0076 §2, §4). A provider the installation
  // withdraws carries its date under its switch until then; from the date it has no switch, like
  // one switched off above. And a workspace that the withdrawal left with no way in is told that the
  // password opened again for the accounts that hold one - the server's word, from the same answer
  // the sign-in card reads - until another way is switched on here.
  //
  // **Asking everybody for a new password is a button, not a field.** It sets a moment, and every
  // password older than it meets the change step at the next sign-in. It is red, it is behind a
  // confirmation, and its sentence says what it does to sessions.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, ErrorState, Input, PageHeader, Select, Spinner, Stack, Switch } from '@hubtask/design-system/components';
  import { TransportError } from '@hubtask/sync-engine';

  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import { withdrawalPhase } from '../lib/instance/withdrawal.ts';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { signInPolicy, type LockOrigin, type Setting } from '../lib/data/signinpolicy.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  $effect(() => untrack(() => signInPolicy.open()));
  $effect(() => untrack(() => identityProvider.open()));

  const policy = $derived(signInPolicy.policy);
  const reading = $derived(signInPolicy.state);
  const unreadable = $derived(
    signInPolicy.failure ? renderProblem(signInPolicy.failure, messages) : undefined,
  );

  /** What is being edited, filled from the read once so a save's re-read does not fight typing. */
  let draft = $state<Record<string, unknown>>({});
  /**
   * What the read said, kept so a save can send what moved and nothing else.
   *
   * Two reasons, and either alone would be enough. A switch the installation locked is refused
   * when it is *changed*, and a form that posts all nineteen rows changes every one of them as far
   * as the wire is concerned - so moving one switch on a screen where another is locked answered a
   * refusal naming a switch nobody touched, and wrote neither. And a value sent is a value this
   * workspace has now decided for itself: posting the whole form would pin all eighteen to
   * whatever they happened to be, which is not what somebody who moved one of them asked for.
   */
  let baseline: Record<string, unknown> = {};
  let filled = false;
  let confirming = $state(false);

  $effect(() => {
    if (!policy || filled) return;
    filled = true;
    fill();
  });

  /**
   * Fills the form from the rule the server answered.
   *
   * Called once on arrival and again after every save, because the answer is not always the
   * request: the product's own bounds clamp a value past them - a minimum age of twenty-six hours
   * is held at twenty-four - and a form that kept showing twenty-six would lie about what is in
   * force and send it again on the next save.
   */
  function fill(): void {
    if (!policy) return;
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
      accessibility_url: policy.legal.accessibility_url.value,
    };
    baseline = { ...draft };
  }

  /** The rows that moved, in the shape the contract takes. Empty means there is nothing to save. */
  function moved(): Record<string, unknown> {
    const changes: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(draft)) {
      if (value !== baseline[key]) changes[key] = value;
    }
    return changes;
  }

  /** Why a switch cannot be touched here, or nothing where it can. */
  function lockedBecause(lock: LockOrigin): string | undefined {
    if (lock === 'INSTANCE') return t('app.signin_settings.locked_installation');
    if (lock === 'PLAN') return t('app.signin_settings.locked_plan');
    return undefined;
  }

  /**
   * Why a switch the installation requires cannot be turned off here: the one choice left is on.
   * Shown with its reason rather than hidden, because the rule is in force here all the same.
   */
  function requiredAbove(setting: Setting<boolean>): string | undefined {
    return lockedBecause(setting.lock) ?? (setting.installation ? t('app.signin_settings.required_above') : undefined);
  }

  /** Where a rule's value came from, said in a sentence; the installation's value beside it where it decided one. */
  function originOf<T>(setting: Setting<T>, shown: (value: T) => string): string {
    if (setting.lock !== null) return lockedBecause(setting.lock) ?? '';
    const above = setting.installation_source === 'DEFAULT'
      ? ''
      : ` ${t('app.signin_settings.installation_default', { value: shown(setting.installation) })}`;
    switch (setting.source) {
      case 'WORKSPACE': return `${t('app.signin_settings.source_workspace')}${above}`;
      case 'INSTANCE': return t('app.signin_settings.source_installation');
      case 'PLAN': return t('app.signin_settings.source_plan');
      default: return t('app.signin_settings.source_default');
    }
  }

  /** A number as a reader says it: zero is the word for "off", not the digit. */
  const amount = (value: number): string => (value === 0 ? t('app.signin_settings.off') : String(value));
  const yesNo = (value: boolean): string => (value ? t('app.signin_settings.on') : t('app.signin_settings.off'));
  const mfaLabel = (value: string): string =>
    value === 'EVERYONE' ? t('app.signin_settings.mfa_everyone')
      : value === 'ADMINS' ? t('app.signin_settings.mfa_admins')
        : t('app.signin_settings.mfa_none');
  const link = (value: string): string => (value === '' ? t('app.signin_settings.no_link') : value);

  /** The refusal the server attached to this rule, if it attached one (check 8). */
  function refusalOf(key: string): string | undefined {
    return signInPolicy.problem?.fields.get(`/sign_in_policy/${key}`);
  }

  /** A refusal that names no rule, which is the one kind a banner is for. */
  const general = $derived(
    signInPolicy.problem && signInPolicy.problem.fields.size === 0 ? signInPolicy.problem : undefined,
  );

  /**
   * The choices for who needs a second factor, without the ones looser than the installation's
   * (check 5): under an installation that demands it of administrators, "Nobody" is not offered.
   */
  const RANK = { NONE: 0, ADMINS: 1, EVERYONE: 2 } as const;
  const mfaOptions = $derived(
    (['NONE', 'ADMINS', 'EVERYONE'] as const)
      .filter((value) => !policy || RANK[value] >= RANK[policy.mfa_required_for.installation])
      .map((value) => ({ value, label: mfaLabel(value) })),
  );

  /**
   * A field where zero means off and a smaller number is stricter - the repeat limit, the expiry,
   * the idle bound. Under an installation that set one, off is looser and so is a larger number.
   */
  function capped(setting: Setting<number>): { min?: number; max?: number } {
    return setting.installation > 0 ? { min: 1, max: setting.installation } : {};
  }

  /** Whether "empty is off" is a choice here: not where the installation set a value, because off is looser. */
  const mayBeOff = (setting: Setting<number>): boolean => setting.installation === 0;

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const changes = moved();
    if (Object.keys(changes).length === 0) {
      // Nothing to send. A `PATCH` of nothing would answer a success banner for a save that did
      // not happen, and would ask for the step-up on the way to doing nothing.
      announcer.say(t('app.signin_settings.unchanged'));
      return;
    }
    if (await signInPolicy.save(changes)) {
      // From the answer rather than from the draft: what is in force is the server's word, and a
      // value it bounded is a value this form has to show.
      fill();
      announcer.say(t('app.signin_settings.saved'));
    }
  }

  // The ways in, and how many of them are on. The last one on cannot be switched off.
  const providers = $derived(identityProvider.all);

  // Whether the password is open only as the fallback, as the sign-in card is told it. Started once
  // the manifest says the route is served, which may be after this screen first draws.
  $effect(() => {
    if (!signInRules.isServed) return;
    return untrack(() => signInRules.read());
  });
  const fallback = $derived(signInRules.rules?.password_fallback === true);
  const passwordOn = $derived(policy?.methods.value.includes('PASSWORD') ?? false);
  const waysOn = $derived((passwordOn ? 1 : 0) + providers.filter((one) => one.offered_here === true).length);
  let switching = $state<string | undefined>(undefined);
  /** The last refusal of a provider's switch, with the row it belongs to. */
  let wayFailure = $state<{ id: string; problem: ReturnType<typeof renderProblem> } | undefined>(undefined);

  /** Why a way that is on cannot be switched off: it is the only one left. */
  function lastWay(isOn: boolean): string | undefined {
    return isOn && waysOn <= 1 ? t('app.signin_settings.last_way') : undefined;
  }

  /** Why the password cannot be switched here, where it cannot. */
  function passwordFixed(): string | undefined {
    if (!policy) return undefined;
    const locked = lockedBecause(policy.methods.lock);
    if (locked) return locked;
    if (!passwordOn && !policy.methods.installation.includes('PASSWORD')) return t('app.signin_settings.not_offered_above');
    return lastWay(passwordOn);
  }

  /**
   * The password on or off, through the rule's `methods`. The providers stay a way in as far as the
   * rule goes wherever the installation permits them, and each provider's own switch decides it.
   */
  async function setPassword(on: boolean): Promise<void> {
    if (!policy) return;
    switching = 'password';
    wayFailure = undefined;
    const methods = [
      ...(on ? ['PASSWORD'] : []),
      ...(policy.methods.installation.includes('OIDC') ? ['OIDC'] : []),
    ];
    await signInPolicy.save({ methods });
    switching = undefined;
  }

  /** A provider on or off as a way in here: `:offer`, the one verb for both kinds of row. */
  async function setProvider(id: string, on: boolean): Promise<void> {
    switching = id;
    wayFailure = undefined;
    try {
      await identityProvider.offer(id, on);
    } catch (cause) {
      if (cause instanceof TransportError) wayFailure = { id, problem: renderProblem(cause, messages) };
    } finally {
      switching = undefined;
    }
  }

  async function rotate(): Promise<void> {
    confirming = false;
    if (await signInPolicy.requireNewPasswords()) announcer.say(t('app.signin_settings.rotated'));
  }

  $effect(() => page.entitle(t('app.signin_settings.title')));
</script>

{#snippet origin(text: string)}
  <!-- Where the value came from, under the control it is about. -->
  <p class="origin" data-origin>{text}</p>
{/snippet}

{#snippet countRow(key: string, setting: Setting<number>, label: string, hint?: string, bounds: { min?: number; max?: number } = { min: setting.installation }, emptyIsOff = false)}
  <div data-rule>
    <Input
      {label}
      {hint}
      type="number"
      min={bounds.min}
      max={bounds.max}
      isRequired={emptyIsOff && !mayBeOff(setting)}
      value={emptyIsOff && !draft[key] ? '' : String(draft[key] ?? '')}
      oninput={(event) => {
        const raw = (event.currentTarget as HTMLInputElement).value;
        draft[key] = raw === '' ? 0 : Number(raw);
      }}
      disabledReason={lockedBecause(setting.lock)}
      error={refusalOf(key)}
    />
    {@render origin(originOf(setting, amount))}
  </div>
{/snippet}

{#snippet flagRow(key: string, setting: Setting<boolean>, label: string, hint: string)}
  <div data-rule>
    <Switch
      {label}
      {hint}
      checked={Boolean(draft[key])}
      onchange={(event) => (draft[key] = (event.currentTarget as HTMLInputElement).checked)}
      disabledReason={requiredAbove(setting)}
    />
    {#if refusalOf(key)}<p class="refusal" role="alert">{refusalOf(key)}</p>{/if}
    {@render origin(originOf(setting, yesNo))}
  </div>
{/snippet}

{#snippet linkRow(key: 'imprint_url' | 'privacy_url' | 'terms_url' | 'accessibility_url', label: string)}
  {#if policy}
    <div data-rule>
      <Input
        {label}
        value={String(draft[key] ?? '')}
        oninput={(event) => (draft[key] = (event.currentTarget as HTMLInputElement).value)}
        disabledReason={lockedBecause(policy.legal[key].lock)}
        error={refusalOf(key)}
      />
      {@render origin(originOf(policy.legal[key], link))}
    </div>
  {/if}
{/snippet}

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

  {#if unreadable && !policy}
    <!-- Never a spinner for a read that failed (check 8, P-11): the server's sentence, and a way to
         ask again. -->
    <ErrorState
      title={unreadable.message}
      reference={unreadable.reference}
      retryLabel={t('app.retry')}
      onRetry={() => signInPolicy.retry()}
    />
  {:else if reading.status === 'ready' && !policy}
    <!-- Read, and the workspace answered no rule: an installation wired without the level above,
         where there is nothing to set. Said, rather than drawn as eighteen rows nobody can save. -->
    <Banner tone="info">{t('app.signin_settings.unavailable')}</Banner>
  {:else if !policy}
    <p class="quiet">
      <Spinner label={t('app.workspace.reading')} />
      <span>{t('app.workspace.reading')}</span>
    </p>
  {:else}
    <Stack gap="300">
      <p class="lead">{t('app.signin_settings.lead')}</p>

      {#if general}
        <Banner tone="danger" title={general.message}>
          {#if general.reference}{general.reference}{/if}
        </Banner>
      {:else if signInPolicy.wasSaved}
        <Banner tone="success">{t('app.signin_settings.saved')}</Banner>
      {/if}

      <form onsubmit={save}>
        <Stack gap="300">
          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.passwords')}</h2>

              {@render countRow('min_length', policy.password.min_length, t('app.signin_settings.min_length'))}

              <fieldset class="classes">
                <legend>{t('app.signin_settings.classes')}</legend>
                <p class="quiet small">{t('app.signin_settings.classes_hint')}</p>
                <div class="grid">
                  {#each [['min_lowercase', policy.password.min_lowercase], ['min_uppercase', policy.password.min_uppercase], ['min_digits', policy.password.min_digits], ['min_symbols', policy.password.min_symbols]] as const as [key, setting] (key)}
                    {@render countRow(key, setting, t(`app.signin_settings.${key}`))}
                  {/each}
                </div>
              </fieldset>

              {@render countRow('min_classes', policy.password.min_classes, t('app.signin_settings.min_classes'))}
              {@render countRow('max_repeat', policy.password.max_repeat, t('app.signin_settings.max_repeat'), mayBeOff(policy.password.max_repeat) ? t('app.signin_settings.max_repeat_hint') : undefined, capped(policy.password.max_repeat), true)}
              {@render flagRow('common_passwords', policy.password.common_passwords, t('app.signin_settings.common_passwords'), t('app.signin_settings.adds_a_line'))}
              {@render flagRow('context_words', policy.password.context_words, t('app.signin_settings.context_words'), t('app.signin_settings.adds_a_line'))}
              {@render flagRow('breach_check', policy.password.breach_check, t('app.signin_settings.breach_check'), t('app.signin_settings.breach_hint'))}
              {@render countRow('max_age_days', policy.password.max_age_days, t('app.signin_settings.max_age_days'), mayBeOff(policy.password.max_age_days) ? t('app.signin_settings.max_age_hint') : undefined, capped(policy.password.max_age_days), true)}
              {@render countRow('history_count', policy.password.history_count, t('app.signin_settings.history_count'), t('app.signin_settings.history_hint'))}
              <!-- The eighteenth rule, which had no control until SC-06 (check 7). -->
              {@render countRow('min_age_hours', policy.password.min_age_hours, t('app.signin_settings.min_age_hours'), t('app.signin_settings.min_age_hint'))}
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.second_factor')}</h2>
              <div data-rule>
                <Select
                  label={t('app.signin_settings.mfa_required_for')}
                  hint={t('app.signin_settings.mfa_hint')}
                  value={String(draft['mfa_required_for'] ?? 'NONE')}
                  onchange={(event) => (draft['mfa_required_for'] = (event.currentTarget as HTMLSelectElement).value)}
                  options={mfaOptions}
                  disabledReason={lockedBecause(policy.mfa_required_for.lock)}
                  error={refusalOf('mfa_required_for')}
                />
                {@render origin(originOf(policy.mfa_required_for, mfaLabel))}
              </div>
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2 id="ways">{t('app.signin_settings.ways')}</h2>
              <p class="quiet small">{t('app.signin_settings.ways_hint')}</p>
              {#if fallback}
                <Banner tone="warning" title={t('app.signin_settings.fallback_title')}>
                  {t('app.signin_settings.fallback')}
                </Banner>
              {/if}
              <ul class="ways" aria-labelledby="ways">
                <li class="way" data-rule>
                  <Switch
                    label={t('app.signin_settings.way_password')}
                    hint={t('app.signin_settings.way_password_hint')}
                    checked={passwordOn}
                    onchange={(event) => void setPassword((event.currentTarget as HTMLInputElement).checked)}
                    disabledReason={switching === 'password' ? t('app.signin_settings.switching') : passwordFixed()}
                  />
                  {#if refusalOf('methods')}<p class="refusal" role="alert">{refusalOf('methods')}</p>{/if}
                  {@render origin(originOf(policy.methods, (value) => (value.includes('PASSWORD') ? t('app.signin_settings.on') : t('app.signin_settings.off'))))}
                </li>
                {#each providers as one (one.id)}
                  <li class="way" data-rule>
                    <ProviderMark kind={one.kind} name={one.display_name} />
                    {#if one.scope === 'installation' && withdrawalPhase(one) === 'withdrawn'}
                      <!-- Switched off where it is offered, or withdrawn from its date: not a choice
                           this workspace has, so no switch - a sentence saying so. -->
                      <span class="name">{one.display_name}</span>
                      <Badge tone="warning">{t('app.signin_settings.way_off_above')}</Badge>
                    {:else}
                      <Switch
                        label={one.display_name}
                        hint={one.scope === 'installation' ? t('app.signin_settings.way_offered') : t('app.signin_settings.way_own')}
                        checked={one.offered_here === true}
                        onchange={(event) => void setProvider(one.id, (event.currentTarget as HTMLInputElement).checked)}
                        disabledReason={switching === one.id ? t('app.signin_settings.switching') : lastWay(one.offered_here === true)}
                      />
                      {#if one.scope === 'installation' && withdrawalPhase(one) === 'withdrawing'}
                        <!-- Said under the switch it is about, while there is still time to act. -->
                        <p class="withdrawing">
                          {t(one.offered_here ? 'app.signin_settings.way_withdrawing' : 'app.signin_settings.way_withdrawing_off', {
                            date: formatDateTime(one.withdraw_at ?? '', messages.locale),
                          })}
                        </p>
                      {/if}
                      <!-- A refusal for this switch lands at this row (check 8), not above the list. -->
                      {#if wayFailure && wayFailure.id === one.id}<p class="refusal" role="alert">{wayFailure.problem.message}</p>{/if}
                    {/if}
                  </li>
                {/each}
              </ul>
              <p class="small"><a href="/administration/identity-provider">{t('app.signin_settings.configure_providers')}</a></p>
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.sessions')}</h2>
              {@render countRow('session_max_days', policy.session.max_days, t('app.signin_settings.session_max_days'), undefined, { min: 1, max: policy.session.max_days.installation || undefined })}
              {@render countRow('session_idle_minutes', policy.session.idle_minutes, t('app.signin_settings.session_idle'), mayBeOff(policy.session.idle_minutes) ? t('app.signin_settings.session_idle_hint') : undefined, capped(policy.session.idle_minutes), true)}
            </Stack>
          </section>

          <section class="panel">
            <Stack gap="200">
              <h2>{t('app.signin_settings.legal')}</h2>
              <p class="quiet small">{t('app.signin_settings.legal_hint')}</p>
              {@render linkRow('imprint_url', t('app.legal.imprint'))}
              {@render linkRow('privacy_url', t('app.legal.privacy'))}
              {@render linkRow('terms_url', t('app.legal.terms'))}
              <!-- The fourth link. The sign-in footer has shown it since the card was built, and
                   it was the one of the four with nowhere to set it: a workspace could only have
                   the installation's, whatever its own statement said. -->
              {@render linkRow('accessibility_url', t('app.legal.accessibility'))}
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

  /* Under the control, in the hint's voice: it answers "who set this", which is a fact about the
     rule rather than an instruction. */
  .origin {
    margin: var(--sp-050) 0 0;
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }

  .refusal {
    margin: var(--sp-050) 0 0;
    color: var(--text-danger);
    font-size: var(--fs-075);
  }

  .withdrawing {
    margin: var(--sp-050) 0 0;
    color: var(--text-warning);
    font-size: var(--fs-075);
  }

  .ways { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-150); }

  .way { display: flex; align-items: flex-start; gap: var(--sp-150); }

  .way .name { color: var(--text-primary); }

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
