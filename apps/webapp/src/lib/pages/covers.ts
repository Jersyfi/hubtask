// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A page's cover (E-18, UC-DOC-16): the gradients and the gallery the client draws by their key,
// every colour a token (rule 15), so both read in the light and the dark mode.

export type CoverSource = 'GRADIENT' | 'GALLERY' | 'IMAGE';
export interface PageCover { readonly source: CoverSource; readonly key: string; readonly position: number }

const chart = (n: number) => `var(--chart-categorical-${n})`;
const soft = (name: string) => `var(--label-${name}-bg)`;

/** Six strong gradients of the seven chart colours, six soft ones of the labels' surfaces. */
export const GRADIENTS: Readonly<Record<string, readonly [string, string]>> = {
  'indigo-teal': [chart(1), chart(2)],
  'rose-amber': [chart(4), chart(3)],
  'green-sky': [chart(5), chart(6)],
  'amber-red': [chart(3), chart(7)],
  'red-indigo': [chart(7), chart(1)],
  'sky-indigo': [chart(6), chart(1)],
  'soft-teal': [soft('teal'), soft('blue')],
  'soft-violet': [soft('violet'), soft('magenta')],
  'soft-amber': [soft('amber'), soft('orange')],
  'soft-green': [soft('green'), soft('lime')],
  'soft-blue': [soft('blue'), soft('violet')],
  'soft-slate': [soft('slate'), soft('blue')],
};

/** The shipped gallery: pictures composed of tokens alone, free to use, no request leaving the page. */
export const GALLERY: Readonly<Record<string, string>> = {
  dawn: `radial-gradient(circle at 20% 110%, ${chart(3)} 0 30%, transparent 31%), radial-gradient(circle at 75% 25%, ${soft('amber')} 0 12%, transparent 13%), linear-gradient(180deg, ${chart(4)}, ${soft('orange')})`,
  hills: `radial-gradient(ellipse 60% 50% at 25% 100%, ${chart(5)} 0 99%, transparent 100%), radial-gradient(ellipse 70% 45% at 80% 100%, ${chart(2)} 0 99%, transparent 100%), linear-gradient(180deg, ${soft('blue')}, ${soft('teal')})`,
  tide: `repeating-radial-gradient(circle at 50% 160%, ${chart(6)} 0 6%, ${chart(1)} 6% 12%)`,
  stripes: `repeating-linear-gradient(135deg, ${chart(1)} 0 var(--sp-300), ${chart(4)} var(--sp-300) var(--sp-600))`,
  dots: `radial-gradient(circle, ${chart(1)} 0 var(--sp-025), transparent var(--sp-050)) 0 0 / var(--sp-300) var(--sp-300), linear-gradient(90deg, ${soft('violet')}, ${soft('blue')})`,
  night: `radial-gradient(circle at 80% 30%, ${soft('amber')} 0 6%, transparent 7%), linear-gradient(180deg, ${chart(1)}, ${chart(7)})`,
};

/** How a cover draws as a background; nothing for a key this client does not know or an image not resolved. */
export function coverBackground(cover: PageCover, imageUrl?: string): string | undefined {
  if (cover.source === 'GRADIENT') {
    const pair = GRADIENTS[cover.key];
    return pair ? `linear-gradient(120deg, ${pair[0]}, ${pair[1]})` : undefined;
  }
  if (cover.source === 'GALLERY') return GALLERY[cover.key];
  return imageUrl ? `url("${imageUrl.replace(/["\\\n]/gu, '')}") center ${clampPosition(cover.position)}% / cover no-repeat` : undefined;
}

/** A position kept between the picture's top (0) and its foot (100), in whole steps. */
export const clampPosition = (position: number): number => Math.min(100, Math.max(0, Math.round(position)));

/** The position after the picture is dragged by `dy` pixels in a band `height` high: down drags show more of the top. */
export function draggedPosition(position: number, dy: number, height: number): number {
  return height > 0 ? clampPosition(position - (dy / height) * 100) : clampPosition(position);
}
