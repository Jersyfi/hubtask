// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// Fails when the built bundle contains anything the CSP of ADR-0028 would block at runtime.
//
// The policy the Go adapter serves has neither 'unsafe-inline' nor 'unsafe-eval', and that
// constraint was placed *before* the framework was chosen (ADR-0030). This check is what keeps
// the promise mechanical after it: a framework upgrade or a plugin that starts injecting an
// inline bootstrap fails the build here, not in a browser console after a release.
//
// It reads the build output, not the source: the promise is about what the binary serves.

import fs from 'node:fs';
import path from 'node:path';

/**
 * Everything in a document the CSP would refuse: an inline <script> (script-src 'self'), a
 * <style> element or a style attribute (style-src 'self'), an on* event handler attribute
 * ('unsafe-hashes' territory). Returns human-readable findings, empty when the document is clean.
 *
 * Regex over a parser, deliberately: the input is Vite's own output, not arbitrary HTML, and a
 * dependency for parsing it would be a supply-chain decision this check exists to avoid needing.
 */
export function violations(html) {
  const found = [];
  const stripped = html.replace(/<!--[\s\S]*?-->/g, '');

  for (const tag of stripped.matchAll(/<script\b([^>]*)>/gi)) {
    if (!/\bsrc\s*=/i.test(tag[1])) found.push('an inline <script> without src');
  }
  if (/<style\b/i.test(stripped)) found.push('a <style> element');
  for (const tag of stripped.matchAll(/<[a-z][^>]*?\s(on\w+)\s*=/gi)) {
    found.push(`an inline event handler (${tag[1]})`);
  }
  if (/<[a-z][^>]*?\sstyle\s*=/i.test(stripped)) found.push('a style attribute');

  return found;
}

function* htmlFiles(dir) {
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const joined = path.join(dir, entry.name);
    if (entry.isDirectory()) yield* htmlFiles(joined);
    else if (entry.name.endsWith('.html')) yield joined;
  }
}

// The CLI half: scan every document in dist/. Run as `node build/check-csp.js` after the build;
// a missing dist/ is a failure rather than a silent pass, because a check that can be skipped by
// not building is not a check.
if (process.argv[1] === new URL(import.meta.url).pathname) {
  const dist = path.resolve(process.argv[2] ?? 'dist');
  if (!fs.existsSync(dist)) {
    console.error(`csp: ${dist} does not exist - build first`);
    process.exit(1);
  }

  let failures = 0;
  let documents = 0;
  for (const file of htmlFiles(dist)) {
    documents++;
    for (const finding of violations(fs.readFileSync(file, 'utf8'))) {
      console.error(`csp: ${path.relative(dist, file)} contains ${finding}`);
      failures++;
    }
  }
  if (documents === 0) {
    console.error('csp: no document found in the build output');
    process.exit(1);
  }
  if (failures > 0) {
    console.error(`csp: ${failures} finding(s) the policy of ADR-0028 would block`);
    process.exit(1);
  }
  console.log(`csp: ${documents} document(s) clean - no inline script, no inline style`);
}
