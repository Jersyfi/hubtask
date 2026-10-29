// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The board as a PDF (research §4.6): one page the size of the picture, holding the picture as a
 * JPEG image. A PDF of a few objects needs no library: the page, the image and the cross-reference
 * table are written here (ISO 32000-1 §7.5), the JPEG passed through as `DCTDecode`.
 */

/**
 * A PDF of one page showing a JPEG picture of a width and height in pixels. The page is the
 * picture's size at 96 pixels an inch, or the size given - a picture drawn at twice the scale
 * for sharpness keeps the page of its own size.
 */
export function pdfOfJpeg(jpeg: Uint8Array, width: number, height: number, title: string, page?: { width: number; height: number }): Uint8Array {
  const pageWidth = round((page?.width ?? width) * 0.75);
  const pageHeight = round((page?.height ?? height) * 0.75);
  const content = `q ${pageWidth} 0 0 ${pageHeight} 0 0 cm /Im0 Do Q`;
  const encoder = new TextEncoder();
  const chunks: Uint8Array[] = [];
  const offsets: number[] = [];
  let length = 0;
  const push = (part: string | Uint8Array) => {
    const bytes = typeof part === 'string' ? encoder.encode(part) : part;
    chunks.push(bytes);
    length += bytes.length;
  };
  const object = (n: number, body: string | (string | Uint8Array)[]) => {
    offsets[n] = length;
    push(`${n} 0 obj\n`);
    for (const part of typeof body === 'string' ? [body] : body) push(part);
    push('\nendobj\n');
  };
  // The binary comment marks the file as binary for tools that would otherwise mangle it.
  push(new Uint8Array([0x25, 0x50, 0x44, 0x46, 0x2d, 0x31, 0x2e, 0x34, 0x0a, 0x25, 0xe2, 0xe3, 0xcf, 0xd3, 0x0a]));
  object(1, '<< /Type /Catalog /Pages 2 0 R >>');
  object(2, '<< /Type /Pages /Kids [3 0 R] /Count 1 >>');
  object(3, `<< /Type /Page /Parent 2 0 R /MediaBox [0 0 ${pageWidth} ${pageHeight}] /Resources << /XObject << /Im0 4 0 R >> >> /Contents 5 0 R >>`);
  object(4, [
    `<< /Type /XObject /Subtype /Image /Width ${Math.round(width)} /Height ${Math.round(height)} /ColorSpace /DeviceRGB /BitsPerComponent 8 /Filter /DCTDecode /Length ${jpeg.length} >>\nstream\n`,
    jpeg,
    '\nendstream',
  ]);
  object(5, `<< /Length ${content.length} >>\nstream\n${content}\nendstream`);
  object(6, `<< /Title ${textString(title)} /Producer (Hubtask) >>`);
  const xref = length;
  push(`xref\n0 7\n0000000000 65535 f \n${offsets.slice(1).map((o) => `${String(o).padStart(10, '0')} 00000 n \n`).join('')}`);
  push(`trailer\n<< /Size 7 /Root 1 0 R /Info 6 0 R >>\nstartxref\n${xref}\n%%EOF\n`);
  const out = new Uint8Array(length);
  let at = 0;
  for (const chunk of chunks) {
    out.set(chunk, at);
    at += chunk.length;
  }
  return out;
}

function round(value: number): number {
  return Math.round(value * 100) / 100;
}

/** A text string of any language: UTF-16BE with its byte-order mark, written as hex. */
function textString(text: string): string {
  let hex = 'FEFF';
  for (let i = 0; i < text.length; i++) hex += text.charCodeAt(i).toString(16).padStart(4, '0').toUpperCase();
  return `<${hex}>`;
}
