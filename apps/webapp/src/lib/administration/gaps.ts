// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What this installation is not set up to do, read from the manifest's `features`, for the
 * administration overview's Features card: a part that is off by its configuration works for
 * nobody, though the health report counts it as no fault (a disabled dependency). Said to the
 * workspace's administrator in words of what they lose, never as the operator's variable.
 */

/** One part the installation does not serve, the sentence that says so, and its page if any. */
export interface Gap {
  readonly feature: string;
  readonly messageCode: string;
  readonly page?: string;
}

type Features = Readonly<Record<string, boolean | undefined>> | undefined;

/**
 * Whether the installation can keep a secret a workspace gives it - a webhook's. A manifest not
 * read yet, or a server too old to say, is taken as able: the server still refuses, and a form
 * hidden on a guess would be a feature taken away.
 */
export const keepsSecrets = (features: Features): boolean => features?.encryption !== false;

/** The gaps, mail first: notifications are what everybody misses. */
export function gapsOf(features: Features): Gap[] {
  const gaps: Gap[] = [];
  if (features?.mail === false) gaps.push({ feature: 'notifications', messageCode: 'app.admin_overview.gap_mail' });
  if (!keepsSecrets(features)) gaps.push({ feature: 'webhooks', messageCode: 'app.admin_overview.gap_secrets', page: '/administration/webhooks' });
  return gaps;
}
