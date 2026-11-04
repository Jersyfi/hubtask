<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's empty answer (user-experience.md §5.5): `EmptyState` filled the one way every
  // product fills it. First use says what the product is for, offers its primary create act and its
  // templates; a reader who may not create is told who can instead of being offered nothing.
  // Filtered offers to clear the filter; settled offers nothing (voice-and-tone.md §4).

  import { Button, EmptyState, type IconName } from '@hubtask/design-system/components';

  import { viewport } from '../frame/viewport.svelte.ts';

  /** One act on the empty screen: resolved label, what pressing it does. */
  interface Act {
    label: string;
    onclick: () => void;
  }

  interface Props {
    kind: 'unused' | 'filtered' | 'settled';
    title: string;
    description?: string;
    icon?: IconName;
    /** The primary create act, where the reader may create. */
    create?: Act;
    /** The filter's way back, on `filtered`. */
    clear?: Act;
    /** The product's templates, two or three, each starting the place from it. */
    templates?: readonly Act[];
    /** Who may create, said where the reader may not (`create` absent). */
    whoCan?: string;
    /**
     * Whether `create` is the page head's primary too: on a phone the head pins it where the thumb
     * is, so the empty state draws no second one beside it (user-experience.md §5.2).
     */
    isInHead?: boolean;
  }

  const { kind, title, description, icon, create, clear, templates = [], whoCan, isInHead = false }: Props = $props();

  const act = $derived(kind === 'filtered' ? clear : isInHead && viewport.isCompact ? undefined : create);
  const offersTemplates = $derived(kind === 'unused' && create !== undefined && templates.length > 0);
  const saysWhoCan = $derived(kind === 'unused' && create === undefined && whoCan !== undefined);
</script>

{#snippet actRow()}
  {#if act}
    <Button tone={kind === 'filtered' ? 'secondary' : 'primary'} icon={kind === 'filtered' ? undefined : 'plus'} onclick={act.onclick}>{act.label}</Button>
  {/if}
{/snippet}

{#snippet templateRow()}
  {#each templates as template (template.label)}
    <Button tone="secondary" onclick={template.onclick}>{template.label}</Button>
  {/each}
{/snippet}

{#snippet whoCanLine()}{whoCan}{/snippet}

<EmptyState
  {kind}
  {title}
  {description}
  {icon}
  action={act ? actRow : undefined}
  secondary={offersTemplates ? templateRow : undefined}
  footnote={saysWhoCan ? whoCanLine : undefined}
/>
