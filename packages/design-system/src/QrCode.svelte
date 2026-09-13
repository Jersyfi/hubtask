<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A QR code, drawn from a matrix `qr.ts` produced (ADR-0053 option B).
  //
  // **Dark on light in both modes, on purpose.** Every other surface in this package follows the
  // theme; this one does not, because a camera is not a reader of the theme. The standard
  // specifies dark modules on a light ground with a light quiet zone around it, and while many
  // scanners also read the inverse, not every authenticator does - and the cost of one that does
  // not is a person who cannot enrol. So the two primitives are named directly rather than
  // through `--text-primary` and `--bg-surface`, which swap in dark mode.
  //
  // **One path, not one rectangle per module.** A version-8 symbol has 2 401 modules; a node per
  // module is a DOM the browser lays out on every resize for no benefit. The path draws each dark
  // module as a unit square in the symbol's own coordinate space, and the viewBox carries the four
  // modules of quiet zone the standard requires on every side.
  //
  // **It is an image and says so.** `role="img"` with the label the caller resolved, because the
  // modules mean nothing to a screen reader and the label is what a person who cannot see the
  // code needs: what it is, and that the value beside it is the same secret typed out.
  //
  // **The value is drawn, never written.** No `<title>` and no `<desc>` carry the payload: a
  // provisioning URI in the DOM as text is the secret in a form a screen reader would read aloud
  // and a copy-all would take. The image is the one place the secret appears in, and it appears
  // there as a picture.

  import type { Matrix } from './qr.ts';

  interface Props {
    /** The symbol, from `encode`. */
    matrix: Matrix;
    /** What the image is, resolved text (ADR-0011): the accessible name. */
    label: string;
  }

  const { matrix, label }: Props = $props();

  /** The standard's quiet zone: four modules on every side. */
  const QUIET = 4;

  const extent = $derived(matrix.size + 2 * QUIET);

  const path = $derived.by(() => {
    const parts: string[] = [];
    matrix.modules.forEach((row, r) => {
      row.forEach((dark, c) => {
        if (dark) parts.push(`M${c + QUIET} ${r + QUIET}h1v1h-1z`);
      });
    });
    return parts.join('');
  });
</script>

<svg
  class="qr"
  viewBox="0 0 {extent} {extent}"
  role="img"
  aria-label={label}
  shape-rendering="crispEdges"
>
  <rect class="ground" width={extent} height={extent} />
  <path class="modules" d={path} />
</svg>

<style>
  /* Square, as wide as its container allows and never wider than the symbol needs to be scanned
     across a desk. The bound is spacing tokens multiplied rather than a length written here. */
  .qr {
    display: block;
    inline-size: 100%;
    max-inline-size: calc(var(--sp-1600) * 2);
    aspect-ratio: 1;
    border-radius: var(--r-md);
  }

  /* The primitives, not the semantic pair: see the note at the top. */
  .ground { fill: var(--n-0); }

  .modules { fill: var(--n-1000); }
</style>
