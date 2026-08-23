// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

// The gate-selftest habit, applied to the CSP check: before the check is trusted, prove it
// fails on a deliberately planted violation of each kind - a check that cannot fail proves
// nothing by passing.

import { test } from 'node:test';
import assert from 'node:assert/strict';

import { violations } from './check-csp.js';

const clean = `<!DOCTYPE html>
<html lang="en" data-theme="dark">
  <head>
    <meta charset="utf-8" />
    <title>Hubtask</title>
    <script type="module" crossorigin src="/assets/index-abc123.js"></script>
    <link rel="stylesheet" crossorigin href="/assets/index-def456.css">
  </head>
  <body><div id="app"></div></body>
</html>`;

test('the document Vite actually emits is clean', () => {
  assert.deepEqual(violations(clean), []);
});

test('a planted inline script fails', () => {
  const planted = clean.replace('</head>', '<script>window.boot()</script></head>');
  assert.ok(violations(planted).some((v) => v.includes('inline <script>')));
});

test('a planted style element fails', () => {
  const planted = clean.replace('</head>', '<style>body{color:red}</style></head>');
  assert.ok(violations(planted).some((v) => v.includes('<style> element')));
});

test('a planted event handler attribute fails', () => {
  const planted = clean.replace('<div id="app">', '<div id="app" onclick="boot()">');
  assert.ok(violations(planted).some((v) => v.includes('onclick')));
});

test('a planted style attribute fails', () => {
  const planted = clean.replace('<div id="app">', '<div id="app" style="display:none">');
  assert.ok(violations(planted).some((v) => v.includes('style attribute')));
});

test('a violation inside a comment is not a violation', () => {
  const commented = clean.replace('</head>', '<!-- <script>old()</script> --></head>');
  assert.deepEqual(violations(commented), []);
});

test('a script tag with src and attributes on both sides stays clean', () => {
  const reordered = clean.replace(
    '<script type="module" crossorigin src="/assets/index-abc123.js"></script>',
    '<script crossorigin src="/assets/index-abc123.js" type="module"></script>',
  );
  assert.deepEqual(violations(reordered), []);
});
