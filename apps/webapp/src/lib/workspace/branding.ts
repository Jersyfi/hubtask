// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The workspace's own name and logo as the setting reasons about them (UC-INS-22, ADR-0112): which
 * picture a surface draws, whether the logo alone may be chosen, whether a file is worth sending,
 * and whether a picture reads on a surface. Pure, so each rule has a test without a browser.
 */

import type { SignInPictures, WorkspaceBranding, WorkspaceHeader, WorkspaceLook, WorkspacePicture } from '@hubtask/sync-engine';

/** The three pictures, in the order the setting draws their rows. */
export const PICTURES: readonly WorkspacePicture[] = ['LOGO_LIGHT', 'LOGO_DARK', 'SYMBOL'];

/** The three choices of what the header shows, in the order they are offered. */
export const HEADERS: readonly WorkspaceHeader[] = ['PRODUCT', 'WORKSPACE_NAME', 'LOGO'];

/** What the server keeps as a workspace's picture: raster images only, an SVG never (T-11). */
export const PICTURE_TYPES: readonly string[] = ['image/png', 'image/jpeg', 'image/gif', 'image/webp'];

export type Surface = 'light' | 'dark';

/** The stored picture of one kind, or undefined. */
export function pictureOf(branding: WorkspaceBranding | undefined, picture: WorkspacePicture): string | undefined {
  if (!branding) return undefined;
  const id = picture === 'LOGO_LIGHT' ? branding.logo_light_media_id : picture === 'LOGO_DARK' ? branding.logo_dark_media_id : branding.symbol_media_id;
  return id ?? undefined;
}

/** Whether "the logo alone" may be chosen: a logo or the symbol exists (G8). */
export function mayShowLogoAlone(pictures: Partial<Record<WorkspacePicture, string>>): boolean {
  return PICTURES.some((picture) => pictures[picture] !== undefined);
}

/**
 * Why a file is refused before a byte leaves, where the browser can tell: a type it names that is
 * not one of the four, or a size past the limit. A file whose type the browser does not know goes
 * to the server, which judges the bytes.
 */
export function refusalOf(file: { readonly type: string; readonly size: number }, limit: number): 'type' | 'size' | undefined {
  if (file.type !== '' && !PICTURE_TYPES.includes(file.type)) return 'type';
  if (file.size > limit) return 'size';
  return undefined;
}

/** What the header draws on one surface. */
export type Drawing =
  | { readonly kind: 'product' }
  | { readonly kind: 'logo'; readonly picture: WorkspacePicture }
  | { readonly kind: 'symbol'; readonly picture: WorkspacePicture }
  | { readonly kind: 'name'; readonly symbol?: WorkspacePicture };

/**
 * What the header shows on a surface for a choice (G8): the logo of that surface, else the other
 * logo, else the symbol alone; the name beside the symbol (or two letters, where there is none);
 * the product's mark where nothing of the workspace's can stand.
 */
export function drawingFor(header: WorkspaceHeader, pictures: Partial<Record<WorkspacePicture, string>>, surface: Surface): Drawing {
  if (header === 'WORKSPACE_NAME') return pictures.SYMBOL ? { kind: 'name', symbol: 'SYMBOL' } : { kind: 'name' };
  if (header === 'LOGO') {
    const [own, other]: readonly [WorkspacePicture, WorkspacePicture] = surface === 'dark' ? ['LOGO_DARK', 'LOGO_LIGHT'] : ['LOGO_LIGHT', 'LOGO_DARK'];
    if (pictures[own]) return { kind: 'logo', picture: own };
    if (pictures[other]) return { kind: 'logo', picture: other };
    if (pictures.SYMBOL) return { kind: 'symbol', picture: 'SYMBOL' };
  }
  return { kind: 'product' };
}

/** What a workspace shows wherever it is named: the choice, and the pictures stored, by identifier. */
export interface ShownLook {
  readonly header: WorkspaceHeader;
  readonly ids: Partial<Record<WorkspacePicture, string>>;
}

const PRODUCT_LOOK: ShownLook = { header: 'PRODUCT', ids: {} };

/**
 * What a member sees, from `GET /tenant/look`: the server already answers the product's look where
 * the workspace may not show its own (ADR-0112), so the choice and the pictures are drawn as they
 * come; the product's while nothing has come.
 */
export function memberLookOf(look: WorkspaceLook | undefined): ShownLook {
  return look ? cardLookOf(look.header, look) : PRODUCT_LOOK;
}

/**
 * What the sign-in card shows, from the rules at this address: they carry the choice and its
 * pictures only at the workspace's own address, and nothing there means the product's name.
 */
export function cardLookOf(header: WorkspaceHeader | undefined, pictures: SignInPictures | WorkspaceLook | undefined): ShownLook {
  if (!header) return PRODUCT_LOOK;
  const ids: Partial<Record<WorkspacePicture, string>> = {};
  if (pictures?.logo_light_media_id) ids.LOGO_LIGHT = pictures.logo_light_media_id;
  if (pictures?.logo_dark_media_id) ids.LOGO_DARK = pictures.logo_dark_media_id;
  if (pictures?.symbol_media_id) ids.SYMBOL = pictures.symbol_media_id;
  return { header, ids };
}

/** A picture as a mark holds it: the address to draw it from, or `null` while that is being asked for. */
export type PictureSource = string | null;

export type PictureSources = Partial<Record<WorkspacePicture, PictureSource>>;

/**
 * The card's pictures, each at the public read under its identifier, at the API this page talks
 * to - the same origin the rules were read at, so the read answers for this address alone.
 */
export function cardSourcesOf(ids: ShownLook['ids'], apiBase: string): PictureSources {
  const sources: PictureSources = {};
  for (const picture of PICTURES) {
    const id = ids[picture];
    if (id) sources[picture] = `${apiBase}/auth/workspace-pictures/${encodeURIComponent(id)}`;
  }
  return sources;
}

/**
 * What a mark draws on each surface (G8), over the pictures it may still draw. One whose address
 * is being asked for counts, so its box is kept rather than another mark drawn and swapped; one
 * that failed to load, or was refused, does not, so the rule falls back past it.
 */
export function markDrawings(
  header: WorkspaceHeader,
  pictures: Partial<Record<WorkspacePicture, PictureSource>>,
  failed: ReadonlySet<WorkspacePicture>,
): Record<Surface, Drawing> {
  const usable: Partial<Record<WorkspacePicture, string>> = {};
  for (const picture of PICTURES) {
    if (pictures[picture] !== undefined && !failed.has(picture)) usable[picture] = picture;
  }
  return { light: drawingFor(header, usable, 'light'), dark: drawingFor(header, usable, 'dark') };
}

/** Whether two drawings draw the same, so a mark alike on both surfaces is drawn once. */
export function isSameDrawing(one: Drawing, other: Drawing): boolean {
  return JSON.stringify(one) === JSON.stringify(other);
}

/** First and last word's first letter: what stands where a workspace has no symbol. */
export function initialsOf(name: string): string {
  const words = name.trim().split(/\s+/).filter(Boolean);
  const first = [...(words[0] ?? '')][0] ?? '';
  const last = words.length > 1 ? ([...(words[words.length - 1] ?? '')][0] ?? '') : '';
  return `${first}${last}`.toUpperCase();
}

/** Whether a picture reads on a surface. */
export type Verdict = 'reads' | 'hard';

/** Below this ratio between a picture's average and the surface, the picture is called hard to read. */
const HARD_BELOW = 2;
const CHANNEL = 255;

function linear(channel: number): number {
  const c = channel / CHANNEL;
  return c <= 0.04045 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
}

/** WCAG's relative luminance of one colour, its channels 0 to 255. */
export function luminance(red: number, green: number, blue: number): number {
  return 0.2126 * linear(red) + 0.7152 * linear(green) + 0.0722 * linear(blue);
}

/** A `#rrggbb` value's luminance, or undefined for anything else. */
export function luminanceOfHex(hex: string): number | undefined {
  const match = /^#([0-9a-f]{2})([0-9a-f]{2})([0-9a-f]{2})$/i.exec(hex);
  if (!match) return undefined;
  return luminance(parseInt(match[1] ?? '', 16), parseInt(match[2] ?? '', 16), parseInt(match[3] ?? '', 16));
}

/**
 * Whether a picture reads on a surface, from its RGBA pixels. A heuristic: the average luminance of
 * what it paints (each pixel weighted by its opacity) against the surface's, as WCAG's ratio. It
 * warns and never blocks; a picture that paints nothing gets no verdict.
 */
export function contrastOf(pixels: ArrayLike<number>, surfaceHex: string): Verdict | undefined {
  const surface = luminanceOfHex(surfaceHex);
  if (surface === undefined) return undefined;
  let painted = 0;
  let sum = 0;
  for (let at = 0; at + 3 < pixels.length; at += 4) {
    const alpha = (pixels[at + 3] ?? 0) / CHANNEL;
    if (alpha === 0) continue;
    painted += alpha;
    sum += alpha * luminance(pixels[at] ?? 0, pixels[at + 1] ?? 0, pixels[at + 2] ?? 0);
  }
  if (painted === 0) return undefined;
  const average = sum / painted;
  const ratio = (Math.max(average, surface) + 0.05) / (Math.min(average, surface) + 0.05);
  return ratio < HARD_BELOW ? 'hard' : 'reads';
}
