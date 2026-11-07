// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What *Machines and apps*' two list pages share: their tabs and the address of each. Both answer
 * "which machine may act here, and on whose behalf" (ux-inventory §E8).
 */
export const MACHINE_TABS = [
  { id: 'service-accounts', code: 'app.service_accounts.title' },
  { id: 'apps', code: 'app.apps.title' },
] as const;

export const MACHINES_PATH = '/administration/machines';

/** A tab's address: the service accounts at the screen's own, the apps by `?tab=apps`. */
export function machinesTabPath(tab: string): string {
  return tab === 'apps' ? `${MACHINES_PATH}?tab=apps` : MACHINES_PATH;
}
