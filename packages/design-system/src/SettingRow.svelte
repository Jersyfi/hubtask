<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // One setting, one row (page-system §6, PS-11): at the start its name, where its value comes
  // from (`SourceChip`) and one line of help; at the end the control at its field width (PS-13),
  // *Reset* where it is set here, the lock's reason where it is locked. Two columns from `medium`
  // up, stacked on a phone. A row inside a region: rows are parted by hairlines, groups by a quiet
  // label, never by a box (§4.2 rule 1).
  //
  // The control is the caller's, handed the id of the name so it can be labelled by it
  // (`aria-labelledby`) and still draw no label of its own.
  import type { Snippet } from 'svelte';

  import Button from './Button.svelte';
  import Icon from './Icon.svelte';

  interface Props {
    /** The setting's name. */
    label: string;
    /** One line of help. */
    help?: string;
    /** A quiet uppercase label above this row: the first row of a group carries its group's name. */
    groupLabel?: string;
    /** Whether the value is required where the row belongs to a form on a page. */
    isRequired?: boolean;
    /** Where the value comes from: a `SourceChip`. */
    source?: Snippet;
    /** The control, handed the id of the name. */
    control: Snippet<[string]>;
    /** How wide the control is: as wide as its content. */
    fieldWidth?: 'xs' | 'sm' | 'md' | 'lg' | 'full';
    /** Words after the control: a unit. */
    unit?: string;
    /** *Reset*, where the value is set here. */
    reset?: { label: string; onclick: () => void };
    /** Why it cannot be changed here. */
    lockReason?: string;
    /** Whether the value differs from what is saved: a bar at the row's start says so. */
    isChanged?: boolean;
  }

  const { label, help, groupLabel, isRequired = false, source, control, fieldWidth = 'md', unit, reset, lockReason, isChanged = false }: Props = $props();

  const uid = $props.id();
  const nameId = `setting-${uid}`;
</script>

<div class="setting setting-row">
  {#if groupLabel}<div class="group-label">{groupLabel}</div>{/if}
  <div class="row setting-line" data-changed={isChanged ? '' : undefined}>
    <div class="about">
      <div class="name">
        <span id={nameId}>{label}</span>
        {#if isRequired}<span class="required" aria-hidden="true">*</span>{/if}
        {#if source}{@render source()}{/if}
      </div>
      {#if help}<div class="help">{help}</div>{/if}
    </div>
    <div class="value">
      <span class="control" data-width={fieldWidth}>{@render control(nameId)}</span>
      {#if unit}<span class="unit">{unit}</span>{/if}
      {#if reset}<Button tone="subtle" size="sm" onclick={reset.onclick}>{reset.label}</Button>{/if}
      {#if lockReason}<span class="lock"><Icon name="lock" size="sm" />{lockReason}</span>{/if}
    </div>
  </div>
</div>

<style>
  /* A hairline between rows; a group's label parts two groups by itself. */
  :global(.setting-row + .setting-row > .setting-line:first-child) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .group-label {
    padding: var(--sp-150) var(--sp-250) var(--sp-050);
    color: var(--text-subtle);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    text-transform: uppercase;
  }

  .row {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
    align-items: center;
    gap: var(--sp-300);
    padding: var(--sp-200) var(--sp-250);
  }

  .row[data-changed] { box-shadow: inset var(--bw-thick) 0 0 var(--accent-primary); }
  :global([dir='rtl']) .row[data-changed] { box-shadow: inset calc(var(--bw-thick) * -1) 0 0 var(--accent-primary); }

  .name { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); font-weight: var(--fw-medium); overflow-wrap: anywhere; }
  .required { color: var(--text-danger); }
  .help { margin-block-start: var(--sp-025); color: var(--text-secondary); font-size: var(--fs-100); }

  .value { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-150); min-width: 0; }
  .control { display: block; max-inline-size: 100%; }
  .control[data-width='xs'] { inline-size: var(--layout-field-xs); }
  .control[data-width='sm'] { inline-size: var(--layout-field-sm); }
  .control[data-width='md'] { inline-size: var(--layout-field-md); }
  .control[data-width='lg'] { inline-size: var(--layout-field-lg); }
  .control[data-width='full'] { inline-size: 100%; }
  .unit { color: var(--text-subtle); font-size: var(--fs-100); }
  .lock { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-secondary); font-size: var(--fs-100); }

  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .row { grid-template-columns: minmax(0, 1fr); gap: var(--sp-150); padding: var(--sp-200); }
    .group-label { padding-inline: var(--sp-200); }
    .control[data-width='md'],
    .control[data-width='lg'] { inline-size: 100%; }
  }
</style>
