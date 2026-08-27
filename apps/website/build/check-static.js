// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when the prerendered output is not the plain static site F1-12 promises: no inline
// <script>, no <style> element, no style attribute, no inline event handler - and, because the
// layout switches client-side rendering off entirely, no script at all, inline or not.
//
// The configuration (`csr = false`, `prerender = true`) is what makes this true; the check is
// what keeps a framework upgrade or a plugin from quietly making it false. It reads the build
// output rather than the source, because the promise is about what the web server serves.
//
// Regex over a parser, deliberately: the input is SvelteKit's own prerendered output, not
// arbitrary HTML, and a dependency for parsing it would be a supply-chain decision this check
// exists to avoid needing (the webapp's check-csp.js reasons the same way).

import fs from 'node:fs';
import path from 'node:path';

/** Everything that would make a document dynamic or inline-styled. Empty when clean. */
export function violations(html) {
  const found = [];
  const stripped = html.replace(/<!--[\s\S]*?-->/g, '');

  for (const tag of stripped.matchAll(/<script\b([^>]*)>/gi)) {
    found.push(/\bsrc\s*=/i.test(tag[1]) ? 'a <script src> (csr is off; nothing should load code)' : 'an inline <script>');
  }
  if (/<style\b/i.test(stripped)) found.push('a <style> element');
  for (const tag of stripped.matchAll(/<[a-z][^>]*?\s(on\w+)\s*=/gi)) {
    found.push(`an inline event handler (${tag[1]})`);
  }
  if (/<[a-z][^>]*?\sstyle\s*=/i.test(stripped)) found.push('a style attribute');

  return found;
}

function* filesEndingIn(dir, suffix) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const joined = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* filesEndingIn(joined, suffix);
    else if (entry.name.endsWith(suffix)) yield joined;
  }
}

// The CLI half: scan every document in dist/. A missing dist/ is a failure rather than a silent
// pass, because a check that can be skipped by not building is not a check.
if (process.argv[1] === new URL(import.meta.url).pathname) {
  const dist = path.resolve(process.argv[2] ?? 'dist');
  if (!fs.existsSync(dist)) {
    console.error(`static: ${dist} does not exist - build first`);
    process.exit(1);
  }

  let failures = 0;
  let documents = 0;
  for (const file of filesEndingIn(dist, '.html')) {
    documents++;
    for (const finding of violations(fs.readFileSync(file, 'utf8'))) {
      failures++;
      console.error(`static: ${path.relative(dist, file)}: ${finding}`);
    }
  }
  if (documents === 0) {
    console.error('static: no documents in dist/ - the prerender produced nothing');
    process.exit(1);
  }
  if (failures > 0) process.exit(1);
  console.log(`static: ${documents} document(s), plain static`);
}
