<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The two choices of a hub's or a collection's look (UC-WRK-33): one of the ten label colours and
  // one symbol from the set on offer, each with "none" first. The same fields in the create dialog
  // and in the edit dialog, so the two never offer different things.
  //
  // **Two radio groups by hand**, as the labels dialog draws its colours: what a reader chooses
  // between is the tiles themselves. Each tile previews the place - the colours carry the chosen
  // symbol, the symbols the chosen colour - so the pair is seen before it is saved. Every option has
  // a name in words (rule 3): a colour is never told by its colour alone, a symbol never by its
  // shape alone. The chosen one is marked by the ring, the shape every control here focuses with.
  //
  // Here rather than in the design system: a picker of a place's look is this application's, and
  // the design system grows in the order of design-system.md §4, never as a side effect.

  import { labelTokens, type LabelToken } from '@hubtask/design-system';

  import { t } from '../i18n/i18n.svelte.ts';
  import ContainerMark from './ContainerMark.svelte';
  import { SYMBOLS, type PlaceSymbol } from './look.ts';

  interface Props {
    type: 'HUB' | 'COLLECTION';
    symbol: PlaceSymbol | undefined;
    colour: LabelToken | undefined;
  }

  let { type, symbol = $bindable(), colour = $bindable() }: Props = $props();

  const id = $props.id();

  // `bind:group` cannot hold `undefined` beside strings, so "none" is the empty string in the
  // group and `undefined` outside it.
  let colourChoice = $derived<string>(colour ?? '');
  let symbolChoice = $derived<string>(symbol ?? '');
</script>

<fieldset class="choices">
  <legend>{t('app.workspace.colour')}</legend>
  {#each ['', ...labelTokens] as token (token)}
    <label class="option">
      <input
        type="radio"
        name="{id}-colour"
        value={token}
        aria-label={token === '' ? t('app.look.colour_none') : t(`app.colour.${token}`)}
        checked={colourChoice === token}
        onchange={() => (colour = token === '' ? undefined : (token as LabelToken))}
      />
      <ContainerMark look={{ type, icon: symbol ?? null, color_token: token === '' ? null : token }} size="md" />
    </label>
  {/each}
</fieldset>

<fieldset class="choices">
  <legend>{t('app.look.symbol')}</legend>
  {#each ['', ...SYMBOLS] as name (name)}
    <label class="option">
      <input
        type="radio"
        name="{id}-symbol"
        value={name}
        aria-label={name === '' ? t('app.look.symbol_none') : t(`app.symbol.${name}`)}
        checked={symbolChoice === name}
        onchange={() => (symbol = name === '' ? undefined : (name as PlaceSymbol))}
      />
      <ContainerMark look={{ type, icon: name === '' ? null : name, color_token: colour ?? null }} size="md" />
    </label>
  {/each}
</fieldset>

<style>
  .choices {
    display: flex;
    flex-wrap: wrap;
    gap: var(--sp-100);
    margin: 0;
    padding: 0;
    border: 0;
  }

  .choices legend {
    padding: 0;
    margin-block-end: var(--sp-050);
    color: var(--text-secondary);
    font-size: var(--fs-075);
  }

  .option {
    display: inline-flex;
    border-radius: var(--r-md);
    cursor: pointer;
  }

  /* The radio stays in the tab order and keeps the arrow keys a group has; the tile is its face.
     A tile with no colour is drawn on the page, so it carries a hairline to be found as a control. */
  .option input { position: absolute; opacity: 0; pointer-events: none; }
  .option :global(.mark:not([data-token])) { box-shadow: inset 0 0 0 var(--bw-hairline) var(--border-subtle); }
  .option:has(input:checked) { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--sp-025); }
  .option:has(input:focus-visible) { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
