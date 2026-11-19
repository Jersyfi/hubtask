// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * PDF pages drawn on a canvas by pdf.js (E-14), loaded only when a PDF is opened.
 *
 * **Without eval** (security.md §9): `isEvalSupported: false` closes CVE-2024-4367 and keeps the
 * interface's policy free of `'unsafe-eval'`; the worker is the bundle's own file, `'self'`. No text
 * or annotation layer and no scripting: a page is pixels, its links and forms do nothing.
 */

import type { PDFDocumentProxy } from 'pdfjs-dist';

let loading: Promise<typeof import('pdfjs-dist')> | undefined;

function library(): Promise<typeof import('pdfjs-dist')> {
  loading ??= import('pdfjs-dist').then((pdfjs) => {
    pdfjs.GlobalWorkerOptions.workerSrc = new URL('pdfjs-dist/build/pdf.worker.min.mjs', import.meta.url).href;
    return pdfjs;
  });
  return loading;
}

/** Opens a PDF from its bytes. The caller destroys it when the viewer moves on. */
export async function openPdf(bytes: ArrayBuffer): Promise<PDFDocumentProxy> {
  const pdfjs = await library();
  // No scripting either: this build of pdf.js runs a document's scripts only in its own viewer
  // with a sandbox bundle, which is never loaded here.
  return pdfjs.getDocument({
    data: new Uint8Array(bytes),
    isEvalSupported: false,
    disableAutoFetch: true,
    // No font or colour-map files fetched from anywhere: the standard fonts fall back to the system's.
    useSystemFonts: true,
  }).promise;
}

/**
 * Draws one page into a canvas at a width in CSS pixels, sharp on the device's pixel ratio. The
 * canvas's displayed size is the stylesheet's (max-inline-size), never an inline style.
 */
export async function drawPage(document: PDFDocumentProxy, pageNumber: number, canvas: HTMLCanvasElement, width: number): Promise<void> {
  const page = await document.getPage(pageNumber);
  const unscaled = page.getViewport({ scale: 1 });
  const ratio = globalThis.devicePixelRatio || 1;
  const viewport = page.getViewport({ scale: (width / unscaled.width) * ratio });
  canvas.width = Math.floor(viewport.width);
  canvas.height = Math.floor(viewport.height);
  const context = canvas.getContext('2d');
  if (!context) return;
  await page.render({ canvasContext: context, viewport }).promise;
}
