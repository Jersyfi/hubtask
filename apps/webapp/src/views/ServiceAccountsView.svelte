<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The accounts that are nothing but access (`security.md` §5).
  //
  // **What a service account is for, in the catalogue's words**: the integration that must outlive
  // the person who wrote it. A rule that runs as somebody dies with their departure; a rule that
  // runs as a service account does not.
  //
  // **The memberships are shown, because a service account with none can do nothing** — and that
  // is a common confusion rather than a rare one. A token minted for an account that holds no role
  // is a credential that authenticates perfectly and is refused by every operation.
  //
  // **Its tokens are minted through the same panel as a person's**, with `account_id` naming it —
  // the one exception the contract carves into "a token is its holder's", and the reason it needs
  // the permission that manages members.

  import { untrack } from 'svelte';

  import { Banner, Button, Input, RoleBadge, Spinner, Stack } from '@hubtask/design-system/components';

  import TokensView from './TokensView.svelte';
  import { people } from '../lib/data/people.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const TENANT = { scopeType: 'TENANT' } as const;

  let newName = $state('');
  let opened = $state<string | undefined>(undefined);
  let failure = $state<string | undefined>(undefined);
  let isWorking = $state(false);

  $effect(() =>
    untrack(() => {
      // The workspace's memberships say what each service account may actually reach.
      people.openScope(TENANT);
      return serviceAccounts.open();
    }),
  );

  const reading = $derived(serviceAccounts.state);
  const all = $derived(serviceAccounts.all);
  const refusal = $derived(
    reading.status === 'failed' ? renderProblem(reading.error, messages) : undefined,
  );

  /** What this account holds at the workspace. Empty is the case the screen exists to explain. */
  function rolesOf(accountId: string) {
    return people.holders({}, TENANT).filter((holder) => holder.accountId === accountId);
  }

  async function create(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const name = newName.trim();
    if (!name) return;
    failure = undefined;
    isWorking = true;
    try {
      await serviceAccounts.create(name);
      newName = '';
    } catch (cause) {
      failure = renderProblem(cause as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
</script>

<div class="screen">
  <Stack gap="300">
    <h1>{t('app.service_accounts.title')}</h1>
    <p class="quiet">{t('app.service_accounts.intro')}</p>

    {#if failure}<Banner tone="danger">{failure}</Banner>{/if}

    {#if reading.status === 'loading' || reading.status === 'idle'}
      <p class="waiting">
        <Spinner label={t('app.service_accounts.reading')} />
        <span>{t('app.service_accounts.reading')}</span>
      </p>
    {:else if refusal}
      <Banner tone="danger" title={refusal.message}>
        {#if refusal.reference}{t('app.error_reference', { request_id: refusal.reference })}{/if}
      </Banner>
    {:else}
      {#each all as account (account.id)}
        <section class="account">
          <Stack gap="150">
            <div class="head">
              <h2 class="name">{account.display_name}</h2>
              <Button
                size="sm"
                tone="subtle"
                onclick={() => (opened = opened === account.id ? undefined : account.id)}
              >
                {opened === account.id ? t('app.service_accounts.hide_tokens') : t('app.service_accounts.show_tokens')}
              </Button>
            </div>

            <div class="roles">
              {#each rolesOf(account.id) as holder (holder.membershipId)}
                <RoleBadge
                  label={t(`app.people.role.${holder.role.toLowerCase()}`)}
                  scopeLabel={t('app.people.at_workspace')}
                  size="sm"
                />
              {:else}
                <!-- The confusion this screen exists to head off: the token will authenticate and
                     every operation will refuse it. -->
                <p class="quiet small">{t('app.service_accounts.no_roles')}</p>
              {/each}
            </div>

            {#if opened === account.id}
              <TokensView accountId={account.id} />
            {/if}
          </Stack>
        </section>
      {:else}
        <p class="quiet">{t('app.service_accounts.none')}</p>
      {/each}
    {/if}

    <Stack gap="150">
      <h2 class="section">{t('app.service_accounts.new_title')}</h2>
      <form onsubmit={create}>
        <Stack gap="150">
          <Input
            label={t('app.service_accounts.name')}
            hint={t('app.service_accounts.name_hint')}
            bind:value={newName}
            isRequired
          />
          <div>
            <Button type="submit" tone="primary" isBusy={isWorking} busyLabel={t('app.service_accounts.creating')}>
              {t('app.service_accounts.create')}
            </Button>
          </div>
        </Stack>
      </form>
      <p class="quiet small">{t('app.service_accounts.then_grant')}</p>
    </Stack>
  </Stack>
</div>

<style>
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .section,
  .name { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .account {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .head { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .head .name { flex: 1 1 auto; min-width: 0; }

  .roles { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  form { margin: 0; }
</style>
