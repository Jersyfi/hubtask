<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Whether the workspace lets a collection take requests through a public form (UC-SVC-13), for
  // somebody who manages its members. Locked by the installation or the plan, it is shown and not
  // switched.

  import { SettingRow, Switch } from '@hubtask/design-system/components';
  import type { PublicLinksPolicy } from '@hubtask/sync-engine';

  import { holdsHere } from '../data/capability.svelte.ts';
  import { publicForms, publicFormsPolicyPath } from '../data/publicforms.svelte.ts';
  import { resource } from '../data/resource.svelte.ts';
  import { toast } from '../frame/toasts.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { renderProblem } from '../problem.ts';

  const administers = $derived(holdsHere('MANAGE_MEMBERS') === true);
  const policy = resource<PublicLinksPolicy>(publicFormsPolicyPath);
  const held = $derived(policy.state.status === 'ready' ? policy.state.data : undefined);
  // Drawn as `allowed === true`: a `Switch` refuses an undefined binding and takes the whole screen
  // down with it, so an answer without the field (a proxy's page) reads as off instead.

  async function set(allowed: boolean) {
    try {
      await publicForms.setAllowed(allowed);
      toast.show({ text: allowed ? t('app.admin.public_forms_on') : t('app.admin.public_forms_off'), tone: 'success' });
    } catch (error) {
      toast.show({ text: renderProblem(error as never, messages).message, tone: 'danger' });
    }
  }
</script>

{#if administers && held}
  <div data-public-forms>
    <SettingRow label={t('app.admin.public_forms')} help={t('app.admin.public_forms_hint')} fieldWidth="sm">
      {#snippet control(nameId)}
        <Switch
          label={t('app.admin.public_forms')}
          isLabelHidden
          aria-labelledby={nameId}
          disabledReason={held.locked_by ? t('app.admin.public_forms_locked') : undefined}
          bind:checked={() => held.allowed === true, (on) => void set(on)}
        />
      {/snippet}
    </SettingRow>
  </div>
{/if}
