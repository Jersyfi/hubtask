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

  import { Banner, Button, Input, PageHeader, RoleBadge, Spinner, Stack } from '@hubtask/design-system/components';

  import TokensView from './TokensView.svelte';
  import { people } from '../lib/data/people.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';

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
      announcer.say(t('app.service_accounts.created_announced'));
      newName = '';
    } catch (cause) {
      failure = renderProblem(cause as never, messages).message;
    } finally {
      isWorking = false;
    }
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.service_accounts.title')));
</script>

<Stack gap="300">
  <PageHeader
    title={t('app.service_accounts.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'service-accounts', label: t('app.service_accounts.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />

    <Stack gap="300">
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
      <form class="panel" onsubmit={create}>
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
</Stack>

<style>
  /* The screen takes the region it is given, and what needs a measure carries one: prose has the
     one `app.css` gives every paragraph, fields have `.fields`, and a table or a list has none
     (ADR-0065 decision 2). The 60ch column that stood here was a document's measure around a
     screen that is not a document. */

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
  }
</style>
