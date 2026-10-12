<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // This installation's redirect address, the one value every registration form at every provider
  // asks for (UC-ID-11 check 6): in full, in a field of its own rather than inside a sentence, with
  // a control that copies it where the browser offers a clipboard. Where it does not, the control is
  // absent rather than disabled - the address is on screen and selectable, the fallback every copy
  // control has.

  import { Button, Input, canCopy } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The address, as the presets answer it. */
    address: string;
  }

  const { address }: Props = $props();

  let copied = $state(false);
  const offersCopy = canCopy(typeof navigator === 'undefined' ? undefined : navigator.clipboard);

  function copy(): void {
    void navigator.clipboard
      .writeText(address)
      .then(() => (copied = true))
      .catch(() => {
        copied = false;
      });
  }
</script>

<div class="redirect">
  <Input
    label={t('app.identity_provider.redirect_label')}
    hint={t('app.identity_provider.redirect_hint')}
    value={address}
    readonly
    spellcheck={false}
  />
  {#if offersCopy}
    <div class="row">
      <Button tone="secondary" onclick={copy}>{t('app.identity_provider.redirect_copy')}</Button>
      <!-- Mounted empty and filled after, so the confirmation is announced. -->
      <span class="quiet" role="status">{copied ? t('app.identity_provider.redirect_copied') : ''}</span>
    </div>
  {/if}
</div>

<style>
  .redirect { display: grid; gap: var(--sp-100); }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .quiet { color: var(--text-secondary); }
</style>
