<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // What a screen says where the workspace's plan leaves its feature out (multi-tenancy.md §4.2):
  // "Not included in your plan", and what that means for what is there - in place of the controls
  // the server would refuse (P-05). It says what rests and that it stays; it sells nothing, names
  // no other plan and no price (UC-INS-13 check 3). Owners and administrators find the provider's
  // account page beside it, where the operator set one (check 11).
  //
  // One card instead of the form, never a form gated line by line (design-system.md §4, case 8):
  // the plan's words as its lead, the notice of the area's state, the account page as its action.

  import { Badge, ReadOnlyNotice } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import AccountLink from './AccountLink.svelte';

  interface Props {
    /** The one line a reader takes away, when the screen has one beyond the label. */
    title?: string;
    /** What rests and that it stays, in words. */
    body: string;
  }

  const { title, body }: Props = $props();
</script>

<div class="not-in-plan" data-testid="not-in-plan">
  <p class="label"><Badge tone="neutral">{t('app.plan.not_included')}</Badge></p>
  <ReadOnlyNotice icon="info" text={title ? `${title} ${body}` : body}>
    {#snippet action()}<AccountLink />{/snippet}
  </ReadOnlyNotice>
</div>

<style>
  .not-in-plan { display: grid; gap: var(--sp-100); }

  .label { margin: 0; }
</style>
