// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// A module loader that lets `node --test` import the application's Svelte components.
//
// The residue test (residue.test.js) renders every route on the server, which needs `.svelte`
// files compiled for the server and `.svelte.ts` modules compiled the same way; Node handles the
// plain `.ts` on its own. Vite does all of this in the build and in the dev server, and nothing
// of Vite is reachable from a test that has to run where the other tests run. So this is the
// smallest loader that gets a component to `svelte/server`'s `render`: TypeScript stripped by the
// compiler the workspace already carries, the template compiled by Svelte's own, and the result
// handed back as JavaScript. It is a test fixture, not part of the bundle.

import { readFile } from 'node:fs/promises';
import { fileURLToPath } from 'node:url';

import { compile, compileModule } from 'svelte/compiler';
import ts from 'typescript';

const stripTypes = (source, fileName) =>
  ts.transpileModule(source, {
    fileName,
    compilerOptions: {
      target: ts.ScriptTarget.ES2022,
      module: ts.ModuleKind.ESNext,
      verbatimModuleSyntax: true,
    },
  }).outputText;

export async function load(url, context, nextLoad) {
  if (url.endsWith('.svelte')) {
    const path = fileURLToPath(url);
    const source = await readFile(path, 'utf8');
    // Types inside `<script lang="ts">` are stripped by the compiler's own preprocessing hook
    // rather than by hand: the template is not TypeScript, and only the script blocks are.
    const { js } = compile(source, {
      filename: path,
      generate: 'server',
      css: 'external',
      // The preprocessor is not reachable here without Vite's plugin, so the script is stripped
      // in place: Svelte 5 accepts `lang="ts"` and strips the types itself.
    });
    return { format: 'module', source: js.code, shortCircuit: true };
  }
  if (url.endsWith('.svelte.ts')) {
    const path = fileURLToPath(url);
    const source = await readFile(path, 'utf8');
    const { js } = compileModule(stripTypes(source, path), { filename: path, generate: 'server' });
    return { format: 'module', source: js.code, shortCircuit: true };
  }
  return nextLoad(url, context);
}
