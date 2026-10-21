// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

import assert from 'node:assert/strict';
import { test } from 'node:test';

import { actionsOfHead } from './actions.ts';

const nothing = () => {};

test('a head\'s lists become actions in the head\'s order', () => {
  const acts = actionsOfHead('container', {
    primary: { label: 'Add an entry', onclick: nothing, menu: { label: 'Ways', items: [{ id: 'template', label: 'From a template…' }], onselect: nothing } },
    secondary: [{ label: 'Filter', onclick: nothing }],
    menu: { items: [{ id: 'select', label: 'Select entries' }, { id: 'people', label: 'People' }], onselect: nothing },
  });
  assert.deepEqual(
    acts.map((act) => act.id),
    ['container:primary', 'container:primary-template', 'container:secondary-0', 'container:menu-select', 'container:menu-people'],
  );
});

test('an archived collection: everything carrying a reason is left out, the rest as main offers it', () => {
  // The head of an archived collection, as `ContainerView` builds it for its owner.
  const archived = 'This collection is archived.';
  const acts = actionsOfHead('container', {
    primary: { label: 'Add an entry', onclick: nothing, disabledReason: archived, menu: { label: 'Ways', items: [{ id: 'template', label: 'From a template…', disabledReason: archived }], onselect: nothing } },
    secondary: [{ label: 'Filter', onclick: nothing }],
    menu: {
      items: [
        { id: 'select', label: 'Select entries' },
        { id: 'rename', label: 'Rename', disabledReason: archived },
        { id: 'move', label: 'Move to another hub…', disabledReason: archived },
        { id: 'archive', label: 'Unarchive' },
        { id: 'up', label: 'Move up', disabledReason: 'Already first.' },
        { id: 'down', label: 'Move down' },
        { id: 'labels', label: 'Labels', disabledReason: archived },
        { id: 'views', label: 'Saved views' },
        { id: 'templates', label: 'Templates', disabledReason: archived },
        { id: 'people', label: 'People' },
        { id: 'trash', label: 'Move to trash', disabledReason: archived },
      ],
      onselect: nothing,
    },
  });
  assert.deepEqual(
    acts.map((act) => act.label),
    ['Filter', 'Select entries', 'Unarchive', 'Move down', 'Saved views', 'People'],
  );
});

test('pressing an action runs what the head\'s control runs, with the item\'s own id', () => {
  const chosen: string[] = [];
  const acts = actionsOfHead('item', { menu: { items: [{ id: 'share', label: 'Share this entry' }], onselect: (id) => chosen.push(id) } });
  acts[0]?.run();
  assert.deepEqual(chosen, ['share']);
});

test('a head with nothing offers nothing', () => {
  assert.deepEqual(actionsOfHead('home', {}), []);
});
