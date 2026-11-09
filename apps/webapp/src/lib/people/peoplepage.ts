// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What *People*'s four list pages share: their tabs and the address of each. The groups and the
 * roles are tabs here (ux-inventory §E8): both answer "who may do what in this workspace".
 */
export const PEOPLE_TABS = [
  { id: 'people', code: 'app.people.tab_people' },
  { id: 'groups', code: 'app.groups.title' },
  { id: 'roles', code: 'app.people.tab_roles' },
  { id: 'permissions', code: 'app.people.tab_permissions' },
  { id: 'settings', code: 'app.people.tab_setting_writers' },
] as const;

export const PEOPLE_PATH = '/administration/people';

/** A tab's address: the people at the screen's own, the others by `?tab=`. */
export function tabPath(tab: string): string {
  return tab === 'people' ? PEOPLE_PATH : `${PEOPLE_PATH}?tab=${tab}`;
}
