// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A PDF's first page as a picture and its page count, for a card that cannot hold a canvas - a
 * file on a whiteboard is an SVG `<image>` (F-4). Drawn once per object and kept as a `blob:` URL,
 * which the interface's policy admits for images.
 */

import { engine } from '../data/engine.ts';
import { media } from '../data/media.svelte.ts';
import { transferTimeoutFor } from '../data/media.ts';
import { drawPage, openPdf } from './pdf.ts';

export interface FirstPage { readonly url: string; readonly pages: number }

/** PDFs past this size keep their mark only: a card is not worth a large download. */
export const PREVIEWED_PDF = 15 * 1024 * 1024;

const drawn = new Map<string, Promise<FirstPage | undefined>>();

export function firstPageOf(mediaId: string, size: number, width = 240): Promise<FirstPage | undefined> {
  if (size > PREVIEWED_PDF) return Promise.resolve(undefined);
  let held = drawn.get(mediaId);
  if (!held) {
    held = draw(mediaId, size, width).catch(() => undefined);
    drawn.set(mediaId, held);
  }
  return held;
}

async function draw(mediaId: string, size: number, width: number): Promise<FirstPage | undefined> {
  const url = await media.downloadUrl(mediaId, Date.now());
  if (!url) return undefined;
  const bytes = await engine.receive({ url, timeoutMs: transferTimeoutFor(size) });
  const document = await openPdf(await bytes.arrayBuffer());
  try {
    const canvas = globalThis.document.createElement('canvas');
    await drawPage(document, 1, canvas, width);
    const picture = await new Promise<Blob | null>((resolve) => canvas.toBlob(resolve, 'image/png'));
    return picture ? { url: URL.createObjectURL(picture), pages: document.numPages } : undefined;
  } finally {
    void document.destroy();
  }
}
