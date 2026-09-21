// SPDX-License-Identifier: BUSL-1.1
// Copyright (c) 2026 Jérôme Bastian Winkel

import { test } from 'node:test';
import assert from 'node:assert/strict';

import type { Rule } from '../data/rules.svelte.ts';
import {
  canPlace,
  compileNode,
  compileSentence,
  countSteps,
  depthOf,
  fromRule,
  insertAt,
  isAutomatic,
  moveStep,
  nameSeed,
  newStep,
  nudge,
  pathOf,
  pointerOf,
  readNode,
  readSentence,
  removeAt,
  stepAt,
  toRuleDraft,
  unreachableFrom,
  type Node,
  type Sentence,
} from './model.ts';

/** A stored rule with everything the old form could write, plus a branch, a wait and a stop. */
const STORED: Rule = {
  id: 'r1',
  name: 'Escalate overdue approvals',
  scope: { type: 'HUB', id: 'hub-1' },
  enabled: false,
  run_as: 'sa-1',
  trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.updated.v1', changed_fields: ['due_at'] },
  conditions: [{ expr: "item.type == 'TASK'" }, { expr: 'now.getHours() >= 8 && now.getHours() < 18' }, { expr: 'size(item.title) > 3' }],
  actions: [
    { kind: 'ADD_LABEL', params: { label_id: 'l-1' } },
    {
      kind: 'BRANCH',
      params: { condition: 'has(item.due_at)' },
      then: [{ kind: 'NOTIFY_GROUP', params: { group_id: 'g-1' } }],
      else: [{ kind: 'WAIT', params: { duration: 'P1D' } }, { kind: 'STOP' }],
    },
    { kind: 'SEND_WEBHOOK', params: { subscription_id: 'w-1' } },
  ],
  throttle: { max_runs_per_hour: 100, dedupe_key_expr: 'item.id' },
  on_error: 'CONTINUE',
  failure_count: 0,
  version: 3,
};

test('a stored rule opens on the canvas and closes back into the document it came from', () => {
  const draft = fromRule(STORED);
  assert.equal(draft.actions[1]?.then?.[0]?.kind, 'NOTIFY_GROUP');
  assert.equal(draft.actions[1]?.else?.[1]?.kind, 'STOP');
  assert.deepEqual(toRuleDraft(draft, draft.name), {
    name: STORED.name,
    scope: { type: 'HUB', id: 'hub-1' },
    run_as: 'sa-1',
    trigger: { kind: 'EVENT', event_type: 'de.hubtask.work.item.updated.v1', changed_fields: ['due_at'] },
    conditions: STORED.conditions,
    actions: [
      { kind: 'ADD_LABEL', params: { label_id: 'l-1' } },
      {
        kind: 'BRANCH',
        params: { condition: 'has(item.due_at)' },
        then: [{ kind: 'NOTIFY_GROUP', params: { group_id: 'g-1' } }],
        else: [{ kind: 'WAIT', params: { duration: 'P1D' } }, { kind: 'STOP' }],
      },
      { kind: 'SEND_WEBHOOK', params: { subscription_id: 'w-1' } },
    ],
    throttle: { max_runs_per_hour: 100, dedupe_key_expr: 'item.id' },
    on_error: 'CONTINUE',
  });
});

test('a trigger sends the fields of its kind and no others, and an empty parameter is an omission', () => {
  const draft = fromRule(STORED);
  draft.trigger = { kind: 'SCHEDULE', rrule: 'FREQ=WEEKLY;BYDAY=MO', timezone: 'Europe/Berlin', event_type: 'left over' };
  draft.actions = [{ kind: 'ADD_COMMENT', params: { body: '', item_id: undefined } }];
  draft.throttle = {};
  const sent = toRuleDraft(draft, 'n');
  assert.deepEqual(sent.trigger, { kind: 'SCHEDULE', rrule: 'FREQ=WEEKLY;BYDAY=MO', timezone: 'Europe/Berlin' });
  assert.deepEqual(sent.actions, [{ kind: 'ADD_COMMENT' }]);
  assert.equal('throttle' in sent, false);
});

test('a path names one step through the arms, and translates to the pointer a refusal carries', () => {
  const { actions } = fromRule(STORED);
  assert.equal(stepAt(actions, '1/then/0')?.kind, 'NOTIFY_GROUP');
  assert.equal(stepAt(actions, '1/else/1')?.kind, 'STOP');
  assert.equal(stepAt(actions, '7'), undefined);
  assert.equal(stepAt(actions, '0/then/0'), undefined);
  assert.equal(countSteps(actions), 6);
  assert.equal(depthOf(''), 0);
  assert.equal(depthOf('1/then'), 1);
  assert.equal(depthOf('1/then/0/else'), 2);
  assert.equal(pointerOf('1/then/0', 'kind'), '/actions/1/params/then/0/kind');
  assert.equal(pointerOf('2', 'params/subscription_id'), '/actions/2/params/subscription_id');
  assert.equal(pathOf('/actions/1/params/then/0/kind'), '1/then/0');
  assert.equal(pathOf('/actions/2/params/subscription_id'), '2');
  assert.equal(pathOf('/actions/1/params/condition'), '1');
  assert.equal(pathOf('/conditions/0/expr'), undefined);
});

test('inserting, removing and moving keep the chain a copy, and a branch never enters its own arm', () => {
  const { actions } = fromRule(STORED);
  const inserted = insertAt(actions, '1/else', 1, newStep('COMPLETE_ITEM'));
  assert.equal(inserted[1]?.else?.map((step) => step.kind).join(','), 'WAIT,COMPLETE_ITEM,STOP');
  assert.equal(actions[1]?.else?.length, 2, 'the original is untouched');

  const removed = removeAt(actions, '1/then/0');
  assert.equal(removed[1]?.then?.length, 0);

  const moved = moveStep(actions, '2', '1/then', 0);
  assert.equal(moved?.[1]?.then?.map((step) => step.kind).join(','), 'SEND_WEBHOOK,NOTIFY_GROUP');
  assert.equal(moved?.length, 2);

  const down = moveStep(actions, '0', '', 2);
  assert.equal(down?.map((step) => step.kind).join(','), 'BRANCH,ADD_LABEL,SEND_WEBHOOK');

  assert.equal(nudge(actions, '0', 1).map((step) => step.kind).join(','), 'BRANCH,ADD_LABEL,SEND_WEBHOOK');
  assert.equal(nudge(actions, '2', 1).map((step) => step.kind).join(','), 'ADD_LABEL,BRANCH,SEND_WEBHOOK', 'the last cannot go down');
  assert.equal(moveStep(actions, '1', '1/then', 0), undefined, 'a branch into its own arm');
  assert.equal(moveStep(actions, '1', '1/else/0/then', 0), undefined, 'or deeper');
  assert.equal(moveStep(actions, '9', '', 0), undefined);
});

// A stop is a terminus and goes last (decision 14): it does not move up, nothing moves or is
// inserted below it, and a gap is asked before anything lands.
test('a stop stays the last step of its list', () => {
  const { actions } = fromRule(STORED);
  assert.equal(nudge(actions, '1/else/1', -1)[1]?.else?.map((step) => step.kind).join(','), 'WAIT,STOP', 'the stop does not move up');
  assert.equal(nudge(actions, '1/else/0', 1)[1]?.else?.map((step) => step.kind).join(','), 'WAIT,STOP', 'nothing moves below it');
  assert.equal(canPlace(actions, '1/else', 2, 'COMPLETE_ITEM'), false, 'nothing after a stop');
  assert.equal(canPlace(actions, '1/else', 1, 'COMPLETE_ITEM'), true, 'before it is fine');
  assert.equal(canPlace(actions, '', 1, 'STOP'), false, 'a stop in the middle');
  assert.equal(canPlace(actions, '', 3, 'STOP'), true, 'a stop at the end');
  assert.equal(canPlace(actions, '1/else', 2, 'STOP'), false, 'a second stop');
  assert.equal(canPlace(actions, '7/then', 0, 'STOP'), false, 'a list that is not there');
  assert.equal(moveStep(actions, '0', '1/else', 2), undefined, 'a move below a stop is refused');
  assert.equal(moveStep(actions, '1/else/1', '', 1), undefined, 'a stop moved into the middle is refused');
  assert.equal(moveStep(actions, '1/else/1', '', 3)?.map((step) => step.kind).join(','), 'ADD_LABEL,BRANCH,SEND_WEBHOOK,STOP', 'a stop moved to the end lands');
  assert.equal(unreachableFrom(actions[1]?.else ?? []), -1, 'a stop that is last leaves nothing unreachable');
  assert.equal(unreachableFrom([newStep('STOP'), newStep('WAIT'), newStep('WAIT')]), 1, 'what follows a stored stop');
});

test('the generated name is seeded by the trigger and the first two steps that are not branches', () => {
  const seed = nameSeed(fromRule(STORED));
  assert.deepEqual(seed.kinds, ['ADD_LABEL', 'NOTIFY_GROUP']);
  assert.equal(seed.more, true);
  assert.equal(isAutomatic('', 'Whatever'), true);
  assert.equal(isAutomatic('When an entry changes: add label, notify group, …', 'When an entry changes: add label, notify group, …'), true);
  assert.equal(isAutomatic('Escalate overdue approvals', 'When an entry changes: …'), false);
});

test('a sentence compiles to the expression the server stores, and reads back from it', () => {
  const cases: readonly [Sentence, string][] = [
    [{ subject: 'type', op: 'is', a: 'TASK' }, "item.type == 'TASK'"],
    [{ subject: 'type', op: 'is_not', a: 'ACTIVITY' }, "item.type != 'ACTIVITY'"],
    [{ subject: 'completed', op: 'no' }, 'item.completed == false'],
    [{ subject: 'due', op: 'lacks' }, '!has(item.due_at)'],
    [{ subject: 'parent', op: 'has' }, 'has(item.parent_id)'],
    [{ subject: 'assignee', op: 'is', a: 'acc-1' }, "item.assignee_id == 'acc-1'"],
    [{ subject: 'assignee', op: 'lacks' }, '!has(item.assignee_id)'],
    [{ subject: 'bucket', op: 'is_not', a: 'b-1' }, "item.bucket_id != 'b-1'"],
    [{ subject: 'actor', op: 'is', a: 'acc-2' }, "actor.id == 'acc-2'"],
    [{ subject: 'hour', op: 'between', a: '8', b: '18' }, 'now.getHours() >= 8 && now.getHours() < 18'],
    [{ subject: 'field', op: 'is', a: 'priority', b: "O'Neil" }, "item.custom_fields['priority'] == 'O\\'Neil'"],
    // The subjects and operators of decision 15.
    [{ subject: 'title', op: 'contains', a: 'permit' }, "item.title.contains('permit')"],
    [{ subject: 'title', op: 'not_contains', a: 'draft' }, "!item.title.contains('draft')"],
    [{ subject: 'title', op: 'starts_with', a: 'RFC' }, "item.title.startsWith('RFC')"],
    [{ subject: 'notes', op: 'empty' }, "item.notes == ''"],
    [{ subject: 'notes', op: 'not_empty' }, "item.notes != ''"],
    [{ subject: 'notes', op: 'contains', a: 'blocked' }, "item.notes.contains('blocked')"],
    [{ subject: 'archived', op: 'yes' }, 'item.archived == true'],
    [{ subject: 'due', op: 'past' }, 'has(item.due_at) && item.due_at < now'],
    [{ subject: 'due', op: 'future' }, 'has(item.due_at) && item.due_at > now'],
    [{ subject: 'due', op: 'within', a: '3' }, "has(item.due_at) && item.due_at < now + duration('72h')"],
    [{ subject: 'depth', op: 'is', a: '0' }, 'item.depth == 0'],
    [{ subject: 'depth', op: 'at_most', a: '2' }, 'item.depth <= 2'],
    [{ subject: 'assignee', op: 'is_not', a: 'acc-1' }, "item.assignee_id != 'acc-1'"],
  ];
  for (const [sentence, expr] of cases) {
    assert.equal(compileSentence(sentence), expr);
    assert.deepEqual(readSentence(expr), sentence, expr);
  }
  assert.equal(readSentence('size(item.title) > 3'), undefined, 'an expression the composer did not write');
  assert.equal(readSentence("item.type == 'TASK' || item.completed"), undefined);
});

// A condition as a tree (decision 15): all / any / none over sentences and groups, compiled with
// parentheses and read back by the same grammar; a shape the composer did not write stays an
// expression, and a sentence alone is unchanged.
test('a tree of conditions compiles with parentheses and reads back the same', () => {
  const type: Sentence = { subject: 'type', op: 'is', a: 'TASK' };
  const due: Sentence = { subject: 'due', op: 'has' };
  const hour: Sentence = { subject: 'hour', op: 'between', a: '8', b: '18' };
  const archived: Sentence = { subject: 'archived', op: 'yes' };
  const cases: readonly [Node, string][] = [
    [type, "item.type == 'TASK'"],
    [{ mode: 'all', items: [type, due] }, "item.type == 'TASK' && has(item.due_at)"],
    [{ mode: 'any', items: [type, due] }, "item.type == 'TASK' || has(item.due_at)"],
    [{ mode: 'none', items: [type, due] }, "!(item.type == 'TASK' || has(item.due_at))"],
    [{ mode: 'none', items: [type] }, "!(item.type == 'TASK')"],
    // Nested two deep, and a sentence that is itself a join parenthesised inside a group.
    [{ mode: 'all', items: [type, { mode: 'any', items: [due, archived] }] }, "item.type == 'TASK' && (has(item.due_at) || item.archived == true)"],
    [{ mode: 'any', items: [{ mode: 'all', items: [type, hour] }, { mode: 'none', items: [archived] }] }, "(item.type == 'TASK' && (now.getHours() >= 8 && now.getHours() < 18)) || (!(item.archived == true))"],
    [{ mode: 'all', items: [{ mode: 'any', items: [type, { mode: 'all', items: [due, archived] }] }, hour] }, "(item.type == 'TASK' || (has(item.due_at) && item.archived == true)) && (now.getHours() >= 8 && now.getHours() < 18)"],
  ];
  for (const [node, expr] of cases) {
    assert.equal(compileNode(node), expr);
    assert.deepEqual(readNode(expr), node, expr);
  }
  // A group of one at the root compiles to the sentence and reads back as one; nested, its
  // parentheses keep it a group, so a group just made does not vanish under the writer's hands.
  assert.equal(compileNode({ mode: 'all', items: [type] }), "item.type == 'TASK'");
  assert.deepEqual(readNode("item.type == 'TASK'"), type);
  const nestedOne: Node = { mode: 'any', items: [type, { mode: 'all', items: [due] }] };
  assert.equal(compileNode(nestedOne), "item.type == 'TASK' || (has(item.due_at))");
  assert.deepEqual(readNode("item.type == 'TASK' || (has(item.due_at))"), nestedOne);
  // An expression the composer did not write, in whole or in part, stays one.
  assert.equal(readNode("item.type == 'TASK' && size(item.title) > 3"), undefined);
  assert.equal(readNode("!(item.type == 'TASK' && has(item.due_at))"), undefined, 'not-all has no mode');
  assert.equal(readNode("(item.type == 'TASK'"), undefined);
  // A quoted join is not a join.
  assert.deepEqual(readNode("item.title.contains('a && b')"), { subject: 'title', op: 'contains', a: 'a && b' });
  // An empty group is no expression.
  assert.equal(compileNode({ mode: 'any', items: [] }), '');
});
