<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A time zone, chosen from the platform's list rather than typed (user-experience.md §5.4). The
  // held value is always among the options - an alias such as UTC included - or the control would
  // show nothing selected (`zoneOptions`). A browser that lists no zones gets free text instead,
  // and the server still refuses a name it cannot resolve. The device's zone as the default is
  // the caller's starting value (`deviceZone()`), so an edit form keeps what it was given.

  import { Input, Select } from '@hubtask/design-system/components';

  import { withWorkspaceChoice, zoneOptions } from '../data/preferences.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    label: string;
    hint?: string;
    /** The hint where the platform lists no zones and the reader types one. */
    freeHint?: string;
    value: string;
    error?: string;
    isRequired?: boolean;
    /** An empty choice in front - "use the workspace's" - for a setting that may inherit. */
    inheritLabel?: string;
    onchange?: (zone: string) => void;
    /** The name drawn beside it rather than above it: a setting row's. */
    isLabelHidden?: boolean;
    'aria-labelledby'?: string;
    [attribute: `data-${string}`]: string | undefined;
  }

  let { label, hint, freeHint, value = $bindable(), error, isRequired = false, inheritLabel, onchange, ...rest }: Props = $props();

  const zones = $derived(zoneOptions(value));
  const options = $derived.by(() => {
    const listed = zones.map((each) => ({ value: each, label: each }));
    return inheritLabel ? withWorkspaceChoice(listed, inheritLabel) : listed;
  });
</script>

{#if zones.length > 0}
  <Select
    {label}
    {hint}
    {error}
    {isRequired}
    {options}
    {value}
    {...rest}
    onchange={(event: Event) => {
      value = (event.currentTarget as HTMLSelectElement).value;
      onchange?.(value);
    }}
  />
{:else}
  <Input
    {label}
    hint={freeHint ?? t('app.desk.zone_hint')}
    {error}
    {isRequired}
    {value}
    {...rest}
    oninput={(event: Event) => {
      // From the event: a handler beside a binding may run before the binding has written.
      value = (event.currentTarget as HTMLInputElement).value;
      onchange?.(value);
    }}
  />
{/if}
