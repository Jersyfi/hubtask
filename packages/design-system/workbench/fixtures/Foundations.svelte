<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The tokens' visual reference, read out of the tokens rather than drawn beside them.
  //
  // This is what ADR-0037 made the condition for retiring `reference/foundations.html`: "worth
  // doing once the foundations pages are generated from `tokens.json` rather than written by hand
  // - which is a task, not a side effect". Every swatch, every bar and every name below comes from
  // `dist/tokens.ts`, so a step added to the source appears here without anybody remembering to
  // add it, and a step removed cannot leave a square behind that no longer stands for anything.
  //
  // A fixture rather than a component: nothing consumes it, §4 plans no `Foundations`, and
  // `check-stories.js` exempts this directory from the inventory for exactly that reason.
  //
  // It gains what a static page could not have: both themes side by side, the writing direction,
  // the +40 % pseudo-locale, 200 % zoom and the reduced-motion axis - the matrix of ADR-0037,
  // applied to the foundations themselves.

  import { labelTokens, primitive, tokens } from '../../dist/tokens.ts';

  interface Props {
    /** Which half of the foundations to show. One page holding all of it scrolls past reading. */
    section?: 'colour' | 'scale' | 'depth' | 'motion' | 'mark';
  }

  const { section = 'colour' }: Props = $props();

  /** `Object.entries` with the shape the loops below want, and the token name kept. */
  const entries = (group: Record<string, string>) => Object.entries(group);

  const families = Object.entries(primitive.color) as [string, Record<string, string>][];

  /** The pairs a reader checks: text on a surface, and the surface it is read on. */
  const readingPairs = [
    { text: tokens.text.primary, on: tokens.bg.canvas, name: 'text.primary on bg.canvas' },
    { text: tokens.text.secondary, on: tokens.bg.surface, name: 'text.secondary on bg.surface' },
    { text: tokens.text.subtle, on: tokens.bg.surface, name: 'text.subtle on bg.surface' },
    { text: tokens.text.brand, on: tokens.bg.surface, name: 'text.brand on bg.surface' },
    { text: tokens.text.danger, on: tokens.bg.surface, name: 'text.danger on bg.surface' },
    { text: tokens.text.success, on: tokens.bg.surface, name: 'text.success on bg.surface' },
    { text: tokens.text.warning, on: tokens.bg.surface, name: 'text.warning on bg.surface' },
    { text: tokens.text.inverse, on: tokens.accent.primary, name: 'text.inverse on accent.primary' },
  ];
</script>

{#if section === 'colour'}
  <section class="block">
    <h2>The families</h2>
    <p class="note">
      Two brand colours with different jobs — blue carries structure and interaction, ember is the
      signature and carries no system meaning — and the three functional ramps, kept apart from
      both. Every step is painted with its own custom property, so each pane shows what that step
      resolves to in its mode.
    </p>
    {#each families as [family, steps] (family)}
      <div class="ramp">
        <span class="ramp-name">{family}</span>
        <div class="swatches">
          {#each entries(steps) as [step, value] (step)}
            <span class="swatch" style:background={value} title={`${family}-${step}`}>
              <span class="swatch-step">{step}</span>
            </span>
          {/each}
        </div>
      </div>
    {/each}
  </section>

  <section class="block">
    <h2>Text on a surface</h2>
    <p class="note">
      The pairs `test/contrast.test.js` measures. It fails below 4.5:1 for text and 3:1 for a
      control boundary, in both modes — so this page shows the pairs and the test decides whether
      they are legal.
    </p>
    <div class="pairs">
      {#each readingPairs as pair (pair.name)}
        <div class="pair" style:background={pair.on} style:color={pair.text}>
          <span class="pair-sample">The quick brown fox</span>
          <span class="pair-name">{pair.name}</span>
        </div>
      {/each}
    </div>
  </section>

  <section class="block">
    <h2>The ten label tokens</h2>
    <p class="note">
      What a label may be, and nothing else (ADR-0029). The core knows these ten names and not
      their colours — `LabelTokens.go` carries the vocabulary and stays colour-blind.
    </p>
    <div class="labels">
      {#each labelTokens as name (name)}
        <span
          class="label-chip"
          style:background={tokens.label[name].bg}
          style:color={tokens.label[name].fg}
        >
          {name}
        </span>
      {/each}
    </div>
  </section>
{:else if section === 'scale'}
  <section class="block">
    <h2>Space</h2>
    <p class="note">Every gap, padding and offset in the product is one of these. Nothing between.</p>
    {#each entries(primitive.space) as [step, value] (step)}
      <div class="row">
        <span class="row-name">{step}</span>
        <span class="bar" style:inline-size={value}></span>
      </div>
    {/each}
  </section>

  <section class="block">
    <h2>Radius, and the widths of a boundary</h2>
    <div class="tiles">
      {#each entries(primitive.radius) as [step, value] (step)}
        <span class="tile" style:border-radius={value}>{step}</span>
      {/each}
    </div>
    <div class="tiles">
      {#each entries(primitive.borderWidth) as [step, value] (step)}
        <span class="tile bordered" style:border-width={value}>{step}</span>
      {/each}
    </div>
  </section>

  <section class="block">
    <h2>One superfamily</h2>
    <p class="note">
      IBM Plex, shipped with the bundle — a self-hosted Hubtask contacts nobody on load. The
      display face carries headings, the text face everything read, the mono face what is copied.
    </p>
    {#each entries(primitive.fontSize) as [step, value] (step)}
      <div class="type-row">
        <span class="row-name">{step}</span>
        <span style:font-size={value}>Hubtask · the quick brown fox</span>
      </div>
    {/each}
    <div class="faces">
      {#each entries(primitive.fontFamily) as [name, value] (name)}
        <span style:font-family={value}>{name}</span>
      {/each}
    </div>
  </section>

  <section class="block">
    <h2>The layering scale</h2>
    <p class="note">
      Where an element sits when it leaves the flow, ten apart so a component may sit one above its
      own layer without borrowing the next one's rank. What `Escape` reaches is a different list
      and lives in `src/layers.ts`.
    </p>
    {#each entries(primitive.layer) as [name, value] (name)}
      <div class="row">
        <span class="row-name">{name}</span>
        <span class="z-value">{value}</span>
      </div>
    {/each}
  </section>
{:else if section === 'depth'}
  <section class="block">
    <h2>Four levels, no more</h2>
    <p class="note">
      Rule 1: depth carries meaning. Raised is a standalone element, nested is a child element,
      glass is a temporary overlay — no shadow without one of those three reasons.
    </p>
    <div class="cards">
      {#each entries(tokens.elevation) as [name, value] (name)}
        <div class="card" style:box-shadow={value}>{name}</div>
      {/each}
    </div>
  </section>

  <section class="block">
    <h2>The lit rim, and the ambient tints</h2>
    <p class="note">
      A glass surface is identified by its shadow and its content; the rim is what lights its edge.
      The ambient tints are laid over the canvas as gradient stops, which is why the contrast test
      measures them as a canvas variant rather than as a pair of their own.
    </p>
    <div class="cards">
      {#each entries(tokens.rim) as [name, value] (name)}
        <div class="card rim" style:border-color={value}>rim.{name}</div>
      {/each}
      {#each entries(tokens.ambient) as [name, value] (name)}
        <div class="card" style:background={value}>ambient.{name}</div>
      {/each}
    </div>
    <div class="cards">
      <div class="card glass">bg.glass over the canvas</div>
    </div>
  </section>
{:else if section === 'motion'}
  <section class="block">
    <h2>Durations and curves</h2>
    <p class="note">
      Rule 6: motion lives in `opacity` and `transform` and nowhere else. Switch the Motion axis to
      Reduced — every bar below stops, which is the floor the rule fixes.
    </p>
    {#each entries(primitive.duration) as [name, value] (name)}
      <div class="row">
        <span class="row-name">{name}</span>
        <span class="mover" style:animation-duration={value}></span>
      </div>
    {/each}
    {#each entries(primitive.easing) as [name, value] (name)}
      <div class="row">
        <span class="row-name">{name}</span>
        <span class="mover" style:animation-timing-function={value}></span>
      </div>
    {/each}
  </section>
{:else}
  <section class="block">
    <h2>The wordmark, unfinished</h2>
    <p class="note">
      Three nested planes, the innermost in the signature bordeaux. It is <strong>not a finished
      mark</strong> — `design-system.md` §9 lists it as missing, and a brand mark is design work
      with an owner rather than a session's output. It is here because it was the only drawn record
      of the idea, and it moved with the page that used to hold it.
    </p>
    <div class="logo">
      <svg class="logo-mark" viewBox="0 0 24 24" aria-hidden="true">
        <rect x="1" y="1" width="22" height="22" rx="6" fill="var(--blue-700)" />
        <rect x="5" y="5" width="14" height="14" rx="4" fill="var(--blue-500)" opacity=".8" />
        <rect x="8.5" y="8.5" width="7" height="7" rx="2.2" fill="var(--ember-400)" />
      </svg>
      <span class="logo-word">Hubtask</span>
    </div>
  </section>
{/if}

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: var(--sp-150);
    margin-block-end: var(--sp-400);
  }

  h2 {
    margin: 0;
    font-family: var(--font-display);
    font-size: var(--fs-300);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-tight);
  }

  .note {
    max-width: 68ch;
    margin: 0;
    color: var(--text-secondary);
    font-size: var(--fs-100);
  }

  .ramp { display: flex; flex-direction: column; gap: var(--sp-050); }

  .ramp-name,
  .row-name {
    color: var(--text-subtle);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }

  .row-name { min-inline-size: 8ch; flex: none; }

  .swatches { display: flex; flex-wrap: wrap; gap: var(--sp-025); }

  .swatch {
    display: flex;
    align-items: flex-end;
    justify-content: center;
    inline-size: var(--sp-600);
    block-size: var(--sp-500);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-sm);
  }

  .swatch-step {
    padding: var(--sp-025);
    background: var(--bg-surface);
    border-radius: var(--r-xs);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
  }

  .pairs { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .pair {
    display: flex;
    flex-direction: column;
    gap: var(--sp-025);
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
  }

  .pair-sample { font-size: var(--fs-200); }
  .pair-name { font-family: var(--font-mono); font-size: var(--fs-050); opacity: 0.8; }

  .labels { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .label-chip {
    padding: var(--sp-025) var(--sp-150);
    border-radius: var(--r-full);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .row,
  .type-row {
    display: flex;
    align-items: center;
    gap: var(--sp-200);
  }

  .type-row { align-items: baseline; }

  .bar {
    block-size: var(--sp-150);
    background: var(--accent-primary);
    border-radius: var(--r-xs);
  }

  .z-value { font-family: var(--font-mono); font-size: var(--fs-100); }

  .tiles { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .tile {
    display: flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-800);
    block-size: var(--sp-600);
    background: var(--bg-surface-sunken);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-075);
  }

  .tile.bordered {
    border-style: solid;
    border-color: var(--border-strong);
    background: var(--bg-surface);
  }

  .faces { display: flex; flex-wrap: wrap; gap: var(--sp-300); font-size: var(--fs-200); }

  .cards { display: flex; flex-wrap: wrap; gap: var(--sp-300); }

  .card {
    display: flex;
    align-items: center;
    justify-content: center;
    inline-size: var(--sp-1000);
    block-size: var(--sp-600);
    padding: var(--sp-100);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    color: var(--text-secondary);
    font-family: var(--font-mono);
    font-size: var(--fs-050);
    text-align: center;
  }

  .card.rim { border: var(--bw-thick) solid var(--border-subtle); }

  .card.glass {
    background: var(--bg-glass);
    border: var(--bw-hairline) solid var(--border-glass);
    box-shadow: var(--shadow-overlay);
    backdrop-filter: blur(var(--sp-100));
    inline-size: auto;
    padding-inline: var(--sp-300);
  }

  /* Rule 6 in the one place this page moves: opacity and transform, and the axis stops it. */
  .mover {
    /* `translate` is physical: its X is the screen's X and not the writing direction's, and there
       is no logical equivalent. So the distance is a custom property the keyframe reads, and the
       rule below negates it in RTL - the same arithmetic Switch.svelte does for its knob, and the
       reason §3 bans a bare `left`/`right`: a bar that travels the wrong way is invisible in
       English and wrong in Arabic. */
    --travel: var(--sp-1000);
    inline-size: var(--sp-250);
    block-size: var(--sp-250);
    border-radius: var(--r-full);
    background: var(--accent-signature);
    animation-name: travel;
    animation-iteration-count: infinite;
    animation-direction: alternate;
  }

  .mover:dir(rtl) { --travel: calc(-1 * var(--sp-1000)); }

  @keyframes travel {
    from { transform: translateX(0); }
    to { transform: translateX(var(--travel)); }
  }

  .logo { display: flex; align-items: center; gap: var(--sp-150); }

  .logo-mark { inline-size: var(--sp-400); block-size: var(--sp-400); flex: none; }

  .logo-word {
    font-family: var(--font-display);
    font-size: var(--fs-400);
    font-weight: var(--fw-semibold);
  }

  @media (prefers-reduced-motion: reduce) {
    .mover { animation-name: none; }
  }

  :global([data-motion='reduced']) .mover { animation-name: none; }
</style>
