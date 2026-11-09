<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // How people in this workspace prove who they are - the one place every sign-in rule is set
  // (UC-ID-12).
  //
  // **Eighteen switches, not one.** Every organisation draws the line somewhere else: one wants
  // NIST's answer - length, a blocklist and nothing more - and the next has to satisfy a rule that
  // demands classes, an expiry and a history. Both are configurations of one product rather than
  // two products, so each switch is its own, each is off by default where NIST says it should be,
  // and each carries the sentence that says what it costs.
  //
  // **Every rule says where its value came from** (check 4, P-06): set here, set by the
  // installation, or Hubtask's own default - and the installation's value beside it only where the
  // installation decided one. A private installation that decided nothing is not a column on this
  // screen (P-10). A locked rule keeps its value, its control is switched off with the reason
  // (`disabledReason`, which is how this design system says "not for you"), and the reason names
  // *who* locked it.
  //
  // **What the installation forbids is not offered** (check 5): a select does not list a choice
  // looser than the installation's, a number field does not go below it, and a switch the
  // installation requires is shown on with the reason. The server refuses the same things; the
  // screen just does not make anybody ask.
  //
  // **A refusal lands at its rule** (check 8): the server names the switch in the field's path, and
  // the sentence is drawn under that control rather than in a banner at the top of eighteen rows.
  //
  // **Every way in is one row with one switch** (check 6, UC-ID-11 check 8): the password, the
  // workspace's own providers and the ones the installation offers, in one list. A switch there acts
  // at once, behind the step-up - the password's through `methods`, a provider's through `:offer`,
  // the one verb for both kinds - and the provider screen switches nothing. The last way in
  // that is on cannot be switched off, here and at the server.
  //
  // **A provider the installation's lock turns on is shown on and set by the installation**
  // (UC-INS-11 check 4, identity.md §18): its switch stays in the list, on, switched off with the
  // reason, as every locked rule is drawn. Which rows that is, the server says per row
  // (`installation_lock`), so the screen never re-derives the rule.
  //
  // **A withdrawal is said where the switch is** (ADR-0076 §2, §4). A provider the installation
  // withdraws carries its date under its switch until then; from the date it has no switch, like
  // one switched off above. And a workspace that the withdrawal left with no way in is told that the
  // password opened again for the accounts that hold one - the server's word, from the same answer
  // the sign-in card reads - until another way is switched on here.
  //
  // **Switching the password off says what it costs first** (P-04, ADR-0078 §1): while the password
  // is on, its row says how many people here no provider switched on here signs in - the people who
  // will connect one by mail, or with their password at the provider's first arrival. A number from
  // the server, never a list.
  //
  // **Asking everybody for a new password is a button, not a field.** It sets a moment, and every
  // password older than it meets the change step at the next sign-in. It is red, it is behind a
  // confirmation, and its sentence says what it does to sessions.
  //
  // **One section per address, one save each** (page-system §7.3, PS-9, PS-10). *Passwords*,
  // *Second factor*, *Sessions*, *Ways and providers* - the list of ways, switched at once, with the
  // providers' table under it -, *Directory* and *Accounts without mail*, each at
  // `/administration/sign-in/<section>`; the list says each one's state. The legal links are not
  // about signing in at all and are the same rule's own page (`part: 'legal'`), so the filling, the
  // source chips and the save are not written twice. A section's save bar counts and sends its own
  // rows only; a row moved in another section waits there for that section's save, which is why a
  // move between sections replaces the address instead of passing the leave guard.

  import { untrack, type Snippet } from 'svelte';

  import { Badge, Banner, Button, Dialog, Input, SaveBar, Select, SettingRow, SettingsLayout, SettingsSection, Spinner, Switch } from '@hubtask/design-system/components';
  import type { SectionLink } from '@hubtask/design-system/components';

  import FailureNote from '../lib/frame/FailureNote.svelte';
  import FailureState from '../lib/frame/FailureState.svelte';
  import { TransportError } from '@hubtask/sync-engine';

  import LdapDirectoryPanel from '../lib/signin/LdapDirectoryPanel.svelte';
  import ProviderMark from '../lib/signin/ProviderMark.svelte';
  import ProvidersTable from '../lib/signin/ProvidersTable.svelte';
  import { withdrawalPhase } from '../lib/instance/withdrawal.ts';
  import { identityProvider } from '../lib/data/identityprovider.svelte.ts';
  import { ldapDirectory } from '../lib/data/ldapdirectory.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { signInPolicy, type LockOrigin, type PolicySource, type Setting } from '../lib/data/signinpolicy.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { enterSubmits } from '../lib/forms/enter.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import { holdsHere, includedHere, mayHere } from '../lib/data/capability.svelte.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import WorkspaceRefusal from '../lib/workspace/WorkspaceRefusal.svelte';
  import ValueSource from '../lib/workspace/ValueSource.svelte';
  import WriteOnly from '../lib/workspace/WriteOnly.svelte';
  import { page } from '../lib/frame/page.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import ViewSectionsAsFile from '../lib/settings/ViewSectionsAsFile.svelte';
  import { pagePart } from '../lib/settings/asFile.ts';

  interface Props {
    /** The sign-in rules by section, or the legal links on their own page. */
    part?: 'sign-in' | 'legal';
    /** The section the address names (`/administration/sign-in/:section`); none is the list. */
    section?: string;
    /** The tab an older address names (`?tab=`), replaced by its section's address. */
    tab?: string;
    /** Opens another address, entering it into the history: a provider's record page. */
    onnavigate?: (path: string) => void;
    /** Replaces the address: a move between sections, so the unsaved rows of another stay. */
    onreplace?: (path: string) => void;
  }
  let { part = 'sign-in', section, tab, onnavigate, onreplace }: Props = $props();

  const BASE = '/administration/sign-in';

  /** Which rows each section saves: the keys of `draft` its save bar counts and sends. */
  const PASSWORD_KEYS = [
    'min_length', 'min_lowercase', 'min_uppercase', 'min_digits', 'min_symbols', 'min_classes',
    'max_repeat', 'common_passwords', 'context_words', 'breach_check', 'max_age_days',
    'history_count', 'min_age_hours',
  ];
  const SCOPES: Readonly<Record<string, readonly string[]>> = {
    passwords: PASSWORD_KEYS,
    'second-factor': ['mfa_required_for'],
    sessions: ['session_max_days', 'session_idle_minutes'],
    ways: [],
    directory: [],
    'accounts-without-mail': ['managed_accounts'],
    legal: ['imprint_url', 'privacy_url', 'terms_url', 'terms_version', 'accessibility_url', 'support_url', 'docs_url'],
  };
  /** The sections, in the list's order; *Accounts without mail* only where the server has it. */
  const SECTIONS = [
    { id: 'passwords', code: 'app.signin_settings.passwords' },
    { id: 'second-factor', code: 'app.signin_settings.second_factor' },
    { id: 'sessions', code: 'app.signin_settings.sessions' },
    { id: 'ways', code: 'app.signin_settings.ways_and_providers' },
    { id: 'directory', code: 'app.signin_settings.directory' },
    { id: 'accounts-without-mail', code: 'app.signin_settings.managed_section' },
  ] as const;
  /** The tabs of the older addresses, by the section each became. */
  const TAB_SECTIONS: Readonly<Record<string, string>> = { passwords: 'passwords', sessions: 'sessions', ways: 'ways', directory: 'directory' };

  $effect(() => untrack(() => signInPolicy.open()));
  $effect(() => untrack(() => identityProvider.open()));
  $effect(() => untrack(() => ldapDirectory.open()));

  const policy = $derived(signInPolicy.policy);
  const reading = $derived(signInPolicy.state);
  const unreadable = $derived(
    signInPolicy.failure ? renderProblem(signInPolicy.failure, messages) : undefined,
  );

  const sections = $derived(SECTIONS.filter((each) => each.id !== 'accounts-without-mail' || policy?.managed_accounts !== undefined));
  const isKnown = (id: string | undefined): boolean => id !== undefined && SECTIONS.some((each) => each.id === id);
  const current = $derived(part === 'legal' ? 'legal' : isKnown(section) ? (section as string) : 'passwords');
  /** At the screen's own address the list - with the first section beside it where there is room. */
  const view = $derived<'list' | 'detail'>(part === 'legal' || isKnown(section) ? 'detail' : 'list');

  // An older address (`?tab=sessions`) lands on its section's own, replacing itself so the back
  // button does not return to it.
  $effect(() => {
    if (part !== 'sign-in' || section !== undefined || !tab) return;
    const target = TAB_SECTIONS[tab];
    if (target) untrack(() => onreplace?.(`${BASE}/${target}`));
  });

  /** What is being edited, filled from the read once so a save's re-read does not fight typing. */
  let draft = $state<Record<string, unknown>>({});
  /**
   * What the read said, kept so a save can send what moved and nothing else.
   *
   * Two reasons, and either alone would be enough. A switch the installation locked is refused
   * when it is *changed*, and a form that posts all nineteen rows changes every one of them as far
   * as the wire is concerned - so moving one switch on a screen where another is locked answered a
   * refusal naming a switch nobody touched, and wrote neither. And a value sent is a value this
   * workspace has now decided for itself: posting the whole form would pin all eighteen to
   * whatever they happened to be, which is not what somebody who moved one of them asked for.
   */
  let baseline = $state<Record<string, unknown>>({});
  let filled = false;
  let confirming = $state(false);
  let isFileOpen = $state(false);
  let form = $state<HTMLFormElement | undefined>(undefined);

  $effect(() => {
    if (!policy || filled) return;
    filled = true;
    fill();
  });

  /**
   * Fills the form from the rule the server answered.
   *
   * Called once on arrival and again after every save, because the answer is not always the
   * request: the product's own bounds clamp a value past them - a minimum age of twenty-six hours
   * is held at twenty-four - and a form that kept showing twenty-six would lie about what is in
   * force and send it again on the next save.
   */
  function fill(): void {
    if (!policy) return;
    draft = {
      min_length: policy.password.min_length.value,
      min_lowercase: policy.password.min_lowercase.value,
      min_uppercase: policy.password.min_uppercase.value,
      min_digits: policy.password.min_digits.value,
      min_symbols: policy.password.min_symbols.value,
      min_classes: policy.password.min_classes.value,
      max_repeat: policy.password.max_repeat.value,
      common_passwords: policy.password.common_passwords.value,
      context_words: policy.password.context_words.value,
      breach_check: policy.password.breach_check.value,
      max_age_days: policy.password.max_age_days.value,
      history_count: policy.password.history_count.value,
      min_age_hours: policy.password.min_age_hours.value,
      mfa_required_for: policy.mfa_required_for.value,
      session_max_days: policy.session.max_days.value,
      session_idle_minutes: policy.session.idle_minutes.value,
      // The workspace's own link, empty where it set none: the installation's is said beside the
      // field rather than filled into it, so an administrator sees what applies before overriding
      // it and an untouched form sends nothing (UC-ID-18 check 3).
      imprint_url: ownLink(policy.legal.imprint_url),
      privacy_url: ownLink(policy.legal.privacy_url),
      terms_url: ownLink(policy.legal.terms_url),
      accessibility_url: ownLink(policy.legal.accessibility_url),
      // The help links, absent from a server that predates them.
      ...(policy.legal.support_url ? { support_url: ownLink(policy.legal.support_url) } : {}),
      ...(policy.legal.docs_url ? { docs_url: ownLink(policy.legal.docs_url) } : {}),
      // The terms' version, own where the workspace's own terms carry one (UC-ID-19).
      ...(policy.legal.terms_version ? { terms_version: ownLink(policy.legal.terms_version) } : {}),
      // Absent from a server that predates it, and then not a row this form sends.
      ...(policy.managed_accounts ? { managed_accounts: policy.managed_accounts.value } : {}),
    };
    baseline = { ...draft };
  }

  /** A link as this workspace set it, or empty where it inherits the installation's or nobody's. */
  function ownLink(setting: Setting<string>): string {
    return setting.source === 'WORKSPACE' ? setting.value : '';
  }

  /** The rule behind a key of `draft`, for its section's state. */
  function settingOf(key: string): Setting<unknown> | undefined {
    if (!policy) return undefined;
    if (key === 'mfa_required_for') return policy.mfa_required_for;
    if (key === 'session_max_days') return policy.session.max_days;
    if (key === 'session_idle_minutes') return policy.session.idle_minutes;
    if (key === 'managed_accounts') return policy.managed_accounts;
    if (key in policy.password) return (policy.password as unknown as Record<string, Setting<unknown>>)[key];
    return (policy.legal as unknown as Record<string, Setting<unknown> | undefined>)[key];
  }

  /** How many of a scope's rules this workspace set, and how many a level above locked. */
  function tally(scope: string): { set: number; locked: number } {
    let set = 0;
    let locked = 0;
    for (const key of SCOPES[scope] ?? []) {
      const setting = settingOf(key);
      if (!setting) continue;
      if (setting.lock !== null) locked += 1;
      else if (setting.source === 'WORKSPACE') set += 1;
    }
    return { set, locked };
  }

  /** A section's state in the list, from what is read: never a guess before the read answers. */
  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    if (!policy) return {};
    if (id === 'ways') return { status: t('app.signin_settings.status_ways_on', { count: waysOn + (passkeyOn ? 1 : 0) }) };
    if (id === 'directory') {
      if (ldapDirectory.state.status !== 'ready' && !ldapDirectory.none) return {};
      return { status: ldapDirectory.directory?.enabled ? t('app.signin_settings.status_on') : t('app.signin_settings.status_off') };
    }
    if (id === 'accounts-without-mail') {
      return { status: policy.managed_accounts?.value ? t('app.signin_settings.status_on') : t('app.signin_settings.status_off') };
    }
    const { set, locked } = tally(id);
    if (set > 0) return { status: t('app.signin_settings.status_set', { count: set }), statusTone: 'set' };
    if (locked > 0) return { status: t('app.signin_settings.status_locked'), statusTone: 'locked' };
    return { status: t('app.signin_settings.status_default') };
  }

  const links = $derived<SectionLink[]>(
    part === 'legal'
      ? [{ id: 'legal', label: t('app.signin_settings.legal'), href: '/administration/legal', ...statusOf('legal') }]
      : sections.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}`, ...statusOf(each.id) })),
  );

  /** The head's one line on Sign-in: what is set here and what is locked above, over its rules. */
  const figureCounts = $derived.by(() => {
    return ['passwords', 'second-factor', 'sessions', 'accounts-without-mail'].map(tally).reduce((sum, each) => ({ set: sum.set + each.set, locked: sum.locked + each.locked }), { set: 0, locked: 0 });
  });

  /** The level above's value in a sentence, naming the plan where the plan decided it. */
  function aboveValue(source: PolicySource, value: string): string {
    return source === 'PLAN'
      ? t('app.signin_settings.plan_default', { value })
      : t('app.signin_settings.installation_default', { value });
  }

  /**
   * What applies above a link, said beside its field next to the source chip: nothing where it is
   * locked (the field shows the installation's link and the chip its lock), the installation's
   * beside the workspace's own where both are set, the installation's where this workspace set
   * none, or that nobody set one and the footer shows none (UC-ID-18 checks 1 to 3).
   */
  function linkOrigin(setting: Setting<string>, own: string): string {
    if (setting.lock !== null) return '';
    const inherited = setting.installation_source !== 'DEFAULT' && setting.installation !== '';
    if (own.trim() !== '') {
      return inherited ? aboveValue(setting.installation_source, setting.installation) : '';
    }
    if (!inherited) return t('app.signin_settings.link_none');
    return setting.installation_source === 'PLAN'
      ? t('app.signin_settings.link_inherited_plan', { value: setting.installation })
      : t('app.signin_settings.link_inherited', { value: setting.installation });
  }

  /**
   * The rows of a section that moved, in the shape the contract takes - the open one's where none
   * is named. Empty means there is nothing to save.
   */
  function moved(scope: string = current): Record<string, unknown> {
    const keys = SCOPES[scope] ?? [];
    const changes: Record<string, unknown> = {};
    for (const [key, value] of Object.entries(draft)) {
      if (keys.includes(key) && value !== baseline[key]) changes[key] = value;
    }
    return changes;
  }

  /** What moved in the other sections, kept through this section's save and its re-filling. */
  function movedElsewhere(): Record<string, unknown> {
    const changes: Record<string, unknown> = {};
    for (const scope of Object.keys(SCOPES)) if (scope !== current) Object.assign(changes, moved(scope));
    return changes;
  }

  const isChanged = (key: string): boolean => key in draft && draft[key] !== baseline[key];

  /** Why a switch cannot be touched here, or nothing where it can. */
  function lockedBecause(lock: LockOrigin): string | undefined {
    if (lock === 'INSTANCE') return t('app.signin_settings.locked_installation');
    if (lock === 'PLAN') return t('app.signin_settings.locked_plan');
    return undefined;
  }

  /**
   * Why a switch the installation requires cannot be turned off here: the one choice left is on.
   * Shown with its reason rather than hidden, because the rule is in force here all the same.
   */
  function requiredAbove(setting: Setting<boolean>): string | undefined {
    const locked = lockedBecause(setting.lock);
    if (locked) return locked;
    if (!setting.installation) return undefined;
    // The level above is the installation over Hubtask's default: where the installation decided
    // nothing, it is Hubtask's default that the workspace may not go below, and saying "the
    // installation" there would contradict the origin line beside it.
    if (setting.installation_source === 'DEFAULT') return t('app.signin_settings.required_default');
    return setting.installation_source === 'PLAN'
      ? t('app.signin_settings.required_plan')
      : t('app.signin_settings.required_above');
  }

  /**
   * The level above's value beside a rule this workspace set itself - "Installation: 12" - so an
   * administrator sees what applies before overriding it (UC-ID-12's story). Where the value in
   * force is the level above's, the control already shows it and the chip says whose it is.
   */
  function originOf<T>(setting: Setting<T>, shown: (value: T) => string): string {
    if (setting.lock !== null || setting.source !== 'WORKSPACE' || setting.installation_source === 'DEFAULT') return '';
    return aboveValue(setting.installation_source, shown(setting.installation));
  }

  /** A number as a reader says it: zero is the word for "off", not the digit. */
  const amount = (value: number): string => (value === 0 ? t('app.signin_settings.off') : String(value));
  const yesNo = (value: boolean): string => (value ? t('app.signin_settings.on') : t('app.signin_settings.off'));
  /** Whether the password, or passkeys, are among the ways in, as a reader says it. */
  const passwordWord = (methods: readonly string[]): string => yesNo(methods.includes('PASSWORD'));
  const passkeyWord = (methods: readonly string[]): string => yesNo(methods.includes('PASSKEY'));
  const mfaLabel = (value: string): string =>
    value === 'EVERYONE' ? t('app.signin_settings.mfa_everyone')
      : value === 'ADMINS' ? t('app.signin_settings.mfa_admins')
        : t('app.signin_settings.mfa_none');

  /** The refusal the server attached to this rule, if it attached one (check 8). */
  function refusalOf(key: string): string | undefined {
    return signInPolicy.problem?.fields.get(`/sign_in_policy/${key}`);
  }

  /** A refusal that names no rule, which is the one kind a banner is for. */
  const general = $derived(
    signInPolicy.problem && signInPolicy.problem.fields.size === 0 ? signInPolicy.problem : undefined,
  );

  /**
   * The choices for who needs a second factor, without the ones looser than the installation's
   * (check 5): under an installation that demands it of administrators, "Nobody" is not offered.
   */
  const RANK = { NONE: 0, ADMINS: 1, EVERYONE: 2 } as const;
  const mfaOptions = $derived(
    (['NONE', 'ADMINS', 'EVERYONE'] as const)
      .filter((value) => !policy || RANK[value] >= RANK[policy.mfa_required_for.installation])
      .map((value) => ({ value, label: mfaLabel(value) })),
  );

  /**
   * A field where zero means off and a smaller number is stricter - the repeat limit, the expiry,
   * the idle bound. Under an installation that set one, off is looser and so is a larger number.
   */
  function capped(setting: Setting<number>): { min?: number; max?: number } {
    return setting.installation > 0 ? { min: 1, max: setting.installation } : {};
  }

  /** Whether "empty is off" is a choice here: not where the installation set a value, because off is looser. */
  const mayBeOff = (setting: Setting<number>): boolean => setting.installation === 0;

  /** How many rules moved: the save bar's count, and whether it is there at all. */
  const changedCount = $derived(Object.keys(moved()).length);
  /** Every section's unsaved rules: what the leave guard holds, as a move between sections keeps them. */
  const unsaved = $derived(Object.keys(SCOPES).reduce((count, scope) => count + Object.keys(moved(scope)).length, 0));
  $effect(() => leaveGuard.hold(() => unsaved));

  async function save(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const changes = moved();
    // Nothing to send - Enter in an untouched field. A `PATCH` of nothing would ask for the
    // step-up on the way to doing nothing.
    if (Object.keys(changes).length === 0) return;
    const waiting = movedElsewhere();
    if (await signInPolicy.save(changes)) {
      // From the answer rather than from the draft: what is in force is the server's word, and a
      // value it bounded is a value this form has to show. What another section holds unsaved stays.
      fill();
      draft = { ...draft, ...waiting };
      toast.show({ text: t('app.signin_settings.saved'), tone: 'success' });
    }
  }

  /** This section's rules back as they were read; another section's stay as they are. */
  function discard(): void {
    const reverted = { ...draft };
    for (const key of SCOPES[current] ?? []) if (key in baseline) reverted[key] = baseline[key];
    draft = reverted;
  }

  function select(next: string): void {
    onreplace?.(`${BASE}/${next}`);
  }

  // The ways in, and how many of them are on. The last one on cannot be switched off.
  const providers = $derived(identityProvider.all);

  // Whether the password is open only as the fallback, as the sign-in card is told it. Started once
  // the manifest says the route is served, which may be after this screen first draws.
  $effect(() => {
    if (!signInRules.isServed) return;
    return untrack(() => signInRules.read());
  });
  const fallback = $derived(signInRules.rules?.password_fallback === true);
  /** Whether the plan leaves the workspace's own sign-in providers out. */
  const ownResting = $derived(includedHere('OWN_SIGN_IN_PROVIDERS') === false);
  // The installation operator's opening of the password (ADR-0078 §3), while it stands. It is the
  // fallback too, as the sign-in card sees it - but a different sentence: a person opened it, for a
  // reason and until a moment, rather than this workspace's settings leaving no way in.
  const opening = $derived(signInPolicy.opening);
  const passwordOn = $derived(policy?.methods.value.includes('PASSWORD') ?? false);
  // Passkeys are a way in beside the others and never the last one (identity.md §23.4), so they do
  // not count in `waysOn`.
  const passkeyOn = $derived(policy?.methods.value.includes('PASSKEY') ?? false);
  const waysOn = $derived((passwordOn ? 1 : 0) + providers.filter((one) => one.offered_here === true).length);
  let switching = $state<string | undefined>(undefined);
  /** Whether the reader may switch the password and passkeys (the rule's) and the providers (`:offer`'s). */
  const mayRule = $derived(mayHere('STRUCTURE').status === 'permitted');
  const mayOffer = $derived(mayHere('MANAGE_MEMBERS').status === 'permitted');
  /** Whether the reader may read the page's part of the configuration document. */
  const mayReadFile = $derived(holdsHere('STRUCTURE') === true || holdsHere('READ_CONFIGURATION') === true);
  /** The last refusal of a provider's switch, with the row it belongs to. */
  let wayFailure = $state<{ id: string; problem: ReturnType<typeof renderProblem> } | undefined>(undefined);

  /** Why a way that is on cannot be switched off: it is the only one left. */
  function lastWay(isOn: boolean): string | undefined {
    return isOn && waysOn <= 1 ? t('app.signin_settings.last_way') : undefined;
  }

  /** Why the password cannot be switched here, where it cannot. */
  function passwordFixed(): string | undefined {
    if (!policy) return undefined;
    const locked = lockedBecause(policy.methods.lock);
    if (locked) return locked;
    if (!passwordOn && !policy.methods.installation.includes('PASSWORD')) return t('app.signin_settings.not_offered_above');
    return lastWay(passwordOn);
  }

  /**
   * The password on or off, through the rule's `methods`. The providers stay a way in as far as the
   * rule goes wherever the installation permits them, and each provider's own switch decides it.
   */
  async function setPassword(on: boolean): Promise<void> {
    if (!policy) return;
    const methods = [
      ...(on ? ['PASSWORD'] : []),
      ...(policy.methods.installation.includes('OIDC') ? ['OIDC'] : []),
      // Kept as it is: switching the password must not switch passkeys off with it.
      ...(passkeyOn ? ['PASSKEY'] : []),
    ];
    await switchMethods('password', methods);
  }

  /**
   * One way's switch, saved at once with its own busy state and its own refusal at its row - never
   * the rules' save bar's (UC-ID-12 check 8).
   */
  async function switchMethods(id: 'password' | 'passkey', methods: readonly string[]): Promise<void> {
    switching = id;
    wayFailure = undefined;
    const problem = await signInPolicy.saveAtOnce({ methods });
    switching = undefined;
    if (problem) wayFailure = { id, problem };
  }

  /** Why passkeys cannot be switched here, where they cannot. */
  function passkeyFixed(): string | undefined {
    if (!policy) return undefined;
    const locked = lockedBecause(policy.methods.lock);
    if (locked) return locked;
    if (!passkeyOn && !policy.methods.installation.includes('PASSKEY')) return t('app.signin_settings.not_offered_above');
    return undefined;
  }

  /** Signing in with a passkey alone on or off, through the rule's `methods`. */
  async function setPasskey(on: boolean): Promise<void> {
    if (!policy) return;
    const methods = [
      ...(passwordOn ? ['PASSWORD'] : []),
      ...(policy.methods.installation.includes('OIDC') ? ['OIDC'] : []),
      ...(on ? ['PASSKEY'] : []),
    ];
    await switchMethods('passkey', methods);
  }

  /** A provider on or off as a way in here: `:offer`, the one verb for both kinds of row. */
  async function setProvider(id: string, on: boolean): Promise<void> {
    switching = id;
    wayFailure = undefined;
    try {
      await identityProvider.offer(id, on);
    } catch (cause) {
      if (cause instanceof TransportError) wayFailure = { id, problem: renderProblem(cause, messages) };
    } finally {
      switching = undefined;
    }
  }

  async function rotate(): Promise<void> {
    confirming = false;
    if (await signInPolicy.requireNewPasswords()) announcer.say(t('app.signin_settings.rotated'));
  }

  const title = $derived(part === 'legal' ? t('app.signin_settings.legal') : t('app.signin_settings.title'));
  $effect(() => page.entitle(title));

  /**
   * The head's ⋯ (page-system §7.3): the destructive act in a group of its own - asking everybody
   * for a new password is a moment set for every member, behind a Dialog S, only for whoever may
   * shape the rules.
   */
  const menuItems = $derived(part === 'sign-in' && mayRule && policy ? [{ id: 'rotate', label: t('app.signin_settings.rotate_more'), isDestructive: true }] : []);
</script>

{#snippet origin(setting: { readonly source: string; readonly lock: string | null }, shown: string, above: string)}
  <!-- Where the value comes from, beside the rule's name: the one chip every administration
       screen draws (UC-WRK-32 check 5), and the level above's value where this workspace set its
       own. -->
  <span class="origin" data-origin>
    <ValueSource source={setting.source} lock={setting.lock} value={shown} />
    {#if above}<span class="above">{above}</span>{/if}
  </span>
{/snippet}

{#snippet countRow(key: string, setting: Setting<number>, label: string, hint?: string, given?: { min?: number; max?: number }, emptyIsOff?: boolean, groupLabel?: string)}
  {@const bounds = given ?? { min: setting.installation }}
  <div class="rule" data-rule>
    <SettingRow {label} help={hint} {groupLabel} fieldWidth="xs" isChanged={isChanged(key)} isRequired={emptyIsOff && !mayBeOff(setting)}>
      {#snippet source()}{@render origin(setting, amount(setting.value), originOf(setting, amount))}{/snippet}
      {#snippet control(nameId)}
        <Input
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          type="number"
          min={bounds.min}
          max={bounds.max}
          isRequired={emptyIsOff && !mayBeOff(setting)}
          value={emptyIsOff && !draft[key] ? '' : String(draft[key] ?? '')}
          oninput={(event) => {
            const raw = (event.currentTarget as HTMLInputElement).value;
            draft[key] = raw === '' ? 0 : Number(raw);
          }}
          disabledReason={lockedBecause(setting.lock)}
          error={refusalOf(key)}
        />
      {/snippet}
    </SettingRow>
  </div>
{/snippet}

{#snippet flagRow(key: string, setting: Setting<boolean>, label: string, hint: string, groupLabel?: string)}
  <div class="rule" data-rule>
    <SettingRow {label} help={hint} {groupLabel} fieldWidth="sm" isChanged={isChanged(key)}>
      {#snippet source()}{@render origin(setting, yesNo(setting.value), originOf(setting, yesNo))}{/snippet}
      {#snippet control(nameId)}
        <Switch
          {label}
          isLabelHidden
          aria-labelledby={nameId}
          checked={Boolean(draft[key])}
          onchange={(event) => (draft[key] = (event.currentTarget as HTMLInputElement).checked)}
          disabledReason={requiredAbove(setting)}
        />
        {#if refusalOf(key)}<p class="refusal" role="alert">{refusalOf(key)}</p>{/if}
      {/snippet}
    </SettingRow>
  </div>
{/snippet}

{#snippet linkRow(key: 'imprint_url' | 'privacy_url' | 'terms_url' | 'accessibility_url' | 'support_url' | 'docs_url', label: string, hint?: string)}
  {@const setting = policy?.legal[key]}
  {#if setting}
    {@const locked = setting.lock !== null}
    <div class="rule" data-rule>
      <SettingRow {label} help={hint} fieldWidth="lg" isChanged={isChanged(key)}>
        {#snippet source()}{@render origin(setting, setting.value || t('app.source.nothing'), linkOrigin(setting, String(draft[key] ?? '')))}{/snippet}
        {#snippet control(nameId)}
          <Input
            {label}
            isLabelHidden
            aria-labelledby={nameId}
            type="url"
            value={locked ? setting.value : String(draft[key] ?? '')}
            oninput={(event) => (draft[key] = (event.currentTarget as HTMLInputElement).value)}
            disabledReason={lockedBecause(setting.lock)}
            error={refusalOf(key)}
          />
        {/snippet}
      </SettingRow>
    </div>
  {/if}
{/snippet}

{#snippet wayRow(id: string, label: string, help: string | undefined, isInert: boolean, control: Snippet<[string]>, source?: Snippet, reach?: string)}
  <li class="rule way" data-rule inert={isInert}>
    <SettingRow {label} {help} fieldWidth="sm" {control} {source} />
    {#if reach}<p class="reach" data-reach>{reach}</p>{/if}
    {#if wayFailure && wayFailure.id === id}<p class="refusal way-refusal" role="alert">{wayFailure.problem.message}</p>{/if}
  </li>
{/snippet}

{#snippet saveBar()}
  <WriteOnly>
    <SaveBar
      label={t('app.save_bar.label')}
      changesLabel={changedCount > 0 ? t('app.save_bar.changes', { count: String(changedCount) }) : undefined}
      discardLabel={t('app.save_bar.discard')}
      saveLabel={t('app.workspace.save')}
      isSaving={signInPolicy.isWorking}
      savingLabel={t('app.workspace.saving')}
      onDiscard={discard}
      onSave={() => form?.requestSubmit()}
    />
  </WriteOnly>
{/snippet}

{#snippet figure()}
  <span class="figure">
    {#if part === 'sign-in'}{t('app.signin_settings.figure', figureCounts)}{/if}
    {#if policy?.plan}
      <!-- The plan a workspace is on, so "Set by your plan" refers to something a person can
           name to their provider. A note, not an offer: no price, no other plan. -->
      <span data-testid="sign-in-plan"><Badge tone="info" size="sm">{t('app.signin_settings.plan', { name: policy.plan })}</Badge></span>
    {/if}
  </span>
{/snippet}

<SettingsLayout
  head={{
    title,
    breadcrumb: {
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: part, label: title },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    },
    secondary: mayReadFile ? [{ label: t('app.settings.view_as_file'), isQuiet: true, onclick: () => (isFileOpen = true) }] : [],
    menu: menuItems.length > 0 ? { label: t('app.page.more_for', { name: title }), items: menuItems, onselect: () => (confirming = true) } : undefined,
    figure: policy && (part === 'sign-in' || policy.plan) ? figure : undefined,
  }}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current, onselect: part === 'legal' ? undefined : select }}
  {view}
  back={part === 'legal'
    ? { label: t('app.admin.title'), onback: () => onnavigate?.('/administration') }
    : { label: t('app.signin_settings.title'), onback: () => onreplace?.(BASE) }}
>
  {#if unreadable && !policy}
    <!-- Never a spinner for a read that failed (check 8, P-11): the server's sentence, and a way to
         ask again. -->
    <FailureState problem={unreadable} onRetry={() => signInPolicy.retry()} />
  {:else if reading.status === 'ready' && !policy}
    <!-- Read, and the workspace answered no rule: an installation wired without the level above,
         where there is nothing to set. Said, rather than drawn as eighteen rows nobody can save. -->
    <Banner tone="info">{t('app.signin_settings.unavailable')}</Banner>
  {:else if !policy}
    <p class="quiet">
      <Spinner label={t('app.workspace.reading')} />
      <span>{t('app.workspace.reading')}</span>
    </p>
  {:else}
    {#if general}
      <FailureNote problem={general}>
        {#if general.reference}{general.reference}{/if}
      </FailureNote>
    {/if}
    {@render panel()}
  {/if}
</SettingsLayout>

{#snippet panel()}
  {#if policy}
    {#if current === 'ways'}
      {@render ways()}
    {:else if current === 'directory'}
      <!-- The LDAP directory the people are synchronised from (UC-ID-31): its own saves. -->
      <LdapDirectoryPanel />
    {:else}
      <WorkspaceGate permission="STRUCTURE">
        <form bind:this={form} onsubmit={save} use:enterSubmits>
          {#if current === 'passwords'}
            <SettingsSection
              title={t('app.signin_settings.passwords')}
              description={t('app.signin_settings.passwords_lead')}
              status={statusOf('passwords').status}
              advanced={{ label: t('app.signin_settings.advanced'), description: t('app.signin_settings.advanced_hint'), rows: passwordRare }}
              save={saveBar}
            >
              {@render countRow('min_length', policy.password.min_length, t('app.signin_settings.min_length'), undefined, undefined, false, t('app.signin_settings.group_strength'))}
              {@render flagRow('common_passwords', policy.password.common_passwords, t('app.signin_settings.common_passwords'), t('app.signin_settings.adds_a_line'))}
              {@render flagRow('context_words', policy.password.context_words, t('app.signin_settings.context_words'), t('app.signin_settings.adds_a_line'))}
              {@render flagRow('breach_check', policy.password.breach_check, t('app.signin_settings.breach_check'), t('app.signin_settings.breach_hint'))}
              {@render countRow('max_age_days', policy.password.max_age_days, t('app.signin_settings.max_age_days'), mayBeOff(policy.password.max_age_days) ? t('app.signin_settings.max_age_hint') : undefined, capped(policy.password.max_age_days), true, t('app.signin_settings.group_change'))}
              {@render countRow('history_count', policy.password.history_count, t('app.signin_settings.history_count'), t('app.signin_settings.history_hint'))}
            </SettingsSection>
          {:else if current === 'second-factor'}
            <SettingsSection title={t('app.signin_settings.second_factor')} description={t('app.signin_settings.mfa_hint')} status={statusOf('second-factor').status} save={saveBar}>
              <div class="rule" data-rule>
                <SettingRow label={t('app.signin_settings.mfa_required_for')} fieldWidth="md" isChanged={isChanged('mfa_required_for')}>
                  {#snippet source()}{@render origin(policy.mfa_required_for, mfaLabel(policy.mfa_required_for.value), originOf(policy.mfa_required_for, mfaLabel))}{/snippet}
                  {#snippet control(nameId)}
                    <Select
                      label={t('app.signin_settings.mfa_required_for')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      value={String(draft['mfa_required_for'] ?? 'NONE')}
                      onchange={(event) => (draft['mfa_required_for'] = (event.currentTarget as HTMLSelectElement).value)}
                      options={mfaOptions}
                      disabledReason={lockedBecause(policy.mfa_required_for.lock)}
                      error={refusalOf('mfa_required_for')}
                    />
                  {/snippet}
                </SettingRow>
              </div>
            </SettingsSection>
          {:else if current === 'sessions'}
            <SettingsSection title={t('app.signin_settings.sessions')} status={statusOf('sessions').status} save={saveBar}>
              {@render countRow('session_max_days', policy.session.max_days, t('app.signin_settings.session_max_days'), undefined, { min: 1, max: policy.session.max_days.installation || undefined })}
              {@render countRow('session_idle_minutes', policy.session.idle_minutes, t('app.signin_settings.session_idle'), mayBeOff(policy.session.idle_minutes) ? t('app.signin_settings.session_idle_hint') : undefined, capped(policy.session.idle_minutes), true)}
            </SettingsSection>
          {:else if current === 'accounts-without-mail' && policy.managed_accounts}
            {@const managed = policy.managed_accounts}
            <SettingsSection title={t('app.signin_settings.managed_title')} status={statusOf('accounts-without-mail').status} save={saveBar}>
              <div class="rule" data-rule>
                <SettingRow label={t('app.signin_settings.managed_accounts')} help={t('app.signin_settings.managed_accounts_hint')} fieldWidth="sm" isChanged={isChanged('managed_accounts')}>
                  {#snippet source()}{@render origin(managed, yesNo(managed.value), originOf(managed, yesNo))}{/snippet}
                  {#snippet control(nameId)}
                    <!-- Off is the stricter value here, the other way round from the password's
                         switches: what the level above switched off cannot be switched on. -->
                    <Switch
                      label={t('app.signin_settings.managed_accounts')}
                      isLabelHidden
                      aria-labelledby={nameId}
                      checked={Boolean(draft.managed_accounts)}
                      onchange={(event) => (draft.managed_accounts = (event.currentTarget as HTMLInputElement).checked)}
                      disabledReason={lockedBecause(managed.lock) ??
                        (managed.installation ? undefined : t('app.signin_settings.managed_off_above'))}
                    />
                    {#if refusalOf('managed_accounts')}<p class="refusal" role="alert">{refusalOf('managed_accounts')}</p>{/if}
                  {/snippet}
                </SettingRow>
              </div>
            </SettingsSection>
          {:else if current === 'legal'}
            {@render legal()}
          {/if}
        </form>
      </WorkspaceGate>
    {/if}
  {/if}
{/snippet}

{#snippet passwordRare()}
  {#if policy}
    {#each [['min_lowercase', policy.password.min_lowercase], ['min_uppercase', policy.password.min_uppercase], ['min_digits', policy.password.min_digits], ['min_symbols', policy.password.min_symbols]] as const as [key, setting], index (key)}
      {@render countRow(key, setting, t(`app.signin_settings.${key}`), index === 0 ? t('app.signin_settings.classes_hint') : undefined, undefined, false, index === 0 ? t('app.signin_settings.classes') : undefined)}
    {/each}
    {@render countRow('min_classes', policy.password.min_classes, t('app.signin_settings.min_classes'))}
    {@render countRow('max_repeat', policy.password.max_repeat, t('app.signin_settings.max_repeat'), mayBeOff(policy.password.max_repeat) ? t('app.signin_settings.max_repeat_hint') : undefined, capped(policy.password.max_repeat), true)}
    <!-- The eighteenth rule (check 7). -->
    {@render countRow('min_age_hours', policy.password.min_age_hours, t('app.signin_settings.min_age_hours'), t('app.signin_settings.min_age_hint'))}
  {/if}
{/snippet}

{#snippet legal()}
  {#if policy}
    {@const hasHelp = policy.legal.support_url !== undefined || policy.legal.docs_url !== undefined}
    <SettingsSection title={t('app.signin_settings.legal')} description={t('app.signin_settings.legal_hint')} status={statusOf('legal').status} save={hasHelp ? undefined : saveBar}>
      {@render linkRow('imprint_url', t('app.legal.imprint'))}
      {@render linkRow('privacy_url', t('app.legal.privacy'))}
      {@render linkRow('terms_url', t('app.legal.terms'))}
      {#if policy.legal.terms_version}
        {@const version = policy.legal.terms_version}
        <!-- The terms' version, beside its link (UC-ID-19): a change of either asks every person to
             agree again at their next sign-in, which the help says before it is saved. Under the
             link's lock, from the link's level. -->
        <div class="rule" data-rule>
          <SettingRow
            label={t('app.signin_settings.terms_version')}
            help={t('app.signin_settings.terms_version_hint')}
            fieldWidth="sm"
            isChanged={isChanged('terms_version')}
          >
            {#snippet source()}{@render origin(version, version.value || t('app.source.nothing'), linkOrigin(version, String(draft['terms_version'] ?? '')))}{/snippet}
            {#snippet control(nameId)}
              <Input
                label={t('app.signin_settings.terms_version')}
                isLabelHidden
                aria-labelledby={nameId}
                value={version.lock !== null ? version.value : String(draft['terms_version'] ?? '')}
                oninput={(event) => (draft['terms_version'] = (event.currentTarget as HTMLInputElement).value)}
                disabledReason={lockedBecause(version.lock)}
                error={refusalOf('terms_version')}
                maxlength={64}
              />
            {/snippet}
          </SettingRow>
        </div>
      {/if}
      <!-- The fourth link, which the sign-in footer shows: without this row a workspace could only
           have the installation's, whatever its own statement said. -->
      {@render linkRow('accessibility_url', t('app.legal.accessibility'))}
    </SettingsSection>
    {#if hasHelp}
      <!-- Not in the footer: Help shows them (user-experience.md §5.8). Saved with the links above,
           by one bar - the page has one save model. -->
      <div class="help-links" data-help-links>
        <SettingsSection title={t('app.signin_settings.help_links')} description={t('app.signin_settings.help_links_hint')} save={saveBar}>
          {@render linkRow('support_url', t('app.help.support_contact'), t('app.signin_settings.support_url_hint'))}
          {@render linkRow('docs_url', t('app.help.documentation'))}
        </SettingsSection>
      </div>
    {/if}
  {/if}
{/snippet}

{#snippet ways()}
  {#if policy}
    <!-- The ways in save the moment a switch moves, so they stand outside the rules' form and its
         save: a form holds what waits for its save, and nothing else. -->
    {#if opening}
      <Banner tone="warning" title={t('app.signin_settings.opening_title')}>
        <!-- Who asked and why come only to a reader of the configuration; without them the
             sentence says the rest rather than printing an empty attribution. -->
        {opening.requester && opening.reason
          ? t('app.signin_settings.opening_body', {
              until: formatDateTime(opening.until, messages.locale),
              requester: opening.requester,
              reason: opening.reason,
            })
          : t('app.signin_settings.opening_body_unattributed', {
              until: formatDateTime(opening.until, messages.locale),
            })}
      </Banner>
    {:else if fallback}
      <Banner tone="warning" title={t('app.signin_settings.fallback_title')}>
        {t('app.signin_settings.fallback_no_way_in')}
      </Banner>
    {/if}
    <!-- One list, one switch each (UC-ID-12 check 6), each row under its own permission: the
         password and passkeys are the rule's (STRUCTURE), a provider goes through `:offer`, which
         asks MANAGE_MEMBERS. A row the reader may not switch stays, inert, to be read; the notice
         says why. -->
    <WorkspaceRefusal permission="MANAGE_MEMBERS" />
    <SettingsSection title={t('app.signin_settings.ways')} description={t('app.signin_settings.ways_hint')} status={statusOf('ways').status}>
      <ul class="ways" aria-label={t('app.signin_settings.ways')}>
        {#snippet passwordSwitch(nameId: string)}
          <Switch
            label={t('app.signin_settings.way_password')}
            isLabelHidden
            aria-labelledby={nameId}
            checked={passwordOn}
            onchange={(event) => void setPassword((event.currentTarget as HTMLInputElement).checked)}
            isBusy={switching === 'password'}
            busyLabel={t('app.signin_settings.switching')}
            disabledReason={passwordFixed()}
          />
        {/snippet}
        {#snippet passwordSource()}
          {@render origin(policy.methods, passwordWord(policy.methods.value), originOf(policy.methods, passwordWord))}
        {/snippet}
        {@render wayRow(
          'password',
          t('app.signin_settings.way_password'),
          t('app.signin_settings.way_password_hint'),
          !mayRule,
          passwordSwitch,
          passwordSource,
          // Only where it can go off: as the last way in, or under the installation's lock, the
          // switch is held and the count would describe a choice nobody has.
          passwordOn && passwordFixed() === undefined && (identityProvider.withoutProvider ?? 0) > 0
            ? t('app.signin_settings.password_off_reach', { count: identityProvider.withoutProvider ?? 0 })
            : undefined,
        )}
        {#snippet passkeySwitch(nameId: string)}
          <Switch
            label={t('app.signin_settings.way_passkey')}
            isLabelHidden
            aria-labelledby={nameId}
            checked={passkeyOn}
            onchange={(event) => void setPasskey((event.currentTarget as HTMLInputElement).checked)}
            isBusy={switching === 'passkey'}
            busyLabel={t('app.signin_settings.switching')}
            disabledReason={passkeyFixed()}
          />
        {/snippet}
        {#snippet passkeySource()}
          {@render origin(policy.methods, passkeyWord(policy.methods.value), originOf(policy.methods, passkeyWord))}
        {/snippet}
        {@render wayRow('passkey', t('app.signin_settings.way_passkey'), t('app.signin_settings.way_passkey_hint'), !mayRule, passkeySwitch, passkeySource)}
        {#each providers as one (one.id)}
          {@const mandated = one.installation_lock === 'TURNS_ON'}
          {@const mandatedByPlan = mandated && policy.methods.lock === 'PLAN'}
          {@const withdrawn = one.scope === 'installation' && withdrawalPhase(one) === 'withdrawn'}
          {@const resting = one.scope === 'workspace' && ownResting}
          {#snippet providerControl(nameId: string)}
            {#if withdrawn}
              <!-- Switched off where it is offered, or withdrawn from its date: not a choice this
                   workspace has, so no switch - a word saying so. -->
              <Badge tone="warning">{t('app.signin_settings.way_off_above')}</Badge>
            {:else if resting && one.enabled}
              <!-- The plan leaves the workspace's own providers out: this one signs nobody in and
                   is not switched on here (multi-tenancy.md §4.2). Switching it off stays, so that
                   it stays off when the plan includes it again; nothing else is offered. -->
              <Switch
                label={one.display_name}
                isLabelHidden
                aria-labelledby={nameId}
                checked
                onchange={(event) => { if (!(event.currentTarget as HTMLInputElement).checked) void setProvider(one.id, false); }}
                isBusy={switching === one.id}
                busyLabel={t('app.signin_settings.switching')}
              />
            {:else if resting}
              <!-- Resting and off: a word, no switch - switching it on is refused. -->
              <Badge tone="neutral">{t('app.plan.not_included')}</Badge>
            {:else}
              <Switch
                label={one.display_name}
                isLabelHidden
                aria-labelledby={nameId}
                checked={one.offered_here === true}
                onchange={(event) => void setProvider(one.id, (event.currentTarget as HTMLInputElement).checked)}
                isBusy={switching === one.id}
                busyLabel={t('app.signin_settings.switching')}
                disabledReason={mandated ? (lockedBecause(policy.methods.lock) ?? t('app.signin_settings.locked_installation')) : lastWay(one.offered_here === true)}
              />
            {/if}
          {/snippet}
          {#snippet providerSource()}<ProviderMark kind={one.kind} name={one.display_name} />{/snippet}
          {@render wayRow(
            one.id,
            one.display_name,
            withdrawn
              ? undefined
              : resting
                ? (one.enabled ? t('app.plan.provider_resting_hint') : undefined)
                : [
                    mandated
                      ? t(mandatedByPlan ? 'app.signin_settings.way_mandated_plan' : 'app.signin_settings.way_mandated')
                      : one.scope === 'installation' ? t('app.signin_settings.way_offered') : t('app.signin_settings.way_own'),
                    // Said under the switch it is about, while there is still time to act.
                    one.scope === 'installation' && withdrawalPhase(one) === 'withdrawing'
                      ? t(one.offered_here ? 'app.signin_settings.way_withdrawing' : 'app.signin_settings.way_withdrawing_off', { date: formatDateTime(one.withdraw_at ?? '', messages.locale) })
                      : undefined,
                  ].filter(Boolean).join(' '),
            !mayOffer,
            providerControl,
            providerSource,
          )}
        {/each}
      </ul>
    </SettingsSection>

    <!-- The providers' configuration, under the switches that turn them on here: a table whose row
         opens the provider's own page. -->
    <ProvidersTable onopen={(path) => onnavigate?.(path)} />
  {/if}
{/snippet}

<ViewSectionsAsFile {...pagePart(part === 'legal' ? '/administration/legal' : BASE)} bind:isOpen={isFileOpen} isTriggerHidden />

<Dialog
  size="s"
  bind:isOpen={confirming}
  title={t('app.signin_settings.rotate_title')}
  dismissLabel={t('app.dismiss')}
  onClose={() => (confirming = false)}
  primary={{ label: t('app.signin_settings.rotate'), tone: 'danger', onclick: () => void rotate() }}
>
  <p class="body">{t('app.signin_settings.rotate_body')}</p>
  <p class="body">{t('app.signin_settings.rotate_confirm')}</p>
  {#snippet actions()}
    <Button tone="secondary" onclick={() => (confirming = false)}>{t('app.dismiss')}</Button>
  {/snippet}
</Dialog>

<style>
  form { margin: 0; display: flex; flex-direction: column; gap: var(--sp-300); }

  .body { margin: 0; }
  .figure { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }

  .quiet { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  /* Each rule is wrapped so a test and a reader find its control, its chip and its reason
     together; the hairline the design system draws between two rows is drawn here between two
     wrapped ones, since its own rule sees siblings. */
  .rule + .rule > :global(.setting-row > .setting-line:first-child) { border-block-start: var(--bw-hairline) solid var(--border-subtle); }

  .origin { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); }
  .above { color: var(--text-subtle); font-size: var(--fs-075); font-weight: var(--fw-regular); }

  .refusal {
    margin: var(--sp-050) 0 0;
    color: var(--text-danger);
    font-size: var(--fs-075);
  }
  .way-refusal,
  .reach { margin: 0; padding: 0 var(--sp-250) var(--sp-150); font-size: var(--fs-075); }
  .reach { color: var(--text-secondary); }

  .ways { margin: 0; padding: 0; list-style: none; }
  .help-links { display: contents; }
</style>
