// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * What the administration's overview says needs attention (UC-ID-24 check 2), decided from reads
 * the screens behind it already make — the limits, the rules, the backup archives and their runs,
 * the privacy register — so the overview and the page a card links to never disagree.
 *
 * Pure: the screen holds the reads, this decides what of them is said. Each answer is ordered so
 * the first line is the one that most needs somebody.
 */

/** A limit as `/quotas` answers it, the part the overview reads. */
export interface LimitStanding {
  readonly quota: string;
  readonly limit: number;
  readonly used?: number | null;
  readonly ratio?: number | null;
}

/** How full a limit counts as close to its ceiling: the same share the quota alert warns at. */
export const NEAR_RATIO = 0.8;

export type LimitState = 'reached' | 'near' | 'room';

export interface LimitLine {
  readonly quota: string;
  readonly limit: number;
  readonly used: number;
  readonly state: LimitState;
}

/**
 * The limits with a ceiling and a live count, fullest first. A limit without a ceiling (`0`) or
 * without a count says nothing about how close anything is, so it is not a line here; the limits
 * page lists it.
 */
export function limitLines(standings: readonly LimitStanding[]): LimitLine[] {
  return standings
    .filter((standing) => standing.limit > 0 && typeof standing.used === 'number')
    .map((standing) => {
      const used = standing.used as number;
      const ratio = typeof standing.ratio === 'number' ? standing.ratio : used / standing.limit;
      const state: LimitState = used >= standing.limit ? 'reached' : ratio >= NEAR_RATIO ? 'near' : 'room';
      return { quota: standing.quota, limit: standing.limit, used, state, ratio };
    })
    .sort((a, b) => b.ratio - a.ratio || a.quota.localeCompare(b.quota))
    .map(({ quota, limit, used, state }) => ({ quota, limit, used, state }));
}

/** A rule as `/automation/rules` answers it, the part the overview reads. */
export interface RuleReading {
  readonly id: string;
  readonly name: string;
  readonly enabled: boolean;
  readonly failure_count: number;
  readonly findings?: readonly { readonly level: 'ATTENTION' | 'BROKEN' }[];
}

export type RuleState = 'failing' | 'broken' | 'attention';

export interface RuleLine {
  readonly id: string;
  readonly name: string;
  readonly state: RuleState;
  readonly failures: number;
}

const RULE_ORDER: Record<RuleState, number> = { failing: 0, broken: 1, attention: 2 };

/**
 * The rules that fail or need attention: failing runs first (most failures first), then a rule the
 * check found broken, then one it found worth a look. A rule that is off and fails nothing is
 * nobody's concern today.
 */
export function ruleLines(rules: readonly RuleReading[]): RuleLine[] {
  const lines: RuleLine[] = [];
  for (const rule of rules) {
    const levels = new Set((rule.findings ?? []).map((finding) => finding.level));
    const state: RuleState | undefined = rule.failure_count > 0
      ? 'failing'
      : levels.has('BROKEN') ? 'broken' : levels.has('ATTENTION') && rule.enabled ? 'attention' : undefined;
    if (state) lines.push({ id: rule.id, name: rule.name, state, failures: rule.failure_count });
  }
  return lines.sort((a, b) => RULE_ORDER[a.state] - RULE_ORDER[b.state] || b.failures - a.failures || a.name.localeCompare(b.name));
}

/** An archive as a target's manifest lists it, the part the overview reads. */
export interface ArchiveReading {
  readonly archive_id: string;
  readonly created_at: string;
  readonly complete?: boolean;
}

/** The newest archive across every target read, with the target it lies at. */
export function newestArchive(
  byTarget: Readonly<Record<string, readonly ArchiveReading[]>>,
): { readonly targetId: string; readonly archive: ArchiveReading } | undefined {
  let newest: { targetId: string; archive: ArchiveReading } | undefined;
  for (const [targetId, archives] of Object.entries(byTarget)) {
    for (const archive of archives) {
      if (!newest || Date.parse(archive.created_at) > Date.parse(newest.archive.created_at)) newest = { targetId, archive };
    }
  }
  return newest;
}

/** Whether the last backup opened: checked and read back, checked and broken, not checked, unfinished. */
export type BackupState = 'opened' | 'broken' | 'unchecked' | 'incomplete';

export function backupState(
  archive: ArchiveReading,
  run: { readonly verified_at?: string | null; readonly verify_ok?: boolean | null } | undefined,
): BackupState {
  if (archive.complete === false) return 'incomplete';
  if (run?.verified_at && run.verify_ok === true) return 'opened';
  if (run?.verified_at && run.verify_ok === false) return 'broken';
  return 'unchecked';
}

/** A privacy request, the part the overview reads. */
export interface RequestReading {
  readonly id: string;
  readonly status: string;
  readonly due_at: string;
}

/**
 * The requests owed soon or already overdue, soonest first, by the register's own window
 * (`standing` is `standingOf` from the register, so the two can never say different things).
 */
export function owedSoon<R extends RequestReading>(
  requests: readonly R[],
  standing: (request: R) => 'overdue' | 'soon' | 'ahead' | undefined,
): R[] {
  return requests
    .filter((request) => {
      const at = standing(request);
      return at === 'overdue' || at === 'soon';
    })
    .sort((a, b) => Date.parse(a.due_at) - Date.parse(b.due_at));
}

/**
 * Where a degraded feature is set in the administration, where it has a page there. A feature the
 * workspace cannot act on — storage, mail — is said without a link: the installation answers it.
 */
const FEATURE_PAGES: Readonly<Record<string, string>> = {
  ai: '/administration/ai',
  automation: '/administration/rules',
  backup: '/administration/backup',
  webhooks: '/administration/webhooks',
};

export const pageOfFeature = (feature: string): string | undefined => FEATURE_PAGES[feature];

/**
 * The page that changes the block a configuration check's finding is about (configuration/ui.md
 * A5): its section's administration page, or a hub's settings for the hub's own scheme.
 */
const CHECK_PAGES: Readonly<Record<string, string>> = {
  workflows: '/administration/workflows',
  rules: '/administration/rules',
  roles: '/administration/permissions',
  types: '/administration/types',
};

export function pageOfFinding(path: string, hubId?: string): string | undefined {
  const section = path.slice(0, Math.max(0, path.indexOf('.'))) || path;
  if (section === 'scheme') return hubId ? `/hubs/${hubId}/settings/structure` : undefined;
  return CHECK_PAGES[section];
}

/** A finding as `/config/check` answers it, the part a page reads. */
export interface FindingAt {
  readonly path: string;
}

/**
 * The findings a block's own page shows (configuration/ui.md A5): those about a block of the
 * sections the page configures, in the check's order - errors first.
 */
export function findingsIn<F extends FindingAt>(findings: readonly F[], sections: readonly string[]): F[] {
  return findings.filter((finding) => sections.includes(finding.path.slice(0, Math.max(0, finding.path.indexOf('.'))) || finding.path));
}
