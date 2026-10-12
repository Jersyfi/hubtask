<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Add an address* for a managed person, from *People* (UC-ID-20 check 7): the form in a dialog,
  // open while there is somebody to add one for.

  import { Button, Dialog } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import AddressForm from './AddressForm.svelte';

  interface Props {
    /** The managed account and its name; the dialog is open exactly while there is one. */
    target: { readonly accountId: string; readonly who: string } | undefined;
    onClose: () => void;
  }

  const { target, onClose }: Props = $props();
</script>

<Dialog
  title={t('app.people.address_title', { who: target?.who ?? '' })}
  isOpen={target !== undefined}
  dismissLabel={t('app.people.address_close')}
  onClose={onClose}
>
  {#snippet actions()}
    <Button onclick={onClose}>{t('app.people.address_close')}</Button>
  {/snippet}
  {#if target}
    <AddressForm accountId={target.accountId} hint={t('app.people.address_hint_other', { who: target.who })} />
  {/if}
</Dialog>
