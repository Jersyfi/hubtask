<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's empty answer (user-experience.md §5.5): `EmptyState` filled the one way every
  // product fills it. First use says what the product is for, offers its primary create act and its
  // templates and links the product's guide; a reader who may not create is told whom to ask - by
  // name, from the place's explanation - instead of being offered nothing.
  // Filtered offers to clear the filter; settled offers nothing (voice-and-tone.md §4).

  import { untrack } from 'svelte';
  import { Button, EmptyState, type IconName } from '@hubtask/design-system/components';
  import type { AccessExplanation, ResourceState } from '@hubtask/sync-engine';

  import { viewport } from '../frame/viewport.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { guideOf, whoToAsk } from './guide.ts';
  import { productOf } from '../navigation.ts';
  import { productTour } from './producttour.svelte.ts';

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
    /** Who may create, said where the reader may not (`create` absent); else read from `place`. */
    whoCan?: string;
    /** The product (its module key), whose guide the first use links where docs_url is set. */
    product?: string;
    /**
     * The place whose explanation names whom to ask where the reader may not create and `whoCan`
     * is not given: the workspace, a hub or a collection.
     */
    place?: { readonly type: 'TENANT' | 'HUB' | 'COLLECTION'; readonly id?: string };
    /**
     * Whether `create` is the page head's primary too: on a phone the head pins it where the thumb
     * is, so the empty state draws no second one beside it (user-experience.md §5.2).
     */
    isInHead?: boolean;
  }

  const { kind, title, description, icon, create, clear, templates = [], whoCan, product, place, isInHead = false }: Props = $props();

  const act = $derived(kind === 'filtered' ? clear : isInHead && viewport.isCompact ? undefined : create);
  const offersTemplates = $derived(kind === 'unused' && create !== undefined && templates.length > 0);
  // Whom to ask, from the place's explanation (§5.5), read only where it is said.
  const asksPlace = $derived(kind === 'unused' && create === undefined && whoCan === undefined && place !== undefined);
  const explainPath = $derived(
    asksPlace && place ? `/access/explain?scope_type=${place.type}${place.id ? `&scope_id=${encodeURIComponent(place.id)}` : ''}` : '',
  );
  let explained = $state<ResourceState<AccessExplanation>>({ status: 'idle' });
  $effect(() => {
    const path = explainPath;
    if (!path) return;
    return untrack(() => engine.subscribe<AccessExplanation>({ path }, (next) => (explained = next)));
  });
  const asked = $derived(
    whoCan ?? (asksPlace && explained.status !== 'loading' && explained.status !== 'idle'
      ? whoToAsk(explained.status === 'ready' ? explained.data.administrators : undefined, t)
      : undefined),
  );
  const saysWhoCan = $derived(kind === 'unused' && create === undefined && asked !== undefined);
  const guide = $derived(kind === 'unused' && product ? guideOf(manifest.value?.legal?.docs_url, product) : undefined);
  // The product's own tour, offered from its first empty state (user-experience.md §3.4). Not on a
  // phone, where the head's pinned primary sits over the footnote; Help offers it there.
  const toured = $derived(kind === 'unused' && product && !viewport.isCompact ? productOf(product) : undefined);
</script>

{#snippet actRow()}
  {#if act}
    <Button tone={kind === 'filtered' ? 'secondary' : 'primary'} icon={kind === 'filtered' ? undefined : 'plus'} data-tour={kind === 'filtered' ? undefined : 'product-create'} onclick={act.onclick}>{act.label}</Button>
  {/if}
{/snippet}

{#snippet templateRow()}
  {#each templates as template (template.label)}
    <Button tone="secondary" onclick={template.onclick}>{template.label}</Button>
  {/each}
{/snippet}

{#snippet footnoteLine()}
  {#if saysWhoCan}{asked}{/if}
  {#if guide}{#if saysWhoCan}{' '}{/if}<a href={guide} target="_blank" rel="noopener">{t('app.product_empty.guide')}</a>{/if}
  {#if toured}
    <Button tone="subtle" size="sm" icon="compass" data-product-tour={toured.module} onclick={() => void productTour.start(toured.module)}>
      {t('app.product_tour.start', { product: t(toured.code) })}
    </Button>
  {/if}
{/snippet}

<EmptyState
  {kind}
  {title}
  {description}
  {icon}
  action={act ? actRow : undefined}
  secondary={offersTemplates ? templateRow : undefined}
  footnote={saysWhoCan || guide || toured ? footnoteLine : undefined}
/>
