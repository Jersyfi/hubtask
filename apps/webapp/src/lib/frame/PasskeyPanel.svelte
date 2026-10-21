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

  import { focusFirst } from '../forms/missing.ts';
  import { toast } from './toasts.svelte.ts';
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
  let addButton = $state<HTMLElement | undefined>(undefined);
  /** A name was asked for and is not usable: said at that field when its button is pressed. */
  let missing = $state<'add' | 'rename' | undefined>(undefined);
  let addForm = $state<HTMLElement | null>(null);
  let renameForm = $state<HTMLElement | null>(null);

  /** Marks the name and moves focus there; answers whether the name may go (design-system.md §4, case 3). */
  function nameIsUsable(value: string, which: 'add' | 'rename', form: HTMLElement | null): boolean {
    if (nameFits(value)) return true;
    missing = which;
    queueMicrotask(() => focusFirst(form));
    return false;
  }

  // The suggestion follows the list until the person types a name of their own.
  $effect(() => {
    const suggestion = suggestedName(t('app.passkeys.default_name'), held);
    if (!untrack(() => nameTouched)) name = suggestion;
  });

  // The control that is working, so that only it shows so: `add`, or a row's `rename:` or `remove:`.
  let acting = $state<string | undefined>(undefined);
  const busy = (control: string): boolean => passkeys.isWorking && acting === control;

  // The one row being renamed, and its draft.
  let renaming = $state<string | undefined>(undefined);
  let draft = $state('');

  function startRenaming(id: string, current: string): void {
    missing = undefined;
    renaming = id;
    draft = current;
  }

  async function rename(event: SubmitEvent, id: string): Promise<void> {
    event.preventDefault();
    if (!nameIsUsable(draft, 'rename', renameForm)) return;
    const chosen = draft.trim();
    acting = `rename:${id}`;
    if (await passkeys.rename(id, chosen)) {
      renaming = undefined;
      toast.show({ text: t('app.passkeys.renamed', { name: chosen }), tone: 'success' });
    }
  }

  async function remove(id: string, name: string): Promise<void> {
    acting = `remove:${id}`;
    if (await passkeys.remove(id)) {
      toast.show({ text: t('app.passkeys.removed', { name }), tone: 'success' });
    }
  }

  /** The distinct reasons keys offer no *Remove*, each said once for the list. */
  const keptReasons = $derived(
    list ? [...new Set(held.map((each) => removalOf(each, list)).filter((each) => !each.offered).map((each) => removalReason(each.reason)))] : [],
  );

  /** Why a row offers no *Remove*, in words. */
  function removalReason(code: string): string {
    return code === 'auth.mfa_required_by_tenant' ? t('app.passkeys.required_by_workspace') : t(code, {});
  }

  async function add(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    if (!nameIsUsable(name, 'add', addForm)) return;
    const chosen = name.trim();
    acting = 'add';
    const ended = await passkeys.add(chosen, (options) => navigator.credentials.create(options));
    if (ended === 'added') {
      toast.show({ text: t('app.passkeys.added', { name: chosen }), tone: 'success' });
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
                <form onsubmit={(event) => rename(event, each.id)} bind:this={renameForm} novalidate>
                  <Stack gap="100">
                    <Input
                      label={t('app.passkeys.rename_label', { name: each.name })}
                      bind:value={draft}
                      maxlength={64}
                      isRequired
                      autocomplete="off"
                      oninput={() => (missing = undefined)}
                      error={missing === 'rename' && !nameFits(draft) ? t('app.passkeys.name_needed') : undefined}
                    />
                    <div class="actions">
                      <Button
                        type="submit"
                        tone="secondary"
                        isBusy={busy(`rename:${each.id}`)}
                        busyLabel={t('app.passkeys.saving')}
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
                    isBusy={busy(`remove:${each.id}`)}
                    busyLabel={t('app.passkeys.removing')}
                    onclick={() => void remove(each.id, each.name)}
                  >
                    {t('app.passkeys.remove', { name: each.name })}
                  </Button>
                {/if}
              </div>
              {/if}
            </li>
          {/each}
        </ul>
        <!-- Why a key offers no *Remove*, said once under the list rather than under each key it
             holds for - "required by the workspace" was one sentence repeated per row. -->
        {#each keptReasons as said (said)}
          <p class="quiet">{said}</p>
        {/each}
      {/if}
      <!-- The limit, said before it is reached (UC-ID-22 check 3): at the limit the reason below
           says it instead. -->
      {#if held.length < list.limit}
        <p class="quiet">
          {held.length === 0
            ? t('app.passkeys.limit', { limit: String(list.limit) })
            : t('app.passkeys.count', { count: String(held.length), limit: String(list.limit) })}
        </p>
      {/if}

      {#if reason}
        <p class="quiet">{reason}</p>
      {:else}
        <form onsubmit={add} bind:this={addForm} novalidate>
          <Stack gap="150">
            <Input
              label={t('app.passkeys.name_label')}
              hint={t('app.passkeys.name_hint')}
              bind:value={name}
              oninput={() => {
                nameTouched = true;
                missing = undefined;
              }}
              error={missing === 'add' && !nameFits(name) ? t('app.passkeys.name_needed') : undefined}
              maxlength={64}
              isRequired
              autocomplete="off"
            />
            <div bind:this={addButton}>
              <Button
                type="submit"
                tone="secondary"
                isBusy={busy('add')}
                busyLabel={t('app.passkeys.adding')}
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
