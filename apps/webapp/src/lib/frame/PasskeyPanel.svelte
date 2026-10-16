<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The passkeys and security keys of the reader's own account (identity.md §23, UC-ID-22).
  //
  // **Each one by the name the person gave it**, with the day it was added and the day it was last
  // used; no maker, no picture (UC-ID-22 *Where it ends*). With none, the panel says so in words.
  //
  // **Add a passkey is drawn only where adding would work** (P-05): the server's own `can_add` -
  // fewer than ten, a public address to bind it to, a proof the account can give - and a browser
  // that can make one, on a page served at the workspace's own host, which a browser insists on.
  // Where it is not drawn, the reason stands in its place.
  //
  // **Proof, then device.** The button asks for the step-up first and the device second (UC-ID-22
  // check 2), with the name typed beforehand, so that the passkey lands in the list already named. A
  // prompt the person closes says nothing and leaves focus on the button (P-11). The first factor of
  // an account without codes shows the ten codes once, as the authenticator's setup does.
  //
  // **Each row renames with the session alone and removes behind a fresh proof** (UC-ID-22 check 4).
  // *Remove* is drawn where the server's `can_remove` says removing would pass - never on the last
  // second factor a workspace requires of this person, where the sentence saying so stands instead
  // (check 5). A passkey made for another address opens nothing here and always goes.

  import { untrack } from 'svelte';

  import { canCopy } from '@hubtask/design-system/components';
  import { Banner, Button, Input, OneTimeSecret, Stack } from '@hubtask/design-system/components';

  import { announcer } from '../announce.svelte.ts';
  import { passkeys } from '../data/passkeys.svelte.ts';
  import { canCreatePasskeys, isAtRelyingParty, isStale, nameFits, removalOf, suggestedName } from '../data/passkeys.ts';
  import { formatDay } from '../i18n/datetime.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  $effect(() => untrack(() => passkeys.open()));
  $effect(() => () => passkeys.forget());

  const list = $derived(passkeys.list);
  const held = $derived(list?.passkeys ?? []);
  const browserCan = canCreatePasskeys(globalThis as never);
  const hereCan = $derived(isAtRelyingParty(list?.rp_id, location.hostname));

  /** Why *Add a passkey* is not drawn, as a sentence; nothing where it is. */
  const reason = $derived.by((): string | undefined => {
    if (!list) return undefined;
    if (!list.can_add) {
      const code = list.cannot_add_reason ?? 'auth.passkey_unavailable';
      return t(code, { limit: String(list.limit) });
    }
    if (!browserCan) return t('app.passkeys.unsupported');
    if (!hereCan) return t('app.passkeys.elsewhere', { host: list.rp_id ?? '' });
    return undefined;
  });

  let name = $state('');
  let nameTouched = $state(false);
  let notice = $state<string | undefined>(undefined);
  let addButton = $state<HTMLElement | undefined>(undefined);

  // The suggestion follows the list until the person types a name of their own.
  $effect(() => {
    const suggestion = suggestedName(t('app.passkeys.default_name'), held);
    if (!untrack(() => nameTouched)) name = suggestion;
  });

  // The one row being renamed, and its draft.
  let renaming = $state<string | undefined>(undefined);
  let draft = $state('');

  function startRenaming(id: string, current: string): void {
    notice = undefined;
    renaming = id;
    draft = current;
  }

  async function rename(event: SubmitEvent, id: string): Promise<void> {
    event.preventDefault();
    const chosen = draft.trim();
    if (await passkeys.rename(id, chosen)) {
      renaming = undefined;
      notice = t('app.passkeys.renamed', { name: chosen });
      announcer.say(notice);
    }
  }

  async function remove(id: string, name: string): Promise<void> {
    notice = undefined;
    if (await passkeys.remove(id)) {
      notice = t('app.passkeys.removed', { name });
      announcer.say(notice);
    }
  }

  /** Why a row offers no *Remove*, in words. */
  function removalReason(code: string): string {
    return code === 'auth.mfa_required_by_tenant' ? t('app.passkeys.required_by_workspace') : t(code, {});
  }

  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    notice = undefined;
    const chosen = name.trim();
    const ended = await passkeys.add(chosen, (options) => navigator.credentials.create(options));
    if (ended === 'added') {
      notice = t('app.passkeys.added', { name: chosen });
      announcer.say(notice);
      nameTouched = false;
    } else if (ended === 'cancelled') {
      addButton?.querySelector('button')?.focus();
    }
  }
</script>

<div class="panel" data-testid="passkeys">
  <Stack gap="150">
    <h2>{t('app.passkeys.title')}</h2>
    <p class="quiet">{t('app.passkeys.body')}</p>
    {#if notice}<p class="quiet">{notice}</p>{/if}
    {#if passkeys.failure}
      <Banner tone="danger" title={passkeys.failure.message}>
        {#if passkeys.failure.reference}{passkeys.failure.reference}{/if}
      </Banner>
    {/if}

    {#if passkeys.fresh}
      <OneTimeSecret
        value={passkeys.fresh.join('\n')}
        label={t('app.recovery.title')}
        hint={t('app.passkeys.recovery_hint')}
        revealLabel={t('app.recovery.reveal')}
        hideLabel={t('app.recovery.hide')}
        copyLabel={canCopy(navigator.clipboard) ? t('app.recovery.copy') : undefined}
        copiedLabel={t('app.recovery.copied')}
        acknowledgementLabel={t('app.recovery.acknowledge')}
        notAcknowledgedReason={t('app.recovery.not_acknowledged')}
        dismissLabel={t('app.recovery.dismiss')}
        onDismiss={() => passkeys.forget()}
      />
    {/if}

    {#if list}
      {#if held.length === 0}
        <p class="quiet">{t('app.passkeys.none')}</p>
      {:else}
        <ul class="held">
          {#each held as each (each.id)}
            {@const removal = removalOf(each, list)}
            <li>
              {#if renaming === each.id}
                <form onsubmit={(event) => rename(event, each.id)}>
                  <Stack gap="100">
                    <Input
                      label={t('app.passkeys.rename_label', { name: each.name })}
                      bind:value={draft}
                      maxlength={64}
                      isRequired
                      autocomplete="off"
                    />
                    <div class="actions">
                      <Button
                        type="submit"
                        tone="secondary"
                        isBusy={passkeys.isWorking}
                        busyLabel={t('app.passkeys.saving')}
                        disabledReason={nameFits(draft) ? undefined : t('app.passkeys.name_needed')}
                      >
                        {t('app.passkeys.save')}
                      </Button>
                      <Button tone="subtle" onclick={() => (renaming = undefined)}>{t('app.passkeys.cancel')}</Button>
                    </div>
                  </Stack>
                </form>
              {:else}
              <span class="name">{each.name}</span>
              <span class="quiet">
                {each.last_used_at
                  ? t('app.passkeys.meta_used', {
                      added: formatDay(each.created_at, messages.locale),
                      used: formatDay(each.last_used_at, messages.locale),
                    })
                  : t('app.passkeys.meta_unused', { added: formatDay(each.created_at, messages.locale) })}
              </span>
              {#if isStale(each, list)}
                <span class="quiet">{t('app.passkeys.stale', { host: each.rp_id ?? '' })}</span>
              {/if}
              <div class="actions">
                <Button tone="subtle" onclick={() => startRenaming(each.id, each.name)}>
                  {t('app.passkeys.rename', { name: each.name })}
                </Button>
                {#if removal.offered}
                  <Button
                    tone="danger"
                    isBusy={passkeys.isWorking}
                    busyLabel={t('app.passkeys.removing')}
                    onclick={() => void remove(each.id, each.name)}
                  >
                    {t('app.passkeys.remove', { name: each.name })}
                  </Button>
                {/if}
              </div>
              {#if !removal.offered}
                <p class="quiet">{removalReason(removal.reason)}</p>
              {/if}
              {/if}
            </li>
          {/each}
        </ul>
      {/if}

      {#if reason}
        <p class="quiet">{reason}</p>
      {:else}
        <form onsubmit={add}>
          <Stack gap="150">
            <Input
              label={t('app.passkeys.name_label')}
              hint={t('app.passkeys.name_hint')}
              bind:value={name}
              oninput={() => (nameTouched = true)}
              maxlength={64}
              isRequired
              autocomplete="off"
            />
            <div bind:this={addButton}>
              <Button
                type="submit"
                tone="secondary"
                isBusy={passkeys.isWorking}
                busyLabel={t('app.passkeys.adding')}
                disabledReason={nameFits(name) ? undefined : t('app.passkeys.name_needed')}
              >
                {t('app.passkeys.add')}
              </Button>
            </div>
          </Stack>
        </form>
      {/if}
    {/if}
  </Stack>
</div>

<style>
  .panel {
    max-inline-size: 60ch;
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  form { margin: 0; max-inline-size: 52ch; }

  .quiet { margin: 0; color: var(--text-secondary); }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .held {
    display: grid;
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .held li {
    display: grid;
    gap: var(--sp-025);
  }

  .name { font-weight: var(--fw-semibold); }

  .actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
  }
</style>
