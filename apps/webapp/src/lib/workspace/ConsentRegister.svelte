<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who keeps their own content out of AI (UC-PRV-05 check 12): the consent register, on the
  // screen that answers data subject requests and nowhere else. Absent where the server refuses it,
  // and in a workspace of one, where nobody's content can be kept out (P-10, data-protection.md
  // §4.1).

  import { untrack } from 'svelte';

  import { Stack } from '@hubtask/design-system/components';

  import { consentRegister } from '../data/consentregister.svelte.ts';
  import { workspace } from '../data/workspace.svelte.ts';
  import { formatDateTime } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  $effect(() => untrack(() => consentRegister.open()));
</script>

{#if consentRegister.isReadable && workspace.workspace?.several_people === true}
  <section class="panel" aria-labelledby="consent-register-title" data-consent-register>
    <Stack gap="150">
      <h2 class="section" id="consent-register-title">{t('app.privacy.register_title')}</h2>
      <p class="quiet">{t('app.privacy.register_note')}</p>
      {#if consentRegister.entries.length === 0}
        <p>{t('app.privacy.register_empty')}</p>
      {:else}
        <ul>
          {#each consentRegister.entries as entry (entry.account_id)}
            <li>
              <span class="name">{entry.display_name}</span>
              <span class="quiet">
                {t(entry.source === 'tenant_admin' ? 'app.privacy.register_by_admin' : 'app.privacy.register_by_self', {
                  since: formatDateTime(entry.since, messages.locale),
                })}
              </span>
            </li>
          {/each}
        </ul>
      {/if}
    </Stack>
  </section>
{/if}

<style>
  .panel {
    max-inline-size: 72ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-300); font-weight: var(--fw-semibold); }

  p { margin: 0; }

  .quiet { color: var(--text-secondary); }

  ul { margin: 0; padding: 0; list-style: none; display: grid; gap: var(--sp-100); }

  li { display: flex; flex-wrap: wrap; gap: var(--sp-100); align-items: baseline; }

  .name { font-weight: var(--fw-semibold); }
</style>
