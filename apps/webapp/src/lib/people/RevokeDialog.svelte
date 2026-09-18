<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The question before a role is revoked (issue 778).
  //
  // Every other irreversible act on a screen confirms in a dialog - a trash, a token withdrawn,
  // a device forgotten - and a role revoked from a list row did not: a single keystroke on a
  // focused control ended somebody's access, with no undo. This is that dialog, shared by the
  // workspace's people screen and the members dialog of a hub, a collection or an entry.
  //
  // It also says what a role of one's own costs. The server allows revoking it - an owner may
  // hand over and leave - so the client is the half that has to say, before the confirm, that
  // this is the reader's own role and what is out of reach afterwards; the shape `RestoreView`
  // uses for the credential cost. Nothing is predicted about permissions: the sentence names
  // the fact, and the server answers what follows.

  import { Banner, Button, Dialog, Stack } from '@hubtask/design-system/components';

  import type { Holder, Ownership } from '../data/people.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  let {
    holder,
    ownership,
    who,
    where,
    isBusy = false,
    onConfirm,
    onCancel,
  }: {
    /** The row being revoked; the dialog is open exactly while there is one. */
    holder: Holder | undefined;
    ownership: Ownership;
    /** The subject as the list names them - a person, or everybody in a group. */
    who: string;
    /** The place the role applies to, in the catalogue's words (`app.people.at_*`). */
    where: string;
    isBusy?: boolean;
    onConfirm: () => void;
    onCancel: () => void;
  } = $props();
</script>

<Dialog
  title={t('app.people.revoke_title')}
  isOpen={holder !== undefined}
  dismissLabel={t('app.workspace.cancel')}
  onClose={onCancel}
>
  {#snippet actions()}
    <Button onclick={onCancel}>{t('app.workspace.cancel')}</Button>
    <Button tone="danger" isBusy={isBusy} busyLabel={t('app.people.working')} onclick={onConfirm}>
      {t('app.people.revoke_confirm')}
    </Button>
  {/snippet}
  {#if holder}
    <Stack gap="150">
      <p class="body">
        {t('app.people.revoke_body', { who, role: t(`app.people.role.${holder.role.toLowerCase()}`), where })}
      </p>
      {#if ownership === 'last'}
        <Banner tone="danger" title={t('app.people.revoke_own_title')}>{t('app.people.revoke_own_last', { where })}</Banner>
      {:else if ownership === 'own'}
        <Banner tone="warning" title={t('app.people.revoke_own_title')}>{t('app.people.revoke_own', { where })}</Banner>
      {/if}
    </Stack>
  {/if}
</Dialog>

<style>
  .body { margin: 0; }
</style>
