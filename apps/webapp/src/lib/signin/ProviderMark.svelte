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
  /** Prefixes the Google mark's mask, clip and filter ids, which must be unique in the document. */
  const uid = $props.id();

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
  <!-- The "G" of Google's sign-in assets (THIRD-PARTY-LICENSES.md names the file), every shape,
       blur and stop as published. Three things differ, none of them visible: the view is the
       G's own 20-unit box rather than the button around it, which is ours; the ids carry this
       instance's prefix, so that two Google buttons on one screen do not share a mask; and the
       two style attributes are the classes below, because the content security policy refuses
       an inline style (ADR-0028) - the declarations are the published ones. -->
  <svg class="mark" viewBox="10 10 20 20" fill="none" aria-hidden="true">
    <mask id="{uid}-g" class="g-mask" maskUnits="userSpaceOnUse" x="10" y="10" width="20" height="20">
      <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
      <path d="M29.3987 18.1814H19.9849V22.0445H25.3598C25.1286 23.294 24.4294 24.3596 23.3676 25.0712C22.4746 25.6716 21.3266 26.0211 19.9849 26.0211C17.3864 26.0211 15.1823 24.2666 14.3947 21.9004C14.1952 21.2989 14.0853 20.6599 14.0853 19.9983C14.0853 19.3367 14.1952 18.6966 14.3947 18.0962C15.1823 15.7311 17.3864 13.9755 19.9849 13.9755C21.4524 13.9755 22.767 14.4816 23.8039 15.4713L26.6653 12.6057C24.936 10.9908 22.6786 10 19.9849 10C16.0832 10 12.705 12.2414 11.0618 15.5076C10.383 16.8592 10 18.3834 10 19.9994C10 21.6155 10.383 23.1396 11.0618 24.4913C12.705 27.7597 16.0832 30 19.9849 30C22.6797 30 24.9485 29.1137 26.6018 27.5861C28.4887 25.8452 29.5732 23.2702 29.5732 20.2275C29.5732 19.5182 29.5131 18.835 29.3987 18.1825V18.1814Z" fill="#E94FFF" />
    </mask>
    <g mask="url(#{uid}-g)">
      <g filter="url(#{uid}-f0)">
        <g clip-path="url(#{uid}-c)">
          <g transform="matrix(0.00804129 -0.00805186 0.00804128 0.00805186 19.6819 19.7927)">
            <foreignObject x="-2105.64" y="-2105.64" width="4211.29" height="4211.29">
              <div class="g-angular"></div>
            </foreignObject>
          </g>
        </g>
        <path d="M7.25922 19.7927C7.25922 12.6759 13.0209 6.90668 20.1283 6.90668C27.2357 6.90668 32.9973 12.6759 32.9973 19.7927C32.9973 26.9094 27.2357 32.6786 20.1283 32.6786C13.0209 32.6786 7.25921 26.9094 7.25922 19.7927Z" />
      </g>
      <g filter="url(#{uid}-f1)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="20.0496" cy="20.2413" rx="5.39634" ry="2.83537" transform="rotate(24.4473 20.0496 20.2413)" fill="#3186FF" />
      </g>
      <g filter="url(#{uid}-f2)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="33.3538" cy="18.2155" rx="7.43918" ry="3.09357" fill="#3186FF" />
      </g>
      <g filter="url(#{uid}-f3)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="25.2744" cy="16.2195" rx="7.40854" ry="2.37805" fill="#FF4641" />
      </g>
      <g filter="url(#{uid}-f4)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="29.5427" cy="12.9268" rx="7.40854" ry="2.37805" fill="#FF5B8B" />
      </g>
      <g filter="url(#{uid}-f5)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="24.4817" cy="19.878" rx="8.5061" ry="3.10976" fill="#3186FF" />
      </g>
      <g filter="url(#{uid}-f6)">
        <!-- design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. -->
        <ellipse cx="25.1842" cy="14.0197" rx="4.53882" ry="2.37805" transform="rotate(-28.6599 25.1842 14.0197)" fill="#FF4641" />
      </g>
    </g>
    <defs>
      <filter id="{uid}-f0" x="5.25922" y="4.90668" width="29.7381" height="29.772" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <clipPath id="{uid}-c"><path d="M7.25922 19.7927C7.25922 12.6759 13.0209 6.90668 20.1283 6.90668C27.2357 6.90668 32.9973 12.6759 32.9973 19.7927C32.9973 26.9094 27.2357 32.6786 20.1283 32.6786C13.0209 32.6786 7.25921 26.9094 7.25922 19.7927Z" /></clipPath>
      <filter id="{uid}-f1" x="12.9977" y="14.828" width="14.1038" height="10.8265" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <filter id="{uid}-f2" x="23.9146" y="13.1219" width="18.8784" height="10.1871" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <filter id="{uid}-f3" x="15.8659" y="11.8415" width="18.8171" height="8.7561" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <filter id="{uid}-f4" x="20.1341" y="8.54878" width="18.8171" height="8.7561" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <filter id="{uid}-f5" x="13.9756" y="14.7683" width="21.0122" height="10.2195" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
      <filter id="{uid}-f6" x="19.0404" y="9.00419" width="12.2878" height="10.0309" filterUnits="userSpaceOnUse" color-interpolation-filters="sRGB">
        <feFlood flood-opacity="0" result="BackgroundImageFix" />
        <feBlend mode="normal" in="SourceGraphic" in2="BackgroundImageFix" result="shape" />
        <feGaussianBlur stdDeviation="1" result="effect1_foregroundBlur_1298_12516" />
      </filter>
    </defs>
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

  /* The two declarations the published Google file carries as style attributes, as classes. */
  .g-mask { mask-type: alpha; }
  .g-angular {
    width: 100%;
    height: 100%;
    opacity: 1;
    /* design-system-lint-ignore: a third-party brand mark, reproduced in its owner's colours. */
    background: conic-gradient(from 90deg,rgba(255, 70, 65, 1) 0deg,rgba(255, 70, 65, 1) 4.14555deg,rgba(49, 134, 255, 1) 39.154deg,rgba(49, 134, 255, 1) 72.0044deg,rgba(0, 165, 183, 1) 96.7463deg,rgba(14, 188, 95, 1) 120.897deg,rgba(14, 188, 95, 1) 154.722deg,rgba(108, 196, 0, 1) 179.136deg,rgba(255, 204, 0, 1) 203.588deg,rgba(255, 211, 20, 1) 226.915deg,rgba(255, 204, 0, 1) 251.688deg,rgba(255, 106, 43, 1) 273.129deg,rgba(253, 70, 65, 1) 289.305deg,rgba(255, 70, 65, 1) 359.593deg,rgba(255, 70, 65, 1) 360deg);
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
