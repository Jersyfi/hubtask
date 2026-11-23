<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The two choices of a hub's or a collection's look (UC-WRK-33): one of the ten label colours and
  // one symbol from the set on offer, each with "none" first. The same fields wherever a place is
  // made or changed, so the two never offer different things.
  //
  // **Two radio groups by hand**, as the labels dialog draws its colours: what a reader chooses
  // between is the tiles themselves - a colour's round swatch, a symbol's square (the create-hub
  // draft). Every option has a name in words (rule 3): a colour is never told by its colour alone,
  // a symbol never by its shape alone. The chosen colour is ringed, the chosen symbol takes the
  // chosen surface and edge, as a chosen tile does; "none" is dashed, as the start from nothing is.
  // The place's own mark beside its name previews the pair.
  //
  // Here rather than in the design system: a picker of a place's look is this application's, and
  // the design system grows in the order of design-system.md §4, never as a side effect.

  import { labelTokens, type LabelToken } from '@hubtask/design-system';
  import { Icon } from '@hubtask/design-system/components';

  import { t } from '../i18n/i18n.svelte.ts';
  import { SYMBOLS, type PlaceSymbol } from './look.ts';

  interface Props {
    symbol: PlaceSymbol | undefined;
    colour: LabelToken | undefined;
  }

  let { symbol = $bindable(), colour = $bindable() }: Props = $props();

  const id = $props.id();

  // `bind:group` cannot hold `undefined` beside strings, so "none" is the empty string in the
  // group and `undefined` outside it.
  let colourChoice = $derived<string>(colour ?? '');
  let symbolChoice = $derived<string>(symbol ?? '');
</script>

<fieldset class="choices">
  <legend>{t('app.workspace.colour')}</legend>
  <div class="options">
    {#each ['', ...labelTokens] as token (token)}
      {@const name = token === '' ? t('app.look.colour_none') : t(`app.colour.${token}`)}
      <label class="option swatch" data-token={token || undefined} data-none={token === '' ? '' : undefined} title={name}>
        <input
          type="radio"
          name="{id}-colour"
          value={token}
          aria-label={name}
          checked={colourChoice === token}
          onchange={() => (colour = token === '' ? undefined : (token as LabelToken))}
        />
        {#if token === ''}<Icon name="ban" size="sm" />{/if}
      </label>
    {/each}
  </div>
</fieldset>

<fieldset class="choices">
  <legend>{t('app.look.symbol')}</legend>
  <div class="options">
    {#each ['', ...SYMBOLS] as each (each)}
      {@const name = each === '' ? t('app.look.symbol_none') : t(`app.symbol.${each}`)}
      <label class="option symbol" data-none={each === '' ? '' : undefined} title={name}>
        <input
          type="radio"
          name="{id}-symbol"
          value={each}
          aria-label={name}
          checked={symbolChoice === each}
          onchange={() => (symbol = each === '' ? undefined : (each as PlaceSymbol))}
        />
        <Icon name={each === '' ? 'ban' : (each as PlaceSymbol)} size="sm" />
      </label>
    {/each}
  </div>
</fieldset>

<style>
  .choices { min-inline-size: 0; margin: 0; padding: 0; border: 0; }

  .choices legend {
    padding: 0;
    color: var(--text-secondary);
    font-size: var(--fs-090);
    font-weight: var(--fw-medium);
  }

  .options { display: flex; flex-wrap: wrap; gap: var(--sp-050); margin-block-start: var(--sp-050); }

  .option {
    box-sizing: border-box;
    position: relative;
    display: inline-grid;
    place-items: center;
    border: var(--bw-hairline) solid transparent;
    cursor: pointer;
  }

  /* The radio stays in the tab order and keeps the arrow keys a group has; the tile is its face. */
  .option input { position: absolute; inset: 0; margin: 0; opacity: 0; cursor: inherit; }

  .swatch {
    inline-size: calc(var(--sp-300) + var(--sp-050));
    block-size: calc(var(--sp-300) + var(--sp-050));
    border-radius: var(--r-full);
  }

  .swatch:has(input:checked) { box-shadow: 0 0 0 var(--bw-ring) var(--bg-surface), 0 0 0 calc(var(--bw-ring) * 2) var(--accent-primary); }

  .symbol {
    inline-size: var(--density-control-sm-min);
    block-size: var(--density-control-sm-min);
    border-radius: var(--r-md);
    color: var(--text-secondary);
  }

  .symbol :global(svg) { inline-size: var(--fs-200); block-size: var(--fs-200); }
  .symbol:hover { background: var(--bg-surface-hover); }
  .symbol:has(input:checked) { border-color: var(--accent-primary); background: var(--accent-primary-subtle); color: var(--text-brand); }

  /* "None" is dashed, as the start from nothing is: a choice that sets nothing reads as one. */
  .option[data-none] { border: var(--bw-hairline) dashed var(--border-default); color: var(--text-subtle); }
  .symbol[data-none]:has(input:checked) { border-style: solid; color: var(--text-brand); }

  .option:has(input:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .swatch[data-token='slate'] { background: var(--label-slate-bg); }
  .swatch[data-token='blue'] { background: var(--label-blue-bg); }
  .swatch[data-token='teal'] { background: var(--label-teal-bg); }
  .swatch[data-token='green'] { background: var(--label-green-bg); }
  .swatch[data-token='lime'] { background: var(--label-lime-bg); }
  .swatch[data-token='amber'] { background: var(--label-amber-bg); }
  .swatch[data-token='orange'] { background: var(--label-orange-bg); }
  .swatch[data-token='red'] { background: var(--label-red-bg); }
  .swatch[data-token='magenta'] { background: var(--label-magenta-bg); }
  .swatch[data-token='violet'] { background: var(--label-violet-bg); }
</style>
