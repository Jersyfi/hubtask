<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts" module>
  import type { IconName } from './icons/index.ts';

  export interface InsertChoice {
    id: string;
    label: string;
    /** One line under the label: what the choice puts in. */
    detail?: string;
    icon?: IconName;
  }

  export interface InsertGroup {
    label: string;
    choices: InsertChoice[];
  }
</script>

<script lang="ts">
  // What "/" or "+" offers at a line (S-1), as a listbox: the arrows move, Enter takes, a letter jumps
  // to the next choice starting with it, Escape closes - as every menu here. Opened by "/" the focus
  // stays in the line, so the editor hands its keys to `keydown`, and the words typed after "/"
  // (`query`) narrow the choices and are marked; opened by a button the list itself has the focus and
  // reads them.

  import Icon from './Icon.svelte';
  import Kbd from './Kbd.svelte';
  import { listKey, markedParts, matchingGroups } from './insert.ts';

  interface Props {
    label: string;
    groups: InsertGroup[];
    /** The words of the keys' line: move, take, close. */
    keys: { move: string; take: string; close: string };
    onchoose: (id: string) => void;
    onclose: () => void;
    /** The list takes the focus when drawn (opened by a button), or leaves it in the line ("/"). */
    takesFocus?: boolean;
    /** The words typed after "/": only the choices holding them stay, marked. */
    query?: string;
    /** Said when no choice holds the typed words. */
    emptyText?: string;
  }

  const { label, groups, keys, onchoose, onclose, takesFocus = false, query = '', emptyText = '' }: Props = $props();

  const uid = $props.id();
  const shown = $derived(matchingGroups(groups, query));
  const flat = $derived(shown.flatMap((group) => group.choices));
  let active = $state(0);
  $effect.pre(() => {
    void query;
    active = 0;
  });
  let list = $state<HTMLElement | null>(null);

  const optionId = (index: number) => `${uid}-choice-${index}`;
  const indexOf = (choice: InsertChoice) => flat.indexOf(choice);

  /** Reads one key; true when the list used it, so the caller stops it. */
  export function keydown(event: KeyboardEvent): boolean {
    const outcome = listKey(event.key, active, flat.map((choice) => choice.label));
    if (outcome === null) return false;
    event.preventDefault();
    if (outcome === 'take') {
      const choice = flat[active];
      if (choice) onchoose(choice.id);
    } else if (outcome === 'close') {
      onclose();
    } else {
      active = outcome;
    }
    return true;
  }

  $effect(() => {
    if (takesFocus) list?.focus();
  });

  $effect(() => {
    list?.querySelector(`#${CSS.escape(optionId(active))}`)?.scrollIntoView({ block: 'nearest' });
  });
</script>

<div class="insert" data-insert-list>
  <div
    class="list"
    role="listbox"
    aria-label={label}
    tabindex="-1"
    aria-activedescendant={flat[active] ? optionId(active) : undefined}
    bind:this={list}
    onkeydown={(event) => void keydown(event)}
  >
    {#if flat.length === 0}
      <div class="none" role="status" data-insert-none>{emptyText}</div>
    {/if}
    {#each shown as group (group.label)}
      <div role="group" aria-label={group.label}>
        <div class="heading" aria-hidden="true">{group.label}</div>
        {#each group.choices as choice (choice.id)}
          {@const index = indexOf(choice)}
          <!-- The pointer takes a choice; the keyboard reaches it through the list (aria-activedescendant). -->
          <!-- svelte-ignore a11y_click_events_have_key_events -->
          <div
            class="option"
            role="option"
            id={optionId(index)}
            aria-selected={index === active}
            tabindex="-1"
            data-insert-choice={choice.id}
            onpointerdown={(event) => event.preventDefault()}
            onpointermove={() => (active = index)}
            onclick={() => onchoose(choice.id)}
          >
            {#if choice.icon}<span class="tile"><Icon name={choice.icon} size="sm" /></span>{/if}
            <span class="text">
              <span class="label">{#each markedParts(choice.label, query) as part, at (at)}{#if part.isMatch}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}</span>
              {#if choice.detail}<span class="detail">{#each markedParts(choice.detail, query) as part, at (at)}{#if part.isMatch}<mark>{part.text}</mark>{:else}{part.text}{/if}{/each}</span>{/if}
            </span>
          </div>
        {/each}
      </div>
    {/each}
  </div>
  <div class="keys" aria-hidden="true">
    <span><Kbd>↑↓</Kbd> {keys.move}</span><span><Kbd>↵</Kbd> {keys.take}</span><span><Kbd>Esc</Kbd> {keys.close}</span>
  </div>
</div>

<style>
  .insert { display: grid; gap: var(--sp-050); min-inline-size: 0; }
  .list { display: grid; gap: var(--bw-hairline); outline: none; }
  .list:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); border-radius: var(--r-sm); }
  .heading {
    padding: var(--sp-100) var(--sp-100) var(--sp-025);
    color: var(--text-subtle);
    font-size: var(--fs-065);
    font-weight: var(--fw-semibold);
    /* design-system-lint-ignore: tracking in em follows the type step it spaces */
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .option {
    display: flex;
    align-items: center;
    gap: var(--sp-100);
    min-block-size: var(--sp-400);
    padding: calc(var(--sp-050) + var(--sp-025)) var(--sp-100);
    border-radius: var(--r-sm);
    color: var(--text-primary);
    cursor: pointer;
  }
  .option[aria-selected='true'] { background: var(--accent-primary-subtle); }
  .tile {
    display: inline-grid;
    place-items: center;
    flex: none;
    inline-size: var(--sp-400);
    block-size: var(--sp-400);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
    color: var(--text-secondary);
  }
  .text { display: grid; min-inline-size: 0; }
  .label { font-weight: var(--fw-medium); }
  .detail { color: var(--text-subtle); font-size: var(--fs-075); }
  mark {
    background: none;
    color: inherit;
    font-weight: var(--fw-semibold);
    text-decoration: underline;
    text-underline-offset: var(--sp-025);
  }
  .none { padding: var(--sp-100); color: var(--text-subtle); font-size: var(--fs-075); }
  .keys {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-200);
    padding: var(--sp-100) var(--sp-100) var(--sp-025);
    border-block-start: var(--bw-hairline) solid var(--border-subtle);
    color: var(--text-subtle);
    font-size: var(--fs-075);
  }
  @media (pointer: coarse) {
    .option { min-block-size: var(--layout-nav-touchrow); }
  }
</style>
