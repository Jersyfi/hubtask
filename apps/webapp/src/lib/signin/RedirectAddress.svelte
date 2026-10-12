<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // This installation's redirect address, the one value every registration form at every provider
  // asks for (UC-ID-11 check 6): in full, on lines of its own rather than inside a sentence, with a
  // control that copies it where the browser offers a clipboard. Text that wraps rather than a
  // field: a field cuts the address off at a phone's width, and "in full" is the point. Where no
  // clipboard is offered the control is absent rather than disabled - the address is on screen and
  // selected whole with one press, the fallback every copy control has.

  import { Button, canCopy } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** The address, as the presets answer it. */
    address: string;
  }

  const { address }: Props = $props();
  const id = $props.id();

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

<div class="redirect" role="group" aria-labelledby="{id}-label" aria-describedby="{id}-hint">
  <span class="label" id="{id}-label">{t('app.identity_provider.redirect_label')}</span>
  <code class="address" data-redirect-address>{address}</code>
  <span class="hint" id="{id}-hint">{t('app.identity_provider.redirect_hint')}</span>
  {#if offersCopy}
    <div class="row">
      <Button tone="secondary" onclick={copy}>{t('app.identity_provider.redirect_copy')}</Button>
      <!-- Mounted empty and filled after, so the confirmation is announced. -->
      <span class="hint" role="status">{copied ? t('app.identity_provider.redirect_copied') : ''}</span>
    </div>
  {/if}
</div>

<style>
  .redirect { display: grid; gap: var(--sp-050); }
  .label { font-weight: var(--fw-medium); font-size: var(--fs-100); }
  .address {
    display: block;
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    font-family: var(--font-mono);
    font-size: var(--fs-100);
    overflow-wrap: anywhere;
    user-select: all;
  }
  .hint { color: var(--text-secondary); font-size: var(--fs-075); }
  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
</style>
