<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The apps that may act in this workspace as the reader.
  //
  // The same question the sessions ask, asked about apps: what is currently able to act as me, and
  // how do I stop it. Withdrawing takes effect on the app's next request, and the screen says so
  // rather than implying that what is running now stops mid-sentence.

  import { untrack } from 'svelte';

  import { Button, EmptyState, Stack, Table } from '@hubtask/design-system/components';

  import SettingsHead from '../lib/frame/SettingsHead.svelte';

  import { actor } from '../lib/data/account.svelte.ts';
  import { consent } from '../lib/data/consent.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  const account = $derived(actor.account);

  $effect(() => untrack(() => consent.openGrants()));

  let failure = $state<string | undefined>(undefined);

  // `$derived`, because the headings are words: a language chosen on the screen beside this one
  // changes them, and a list built once would keep the language it was built in.
  const columns = $derived([
    { id: 'app', label: t('app.grants.app_column') },
    { id: 'scopes', label: t('app.grants.scopes_column') },
    { id: 'withdraw', label: t('app.grants.withdraw'), isLabelHidden: true, align: 'end' as const },
  ]);

  async function withdraw(grantId: string): Promise<void> {
    failure = undefined;
    try {
      await consent.withdraw(grantId);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
    }
  }
</script>

{#if !account}
  <EmptyState kind="filtered" title={t('app.profile.signed_out')} />
{:else}
  <Stack gap="300">
    <SettingsHead row="grants" />

    <Stack gap="150">
      <p class="quiet">{t('app.grants.intro')}</p>

      {#if failure}<p class="failure" role="alert">{failure}</p>{/if}

      {#if consent.grants.length === 0}
        <p class="quiet">{t('app.grants.none')}</p>
      {:else}
        <Table label={t('app.grants.title')} isLabelHidden {columns}>
          {#each consent.grants as grant (grant.id)}
            <tr>
              <th scope="row" class="what">{grant.client_name}</th>
              <!-- The scopes as the app holds them. Sentences where this build knows them, and the
                   identifier where it does not — the same rule as the consent screen. -->
              <td class="scopes">{grant.scopes.join(', ')}</td>
              <td class="end">
                <Button size="sm" tone="subtle" onclick={() => void withdraw(grant.id)}>
                  {t('app.grants.withdraw')}
                </Button>
              </td>
            </tr>
          {/each}
        </Table>
      {/if}

      <p class="quiet small">{t('app.grants.next_request')}</p>
    </Stack>
  </Stack>
{/if}

<style>
  .what { font-weight: var(--fw-medium); }

  .scopes { color: var(--text-secondary); font-size: var(--fs-075); }

  .end { text-align: end; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .failure { margin: 0; color: var(--text-danger); }
</style>
