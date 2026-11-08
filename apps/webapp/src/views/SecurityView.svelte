<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The three things an account's own security is made of: the password, the second factor, and
  // the codes that get somebody back in when the factor is gone.
  //
  // **Changing the password asks for the step-up rather than for the old one.** Presenting the old
  // password *is* what a step-up is; a field for it beside one would be asking twice for a single
  // thing (3.3.7). What the change costs is said before it happens: every other session ends.
  //
  // **The recovery codes are shown the way every other one-time secret in this product is** -
  // `OneTimeSecret`, with reveal, copy and an acknowledgement, which is what the component exists
  // for. A plain list with no way to copy them is a set of ten codes somebody has to retype by
  // hand at the worst possible moment.
  //
  // **Whether one is armed decides what this screen offers.** `GET /accounts/me` answers
  // `has_second_factor`, and everything below the password hangs off it. Before it was answered,
  // this screen could not tell an account with no authenticator from one whose codes had all been
  // spent, and so it showed somebody with no authenticator a red "none left", a button to make new
  // codes the server would refuse to make, and a way to switch off a factor they did not have. An
  // action that cannot be taken is not offered here, in any of the three places - the whole panel
  // is absent, and what stands in its place says what would put it there.
  //
  // **Taking it off asks for the password again**, which is the one case where being signed in is
  // not enough: a stolen session removing the factor is exactly the attack the factor exists
  // against (`security.md` §5). And it says what it costs before it is opened, because "turn it
  // off" on its own names neither the thing nor the consequence.
  //
  // **The sign-in providers are the account's other ways in** (ADR-0078 §1, UC-ID-10 check 8): the
  // ones connected, and *Connect* for each provider this workspace offers that is not - also for an
  // identity whose address is not the account's. Connecting asks for the proof first and then a
  // fresh sign-in at the provider, so the button leaves the page. An account without an address is
  // one no provider reaches (identity.md §4.1): the panel is absent for it, as it is where the
  // workspace offers no provider and none is connected.
  //
  // **Passkeys and security keys are the account's own devices** (identity.md §23, UC-ID-22), a panel
  // of their own beside the password: `PasskeyPanel` lists them and adds one where adding would work.
  //
  // A settings page (page-system §9.5): *Password*, *Passkeys*, *Second factor* and *Other ways in*
  // each at its own address. Changing the password is a Dialog M; setting up the authenticator app
  // is a create page of its own with its steps (`SecondFactorView`).

  import { untrack } from 'svelte';

  import { canCopy } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import { Button, Dialog, EmptyState, OneTimeSecret, SettingRow, SettingsLayout, SettingsSection, Stack, type SectionLink } from '@hubtask/design-system/components';

  import { page } from '../lib/frame/page.svelte.ts';
  import { profileHead, profileTitle } from '../lib/profile/profilehead.ts';
  import AuthenticatorReplacement from '../lib/frame/AuthenticatorReplacement.svelte';
  import PasskeyPanel from '../lib/frame/PasskeyPanel.svelte';
  import PasswordField from '../lib/signin/PasswordField.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { mfa } from '../lib/data/mfa.svelte.ts';
  import { password as passwordApi } from '../lib/data/password.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { identities } from '../lib/data/identities.svelte.ts';
  import { passkeys } from '../lib/data/passkeys.svelte.ts';
  import { isStale } from '../lib/data/passkeys.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    /** The section the address names (`/profile/security/:section`); none is the list. */
    section?: string;
    /** Opens another address, entering it into the history. */
    onnavigate?: (path: string) => void;
    /** Moves between the sections, replacing the address. */
    onreplace?: (path: string) => void;
  }
  const { section, onnavigate, onreplace }: Props = $props();

  const BASE = '/profile/security';
  const title = $derived(profileTitle('security'));
  $effect(() => page.entitle(title));

  const account = $derived(actor.account);

  // Whether a second factor is armed. The bundle ships inside the binary that answers this
  // (ADR-0028), so the field is never missing from a server this client is talking to; `=== true`
  // is nevertheless what is asked, because the permissive reading of an unanswered question is
  // what put three refusals on this screen.
  const armed = $derived(actor.hasAuthenticatorApp);
  // Whether any second factor stands - the app or a passkey. The recovery codes back whichever the
  // account holds (identity.md §9), so their count and *New codes* hang off this, not off the app.
  const factor = $derived(actor.hasSecondFactor === true);
  // Whether a passkey of this workspace's address stands beside the app: then the codes stay when
  // the app goes, and the account is not left without a second factor.
  const keyHeld = $derived.by(() => {
    const list = passkeys.list;
    return list !== undefined && list.passkeys.some((each) => !isStale(each, list));
  });

  const isOn = $derived(armed === true);

  // How many recovery codes are left. Answered only where a factor stands, so it needs no defence
  // of its own: the section that reads it is drawn only then.
  const left = $derived(actor.recoveryCodesLeft);

  let newPassword = $state('');
  // One per panel: a sentence is drawn beside the thing it is about, never in the panel below it.

  // The rules this account's workspace applies, so the list under the field is this workspace's.
  $effect(() => signInRules.read());
  $effect(() => untrack(() => identities.open()));

  // The providers offered here that this account is not connected to - what *Connect* is drawn for.
  const connectable = $derived(
    identities.isRead
      ? signInRules.providers.filter((offered) => !identities.connected.some((held) => held.provider_id === offered.id))
      : [],
  );
  // Whether the panel is drawn at all: an account with an address, and something to show.
  const showsProviders = $derived(
    Boolean(account?.email) && (identities.connected.length > 0 || connectable.length > 0),
  );

  /**
   * What just landed, as a passing message the frame also says once - not a line in the panel,
   * which pushed everything under it down (design-system.md §4, case 7).
   */
  function confirm(text: string): void {
    toast.show({ text, tone: 'success' });
  }

  let isChanging = $state(false);
  let passwordForm = $state<HTMLFormElement | undefined>(undefined);

  async function changePassword(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const candidate = newPassword;
    newPassword = '';
    if (await passwordApi.change(candidate)) {
      isChanging = false;
      confirm(t('app.password.changed'));
    }
  }

  const sections = $derived([
    { id: 'password', code: 'app.password.change_title' },
    { id: 'passkeys', code: 'app.passkeys.title' },
    { id: 'second-factor', code: 'app.mfa.title' },
    ...(showsProviders ? [{ id: 'providers', code: 'app.identities.title' }] : []),
  ]);
  const isKnown = (id: string | undefined) => sections.some((each) => each.id === id);
  const current = $derived(isKnown(section) ? (section as string) : 'password');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');
  const links = $derived<SectionLink[]>(
    sections.map((each) => ({
      id: each.id,
      label: t(each.code),
      href: `${BASE}/${each.id}`,
      status: each.id === 'second-factor' ? (isOn ? t('app.mfa.state_on') : t('app.mfa.state_off')) : undefined,
    })),
  );

  async function newCodes(): Promise<void> {
    if (await mfa.regenerate()) {
      announcer.say(t('auth.recovery_regenerated'));
      // The count moved. Nothing pushes it, so it is read again rather than guessed at.
      await actor.reread();
    }
  }

  async function replaced(): Promise<void> {
    confirm(t('app.mfa.replaced'));
    await actor.reread();
  }

  async function disableFactor(): Promise<void> {
    if (await mfa.disable()) {
      confirm(t('app.mfa.disabled'));
      await actor.reread();
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <SettingsLayout
    head={profileHead('security')}
    nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current, onselect: (next: string) => onreplace?.(`${BASE}/${next}`) }}
    {view}
    back={{ label: title, onback: () => onreplace?.(BASE) }}
  >
    {#if current === 'password'}
      <div data-setting="password">
        <SettingsSection title={t('app.password.change_title')} description={actor.hasPassword ? t('app.password.change_body') : undefined}>
          {#if !actor.hasPassword}
            <!-- An account that signs in only through a provider: there is no password to change,
                 and offering to would be offering a control the server cannot honour (UC-ID-05
                 check 5). -->
            <EmptyState kind="unused" title={t('app.password.none')} />
          {:else}
            <SettingRow label={t('app.password.row')} help={t('app.password.row_help')}>
              {#snippet control()}
                <Button tone="secondary" data-opener="change-password" onclick={() => (isChanging = true)}>{t('app.password.change_open')}</Button>
              {/snippet}
            </SettingRow>
          {/if}
        </SettingsSection>
      </div>
    {:else if current === 'passkeys'}
      <PasskeyPanel />
    {:else if current === 'providers' && showsProviders}
      <div data-testid="sign-in-providers">
        <SettingsSection title={t('app.identities.title')} description={t('app.identities.body')}>
          <div class="pad">
            <Stack gap="150">
          {#if identities.failure}
            <FailureNote problem={identities.failure}>
              {#if identities.failure.reference}{identities.failure.reference}{/if}
            </FailureNote>
          {/if}
          {#if identities.connected.length > 0}
            <ul class="identities">
              {#each identities.connected as held (held.provider_id)}
                <li>
                  {t('app.identities.connected', {
                    provider: held.provider_name,
                    since: formatDateTime(held.linked_at, messages.locale),
                  })}
                </li>
              {/each}
            </ul>
          {/if}
          {#each connectable as offered (offered.id)}
            <div>
              <Button
                tone="secondary"
                isBusy={identities.isWorking}
                busyLabel={t('app.identities.connecting')}
                onclick={() => identities.connect(offered.id)}
              >
                {t('app.identities.connect', { provider: offered.display_name })}
              </Button>
            </div>
          {/each}
            </Stack>
          </div>
        </SettingsSection>
      </div>
    {:else}
      <!-- The factor and the codes under one heading, because the codes back the factor: they come
           with the first one, they stop working when the last one goes. The codes hang off any
           factor - the app or a passkey - and the app's own controls off the app alone. -->
      <div data-setting="second-factor">
        <SettingsSection title={t('app.mfa.title')} description={isOn ? t('app.mfa.on') : factor ? t('app.mfa.off_beside_key') : t('app.mfa.off')}>
          <div class="pad">
            <Stack gap="200">
        {#if factor}
          <div class="section">
            <Stack gap="150">
              <h3>{t('app.recovery.title')}</h3>
              {#if mfa.fresh}
                <OneTimeSecret
                  value={mfa.fresh.join('\n')}
                  label={t('app.recovery.title')}
                  hint={t('app.mfa.recovery_hint')}
                  revealLabel={t('app.recovery.reveal')}
                  hideLabel={t('app.recovery.hide')}
                  copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
                  copiedLabel={t('app.recovery.copied')}
                  acknowledgementLabel={t('app.recovery.acknowledge')}
                  notAcknowledgedReason={t('app.recovery.not_acknowledged')}
                  dismissLabel={t('app.recovery.dismiss')}
                  onDismiss={() => mfa.forget()}
                />
              {:else}
                <!-- Zero is the number to act on, so it is the one said in the danger tone - and
                     here it is always a number somebody can act on, because the button beside it
                     works. -->
                {#if left === 0}
                  <FailureNote problem={t('app.recovery.none_left')} />
                {:else if left !== undefined}
                  <p class="quiet">{t('app.recovery.left', { count: String(left), total: '10' })}</p>
                {/if}
                <p class="quiet">{t('app.recovery.regenerate_hint')}</p>
                <div>
                  <Button
                    tone="secondary"
                    isBusy={mfa.isWorking}
                    busyLabel={t('app.recovery.regenerating')}
                    onclick={() => void newCodes()}
                  >
                    {t('app.recovery.regenerate')}
                  </Button>
                </div>
              {/if}
            </Stack>
          </div>

        {/if}

        {#if armed}
          <div class="section">
            <!-- Offered whenever a factor is on, also where the workspace requires one: the old
                 factor works until the new app confirms the swap, so there is never a moment
                 without a factor. A section rather than a disclosure (ux-inventory.md §E7): the
                 recovery code's note links here, and what it asks for is in sight on arrival. -->
            <Stack gap="150">
              <h3>{t('app.mfa.replace_summary')}</h3>
              <AuthenticatorReplacement onreplaced={() => void replaced()} />
            </Stack>
          </div>

          <div class="section">
            {#if !actor.authenticatorAppRemovable}
              <!-- Not a switch the server would refuse (P-05): the workspace's rule demands a
                   factor of this person and the app is the last one, and the sentence says so where
                   the control would be. -->
              <p class="quiet">{t('app.mfa.required_by_workspace')}</p>
            {:else}
            <!-- Offered wherever the rule does not require the factor, to every account: the proof
                 is the step-up's, with whatever the account holds (ADR-0075 §3), so an account
                 without a password is not turned away here. -->
            <Stack gap="150">
              <h3>{t('app.mfa.disable_summary')}</h3>
              <Stack gap="150">
                <p class="quiet">{keyHeld ? t('app.mfa.disable_body_beside_key') : t('app.mfa.disable_body')}</p>
                <p class="quiet">{t('app.mfa.disable_hint')}</p>
                <div>
                  <Button tone="danger" isBusy={mfa.isWorking} busyLabel={t('app.mfa.disabling')} onclick={() => void disableFactor()}>
                    {t('app.mfa.disable')}
                  </Button>
                </div>
              </Stack>
            </Stack>
            {/if}
          </div>
        {:else}
          <SettingRow label={t('app.mfa.app_row')} help={t('app.mfa.intro')}>
            {#snippet control()}
              <Button tone="secondary" onclick={() => onnavigate?.(`${BASE}/second-factor/new`)}>{t('app.mfa.set_up')}</Button>
            {/snippet}
          </SettingRow>
          <!-- Said where the codes would be, so that "where are my recovery codes" has an answer on
               the screen rather than a button that refuses. -->
          {#if !factor}<p class="quiet">{t('app.recovery.none_yet')}</p>{/if}
        {/if}
            </Stack>
          </div>
        </SettingsSection>
      </div>
    {/if}
  </SettingsLayout>

  {#if actor.hasPassword}
    <Dialog
      size="m"
      title={t('app.password.change_title')}
      bind:isOpen={isChanging}
      dismissLabel={t('app.password.cancel')}
      onClose={() => (newPassword = '')}
      primary={{ label: t('app.password.change_submit'), isBusy: passwordApi.isWorking, busyLabel: t('app.password.changing'), onclick: () => passwordForm?.requestSubmit() }}
    >
      <form bind:this={passwordForm} onsubmit={changePassword}>
        <Stack gap="150">
          <p class="quiet">{t('app.password.change_body')}</p>
          {#if passwordApi.problem}
            <FailureNote problem={passwordApi.problem}>
              {#if passwordApi.problem.reference}{passwordApi.problem.reference}{/if}
            </FailureNote>
          {/if}
          <PasswordField
            bind:value={newPassword}
            label={t('app.password.new_label')}
            context={{ email: account.email ?? undefined, displayName: account.display_name }}
            proof={{ kind: 'bearer' }}
            hint={t('app.redeem.password_hint')}
          />
        </Stack>
      </form>
      {#snippet actions()}
        <Button tone="subtle" onclick={() => (isChanging = false)}>{t('app.password.cancel')}</Button>
      {/snippet}
    </Dialog>
  {/if}
{/if}

<style>
  /* Inside a section's region: its rows' own inset, and a rule rather than a second box between the
     parts that belong together (page-system §4.2). */
  .pad { padding: var(--sp-200) var(--sp-250); }
  .pad :global(.setting-row) { margin: calc(var(--sp-200) * -1) calc(var(--sp-250) * -1) 0; }

  .section {
    padding-block-start: var(--sp-150);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
  }

  form { margin: 0; }

  .quiet { margin: 0; color: var(--text-secondary); }

  h3 {
    margin: 0;
    font-size: var(--fs-200);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .identities {
    margin: 0;
    padding-inline-start: var(--sp-300);
  }
</style>
