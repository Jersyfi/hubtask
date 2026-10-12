<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The mark before "Sign in with …".
  //
  // **A brand mark is content, not a token.** ADR-0041 governs *our* icon set, where a mark takes
  // `currentColor` because it is ours to colour. A provider's mark is theirs: it is reproduced as
  // its owner publishes it or not at all, which is why the colours below carry a lint exemption
  // each and why they live here rather than in the design system's icon set. Which marks may ship,
  // and in which form, is a question about each owner's guidelines rather than about this file.
  // A mark is drawn only for a preset the server can send (`IdentityProviderKind`, ADR-0069 §3),
  // and each has its row in THIRD-PARTY-LICENSES.md; `make gate-licenses` refuses either missing.
  //
  // **The button stays ours.** Google, Microsoft and Apple all publish a neutral form of their
  // sign-in button - a white surface, a hairline border, the mark at the start - and that is
  // `Button` with `tone="secondary"` and a `lead`. A column of providers then has its marks in one
  // line and its labels on one axis, instead of four brand colours arguing about which matters
  // most.
  //
  // **A workspace's own provider has no brand**, and is not given a borrowed one: it carries a
  // square with the first character of the name it was given, in one of the ten label colours the
  // core already validates. The same answer `Avatar` gives a person without a picture.

  import { labelTokens } from '@hubtask/design-system';

  interface Props {
    /** The preset the provider was configured from. `GENERIC` draws no brand. */
    kind: string;
    /** What it is called here, which is where the letter comes from. */
    name: string;
  }

  const { kind, name }: Props = $props();

  const preset = $derived(kind.toUpperCase());
  /** The first character, as a code point: an emoji or a Greek letter is one character. */
  const initial = $derived([...name.trim()][0]?.toLocaleUpperCase() ?? '?');
  /**
   * Which of the ten, chosen from the name so that the same provider always looks the same. Not
   * random, and not a preference: a colour somebody has to pick is a decision on a screen that has
   * enough of them.
   */
  const token = $derived(
    labelTokens[[...name].reduce((sum, character) => sum + (character.codePointAt(0) ?? 0), 0) % labelTokens.length],
  );
</script>

{#if preset === 'GOOGLE'}
  <svg class="mark" viewBox="0 0 24 24" aria-hidden="true">
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <path d="M21.6 12.2c0-.7-.1-1.3-.2-1.9H12v3.7h5.4a4.6 4.6 0 0 1-2 3v2.5h3.2c1.9-1.7 3-4.3 3-7.3z" fill="#4285F4" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <path d="M12 22c2.7 0 5-.9 6.6-2.4l-3.2-2.5c-.9.6-2 1-3.4 1-2.6 0-4.8-1.8-5.6-4.1H3.1v2.6A10 10 0 0 0 12 22z" fill="#34A853" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <path d="M6.4 13.9a6 6 0 0 1 0-3.8V7.5H3.1a10 10 0 0 0 0 9z" fill="#FBBC05" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <path d="M12 6c1.5 0 2.8.5 3.8 1.5l2.9-2.9A10 10 0 0 0 3.1 7.5l3.3 2.6C7.2 7.8 9.4 6 12 6z" fill="#EA4335" />
  </svg>
{:else if preset === 'MICROSOFT'}
  <!-- ms-symbollockup_mssymbol_19.svg as Microsoft publishes it (THIRD-PARTY-LICENSES.md), its
       shapes and viewBox unchanged; only the drawn size is ours. -->
  <svg class="mark" viewBox="0 0 21 21" aria-hidden="true">
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <rect x="1" y="1" width="9" height="9" fill="#f25022" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <rect x="1" y="11" width="9" height="9" fill="#00a4ef" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <rect x="11" y="1" width="9" height="9" fill="#7fba00" />
    <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
    <rect x="11" y="11" width="9" height="9" fill="#ffb900" />
  </svg>
{:else}
  <!-- Ours, and therefore a token: a square in one of the ten label colours with the first
       character of the name. Square, because it sits in a column with the brand marks and a round
       one would be the only thing out of line. -->
  <span class="tile" data-token={token} aria-hidden="true">{initial}</span>
{/if}

<style>
  .mark {
    width: var(--sp-250);
    height: var(--sp-250);
    display: block;
  }

  .tile {
    width: var(--sp-250);
    height: var(--sp-250);
    display: inline-flex;
    align-items: center;
    justify-content: center;
    border-radius: var(--r-xs);
    font-family: var(--font-display);
    font-size: var(--fs-075);
    font-weight: var(--fw-bold);
    line-height: 1;
  }

  .tile[data-token='slate'] { background: var(--label-slate-bg); color: var(--label-slate-fg); }
  .tile[data-token='blue'] { background: var(--label-blue-bg); color: var(--label-blue-fg); }
  .tile[data-token='teal'] { background: var(--label-teal-bg); color: var(--label-teal-fg); }
  .tile[data-token='green'] { background: var(--label-green-bg); color: var(--label-green-fg); }
  .tile[data-token='lime'] { background: var(--label-lime-bg); color: var(--label-lime-fg); }
  .tile[data-token='amber'] { background: var(--label-amber-bg); color: var(--label-amber-fg); }
  .tile[data-token='orange'] { background: var(--label-orange-bg); color: var(--label-orange-fg); }
  .tile[data-token='red'] { background: var(--label-red-bg); color: var(--label-red-fg); }
  .tile[data-token='magenta'] { background: var(--label-magenta-bg); color: var(--label-magenta-fg); }
  .tile[data-token='violet'] { background: var(--label-violet-bg); color: var(--label-violet-fg); }
</style>
