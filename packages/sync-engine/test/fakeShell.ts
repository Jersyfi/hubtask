// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The shell's copy commands (`apps/webapp/src-tauri/src/copy.rs`) in memory, so that the shared
// storage test holds `SqliteStorage` to the port's promises where the tests run - Node has no
// shell. It keeps what the shell keeps and nothing more: each record as the text it was given, a
// file per name that `copy_clear` deletes, and a key per name that only `copy_forget` removes.
// What SQLCipher and the keystore do with the same calls is the shell's own tests' question
// (`cargo test` in the `shell` lane).

import type { ShellCall } from '../src/storage/SqliteStorage.ts';

interface Args {
  readonly name: string;
  readonly collection?: string;
  readonly id?: string;
  readonly value?: string;
}

export class FakeShell {
  /** The databases on disk, by name: `collection`, a NUL, `id` to the text held. */
  readonly files = new Map<string, Map<string, string>>();
  /** The names a key is held for in the keystore. */
  readonly keys = new Set<string>();
  /** Every command, in order. */
  readonly commands: string[] = [];

  readonly call: ShellCall = async (command, raw) => {
    await Promise.resolve();
    this.commands.push(command);
    const { name, collection = '', id = '', value } = raw as unknown as Args;
    const rows = () => {
      this.keys.add(name);
      let file = this.files.get(name);
      if (!file) {
        file = new Map();
        this.files.set(name, file);
      }
      return file;
    };
    const ofCollection = () => [...rows().entries()]
      .filter(([key]) => key.startsWith(`${collection}\u0000`))
      .sort(([a], [b]) => (a < b ? -1 : 1));
    switch (command) {
      case 'copy_get':
        return rows().get(`${collection}\u0000${id}`) ?? null;
      case 'copy_put':
        if (typeof value !== 'string') throw new TypeError('the shell holds text');
        rows().set(`${collection}\u0000${id}`, value);
        return null;
      case 'copy_delete':
        rows().delete(`${collection}\u0000${id}`);
        return null;
      case 'copy_all':
        return ofCollection().map(([, text]) => text);
      case 'copy_collections':
        return [...new Set([...rows().keys()].map((key) => key.split('\u0000')[0]))].sort();
      case 'copy_clear':
        this.files.delete(name);
        return null;
      case 'copy_forget':
        this.files.delete(name);
        this.keys.delete(name);
        return null;
      default:
        throw new Error(`no such command: ${command}`);
    }
  };
}
