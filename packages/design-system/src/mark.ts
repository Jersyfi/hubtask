// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The Hubtask mark's one geometry (design-system.md §9), read by `Logo.svelte` and by
// `build/mark.js`, which writes the tab icon and the app icon source from it. A second drawing of
// the mark anywhere else is how two marks come to exist.
//
// The parts, outermost first: an open outline (the hub), a plane inside it (the collection), a
// core inside the plane (the task), and a spark at the outline's open end (the moment of
// completion). The coordinates are the owner's, as §9 fixes it, on a grid of 64 units; they are
// drawing, like an icon's nodes, not a design measure, and carry no unit.
//
// Two forms. The full one from the medium step up; the reduced one below, where a core and a thin
// stroke turn to noise: a heavier outline, no core, a larger spark. Where the owner gave a range
// for the reduced form, the lighter stroke and the smaller spark are taken - together they keep a
// gap between the outline's open end and the spark at the smallest step, which the heavier pair
// closes - and the plane stays the full form's, so the two forms differ only where they must.

/** The grid every form is drawn on. */
export const MARK_GRID = 64;

export interface MarkForm {
  /** The open outline: a stroked path with round caps, never filled. */
  readonly outline: { readonly d: string; readonly stroke: number };
  readonly plane: RoundedSquare;
  /** Absent in the reduced form. */
  readonly core?: RoundedSquare;
  readonly spark: { readonly cx: number; readonly cy: number; readonly r: number };
}

export interface RoundedSquare {
  readonly x: number;
  readonly y: number;
  readonly size: number;
  readonly r: number;
}

export const FULL_MARK: MarkForm = {
  outline: {
    d: 'M42 5.5H22A16.5 16.5 0 0 0 5.5 22v20A16.5 16.5 0 0 0 22 58.5h20A16.5 16.5 0 0 0 58.5 42V30',
    stroke: 7,
  },
  plane: { x: 15, y: 23, size: 26, r: 8.5 },
  core: { x: 21, y: 31, size: 12, r: 4 },
  spark: { cx: 54, cy: 12, r: 5.5 },
};

export const REDUCED_MARK: MarkForm = {
  outline: { d: 'M42 6H22A16 16 0 0 0 6 22v20a16 16 0 0 0 16 16h20a16 16 0 0 0 16-16V30', stroke: 9 },
  plane: { x: 15, y: 23, size: 26, r: 8.5 },
  spark: { cx: 54, cy: 12, r: 7 },
};

/**
 * The size steps of the mark, each one a step of the space scale (`--sp-*`), so that a mark beside
 * an icon or an avatar of the same step is exactly as tall.
 */
export type LogoSize = 'sm' | 'md' | 'lg' | 'xl';

export const LOGO_SPACE_STEP: Readonly<Record<LogoSize, string>> = { sm: '200', md: '300', lg: '400', xl: '600' };

/**
 * Which form a step draws. Chosen by the step, never measured: the only step below the use case's
 * threshold for the reduced mark is `sm`, and `test/mark.test.js` holds that against the space
 * scale in tokens.json.
 */
export function formFor(size: LogoSize): MarkForm {
  return size === 'sm' ? REDUCED_MARK : FULL_MARK;
}

/** A rounded square as one closed subpath, clockwise. */
export function squarePath({ x, y, size, r }: RoundedSquare): string {
  const end = x + size;
  const bottom = y + size;
  return (
    `M${x + r} ${y}H${end - r}A${r} ${r} 0 0 1 ${end} ${y + r}V${bottom - r}` +
    `A${r} ${r} 0 0 1 ${end - r} ${bottom}H${x + r}A${r} ${r} 0 0 1 ${x} ${bottom - r}` +
    `V${y + r}A${r} ${r} 0 0 1 ${x + r} ${y}Z`
  );
}

/**
 * The plane with the core knocked out, as one path to be filled `evenodd`: the mono form's one
 * colour cannot tell the core from the plane any other way. A path rather than a mask, because a
 * mask needs an `id`, and two marks on one page would share it.
 */
export function knockedOutPlane(form: MarkForm): string {
  return form.core ? `${squarePath(form.plane)}${squarePath(form.core)}` : squarePath(form.plane);
}
