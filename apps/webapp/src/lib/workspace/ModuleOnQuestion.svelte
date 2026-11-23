<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The question a workspace's switch-on asks (UC-INS-21 check 13): in which existing hubs the
  // product goes on as well, none chosen at first - on at the workspace changes no hub by itself
  // (ADR-0109). One part for every door to that switch: *Administration → Products* and a
  // product's page while it is off (user-experience.md §2.8).
  //
  // `ask` answers with the hubs chosen, with none at once where no hub is offered (check 2), or
  // with nothing where the person cancelled. It waits until the offer is decided: decided on the
  // hubs read so far, the switch would go through unasked.

  import { untrack } from 'svelte';

  import { Button, Dialog, MultiSelect } from '@hubtask/design-system/components';
  import type { ModuleState } from '@hubtask/sync-engine';

  import { actor } from '../data/account.svelte.ts';
  import { holds, holdsHere, roleOrder } from '../data/capability.svelte.ts';
  import { containers } from '../data/containers.svelte.ts';
  import { asksWhichHubs, hubsOffered, isOfferSettled, moduleCodes } from '../data/modules.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { chooseSeveral, FROM_DATA } from '../forms/chooseSeveral.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    /** Whether a switch-on here may ask: at the workspace, for a reader who may shape it. */
    isActive: boolean;
  }

  const { isActive }: Props = $props();

  // A private hub counts only a role held on it, so its memberships are read; a shared hub
  // follows the workspace's, which the manifest already answers.
  $effect(() => {
    if (!isActive) return;
    const hidden = containers.hubs.filter((hub) => hub.private === true).map((hub) => hub.id);
    untrack(() => hidden.forEach((id) => people.open({ hubId: id })));
  });
  /** Whether the reader may shape a private hub, by the role held on it; `undefined` while that is out. */
  const shapesPrivate = (hub: { readonly id: string }): boolean | undefined => {
    const role = people.roleOf({ hubId: hub.id }, actor.account?.id, roleOrder());
    if (role === undefined) return undefined;
    return role !== null && holds(role, 'STRUCTURE').status === 'permitted';
  };
  // The hubs offered: those the reader may shape (check 13) among the hub list, which is one page -
  // the first 200 hubs that are not archived (containers.svelte.ts). A hub beyond it is not
  // offered here and is switched on in its own settings (user-experience.md §2.8).
  const offered = $derived(isActive ? hubsOffered(containers.hubs, holdsHere('STRUCTURE'), shapesPrivate) : []);
  /**
   * Whether the offer can be decided: the hub list and every hub's answer are in. A list that
   * failed is decided with no hub - the switch then changes none, as with none chosen.
   */
  const isSettled = $derived.by(() => {
    const list = containers.hubsState.status;
    if (list === 'failed') return true;
    return isOfferSettled(list === 'ready' ? containers.hubs : undefined, holdsHere('STRUCTURE'), shapesPrivate);
  });

  type Answer = readonly string[] | undefined;
  /** The switch-on waiting for the offer to be decided, and the answer of the question standing. */
  let waiting = $state<{ state: Pick<ModuleState, 'module' | 'on'>; answer: (hubs: Answer) => void; onAsk?: () => void }>();
  let asking = $state<(hubs: Answer) => void>();
  /**
   * The module last asked about. Kept after the question closes, so that every question is the
   * same dialog: a dialog drawn anew under the same bound state is closed again by its
   * predecessor's close, which the platform delivers a task later.
   */
  let module = $state<ModuleState['module']>();
  let isOpen = $state(false);
  let chosen = $state<readonly string[]>([]);
  let list = $state<HTMLElement>();

  /**
   * Asks in which existing hubs `state`'s switch-on goes as well. `onAsk` runs once the offer is
   * decided, whether a question then stands or not.
   */
  export function ask(state: Pick<ModuleState, 'module' | 'on'>, onAsk?: () => void): Promise<Answer> {
    waiting?.answer(undefined);
    return new Promise((answer) => {
      waiting = { state, answer, ...(onAsk ? { onAsk } : {}) };
    });
  }

  $effect(() => {
    const next = waiting;
    const settled = isSettled;
    const hubs = offered;
    if (!next || !settled) return;
    untrack(() => {
      waiting = undefined;
      next.onAsk?.();
      if (!asksWhichHubs(next.state, true, 'WORKSPACE', hubs)) {
        next.answer([]);
        return;
      }
      chosen = [];
      module = next.state.module;
      asking = next.answer;
      isOpen = true;
    });
  });

  // The focus starts on the list, as it did on its first box: the dialog itself focuses only a
  // text field, and the list is a button that opens it.
  $effect(() => {
    if (!isOpen || !list) return;
    const field = list;
    queueMicrotask(() => field.querySelector<HTMLElement>('button[aria-haspopup="listbox"]')?.focus());
  });

  function confirm() {
    const standing = asking;
    // A hub that left the list while the question stood open is not sent.
    const hubs = chosen.filter((id) => offered.some((hub) => hub.id === id));
    asking = undefined;
    isOpen = false;
    standing?.(hubs);
  }

  /** However it closed without *Switch on*: nothing switches. */
  function closed() {
    const standing = asking;
    asking = undefined;
    standing?.(undefined);
  }
</script>

{#if module}
  <Dialog
    size="m"
    bind:isOpen
    title={t('app.modules.on_hubs_title', { module: t(moduleCodes({ module }).name) })}
    dismissLabel={t('app.modules.on_hubs_cancel')}
    onClose={closed}
    primary={{ label: t('app.modules.on_hubs_confirm'), onclick: confirm }}
  >
    <div data-on-hubs={module} bind:this={list}>
      <MultiSelect
        label={t('app.modules.on_hubs_question')}
        hint={t('app.modules.on_hubs_hint', { module: t(moduleCodes({ module }).name) })}
        presentation={FROM_DATA}
        options={offered.map((hub) => ({ value: hub.id, label: hub.name }))}
        selected={chosen}
        onChange={(next) => (chosen = next)}
        {...chooseSeveral()}
      />
    </div>
    {#snippet actions()}
      <Button tone="secondary" onclick={() => (isOpen = false)}>{t('app.modules.on_hubs_cancel')}</Button>
    {/snippet}
  </Dialog>
{/if}
