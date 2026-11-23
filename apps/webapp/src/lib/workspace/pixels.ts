// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * A picture's pixels, small: what `contrastOf` judges. The picture is drawn into a canvas no
 * longer than `SIDE` on its long edge - an average needs no more - and read back as RGBA.
 * Nothing where the browser cannot decode the file or read the canvas back.
 */

const SIDE = 64;

export async function pixelsOf(blob: Blob): Promise<Uint8ClampedArray | undefined> {
  try {
    const bitmap = await createImageBitmap(blob);
    const scale = Math.min(1, SIDE / Math.max(bitmap.width, bitmap.height, 1));
    const width = Math.max(1, Math.round(bitmap.width * scale));
    const height = Math.max(1, Math.round(bitmap.height * scale));
    const canvas = document.createElement('canvas');
    canvas.width = width;
    canvas.height = height;
    const context = canvas.getContext('2d');
    if (!context) return undefined;
    context.drawImage(bitmap, 0, 0, width, height);
    bitmap.close();
    return context.getImageData(0, 0, width, height).data;
  } catch {
    return undefined;
  }
}
