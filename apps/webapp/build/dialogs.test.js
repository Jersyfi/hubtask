// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// A dialog is never overloaded (page-system §5.1, its hard limits): every `Dialog` names its rung
// (`size` s, m or l), and none holds a table, tabs, a second dialog - *Discard changes?* is the one
// allowed over another - or more than one `Disclosure`. What a dialog holds includes what the
// components drawn inside it draw. Read from the source, as the page-kind gate is.

import { test } from 'node:test';
import assert from 'node:assert/strict';
import fs from 'node:fs';
import path from 'node:path';

import { SRC, elements, read, reachable, relative, svelteFiles, uses, within } from './markup.js';

/**
 * The files whose dialogs are not yet held to the ladder. Later steps of the concept migrate them
 * area by area (page-system §13); this list may only shrink - a file that meets the rule must leave
 * it, which the last test holds - and it is empty when the concept lands.
 */
const NOT_YET_MIGRATED = [
  'lib/agents/AgentDetail.svelte',
  'lib/ask/AskDialog.svelte',
  'lib/assets/Campaigns.svelte',
  'lib/assets/CiAssetPart.svelte',
  'lib/assets/Connectors.svelte',
  'lib/assets/Groups.svelte',
  'lib/automation/DescribeRule.svelte',
  'lib/capture/CaptureDialog.svelte',
  'lib/chat/ChatBots.svelte',
  'lib/code/CodeHooks.svelte',
  'lib/entries/AffectedCis.svelte',
  'lib/entries/Board.svelte',
  'lib/entries/BucketDialog.svelte',
  'lib/entries/CategoriesDialog.svelte',
  'lib/entries/CustomFieldsDialog.svelte',
  'lib/entries/DuplicateDialog.svelte',
  'lib/entries/EntryActions.svelte',
  'lib/entries/EntryList.svelte',
  'lib/entries/ExportDialog.svelte',
  'lib/entries/FeedsDialog.svelte',
  'lib/entries/LinksSection.svelte',
  'lib/entries/MentionDialog.svelte',
  'lib/entries/MergeSplitDialog.svelte',
  'lib/entries/MoveDialog.svelte',
  'lib/entries/RecurrencePanel.svelte',
  'lib/entries/StepControl.svelte',
  'lib/entries/TemplatesDialog.svelte',
  'lib/entries/TypeDropsDialog.svelte',
  'lib/entries/ViewsPanel.svelte',
  'lib/files/EntryFiles.svelte',
  'lib/frame/AppWorkspaces.svelte',
  'lib/frame/HelpDialog.svelte',
  'lib/frame/LeaveDialog.svelte',
  'lib/frame/RefusedDraft.svelte',
  'lib/frame/StepUpPrompt.svelte',
  'lib/frame/SwitchWorkspace.svelte',
  'lib/frame/UnsentChangesDialog.svelte',
  'lib/instance/MovePlanDialog.svelte',
  'lib/instance/OwnDomainsTable.svelte',
  'lib/pages/PageAccess.svelte',
  'lib/palette/Palette.svelte',
  'lib/product/CreateDialog.svelte',
  'lib/time/AbsenceCalendar.svelte',
  'lib/time/BillingRates.svelte',
  'lib/time/BudgetPanel.svelte',
  'lib/time/CostRates.svelte',
  'lib/time/RunningTimer.svelte',
  'lib/whiteboards/BoardCanvas.svelte',
  'lib/workspace/CreateContainerDialog.svelte',
  'lib/workspace/NarrowingDialog.svelte',
  'lib/workspace/PanelOrDialog.svelte',
  'lib/workspace/PositionDialog.svelte',
  'lib/workspace/StarterDialog.svelte',
  'views/AgentsView.svelte',
  'views/CmdbView.svelte',
  'views/ContainerView.svelte',
  'views/DashboardView.svelte',
  'views/GoalView.svelte',
  'views/GoalsView.svelte',
  'views/InstanceAiView.svelte',
  'views/InstanceOperatorsView.svelte',
  'views/InstancePlanView.svelte',
  'views/InstancePlansView.svelte',
  'views/InstancePlatformView.svelte',
  'views/InstanceProvidersView.svelte',
  'views/InstanceTargetsView.svelte',
  'views/InstanceWorkspacesView.svelte',
  'views/PagesView.svelte',
  'views/PlanView.svelte',
  'views/PortfolioView.svelte',
  'views/ReleaseView.svelte',
  'views/RuleEditorView.svelte',
  'views/ServiceDeskView.svelte',
  'views/SprintView.svelte',
  'views/TimeSettingsView.svelte',
  'views/TimeView.svelte',
  'views/TrashView.svelte',
  'views/WhiteboardsView.svelte',
];

/** The confirmation the ladder allows over another dialog. */
const DISCARD = 'DiscardDialog';

/**
 * A failure's *Details for support* (PS-15) folds the failure, not the dialog's content: it is
 * drawn only after a refusal and is no second fold of what the dialog asks.
 */
const FAILURE = reachable(path.join(SRC, 'lib/frame/FailureNote.svelte'));

/** Everything the components drawn inside a range draw, by file. */
function drawnInside(file, ranges) {
  const files = new Set();
  for (const use of uses(file)) {
    if (use.name === DISCARD || !within(ranges, use.at) || !fs.existsSync(use.file)) continue;
    for (const inner of reachable(use.file)) files.add(inner);
  }
  return files;
}

const count = (markup, pattern) => [...markup.matchAll(pattern)].length;

/** What is wrong with each dialog of a file. */
function faults(file) {
  const { markup } = read(file);
  const found = [];
  for (const dialog of elements(markup, 'Dialog')) {
    const where = `${relative(file)}:${markup.slice(0, dialog.start).split('\n').length}`;
    if (!/\bsize=|\{size\}/.test(dialog.tag)) found.push(`${where}: no size`);
    const body = markup.slice(dialog.start + dialog.tag.length, dialog.end);
    const inner = [...drawnInside(file, [dialog])].map((each) => read(each).markup);
    const all = [body, ...inner];
    if (all.some((text) => /<(Table|DataTable)(?=[\s/>])|<table(?=[\s>])/.test(text))) found.push(`${where}: a table`);
    if (all.some((text) => /<(Tabs|PlaceTabs)(?=[\s/>])/.test(text))) found.push(`${where}: tabs`);
    if (all.some((text) => /<Dialog(?=[\s/>])/.test(text))) found.push(`${where}: a second dialog`);
    const folds = [body, ...[...drawnInside(file, [dialog])].filter((each) => !FAILURE.has(each)).map((each) => read(each).markup)];
    if (folds.reduce((sum, text) => sum + count(text, /<Disclosure(?=[\s/>])/g), 0) > 1) found.push(`${where}: more than one Disclosure`);
  }
  return found;
}

const files = () => svelteFiles().filter((file) => /<Dialog(?=[\s/>])/.test(read(file).markup));

test('every dialog names its size and holds no table, tabs, second dialog or second Disclosure', () => {
  const wrong = files()
    .filter((file) => !NOT_YET_MIGRATED.includes(relative(file)))
    .flatMap(faults);
  assert.deepEqual(wrong, []);
});

test('the list of files not yet migrated only shrinks', () => {
  const missing = NOT_YET_MIGRATED.filter((file) => !fs.existsSync(path.join(SRC, file)));
  assert.deepEqual(missing, [], 'a file that is gone leaves the list');
  const done = NOT_YET_MIGRATED.filter((file) => faults(path.join(SRC, file)).length === 0);
  assert.deepEqual(done, [], 'a file whose dialogs meet the ladder leaves the list');
});
