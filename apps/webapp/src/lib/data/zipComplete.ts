// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import { TransportError } from '@hubtask/sync-engine';

/** The end-of-central-directory record of an archive with no comment: its signature and 18 bytes. */
const END_LENGTH = 22;
const END_SIGNATURE = [0x50, 0x4b, 0x05, 0x06];

/**
 * Writes an archive through to `target` and keeps it only when it is whole.
 *
 * The workspace download is a ZIP streamed as it is made; when the server breaks it off, the
 * archive goes without its end record while the response itself ends cleanly. So the last bytes
 * decide: an archive that does not end with the end record aborts `target` - a file system
 * stream then discards what it wrote - and the close rejects with `app.backup.download_incomplete`, a broken connection's kind. The server
 * writes no archive comment, which is what puts the record in the last 22 bytes.
 */
export function zipComplete(target: WritableStream<Uint8Array>): WritableStream<Uint8Array> {
  const writer = target.getWriter();
  let tail = new Uint8Array(0);
  return new WritableStream<Uint8Array>({
    async write(chunk) {
      if (chunk.length >= END_LENGTH) {
        tail = chunk.slice(-END_LENGTH);
      } else {
        const joined = new Uint8Array(tail.length + chunk.length);
        joined.set(tail);
        joined.set(chunk, tail.length);
        tail = joined.slice(-END_LENGTH);
      }
      await writer.write(chunk);
    },
    async close() {
      const whole = tail.length === END_LENGTH && END_SIGNATURE.every((byte, i) => tail[i] === byte);
      if (!whole) {
        const cut = new TransportError('offline', { detailCode: 'app.backup.download_incomplete' });
        await writer.abort(cut);
        throw cut;
      }
      await writer.close();
    },
    async abort(reason) {
      await writer.abort(reason);
    },
  });
}
