// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when the prerendered output is not the plain static site it has to be: no inline
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
import { createRequire } from 'node:module';
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
  // The tab icon every document links: the design system's generated file, byte for byte, so the
  // site's icon cannot drift from the mark the web app ships (UC-WRK-33 check 4).
  const icon = path.join(dist, 'favicon.svg');
  const generated = createRequire(import.meta.url).resolve('@hubtask/design-system/mark/favicon.svg');
  if (!fs.existsSync(icon) || !fs.readFileSync(icon).equals(fs.readFileSync(generated))) {
    failures++;
    console.error('static: favicon.svg is missing or is not the design system\'s generated tab icon');
  }
  // RFC 9116's contact file: a route under a dot directory is the kind a framework upgrade could
  // quietly stop prerendering (security.md §14.2).
  const contact = path.join(dist, '.well-known', 'security.txt');
  if (!fs.existsSync(contact) || !/^Contact: /m.test(fs.readFileSync(contact, 'utf8'))) {
    failures++;
    console.error('static: .well-known/security.txt is missing or names no contact');
  }
  if (failures > 0) process.exit(1);
  console.log(`static: ${documents} document(s), plain static, the generated tab icon, security.txt`);
}
