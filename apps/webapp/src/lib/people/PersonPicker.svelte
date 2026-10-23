<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Choosing a person by name instead of typing their identifier (UC-ID-24 check 4): the trail's
  // actor, a request's subject, whose consent is withdrawn, whose processing is restricted.
  //
  // **It lists only whom the reader may see.** The candidates are the workspace's people as the
  // memberships read answers this reader, plus whoever the caller already holds from its own
  // authorised read (the actors of the trail entries on screen). A reader whose memberships read is
  // refused is offered only the latter; nothing here asks the server for anybody else.
  //
  // The value is the account's identifier, as the request needs it; an empty value is "nobody
  // chosen", which a filter reads as "anyone".

  import { AssigneeControl, Drawer, Popover } from '@hubtask/design-system/components';

  import { untrack } from 'svelte';

  import { accounts } from '../data/accounts.svelte.ts';
  import { holdsHere } from '../data/capability.svelte.ts';
  import { people } from '../data/people.svelte.ts';
  import { viewport } from '../frame/viewport.svelte.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';
  import { byName } from '../i18n/collation.ts';

  interface Props {
    label: string;
    hint?: string;
    /** The chosen account's identifier, or empty. */
    value: string;
    /** What the face says while nobody is chosen: "Anyone" for a filter, "Choose a person" otherwise. */
    noneLabel: string;
    /** People the caller already holds from its own read, offered beside the workspace's. */
    known?: readonly string[];
    error?: string;
  }

  let { label, hint, value = $bindable(''), noneLabel, known = [], error }: Props = $props();

  // The workspace's people where this reader reads the workspace; an auditor, who does not, is
  // not sent the read the server would refuse and record.
  $effect(() => {
    if (holdsHere('READ') === true) return untrack(() => people.open({}));
  });

  let isOpen = $state(false);
  const id = $props.id();

  const ids = $derived([...new Set([...people.candidates({}), ...known.filter((each) => each)])]);
  const candidates = $derived(
    ids
      .map((each) => ({ id: each, name: accounts.nameOf(each) ?? t('app.people.unnamed') }))
      .sort((left, right) => byName(messages.locale)(left.name, right.name)),
  );
  $effect(() => {
    const wanted = ids;
    untrack(() => accounts.resolve(wanted));
  });

  const chosen = $derived(value ? (accounts.nameOf(value) ?? t('app.people.unnamed')) : undefined);

  function choose(next: readonly string[]): void {
    value = next[0] ?? '';
    isOpen = false;
  }
</script>

<div class="picker" data-person-picker="">
  <span class="label" id={`${id}-label`}>{label}</span>
  {#snippet face(props: Record<string, unknown>)}
    <button
      type="button"
      class="face"
      class:none={!chosen}
      aria-labelledby={`${id}-label ${id}-value`}
      data-invalid={error ? '' : undefined}
      aria-describedby={error || hint ? `${id}-note` : undefined}
      {...props}
    >
      <span id={`${id}-value`}>{chosen ?? noneLabel}</span>
    </button>
  {/snippet}
  {#snippet list()}
    <AssigneeControl
      {label}
      {candidates}
      selection="single"
      selected={value ? [value] : []}
      filterLabel={t('app.people.filter')}
      locale={messages.locale}
      emptyLabel={t('app.person_picker.nobody')}
      noMatchLabel={t('app.people.no_match')}
      chosenLabel={t('app.person_picker.chosen')}
      unassignedLabel={noneLabel}
      onSelect={choose}
    />
  {/snippet}
  {#if viewport.isCompact}
    {@render face({ 'aria-haspopup': 'dialog', 'aria-expanded': isOpen, onclick: () => (isOpen = true) })}
    <Drawer bind:isOpen edge="block-end" title={label} dismissLabel={t('app.dismiss')}>
      {@render list()}
    </Drawer>
  {:else}
    <Popover {label} bind:isOpen placement={{ side: 'block-end', align: 'start' }}>
      {#snippet trigger(props)}
        {@render face(props)}
      {/snippet}
      {@render list()}
    </Popover>
  {/if}
  {#if error}
    <span class="error" id={`${id}-note`}>{error}</span>
  {:else if hint}
    <span class="hint" id={`${id}-note`}>{hint}</span>
  {/if}
</div>

<style>
  .picker { display: grid; gap: var(--sp-050); align-content: start; }

  .label { color: var(--text-primary); font-size: var(--fs-100); font-weight: var(--fw-medium); }

  .face {
    display: flex;
    align-items: center;
    min-height: var(--density-control-md-min);
    padding-inline: var(--sp-150);
    padding-block: var(--density-control-md-block);
    border: var(--bw-hairline) solid var(--border-default);
    border-radius: var(--r-md);
    background: var(--bg-surface);
    color: var(--text-primary);
    font: inherit;
    text-align: start;
    cursor: pointer;
  }

  .face.none { color: var(--text-secondary); }

  .face:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-color: var(--border-strong); }

  .face[data-invalid] { border-color: var(--status-danger-border); border-width: var(--bw-thick); background: var(--status-danger-surface); }

  .hint { color: var(--text-secondary); font-size: var(--fs-075); }

  .error { color: var(--text-danger); font-size: var(--fs-075); }
</style>
