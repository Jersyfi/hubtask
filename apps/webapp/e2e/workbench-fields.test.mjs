// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// What is missing is marked, said and reachable; what may never be done is not offered; a box
// that shares an area's reason does not flip (UC-WRK-32 checks 2, 3 and 4, design-system.md §4).
//
// The design system's half, read in the built workbench as workbench-layout.test.mjs reads it, and
// served under the same `style-src 'self'`. That test measures that a reason moves nothing; this
// one cannot measure layout, because an error adds a line under its field by design. It reads what
// a form and a screen reader rely on instead: the control that can carry `aria-invalid` carries it,
// a part that cannot marks its root `data-invalid` (the selector a form's focus helper looks for,
// apps/webapp/src/lib/forms/missing.ts), the error is the control's description, and the error's
// text is on the page.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import { existsSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { join, dirname } from 'node:path';

import { chromium } from 'playwright';

import { serve } from './serve.mjs';

const WORKBENCH = join(dirname(fileURLToPath(import.meta.url)), '..', '..', '..', 'packages', 'design-system', 'dist', 'workbench');

/** Each story drawn in its marked state, the element that must say so, and the words it says. */
const MARKED = [
  { id: 'wave-1-forms-checkbox--invalid', invalid: '.host input[type="checkbox"]', text: 'Tick this once the codes are stored' },
  // A box of a group names the group's error as its `aria-errormessage`; the group is described by it.
  { id: 'wave-1-forms-checkbox-group--missing', errorOf: '.host input[type="checkbox"]', root: '.host fieldset', described: '.host fieldset', text: 'Tick at least one' },
  { id: 'wave-3-domain-upload-field--missing', root: '.host .field', described: '.host button', text: 'Choose the file to import first' },
  { id: 'wave-3-domain-assignee-control--missing', invalid: '.host [role="listbox"]', text: 'Choose who to assign them to' },
  { id: 'wave-3-time-reminder-editor--missing', root: '.host [role="group"]', text: 'needs the entry to have one' },
];

/** The ids an element's description names, resolved to their text. */
async function description(locator) {
  return locator.evaluate((node) =>
    (node.getAttribute('aria-describedby') ?? '')
      .split(' ')
      .filter(Boolean)
      .map((id) => document.getElementById(id)?.textContent ?? '')
      .join(' '),
  );
}

test('a missing value is marked, said and described, in chromium', { timeout: 5 * 60_000 }, async () => {
  assert.ok(existsSync(join(WORKBENCH, 'index.html')), 'no built workbench: run `make workbench-build` first');
  const server = await serve(WORKBENCH, { headers: { 'content-security-policy': "style-src 'self'" } });
  const browser = await chromium.launch();
  try {
    const page = await browser.newPage({ viewport: { width: 1400, height: 1000 } });
    for (const story of MARKED) {
      await page.goto(`${server.origin}/?story=${story.id}`);
      await page.getByText(story.text, { exact: false }).first().waitFor({ timeout: 10_000 });
      if (story.invalid) {
        const control = page.locator(story.invalid).first();
        assert.equal(await control.getAttribute('aria-invalid'), 'true', `${story.id}: the control is not aria-invalid`);
        assert.match(await description(control), new RegExp(story.text), `${story.id}: the control is not described by its error`);
      }
      if (story.errorOf) {
        const boxes = page.locator(story.errorOf);
        for (let index = 0; index < (await boxes.count()); index += 1) {
          const each = boxes.nth(index);
          assert.equal(await each.getAttribute('aria-invalid'), 'true', `${story.id}: box ${index} is not aria-invalid`);
          const named = await each.evaluate((node) => document.getElementById(node.getAttribute('aria-errormessage') ?? '')?.textContent ?? '');
          assert.match(named, new RegExp(story.text), `${story.id}: box ${index} names no error`);
        }
      }
      if (story.root) {
        assert.equal(await page.locator(`${story.root}[data-invalid]`).count(), 1, `${story.id}: the field's root is not data-invalid`);
      }
      if (story.described) {
        assert.match(await description(page.locator(story.described).first()), new RegExp(story.text), `${story.id}: the way in is not described by the error`);
      }
    }

    // The one-time secret: *Done* is never switched off; pressed before the tick it marks the box
    // and moves focus there.
    await page.goto(`${server.origin}/?story=wave-3-domain-one-time-secret--acknowledge`);
    const done = page.locator('.host button', { hasText: 'Done' });
    await done.waitFor();
    assert.equal(await done.getAttribute('aria-disabled'), null, 'Done is switched off before the tick');
    await done.click();
    const box = page.locator('.host input[type="checkbox"]');
    await page.waitForFunction(() => document.activeElement?.matches('.host input[type="checkbox"]'));
    assert.equal(await box.getAttribute('aria-invalid'), 'true', 'the unticked box is not marked');
    await box.check();
    assert.equal(await box.getAttribute('aria-invalid'), null, 'the mark stays after the tick');

    // Somebody else's comment offers neither control and says nothing about it.
    await page.goto(`${server.origin}/?story=wave-3-domain-comment-thread--not-offered`);
    const comments = page.locator('.host article');
    await comments.first().waitFor();
    assert.equal(await comments.nth(0).getByRole('button', { name: 'Edit' }).count(), 1, 'the own comment lost Edit');
    assert.equal(await comments.nth(1).getByRole('button').count(), 0, 'somebody else’s comment offers a control');

    // A box that shares the area's reason points at the notice, draws no bubble and does not flip.
    await page.goto(`${server.origin}/?story=wave-2-structure-read-only-notice--rows`);
    const note = page.locator('.host [role="note"] p');
    await note.waitFor();
    const noteId = await note.getAttribute('id');
    assert.ok(noteId, 'the notice’s sentence has no id to point at');
    const boxes = page.locator('.host input[type="checkbox"]');
    assert.equal(await boxes.count(), 2);
    for (let index = 0; index < 2; index += 1) {
      const each = boxes.nth(index);
      assert.equal(await each.getAttribute('aria-disabled'), 'true');
      assert.equal(await each.getAttribute('aria-describedby'), noteId, 'the box is described by more or less than the notice');
      const was = await each.isChecked();
      const rect = await each.boundingBox();
      await page.mouse.click(rect.x + rect.width / 2, rect.y + rect.height / 2);
      await each.focus();
      await page.keyboard.press(' ');
      assert.equal(await each.isChecked(), was, 'a box that says it is unavailable flipped');
    }
    assert.equal(await page.locator('.host [role="tooltip"]:visible').count(), 0, 'a shared box drew a bubble of its own');
    assert.equal(await page.locator('.host .list').getAttribute('data-pressed'), '0', 'the row heard a press');
  } finally {
    await browser.close();
    await server.close();
  }
});
