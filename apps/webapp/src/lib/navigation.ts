// SPDX-License-Identifier: Apache-2.0
// Copyright (c) 2026 Jérôme Bastian Winkel

/**
 * The one list of destinations (ADR-0061 decision 1).
 *
 * Every width draws this list and nothing else: the bands as a pinned `SideNav` from `expanded`, in
 * the `NavDrawer` on `medium` and behind *More* on `compact`, where the `BottomBar` carries the
 * places a thumb needs; the account group behind the avatar on every width. A destination that is
 * in one drawing and not another is the failure this file exists to prevent, which is why the
 * frame has no list of its own and why the test beside this file reads the route table rather
 * than trusting the strings here.
 *
 * The list has a **shape**, and the frame draws it and no other (ADR-0063 decision 1,
 * user-experience.md §2.1): five bands in one order — the places that are not the tree, the
 * products switched on, the tree itself, the workspace's administration, and `KEEPING`, pinned to
 * the foot of the column. They never mix, and nothing is drawn outside one.
 *
 * `area` is ADR-0032's: the mobile build renders the administration row as an entry that says
 * where the capability lives, and nothing else about the list changes.
 */

import type { IconName } from '@hubtask/design-system/components';

import type { Area } from './router.ts';
import { PLACE_ROUTES } from './routes.ts';

export type Group = 'primary' | 'account';

/**
 * The five bands the navigation is drawn in, in this order and no other (ADR-0063 decision 1,
 * user-experience.md §2.1): the places, the products switched on, the tree, the workspace's
 * administration and the keeping band.
 *
 * `places` are the rooms of the product that are not the tree; `tree` is the workspace's own
 * structure; `keeping` is where somebody looks when something is **missing**, and it is pinned to
 * the foot of the column so that it is never mixed in with the hubs above it. Nothing is drawn
 * outside a band, and nothing is in two.
 */
export type Band = 'places' | 'products' | 'tree' | 'workspace' | 'keeping';

/** The account group's verbs. */
export type AccountAction = 'help' | 'sign-out' | 'switch-workspace' | 'add-workspace';

/**
 * What choosing a destination does: goes somewhere, performs one of the two account verbs, or
 * opens the capture field - the one row of the primary group that is an act rather than a place,
 * drawn as a button where it is drawn at all (UC-WRK-26 check 1).
 */
export type Target =
  | { readonly kind: 'route'; readonly path: string }
  | { readonly kind: 'action'; readonly action: AccountAction }
  | { readonly kind: 'capture' };

export interface Destination {
  /** A stable identifier the drawings key on and the bottom bar announces; never display text. */
  readonly id: string;
  readonly group: Group;
  readonly icon: IconName;
  /** The message code of its word (ADR-0011). */
  readonly code: string;
  readonly target: Target;
  /**
   * The route name the destination is current for, so `aria-current` can be set from the
   * resolved route rather than by comparing paths. An account destination is current for its
   * own route only; the profile also owns `/profile/tokens`.
   */
  readonly routes: readonly string[];
  /** ADR-0032's area, where the row is one the mobile build treats differently. */
  readonly area?: Area;
  /** The suite module whose screen this is: the row is drawn only where it is on (UC-INS-21). */
  readonly module?: string;
  /** A code of synonyms the palette also finds the row by (user-experience.md §2.7). */
  readonly words?: string;
  /** A place the phone's bottom bar leaves to the drawer: the bar has five slots and no more. */
  readonly isOffBar?: boolean;
  /**
   * Whether a separator opens this row in the account group: the ways through start below the
   * places that are the reader's own and the switch between workspaces (the approved CD-09 preview).
   */
  readonly startsBand?: boolean;
}

export const DESTINATIONS: readonly Destination[] = [
  // The primary group: the three spaces one moves between. The first is *Today* (UC-WRK-34):
  // what is the reader's today across every hub, the page the app opens on - not a list of the
  // hubs the tree below it lists (ADR-0063 decision 1). Its id stays `workspace`, the key the
  // drawings and the walks know it by; the workspace keeps its own name for what it is called.
  { id: 'workspace', group: 'primary', icon: 'sun', code: 'app.nav.today', target: { kind: 'route', path: '/' }, routes: ['home', ...PLACE_ROUTES, 'item'] },
  { id: 'search', group: 'primary', icon: 'search', code: 'app.nav.search', target: { kind: 'route', path: '/search' }, routes: ['search'], words: 'app.nav.words_search' },
  // Capture: the phone's ＋ between Search and the jumble (ux-inventory.md §E0). Only the bottom bar
  // draws it; from `medium` up the bar carries *Capture* as a button of its own, and the column
  // lists places, not acts (`primary`'s `hasCapture`).
  { id: 'capture', group: 'primary', icon: 'plus', code: 'app.nav.capture', target: { kind: 'capture' }, routes: [] },
  { id: 'jumble', group: 'primary', icon: 'jumble', code: 'app.nav.jumble', target: { kind: 'route', path: '/jumble' }, routes: ['jumble'], words: 'app.nav.words_jumble' },
  // What waits for the reader's yes (UC-SVC-09 check 1): a place of the column, where the service
  // desk is on - the approvals are the desk's (user-experience.md §2.1). The phone reaches it
  // through *More*.
  { id: 'approvals', group: 'primary', icon: 'user-check', code: 'app.nav.approvals', target: { kind: 'route', path: '/approvals' }, routes: ['approvals'], module: 'SERVICE_DESK', isOffBar: true },
  // The account group: what the avatar opens. Reachable from every screen, because the profile
  // is where somebody goes when the product is speaking to them in the wrong language — which is
  // exactly the moment a buried link is no use.
  // The marks say what each row is about (ADR-0063 decision 6). `user` for what is the reader's
  // own; the **gear moves to the administration**, where "the application's settings" is what it
  // actually means — one icon fewer than the `sliders` the ADR named; `compass` for a tour, which
  // is a way through rather than a notice, so the tour and the installation do not share the
  // outlined `info`: two rows of one menu would say the same thing with one mark.
  { id: 'profile', group: 'account', icon: 'user', code: 'app.nav.profile', target: { kind: 'route', path: '/profile' }, routes: ['profile', 'tokens'], area: 'profile' },
  // *Appearance*: the theme and reduced motion, the person's device (user-experience.md §2.3).
  { id: 'appearance', group: 'account', icon: 'sun-moon', code: 'app.nav.appearance', target: { kind: 'route', path: '/profile/appearance' }, routes: ['appearance'], area: 'profile' },
  // The level above the workspaces (ADR-0070 §5). Drawn only for an account in the operator
  // register, which the manifest answers — never compiled in, and **absent** rather than disabled
  // where it does not apply: somebody who is not an operator was never going to have it, and a
  // greyed row would be an invitation to ask why.
  { id: 'instance', group: 'account', icon: 'gauge', code: 'app.nav.instance', target: { kind: 'route', path: '/instance' }, routes: ['instance'], area: 'instance' },
  // Help and the list of shortcuts, with the tour inside it (ux-inventory.md §E1): one row for
  // "how does this work", where the tour was a row of its own.
  // The device's list of workspaces (UC-ID-25 check 2): a verb, opening the list as a dialog - a
  // sheet on a phone - rather than the list inside this menu, which would grow with every
  // workspace. Drawn only in multi mode, where there is a list (check 8).
  { id: 'switch-workspace', group: 'account', icon: 'arrow-right-left', code: 'app.workspaces.switch', target: { kind: 'action', action: 'switch-workspace' }, routes: [] },
  // An installed app adds a workspace by its address, of this installation or another (UC-SYN-07
  // check 4): a row of its own beside the switch, and the dialog's foot (the approved preview).
  { id: 'add-workspace', group: 'account', icon: 'plus', code: 'app.app_workspaces.add', target: { kind: 'action', action: 'add-workspace' }, routes: [] },
  { id: 'help', group: 'account', icon: 'compass', code: 'app.help.label', target: { kind: 'action', action: 'help' }, routes: [], startsBand: true },
  // The installation: four facts — the product version, the API version, the tenancy and the
  // languages — that nobody navigates to and everybody quotes when they report a problem. It keeps
  // its address and its page; what it loses is a place in somebody's navigation. Everyone may see
  // it: it names no person and no content, and a reader who cannot read their own product's
  // version cannot file a useful report.
  //
  // **The row does not carry the version** (ADR-0065 decision 5). `product_version` is a release
  // version on a release and a build reference - `main-<40 hex>` - on everything else, and a build
  // reference is forty characters of noise where a menu row's name belongs. The row names the
  // destination; the page behind it quotes the version whole, which is what that page is for.
  { id: 'about', group: 'account', icon: 'info', code: 'app.nav.about', target: { kind: 'route', path: '/installation' }, routes: ['installation'] },
  // Signing out is the **last** row, which reverses ADR-0063 decision 6's order (ADR-0065
  // decision 5): it is the last thing a reader does in a session, and a row under it is a row
  // somebody reaches past.
  { id: 'sign-out', group: 'account', icon: 'log-out', code: 'app.sign_out', target: { kind: 'action', action: 'sign-out' }, routes: [] },
];

/**
 * The `keeping` band: where a reader goes when something is missing, at the foot of the column.
 *
 * The archive and the trash. Neither is "the tree's last node" (ADR-0063 decision 1), because a row somebody reaches for when something has gone
 * should not sit under the last hub as though it were one.
 */
export interface KeepingRow {
  readonly id: string;
  readonly icon: IconName;
  /** The message code of its word (ADR-0011). */
  readonly code: string;
  readonly path: string;
  readonly routes: readonly string[];
  /** The ADR-0032 area its screens belong to, where a build may leave it to the web app. */
  readonly area?: Area;
}

const ARCHIVE_ROW: KeepingRow = { id: 'archive', icon: 'archive', code: 'app.nav.archive', path: '/archive', routes: ['archive'] };
const TRASH_ROW: KeepingRow = { id: 'trash', icon: 'trash', code: 'app.nav.trash', path: '/trash', routes: ['trash'] };

// The archive before the trash: what was put aside is the milder of the two, and the one somebody
// reaches for first when something has gone.
export const KEEPING: readonly KeepingRow[] = [ARCHIVE_ROW, TRASH_ROW];

/**
 * A product's row in the `products` band (user-experience.md §2.2): one per module with a home,
 * in `Module.Keys()` order so the band never reorders itself when a module is switched. WORKFLOWS
 * has none - workflows shape entries in every product.
 */
export interface ProductRow {
  readonly id: string;
  readonly module: string;
  readonly icon: IconName;
  readonly code: string;
  /** A code of synonyms the palette also finds the row by (§2.7). */
  readonly words: string;
  readonly path: string;
  readonly routes: readonly string[];
}

export const PRODUCTS: readonly ProductRow[] = [
  { id: 'plans', module: 'PLANNING', icon: 'calendar', code: 'app.nav.plans', words: 'app.nav.words_plans', path: '/plans', routes: ['plans-home'] },
  // *My notes* is a tab of the pages' home (§2.2); its own address is kept and is this row's.
  { id: 'pages', module: 'KNOWLEDGE', icon: 'book-open', code: 'app.nav.pages', words: 'app.nav.words_pages', path: '/pages', routes: ['pages-home', 'notes'] },
  { id: 'whiteboards', module: 'WHITEBOARD', icon: 'layout-template', code: 'app.nav.whiteboards', words: 'app.nav.words_whiteboards', path: '/whiteboards', routes: ['whiteboards-home'] },
  { id: 'desk', module: 'SERVICE_DESK', icon: 'inbox', code: 'app.nav.desk', words: 'app.nav.words_desk', path: '/desk', routes: ['desk-home', 'desk-desks', 'desk-settings'] },
  { id: 'assets', module: 'ASSETS', icon: 'shield', code: 'app.nav.assets', words: 'app.nav.words_assets', path: '/assets', routes: ['assets-home', 'assets-settings'] },
  { id: 'time', module: 'TIME', icon: 'clock', code: 'app.nav.time', words: 'app.nav.words_time', path: '/time', routes: ['time'] },
  { id: 'goals', module: 'GOALS', icon: 'flag', code: 'app.nav.goals', words: 'app.nav.words_goals', path: '/goals', routes: ['goals', 'goal', 'goal-results', 'portfolio', 'dashboard'] },
  { id: 'agents', module: 'AGENTS', icon: 'zap', code: 'app.nav.agents', words: 'app.nav.words_agents', path: '/agents', routes: ['agents-home', 'agent'] },
];

/** The products band: the rows of the modules switched on, in the one order; none while none is on. */
export function products(options: { readonly modulesOn?: readonly string[] }): readonly ProductRow[] {
  const on = options.modulesOn ?? [];
  return PRODUCTS.filter((row) => on.includes(row.module));
}

/** The product row a module is drawn by, or nothing for a module without a home. */
export const productOf = (module: string): ProductRow | undefined => PRODUCTS.find((row) => row.module === module);

/**
 * The `workspace` band: *Administration*, in the column rather than behind the avatar (§2.1),
 * drawn where the manifest says the reader reaches it - `STRUCTURE` or `READ_CONFIGURATION` on the
 * workspace, the condition the account menu's row had.
 */
const ADMINISTRATION_ROW: KeepingRow = { id: 'administration', icon: 'settings', code: 'app.nav.administration', path: '/administration', routes: ['administration'], area: 'administration' };

export function workspaceBand(options: { readonly isAdministrationReachable: boolean }): readonly KeepingRow[] {
  return options.isAdministrationReachable ? [ADMINISTRATION_ROW] : [];
}

/**
 * The administration's own navigation, in the five groups ADR-0063 decision 7 names.
 *
 * A second list rather than a band of the first, because it is a second *place*: while the route's
 * area is `administration` the column shows this and the workspace's tree is not drawn at all. The
 * reader is inside the section, and a tree of hubs beside sixteen settings screens would say they
 * are somewhere they are not.
 *
 * Who sees it is not decided here and not by this list. The area is offered where the manifest says
 * the reader holds `STRUCTURE` or `READ_CONFIGURATION` on the workspace, the condition of the
 * area's reads (ADR-0061 decision 1); a reader without it has no row, no section and no route, and
 * the server refuses the screens regardless.
 */
export interface SectionRow {
  readonly id: string;
  readonly icon: IconName;
  /** The word the row carries in the column. */
  readonly code: string;
  /**
   * The word the screen wears as its heading, where a column has less room than a heading does.
   *
   * "Where you are signed in" is the right sentence over a table and four words too many in a
   * column 240 px wide, where it would be cut to "Where you are sign…". So the row says *Signed
   * in* and the screen says the sentence; absent, they are the same word, which is what every row
   * of the administration is.
   */
  readonly titleCode?: string;
  readonly path: string;
  /** The route names this row is current for. */
  readonly routes: readonly string[];
  /** The suite module whose screen this is: the row is drawn only where it is on (UC-INS-21). */
  readonly module?: string;
  /** A code of synonyms the palette also finds the row by (user-experience.md §2.7). */
  readonly words?: string;
  /** A screen few workspaces need at the start, folded under *More* while the workspace is small. */
  readonly isAdvanced?: boolean;
  /** Drawn under its group's *More* for this reader; still found by the palette (§8.2). */
  readonly isFolded?: boolean;
}

export interface SectionGroup {
  readonly id: string;
  /** The group's caption, or nothing for the head, which is one row and needs none. */
  readonly code?: string;
  readonly rows: readonly SectionRow[];
}

/**
 * The marks come from the set ADR-0041 declares. The groups are the suite's six
 * (`backlog/suite/configuration/ui.md` §1): the overview, how we work, people and access,
 * connections, data and records - *Changes* joins with its pages. Every page is one row away, so it is reached in at most two steps
 * from anywhere in the section (UC-ID-24 check 3).
 */
export const ADMINISTRATION: readonly SectionGroup[] = [
  // The way back, first and alone: a section a reader cannot leave is a trap, and the row that
  // leads out is the one they look for at the top rather than at the foot. The overview beside
  // it is the section's front door (UC-ID-24 check 1).
  {
    id: 'back',
    rows: [
      { id: 'back', icon: 'chevron-left', code: 'app.admin.back', path: '/', routes: [] },
      { id: 'overview', icon: 'gauge', code: 'app.admin_overview.title', path: '/administration', routes: ['administration'] },
    ],
  },
  // The six groups of the suite's administration (configuration/ui.md §1, D-18): how we work ·
  // people and access · connections · data and records, under the way out and the overview.
  // *Changes* joins when its pages exist (the configuration document, S-09).
  {
    id: 'how',
    code: 'app.admin.group_how_we_work',
    rows: [
      { id: 'workspace', icon: 'settings', code: 'app.admin.workspace', path: '/administration/workspace', routes: ['workspace-settings', 'workspace-section'] },
      // The main address, and who may change it (UC-INS-19 check 3).
      { id: 'address', icon: 'globe', code: 'app.address.title', path: '/administration/address', routes: ['workspace-address'] },
      { id: 'templates', icon: 'sliders-horizontal', code: 'app.presets.title', path: '/administration/templates', routes: ['templates'] },
      { id: 'modules', icon: 'layout-template', code: 'app.modules.title', path: '/administration/modules', routes: ['modules'] },
      { id: 'entry-types', icon: 'tag', code: 'app.entry_types.title', path: '/administration/types', routes: ['entry-types', 'entry-type'], module: 'WORKFLOWS' },
      { id: 'workflows', icon: 'git-branch', code: 'app.workflow_designer.title', path: '/administration/workflows', routes: ['workflows', 'workflow'], module: 'WORKFLOWS' },
      // Links are the core's, not a module's (UC-PLN-01 check 5): the row is there in every workspace.
      { id: 'link-types', icon: 'link', code: 'app.link_types.title', path: '/administration/link-types', routes: ['link-types'], isAdvanced: true },
      // The editor's two routes are the rules' row as well: a reader inside a rule is inside
      // automation, and a column that marked nothing current there would say they are nowhere.
      { id: 'rules', icon: 'automation', code: 'app.admin.rules', path: '/administration/rules', routes: ['rules', 'rule', 'rule-new'], words: 'app.nav.words_rules' },
      { id: 'runs', icon: 'play', code: 'app.admin.runs', path: '/administration/runs', routes: ['runs'], words: 'app.nav.words_runs', isAdvanced: true },
    ],
  },
  {
    id: 'people',
    code: 'app.admin.group_people',
    rows: [
      // The groups and the roles are *People*'s tabs (§E8); their old addresses land there.
      { id: 'people', icon: 'user', code: 'app.admin.people', path: '/administration/people', routes: ['people', 'groups', 'permissions', 'person', 'group', 'role'], words: 'app.nav.words_people' },
      // The providers are the sign-in screen's *Ways and providers* tab (§E9); their old address
      // moved there and is claimed here, so the row is current the moment it lands.
      { id: 'sign-in-settings', icon: 'shield', code: 'app.admin.sign_in', path: '/administration/sign-in', routes: ['sign-in-settings', 'sign-in-section', 'sign-in-provider', 'identity-provider'] },
      { id: 'legal', icon: 'book-open', code: 'app.signin_settings.legal', path: '/administration/legal', routes: ['legal'] },
      { id: 'machines', icon: 'key', code: 'app.admin.machines', path: '/administration/machines', routes: ['machines', 'service-accounts', 'apps', 'service-account', 'app'], isAdvanced: true },
    ],
  },
  {
    id: 'connections',
    code: 'app.admin.group_connections',
    rows: [
      // The mail intake: a workspace's one inbox, sorted by handles and ticket keys (ADR-0094).
      { id: 'mail', icon: 'mail', code: 'app.admin.mail', path: '/administration/mail', routes: ['mail', 'mail-oauth', 'mail-section', 'mailbox'], words: 'app.nav.words_mail' },
      { id: 'webhooks', icon: 'send', code: 'app.admin.webhooks', path: '/administration/webhooks', routes: ['webhooks', 'webhook'], isAdvanced: true },
      // Voice assistants: Alexa puts things on lists by voice (ADR-0093).
      { id: 'voice', icon: 'mic', code: 'app.admin.voice', path: '/administration/voice', routes: ['voice', 'voice-section'], isAdvanced: true },
      { id: 'ai', icon: 'sparkles', code: 'app.admin.ai', path: '/administration/ai', routes: ['ai', 'ai-section'] },
    ],
  },
  {
    id: 'data',
    code: 'app.admin.group_data',
    rows: [
      // Drawn only where a private hub exists or more than one person is here (`administration`
      // below): the page is also where *Private hubs allowed* is set, and a workspace of one has
      // neither a hub to show nor anybody to keep one from.
      { id: 'private-hubs', icon: 'lock', code: 'app.admin.private_hubs', path: '/administration/private-hubs', routes: ['private-hubs'] },
      { id: 'backup', icon: 'cloud-upload', code: 'app.admin.backup', path: '/administration/backup', routes: ['backup', 'backup-section', 'backup-target'] },
      { id: 'retention', icon: 'clock', code: 'app.admin.retention', path: '/administration/retention', routes: ['retention', 'retention-section'] },
      { id: 'restore', icon: 'rotate-ccw', code: 'app.admin.restore', path: '/administration/restore', routes: ['restore'] },
      { id: 'audit', icon: 'file-text', code: 'app.admin.audit', path: '/administration/audit', routes: ['audit'], words: 'app.nav.words_audit' },
      { id: 'privacy', icon: 'file-user', code: 'app.admin.privacy', path: '/administration/privacy', routes: ['privacy', 'privacy-case'], words: 'app.nav.words_privacy' },
      { id: 'quotas', icon: 'sliders-horizontal', code: 'app.admin.quotas', path: '/administration/quotas', routes: ['quotas'], isAdvanced: true },
    ],
  },
  {
    id: 'changes',
    code: 'app.admin.group_changes',
    rows: [
      // The configuration's versions, the changes proposed for a person and the file (F-6).
      { id: 'changes', icon: 'clock', code: 'app.changes.title', path: '/administration/changes', routes: ['changes', 'change'] },
    ],
  },
];

/**
 * The installation as the administration's last group, on a private installation (UC-INS-04's
 * story): there the one workspace *is* the installation, so its owner finds the level above it
 * where they find everything else about it, and the row says what it applies to rather than
 * naming a register they never configured. It opens the instance area, which keeps its section.
 */
const INSTALLATION_GROUP: SectionGroup = {
  id: 'installation',
  code: 'app.admin.group_installation',
  rows: [
    { id: 'instance', icon: 'gauge', code: 'app.admin.installation', path: '/instance', routes: [] },
  ],
};

/**
 * The administration's navigation as this reader is shown it: the *Private hubs* row only where
 * the server answers the list and either a private hub exists or the workspace holds more than one
 * person (UC-ID-16 checks 3, 7) - a refusal, or a workspace of one without one, and it is absent;
 * and the installation as the last group where `withInstallation` says so (`installationPlace`).
 */
export function administration(options: {
  readonly hasPrivateHubs: boolean;
  readonly withInstallation?: boolean;
  /** The modules switched on (`modules_on`); a row of a module that is off is not drawn. */
  readonly modulesOn?: readonly string[];
  /** Whether the advanced rows fold under *More*: while the workspace holds one person (§8.2). */
  readonly isFoldingAdvanced?: boolean;
  /** Advanced rows already used here, by id: they stay unfolded. */
  readonly inUse?: readonly string[];
}): readonly SectionGroup[] {
  const on = options.modulesOn ?? [];
  const used = options.inUse ?? [];
  const groups = ADMINISTRATION.map((group) => {
    const rows = group.rows.filter(
      (row) => (options.hasPrivateHubs || row.id !== 'private-hubs') && (!row.module || on.includes(row.module)),
    );
    const folds = (row: SectionRow) => options.isFoldingAdvanced === true && row.isAdvanced === true && !used.includes(row.id);
    // A *More* that would hold one row hides as much as it shows, so a group folds two or none.
    if (rows.filter(folds).length < 2) return { ...group, rows };
    return { ...group, rows: rows.map((row) => (folds(row) ? { ...row, isFolded: true } : row)) };
  });
  return options.withInstallation ? [...groups, INSTALLATION_GROUP] : groups;
}

/**
 * The administration's navigation from what the frame knows about this reader - the one place the
 * inputs of `administration` are derived, so the column and the palette list the same rows.
 *
 * *Private hubs* is drawn where the list is readable and either a private hub exists or the
 * workspace holds more than one person, because there it is where *Private hubs allowed* is set
 * (ADR-0073 §6).
 */
export function administrationFor(options: {
  readonly isPrivateHubsReadable: boolean;
  readonly hasAnyPrivateHub: boolean;
  readonly hasSeveralPeople: boolean;
  readonly isInstanceReachable: boolean;
  readonly isAdministrationReachable: boolean;
  readonly isSingleWorkspace: boolean;
  readonly modulesOn?: readonly string[];
  /**
   * Whether the workspace is known to hold one person: only then do the advanced rows fold. The
   * server answers whether there are several, not how many, so §8.2's "more than five" is read as
   * "more than one"; unknown is not one, so nothing folds and unfolds while the workspace is read.
   */
  readonly isOnePerson?: boolean;
  /** Advanced rows the workspace already uses, by id (`administration`'s `inUse`). */
  readonly inUse?: readonly string[];
}): readonly SectionGroup[] {
  return administration({
    hasPrivateHubs: options.isPrivateHubsReadable && (options.hasAnyPrivateHub || options.hasSeveralPeople),
    modulesOn: options.modulesOn,
    isFoldingAdvanced: options.isOnePerson === true,
    ...(options.inUse ? { inUse: options.inUse } : {}),
    withInstallation:
      installationPlace({
        isInstanceReachable: options.isInstanceReachable,
        isAdministrationReachable: options.isAdministrationReachable,
        isSingleWorkspace: options.isSingleWorkspace,
      }) === 'administration',
  });
}

/**
 * Where the installation is offered to this reader: as the administration's last group on a
 * single-workspace installation whose operator can reach the administration, in the account menu
 * otherwise, and nowhere for somebody who is not an operator. Never in both, and never nowhere for
 * an operator - one who cannot reach the administration keeps the menu row (P-05).
 */
export function installationPlace(options: {
  readonly isInstanceReachable: boolean;
  readonly isAdministrationReachable: boolean;
  readonly isSingleWorkspace: boolean;
}): 'administration' | 'menu' | 'none' {
  if (!options.isInstanceReachable) return 'none';
  return options.isSingleWorkspace && options.isAdministrationReachable ? 'administration' : 'menu';
}

/**
 * Your settings, as a section of its own (ADR-0065 decision 3).
 *
 * The same anatomy the administration has, for the same reason: what is the reader's own was one
 * screen of nine sections, read by scrolling, with two lists in it that grow without bound. A
 * section gives each question an address, a heading and a place in a column that says where the
 * reader is.
 *
 * The rows reuse the words the screens already have - "Where you are signed in", "Second factor",
 * "Apps you have allowed" - because a row and the screen it opens saying two different things is
 * how a navigation stops being trustworthy.
 *
 * Who sees it is nobody's decision: every one of these screens is about the reader themselves, and
 * the area is ADR-0032's `profile`, which the mobile shell ships in full.
 */
export const SETTINGS: readonly SectionGroup[] = [
  // The way back, first and alone, exactly as the administration's is.
  {
    id: 'back',
    rows: [
      { id: 'back', icon: 'chevron-left', code: 'app.you.back', path: '/', routes: [] },
    ],
  },
  {
    id: 'you',
    code: 'app.you.group_you',
    rows: [
      { id: 'profile', icon: 'user', code: 'app.you.row_profile', titleCode: 'app.profile.title', path: '/profile', routes: ['profile', 'profile-section'] },
      // The theme and reduced motion: the device's (ADR-0043), and nothing else - the moments are
      // the account's and stand on *You*.
      { id: 'appearance', icon: 'sun-moon', code: 'app.profile.device_section', path: '/profile/appearance', routes: ['appearance'] },
    ],
  },
  {
    id: 'told',
    code: 'app.you.group_told',
    rows: [
      { id: 'notifications', icon: 'bell', code: 'app.you.row_notifications', titleCode: 'app.profile.notifications', path: '/profile/notifications', routes: ['notifications'] },
    ],
  },
  {
    id: 'entry',
    code: 'app.you.group_entry',
    rows: [
      // Three things rather than one: the password, the second factor and the recovery codes. A row
      // called after the second of them sends a reader looking for "where do I change my password"
      // to every other screen first.
      { id: 'security', icon: 'shield', code: 'app.security.title', path: '/profile/security', routes: ['security', 'security-section', 'second-factor-new'] },
      { id: 'sessions', icon: 'user-check', code: 'app.you.row_sessions', titleCode: 'app.sessions.title', path: '/profile/sessions', routes: ['sessions'] },
      { id: 'devices', icon: 'arrow-right-left', code: 'app.you.row_devices', titleCode: 'app.devices.title', path: '/profile/devices', routes: ['devices'] },
    ],
  },
  {
    id: 'acting',
    code: 'app.you.group_acting',
    rows: [
      { id: 'grants', icon: 'link', code: 'app.you.row_grants', titleCode: 'app.grants.title', path: '/profile/apps', routes: ['grants'] },
      { id: 'tokens', icon: 'key', code: 'app.tokens.title', path: '/profile/tokens', routes: ['tokens', 'token-new', 'token'] },
      { id: 'calendars', icon: 'calendar', code: 'app.you.row_calendars', titleCode: 'app.pushes.title', path: '/profile/calendars', routes: ['calendars'] },
      { id: 'files', icon: 'paperclip', code: 'app.you.row_files', titleCode: 'app.filestores.title', path: '/profile/files', routes: ['filestores'] },
      { id: 'rules', icon: 'automation', code: 'app.you.row_rules', titleCode: 'app.rules.mine_title', path: '/profile/rules', routes: ['own-rules', 'own-rule', 'own-rule-new'] },
      { id: 'agents', icon: 'zap', code: 'app.you.row_agents', titleCode: 'app.agents.title', path: '/profile/agents', routes: ['own-agents', 'own-agent'], module: 'AGENTS' },
    ],
  },
];

/**
 * The level above the workspaces (ADR-0070 §5).
 *
 * A third section beside the administration and Your settings, drawn by the same column for the
 * reason the ADR gives: "not a second bundle, not a second frame, not a second translation". What
 * makes it different is who it is for - an administrator runs a workspace, an operator runs the
 * installation - and the area tag is where that difference lives.
 *
 * No row showing anything inside a workspace. What an operator needs to run an installation is
 * counts, states, limits and what it offers every workspace; a row reaching into one would be the
 * thing ADR-0070 §5 forbids.
 */
export const INSTANCE: readonly SectionGroup[] = [
  // The way back, first and alone, exactly as the other two sections' is: a section a reader cannot
  // leave is a trap.
  {
    id: 'back',
    rows: [
      { id: 'back', icon: 'chevron-left', code: 'app.instance.back', path: '/', routes: [] },
    ],
  },
  {
    id: 'installation',
    code: 'app.instance.group_installation',
    rows: [
      { id: 'overview', icon: 'gauge', code: 'app.instance.overview', path: '/instance', routes: ['instance'] },
      { id: 'workspaces', icon: 'workspace', code: 'app.instance.workspaces', path: '/instance/workspaces', routes: ['instance-workspaces', 'instance-workspace', 'instance-workspace-new'] },
      // Each workspace's people, entries and stored bytes, once metering is on (§8.4).
      { id: 'usage', icon: 'gauge', code: 'app.instance.usage', path: '/instance/usage', routes: ['instance-usage'] },
      { id: 'plans', icon: 'sliders-horizontal', code: 'app.instance.plans', path: '/instance/plans', routes: ['instance-plans', 'instance-plan'] },
      { id: 'settings', icon: 'settings', code: 'app.instance.settings', path: '/instance/settings', routes: ['instance-settings', 'instance-settings-section'] },
      // The workspace's anatomy at this level (user-experience.md §8.4): its templates, its
      // module switches - the settings' *Modules* area - and the file in force.
      { id: 'templates', icon: 'layout-template', code: 'app.instance.templates', path: '/instance/templates', routes: ['instance-templates'] },
      { id: 'modules', icon: 'shapes', code: 'app.instance.modules', path: '/instance/settings/module', routes: [] },
      { id: 'file', icon: 'code', code: 'app.instance.file', path: '/instance/configuration-file', routes: ['instance-file'] },
      { id: 'providers', icon: 'key', code: 'app.instance.providers', path: '/instance/providers', routes: ['instance-providers'] },
      { id: 'ai', icon: 'sparkles', code: 'app.instance.ai', path: '/instance/ai', routes: ['instance-ai'] },
      { id: 'targets', icon: 'cloud-upload', code: 'app.instance.targets', path: '/instance/backup-targets', routes: ['instance-targets'] },
      { id: 'platform', icon: 'send', code: 'app.instance.platform', path: '/instance/platform', routes: ['instance-platform'] },
      { id: 'encryption', icon: 'lock', code: 'app.instance.encryption', path: '/instance/encryption', routes: ['instance-encryption'] },
    ],
  },
  {
    id: 'evidence',
    code: 'app.instance.group_evidence',
    rows: [
      { id: 'operators', icon: 'shield', code: 'app.instance.operators', path: '/instance/operators', routes: ['instance-operators'] },
      { id: 'journal', icon: 'clock', code: 'app.instance.journal', path: '/instance/journal', routes: ['instance-journal'] },
      { id: 'dead-letters', icon: 'triangle-alert', code: 'app.instance.dead_letters', path: '/instance/dead-letters', routes: ['instance-dead-letters'] },
    ],
  },
];

/**
 * The instance area's navigation as this installation draws it: without *AI* on a single-workspace
 * installation, where there is no model to offer anybody - the one workspace configures its own
 * (UC-AI-06 check 8) - and without the events to a purchase platform, which single mode offers
 * none of (UC-INS-14 check 7), nor plans (multi-tenancy.md §4.2).
 */
const MULTI_ONLY = new Set(['ai', 'platform', 'plans', 'usage', 'templates']);

export function instanceSection(options: { readonly isSingleWorkspace: boolean }): readonly SectionGroup[] {
  if (!options.isSingleWorkspace) return INSTANCE;
  return INSTANCE.map((group) => ({ ...group, rows: group.rows.filter((row) => !MULTI_ONLY.has(row.id)) }));
}

/** Kept as the name the rest of the client uses for the trash's row. */
export const TRASH = TRASH_ROW;

/**
 * The phone's fifth slot (user-experience.md §2.6): *More* opens the drawer, whose first band is the
 * products. The account is the avatar in the bar, as on every other width.
 */
export const MORE_CODE = 'app.nav.more';

/**
 * The primary group, whole — Search included, whatever the bar is carrying — and the jumble where
 * it is the reader's: `READ` on the workspace itself (UC-JUM-01 check 7). A guest on one collection
 * or an auditor has no jumble, and a row leading to one would lead to a refusal.
 *
 * **This reverses one sentence.** The rule was: from `medium` up the bar holds the entry to search
 * (ADR-0063 decision 4), so a row for it beside the tree would be a second entry to one
 * destination. That reading treated a *field* and a *destination* as the same thing, and they are
 * not: the field is where a search is typed, and Search is a place with a filter on it that
 * somebody goes to with nothing typed at all — to press a narrowing, to open one they were sent,
 * to go back to the one they were building. A reader who wants the place and is offered only a
 * field has to type something to reach it.
 *
 * Below `medium` nothing changes: the tree draws no primary group there, because the bottom bar
 * holds those destinations and Search is already one of them.
 */
export function primary(options: {
  readonly hasJumble: boolean;
  readonly hasCapture?: boolean;
  /** The modules switched on (`modules_on`); a place of a module that is off is not drawn. */
  readonly modulesOn?: readonly string[];
}): readonly Destination[] {
  const on = options.modulesOn ?? [];
  return DESTINATIONS.filter(
    (destination) =>
      destination.group === 'primary' &&
      (destination.id !== 'jumble' || options.hasJumble) &&
      (!destination.module || on.includes(destination.module)) &&
      (destination.target.kind !== 'capture' || options.hasCapture === true),
  );
}

/**
 * The phone's bottom bar before *More*: the places it carries itself (Today, Search, Capture,
 * Inbox). What it leaves out is the drawer's, which *More* opens (§2.6).
 */
export function bar(options: { readonly hasJumble: boolean; readonly hasCapture?: boolean }): readonly Destination[] {
  return primary(options).filter((destination) => !destination.isOffBar);
}

/**
 * The account group, with the two rows the server decides.
 *
 * The administration area is offered only where the manifest says this reader holds `STRUCTURE`
 * or `READ_CONFIGURATION` on the workspace — the area's condition exactly. The instance area is offered only
 * where the manifest says this account is in the operator register (ADR-0070 §1).
 *
 * These are the places in this client where **hiding beats a gate**, and the rule that decides it
 * is the same one `CapabilityGate` follows: a gate explains a refusal somebody might otherwise
 * have expected, and hiding is right where they never might have. A member who is not an operator
 * is not being refused the control plane — it is not theirs, the way another workspace is not.
 */
export function account(options: {
  readonly isAdministrationReachable: boolean;
  readonly isInstanceReachable: boolean;
  /** A single-workspace installation, where the installation is the administration's last group. */
  readonly isSingleWorkspace?: boolean;
  /** Whether this device keeps a list of workspaces: multi mode only (UC-ID-25 check 8). */
  readonly hasWorkspaceList?: boolean;
  /**
   * Whether this build holds workspaces of its own - an installed app, which lists them in single
   * mode too, because it may hold workspaces of several installations (UC-ID-25 check 7).
   */
  readonly holdsWorkspaces?: boolean;
}): readonly Destination[] {
  const place = installationPlace({
    isInstanceReachable: options.isInstanceReachable,
    isAdministrationReachable: options.isAdministrationReachable,
    isSingleWorkspace: options.isSingleWorkspace === true,
  });
  return DESTINATIONS.filter((destination) => {
    if (destination.group !== 'account') return false;
    if (destination.area === 'instance') return place === 'menu';
    if (destination.id === 'switch-workspace') return options.hasWorkspaceList === true || options.holdsWorkspaces === true;
    if (destination.id === 'add-workspace') return options.holdsWorkspaces === true;
    return true;
  });
}

/**
 * Which destination a resolved route belongs to, or nothing for a route outside the list — the
 * sign-in screens, the consent page. The area answers for the whole administration: the table
 * names each of its screens, and the list should not have to.
 */
export function currentDestination(route: { readonly name: string | null; readonly area: Area }): string | undefined {
  if (route.area === 'administration') return 'administration';
  // And the same for the instance area: the table names its screens, and the list carries one row
  // for all of them.
  if (route.area === 'instance') return 'instance';
  // And the same for Your settings, which is a section of its own since ADR-0065 decision 3: its
  // screens are the area, and the list should not have to name each of them twice.
  // *Appearance* is a row of its own in the account menu (user-experience.md §2.3), and its screen
  // is that row's.
  if (route.area === 'profile') return route.name === 'appearance' ? 'appearance' : 'profile';
  if (route.name === null) return undefined;
  const keeping = KEEPING.find((each) => each.routes.includes(route.name as string));
  if (keeping) return keeping.id;
  const product = PRODUCTS.find((each) => each.routes.includes(route.name as string));
  if (product) return product.id;
  return DESTINATIONS.find((destination) => destination.routes.includes(route.name as string))?.id;
}

/**
 * The screen a section's own address opens (ADR-0065 decision 1).
 *
 * A section needs no index, because the column already lists every screen: an index beside it is
 * the same list drawn twice. An overview is not an index - it says what needs attention - so the
 * administration's and the installation's first rows are theirs (UC-ID-24, UC-INS-12).
 *
 * The first row with a route of its own, which is what skips the way back: that row leads out of
 * the section, and a front door that led out would be a door somebody falls through.
 */
export function firstScreen(groups: readonly SectionGroup[]): string {
  for (const group of groups) {
    for (const row of group.rows) {
      if (row.routes.length > 0) return row.path;
    }
  }
  // Unreachable for a section that has a screen; a section that has none has nothing to open.
  return '/';
}

/**
 * The area of a row a build leaves to the web app (ADR-0032, UC-SYN-07 check 7), or nothing for a
 * row this build draws itself: the Android app keeps the administration's row in the column and the
 * installation's in the account menu, and they open the web app rather than a screen of its own.
 */
export function leftToWebApp(id: string, areas: readonly Area[] | undefined): Area | undefined {
  const area = id === ADMINISTRATION_ROW.id ? ADMINISTRATION_ROW.area : DESTINATIONS.find((each) => each.id === id)?.area;
  return area !== undefined && areas?.includes(area) ? area : undefined;
}
