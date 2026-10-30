// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The writes of a hub's forge hooks (UC-INT-13). A secret arrives in a hook's answer once; the
// caller shows it and keeps nothing.

import type { CodeHook, CodeHookMinted } from '@hubtask/sync-engine';

import { codeHookPath, codeHookSecretPath, codeHooksPath } from './code.ts';
import { engine } from './engine.ts';

export const code = {
  create(hubId: string, kind: CodeHook['kind'], name: string): Promise<CodeHookMinted> {
    return engine.mutate<CodeHookMinted>('POST', codeHooksPath(hubId), { kind, name }, { invalidates: [codeHooksPath(hubId)] });
  },
  update(hook: CodeHook, change: { name?: string; enabled?: boolean }): Promise<CodeHook> {
    return engine.mutate<CodeHook>('PATCH', codeHookPath(hook.id), change, { invalidates: [codeHooksPath(hook.hub_id)] });
  },
  rotate(hook: CodeHook): Promise<CodeHookMinted> {
    return engine.mutate<CodeHookMinted>('POST', codeHookSecretPath(hook.id), {}, { invalidates: [codeHooksPath(hook.hub_id)] });
  },
};
