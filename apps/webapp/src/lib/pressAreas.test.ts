// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A thumb's target (§7, N20, P-1): at a phone's width every control the final review measured
// under 44 px is a whole press tall itself - a press area drawn past the box is clipped by an
// ellipsis or covered by the next row, so the box grows instead.

import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { test } from 'node:test';

const read = (path: string) => readFileSync(new URL(path, import.meta.url), 'utf8');
const PHONE = '@media (pointer: coarse), (width < ';
const TALL = 'min-block-size: var(--layout-target);';

const CASES: readonly [string, string][] = [
  ['./pages/PageStarts.svelte', 'button.link {'],
  ['../views/PagesView.svelte', '.which button {'],
  ['../../../../packages/design-system/src/TrustLine.svelte', '.verify {'],
  ['./workspace/PlacePeople.svelte', '.members {'],
  ['../views/PlanView.svelte', '.what a {'],
  ['./files/EntryFiles.svelte', '.what a {'],
  ['./assets/ServiceGraph.svelte', '.name a {'],
  ['./goals/WidgetChart.svelte', '.goal a {'],
  ['./assets/Care.svelte', '.back, .act {'],
  ['./assets/Alerts.svelte', 'a {'],
  ['../views/PortfolioView.svelte', '.project {'],
  ['../views/DashboardView.svelte', '.handle {'],
  ['../../../../packages/design-system/src/ListRow.svelte', 'button.content {'],
  ['../../../../packages/design-system/src/Timeline.svelte', '.lead > .title {'],
  ['../../../../packages/design-system/src/DataTable.svelte', '.press {'],
  ['../../../../packages/design-system/src/WorkItemCard.svelte', 'a.title {'],
  ['../views/ProductHomeView.svelte', '.review-rows .name, .space-rows .name {'],
  ['./assets/Campaigns.svelte', '.pick {'],
];

for (const [path, selector] of CASES) {
  test(`${path} ${selector} is a press tall at a phone's width (N20)`, () => {
    const source = read(path);
    const block = source.slice(source.lastIndexOf(PHONE, source.indexOf(`    ${selector}`)));
    assert.ok(source.includes(`    ${selector}`), `no ${selector}`);
    const rule = block.slice(block.indexOf(`    ${selector}`), block.indexOf('}', block.indexOf(`    ${selector}`)));
    assert.ok(rule.includes(TALL), `${selector} in ${path}: ${rule}`);
  });
}

test('a picker\'s search is a press tall at a phone\'s width (P-1)', () => {
  const search = read('../../../../packages/design-system/src/SearchField.svelte');
  assert.match(search, /@media \(pointer: coarse\), \(width < \d+px\) \{\s*\.shell\[data-size\] \{ min-height: var\(--layout-target\); \}\s*\.shell\[data-size\] \.input \{ align-self: stretch; \}/u);
});

test('a status chip in a phone row stands over the row\'s link, so its press reaches it (N20)', () => {
  assert.match(read('./entries/StateChip.svelte'), /button\.chip \{ position: relative; z-index: var\(--z-raised\); \}/u);
});
