// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

// The mockup's content (docs/backlog/page-system/mockups.html), shared by the workbench stories of
// wave 6 so every composition and page kind shows the same workspace. None of the names are real.

import type { MenuItem } from './overlay.ts';

export interface Person {
  readonly id: string;
  readonly name: string;
  readonly email: string;
  readonly role: string;
  readonly groups: string;
  readonly standing: 'Active' | 'Invited' | 'Suspended';
  readonly seen: string;
}

export const people: readonly Person[] = [
  { id: 'jk', name: 'Jana Krüger', email: 'owner@nordlicht.example.org', role: 'Owner', groups: '—', standing: 'Active', seen: 'now' },
  { id: 'lb', name: 'Lukas Brandt', email: 'lukas.brandt@nordlicht.example.org', role: 'Administrator', groups: 'CAB', standing: 'Active', seen: '2 h ago' },
  { id: 'ah', name: 'Amelia Hughes', email: 'amelia.hughes@nordlicht.example.org', role: 'Member', groups: 'Platform team', standing: 'Active', seen: 'yesterday' },
  { id: 'my', name: 'Mehmet Yılmaz', email: 'mehmet.yilmaz@nordlicht.example.org', role: 'Member', groups: 'Platform team, 1st level', standing: 'Active', seen: '3 days ago' },
  { id: 'pn', name: 'Priya Natarajan', email: 'priya.natarajan@nordlicht.example.org', role: 'Member', groups: 'CAB', standing: 'Invited', seen: '—' },
  { id: 'sr', name: 'Sofia Rossi', email: 'sofia.rossi@nordlicht.example.org', role: 'Member', groups: '1st level', standing: 'Active', seen: 'today' },
  { id: 'jw', name: 'Jonas Weber', email: 'jonas.weber@nordlicht.example.org', role: 'Contributor', groups: 'Platform team', standing: 'Active', seen: 'today' },
  { id: 'cn', name: 'Clara Neumann', email: 'clara.neumann@nordlicht.example.org', role: 'Viewer', groups: '—', standing: 'Active', seen: 'last week' },
  { id: 'tb', name: 'Thomas Becker', email: 'thomas.becker@nordlicht.example.org', role: 'Auditor', groups: '—', standing: 'Suspended', seen: 'a month ago' },
];

export const peopleColumns = [
  { id: 'name', label: 'Name', priority: 1, isSortable: true },
  { id: 'email', label: 'E-mail', priority: 2 },
  { id: 'role', label: 'Role', priority: 2, isHideable: true },
  { id: 'groups', label: 'Groups', priority: 3, isHideable: true },
  { id: 'standing', label: 'Standing', priority: 2, isStatus: true },
  { id: 'seen', label: 'Last seen', priority: 3, isHideable: true },
] as const;

export const standingTone = { Active: 'success', Invited: 'warning', Suspended: 'neutral' } as const;

export function personMenu(person: Person): { label: string; items: readonly MenuItem[] } {
  const items: MenuItem[] = [{ id: 'open', label: 'Open' }];
  if (person.standing === 'Invited') items.push({ id: 'renew', label: 'Renew the invitation' });
  else if (person.standing === 'Suspended') items.push({ id: 'resume', label: 'Resume' });
  else items.push({ id: 'role', label: 'Change role…' }, { id: 'view', label: 'View as' });
  if (person.role !== 'Owner' && person.standing !== 'Suspended') {
    items.push({ id: 'revoke', label: person.standing === 'Invited' ? 'Withdraw…' : 'Revoke access…', isDestructive: true, hasSeparatorBefore: true });
  }
  return { label: `More for ${person.name}`, items };
}

export interface AuditRecord {
  readonly id: string;
  readonly day: string;
  readonly time: string;
  readonly who: string;
  readonly what: string;
  readonly about: string;
}

export const audit: readonly AuditRecord[] = [
  { id: 'a1', day: 'Today', time: '16:50', who: 'Jana Krüger', what: 'Read an approval', about: 'Change “Firewall rule”' },
  { id: 'a2', day: 'Today', time: '16:42', who: 'Lukas Brandt', what: 'Changed a role', about: 'Role “Agent”' },
  { id: 'a3', day: 'Today', time: '09:13', who: 'Rule “Escalate SLA”', what: 'Moved an entry', about: 'SA-4' },
  { id: 'a4', day: 'Yesterday', time: '18:05', who: 'Sofia Rossi', what: 'Signed in', about: '—' },
];

export const auditColumns = [
  { id: 'when', label: 'When', priority: 2 },
  { id: 'who', label: 'Who', priority: 3 },
  { id: 'what', label: 'What', priority: 1 },
  { id: 'about', label: 'About', priority: 3 },
  { id: 'outcome', label: 'Outcome', priority: 2, isStatus: true },
] as const;

export const signInSections = [
  { id: 'passwords', label: 'Passwords', href: '#passwords', status: '2 set here', statusTone: 'set' },
  { id: 'second-factor', label: 'Second factor', href: '#second-factor', status: 'Locked', statusTone: 'locked' },
  { id: 'sessions', label: 'Sessions', href: '#sessions', status: 'Default' },
  { id: 'ways', label: 'Ways and providers', href: '#ways', status: '1 set here', statusTone: 'set' },
  { id: 'directory', label: 'Directory', href: '#directory', status: 'Off' },
  { id: 'no-mail', label: 'Accounts without mail', href: '#no-mail', status: 'On' },
] as const;

export const workflows = [
  { id: 'standard', label: 'Standard', href: '#standard', status: '4 types' },
  { id: 'incident', label: 'Incident', href: '#incident', status: 'live', statusTone: 'success' },
  { id: 'change', label: 'Change', href: '#change', status: 'draft', statusTone: 'warning' },
  { id: 'problem', label: 'Problem', href: '#problem', status: '1 type' },
  { id: 'request', label: 'Service request', href: '#request', status: '1 type' },
  { id: 'story', label: 'Story', href: '#story', status: '1 type' },
  { id: 'bug', label: 'Bug', href: '#bug', status: '1 type' },
  { id: 'risk', label: 'Risk', href: '#risk', status: '1 type' },
] as const;

export const templates = [
  { id: 'scrum', title: 'Scrum team', description: 'Stories and bugs from backlog to done in sprints.', mark: 'columns-3', chips: ['Pages', 'Planning'], state: { label: 'In use · update', tone: 'success' } },
  { id: 'itil', title: 'ITIL service desk', description: 'Incidents, requests, problems and changes with targets.', mark: 'inbox', chips: ['Service desk'], state: { label: 'In use', tone: 'success' } },
  { id: 'classic', title: 'Classic project', description: 'Deliverables, risks and changes on a timeline.', mark: 'calendar', chips: ['Planning'], state: { label: 'In use', tone: 'success' } },
  { id: 'kanban', title: 'Kanban team', description: 'Cards through To do, Doing, Review and Done.', mark: 'columns-3', chips: [], state: { label: 'In use', tone: 'success' } },
  { id: 'approvals', title: 'Approvals', description: 'Purchase, leave and travel requests, each decided.', mark: 'circle-check', chips: ['Service desk'] },
  { id: 'bugs', title: 'Bug tracking', description: 'Defects from report to fix, confirmed or closed.', mark: 'circle-alert', chips: [] },
  { id: 'okr', title: 'OKR', description: 'Objectives and key results, measured by the work.', mark: 'flag', chips: ['Goals'] },
  { id: 'household', title: 'Household', description: 'The chores of a household.', mark: 'house', chips: [] },
] as const;

export const statuses = [
  { id: 'backlog', name: 'Backlog', tone: 'neutral', summary: 'To do · not on the board' },
  { id: 'todo', name: 'To do', tone: 'neutral', summary: 'To do · new entries start here' },
  { id: 'progress', name: 'In progress', tone: 'warning', summary: 'In progress' },
  { id: 'done', name: 'Done', tone: 'success', summary: 'Done · completes the entry' },
] as const;

export const steps = [
  { id: 'start', name: 'Start work', summary: 'To do → In progress · anyone who may write' },
  { id: 'finish', name: 'Finish', summary: 'In progress → Done · Owner, Member · needs approval' },
  { id: 'reopen', name: 'Reopen', summary: 'Done → To do · Owner' },
] as const;

export const blockMenu = (name: string): { label: string; items: readonly MenuItem[] } => ({
  label: `More for ${name}`,
  items: [
    { id: 'edit', label: 'Edit…' },
    { id: 'up', label: 'Move up' },
    { id: 'down', label: 'Move down' },
    { id: 'delete', label: 'Delete…', isDestructive: true, hasSeparatorBefore: true },
  ],
});
