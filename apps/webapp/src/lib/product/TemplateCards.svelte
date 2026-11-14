<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A product's templates as cards under its first empty state (scene empty): name and one line,
  // and Use where the product's mockup draws it - `stacked` with *Use*, `row` mark beside the words.
  import { ProductTile, type IconName } from '@hubtask/design-system/components';
  import type { ComponentProps } from 'svelte';

  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    templates: readonly { label: string; onclick: () => void; description?: string; icon?: IconName }[];
    shape?: 'stacked' | 'row';
    /** The product's tile, whose colour each card's mark takes. */
    tile?: ComponentProps<typeof ProductTile>['product'];
  }

  const { templates, shape: templateShape = 'stacked', tile }: Props = $props();
</script>

<section class="starts" aria-labelledby="product-empty-starts" data-empty-templates>
  <h2 class="eyebrow" id="product-empty-starts">{t('app.product_empty.or_template')}</h2>
  <ul class="cards">
    {#each templates as template (template.label)}
      <li>
        <!-- The whole card starts it; its name is the button's. -->
        <button type="button" class="card" data-shape={templateShape} aria-label={template.label} onclick={template.onclick}>
          {#if template.icon && tile}<span class="mark"><ProductTile icon={template.icon} product={tile} size="sm" /></span>{/if}
          <span class="name">{template.label}</span>
          {#if template.description}<span class="line">{template.description}</span>{/if}
          {#if templateShape === 'stacked'}<span class="use" aria-hidden="true">{t('app.product_empty.use')}</span>{/if}
        </button>
      </li>
    {/each}
  </ul>
</section>

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
