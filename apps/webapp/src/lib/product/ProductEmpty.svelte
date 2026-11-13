<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's empty answer (user-experience.md §5.5): `EmptyState` filled the one way every
  // product fills it. First use says what the product is for, offers its primary create act and its
  // templates and links the product's guide; a reader who may not create is told whom to ask - by
  // name, from the place's explanation - instead of being offered nothing.
  // Filtered offers to clear the filter; settled offers nothing (voice-and-tone.md §4).

  import { untrack } from 'svelte';
  import { Button, EmptyState, ProductTile, type IconName } from '@hubtask/design-system/components';
  import type { AccessExplanation, ResourceState } from '@hubtask/sync-engine';

  import { viewport } from '../frame/viewport.svelte.ts';
  import { manifest } from '../data/capabilities.svelte.ts';
  import { engine } from '../data/engine.ts';
  import { t } from '../i18n/i18n.svelte.ts';
  import { guideOf, whoToAsk } from './guide.ts';
  import { productOf } from '../navigation.ts';
  import { productTour } from './producttour.svelte.ts';
  import { PRODUCT_TILES } from './tiles.ts';

  /** One act on the empty screen: resolved label, what pressing it does. */
  interface Act {
    label: string;
    onclick: () => void;
    /** A template's one line and mark, which draw it as a card (scene empty). */
    description?: string;
    icon?: IconName;
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
    /**
     * A product's home before its first use (scene empty): the product's tile in its colour, the
     * side way beside the main one, and the templates as cards under it.
     */
    isProductHome?: boolean;
    /** The side way beside the main one: start empty, set up first. */
    side?: Act;
    /**
     * How the template cards are drawn, as each product's mockup draws them: `stacked` - mark, name,
     * line and a *Use* mark (scene empty); `row` - mark beside name and line, no *Use* (the plans',
     * inventory's, agents', goals' and time's homes).
     */
    templateShape?: 'stacked' | 'row';
  }

  const { kind, title, description, icon, create, clear, templates = [], whoCan, product, place, isInHead = false, isProductHome = false, side, templateShape = 'stacked' }: Props = $props();
  const tile = $derived(isProductHome && kind === 'unused' && product ? PRODUCT_TILES[product] : undefined);

  const act = $derived(kind === 'filtered' ? clear : isInHead && viewport.isCompact ? undefined : create);
  const offersTemplates = $derived(kind === 'unused' && create !== undefined && templates.length > 0);
  const templateCards = $derived(offersTemplates && isProductHome);
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
  {#if side && kind === 'unused'}<Button tone="secondary" data-empty-side onclick={side.onclick}>{side.label}</Button>{/if}
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
  product={tile}
  action={act || (side && kind === 'unused') ? actRow : undefined}
  secondary={offersTemplates && !templateCards ? templateRow : undefined}
  footnote={saysWhoCan || guide || toured ? footnoteLine : undefined}
/>
{#if templateCards}
  <!-- The templates as cards under the empty state: name and one line, and Use where the product's mockup draws it. -->
  <section class="starts" aria-labelledby="product-empty-starts" data-empty-templates>
    <h2 class="eyebrow" id="product-empty-starts">{t('app.product_empty.or_template')}</h2>
    <ul class="cards">
      {#each templates as template (template.label)}
        <li>
          <!-- The whole card starts it; its name is the button's. -->
          <button type="button" class="card" data-shape={templateShape} onclick={template.onclick}>
            {#if template.icon && tile}<span class="mark"><ProductTile icon={template.icon} product={tile} size="sm" /></span>{/if}
            <span class="name">{template.label}</span>
            {#if template.description}<span class="line">{template.description}</span>{/if}
            {#if templateShape === 'stacked'}<span class="use" aria-hidden="true">{t('app.product_empty.use')}</span>{/if}
          </button>
        </li>
      {/each}
    </ul>
  </section>
{/if}

<style>
  .starts { display: grid; gap: var(--sp-100); margin-block-start: var(--sp-300); }
  .eyebrow { margin: 0; color: var(--text-secondary); font-size: var(--fs-075); font-weight: var(--fw-semibold); text-transform: uppercase; }
  /* design-system-lint-ignore: tracking in em follows the type step it spaces */
  .eyebrow { letter-spacing: 0.06em; }
  .cards { display: grid; grid-template-columns: repeat(auto-fill, minmax(min(100%, var(--layout-gallery-card)), 1fr)); gap: var(--sp-150); margin: 0; padding: 0; list-style: none; }
  .card { display: grid; align-content: start; justify-items: start; gap: var(--sp-100); box-sizing: border-box; inline-size: 100%; block-size: 100%; padding: var(--sp-200); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-lg); background: var(--bg-surface); color: var(--text-primary); font: inherit; text-align: start; cursor: pointer; }
  .card[data-shape='row'] { grid-template-columns: auto minmax(0, 1fr); align-items: center; gap: var(--sp-025) var(--sp-150); padding: var(--sp-150) var(--sp-200); }
  .card[data-shape='row'] .mark { grid-row: span 2; }
  .card[data-shape='row'] .name, .card[data-shape='row'] .line { grid-column: 2; }
  .card:hover { border-color: var(--border-strong); }
  .card:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--bw-ring); }
  .use { padding: var(--sp-050) var(--sp-150); border: var(--bw-hairline) solid var(--border-strong); border-radius: var(--r-md); font-size: var(--fs-090); font-weight: var(--fw-medium); }
  .name { font-weight: var(--fw-semibold); }
  .line { color: var(--text-secondary); font-size: var(--fs-090); }
</style>
