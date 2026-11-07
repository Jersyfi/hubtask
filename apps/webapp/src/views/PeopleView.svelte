<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Who holds a role at the workspace itself.
  //
  // **"The workspace's people" means "who holds a membership at it", and that is a limit of the
  // contract rather than a choice.** There is no `GET /accounts`: the contract lists no accounts
  // at all — only `/accounts/me`, `/accounts/{id}` and the invitation — so the only way to a list
  // of people is `GET /memberships?scope_type=TENANT`. An account with no membership anywhere is
  // invisible here, which is why the invitation below grants a role in the same breath.
  //
  // **This is the workspace half; a container has its own.** Same store, same grant and revoke,
  // one scope higher — and `people.svelte.ts` carries the step-up for both.
  //
  // **Read-only rather than absent where the write is refused.** Reading the memberships needs
  // `READ`; changing them needs `MANAGE_MEMBERS`. Somebody who holds the first and not the second
  // sees the people and no controls — a screen that hid itself would leave them unable to tell who
  // is in their own workspace.

  import { tick, untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';

  import {
    Badge,
    Banner,
    Button,
    DataTable,
    Dialog,
    EmptyState,
    FilterBar,
    Input,
    ListPage,
    OneTimeSecret,
    Radio,
    Select,
    Spinner,
    Stack,
    type DataColumn,
    type DataRowMenu,
    type Filter,
    type MenuItem,
  } from '@hubtask/design-system/components';
  import type { InvitationDelivery, MembershipRole, SignInDetail } from '@hubtask/sync-engine';

  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { actor } from '../lib/data/account.svelte.ts';
  import { viewAs } from '../lib/data/viewas.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { endsTheLastOwner, grantableOf, mayIssueWayIn, mayResetFactor, mayRevoke, ownershipOf, type Holder } from '../lib/data/people.ts';
  import RevokeDialog from '../lib/people/RevokeDialog.svelte';
  import ManagedAccountDialog from '../lib/people/ManagedAccountDialog.svelte';
  import DiscardDialog from '../lib/page/DiscardDialog.svelte';
  import { DiscardGuard } from '../lib/page/discard.svelte.ts';
  import { matchesChoice, matchesSearch } from '../lib/page/list.ts';
  import { toast } from '../lib/frame/toasts.svelte.ts';
  import AddressDialog from '../lib/people/AddressDialog.svelte';
  import RenewDialog from '../lib/people/RenewDialog.svelte';
  import ResetFactorDialog from '../lib/people/ResetFactorDialog.svelte';
  import StartPasswordResult from '../lib/people/StartPasswordResult.svelte';
  import { signInPolicy } from '../lib/data/signinpolicy.svelte.ts';
  import { signInRules } from '../lib/data/signinrules.svelte.ts';
  import { collectionChoice, deliveryLine, invitePlaceOf, inviteRoleChoices, inviteWay } from '../lib/people/invitation.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { roles as workspaceRoles } from '../lib/data/roles.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import { check, focusFirst, isComplete, type Missing } from '../lib/forms/missing.ts';
  import { marked } from '../lib/forms/press.ts';

  /** What *Invite* or *Grant* found missing when pressed, marked at the field (UC-WRK-32 check 3). */
  let missing = $state<Missing>({});
  import { page } from '../lib/frame/page.svelte.ts';
  import { sorting } from '../lib/sorting.svelte.ts';
  import GroupsView from './GroupsView.svelte';
  import PeopleStrip from '../lib/people/PeopleStrip.svelte';
  import { PEOPLE_TABS } from '../lib/people/peoplepage.ts';
  import PermissionsView from './PermissionsView.svelte';
  import RolesView from './RolesView.svelte';
  import { holdsHere } from '../lib/data/capability.svelte.ts';

  interface Props {
    /** The tab the address names (`?tab=`); the people where it names none or an unknown one. */
    tab?: string;
    /** The person *view as* chose (`?person=`), for the permissions tab. */
    person?: string;
    /** Replaces the address when the tab changes, so a reload or a link keeps it. */
    onnavigate?: (path: string) => void;
    /** Moves to another screen - a person's record - keeping this one in the history. */
    onopen?: (path: string) => void;
  }

  let { tab, person, onnavigate, onopen }: Props = $props();

  const current = $derived(PEOPLE_TABS.some((each) => each.id === tab) ? (tab as string) : 'people');

  /** Whether the reader may ask what somebody else holds: what `GET /access/explain` asks. */
  const mayViewAs = $derived(holdsHere('MANAGE_MEMBERS') === true || holdsHere('READ_CONFIGURATION') === true);
  /** Whether the reader may look at somebody's screens (UC-ID-29): what `POST /auth/sessions:view-as` asks. */
  const mayViewScreens = $derived(holdsHere('MANAGE_MEMBERS') === true && !viewAs.running);
  const mayManage = $derived(holdsHere('MANAGE_MEMBERS') === true);
  let viewingScreens = $state<string | undefined>(undefined);

  async function viewScreens(accountId: string): Promise<void> {
    viewingScreens = accountId;
    failure = undefined;
    try {
      await viewAs.start(accountId);
    } catch (error) {
      failure = renderProblem(error as never, messages).message;
      viewingScreens = undefined;
    }
  }

  // The three ways in - an invitation, an account without an address, a role for somebody already
  // here - each a Dialog M (page-system §5.1: two to six fields in one group), opened from the
  // head's one primary, so the screen is the list first and a form only while somebody is added.
  let isInviting = $state(false);
  let isGranting = $state(false);
  let isAddingManaged = $state(false);
  let inviteForm = $state<HTMLFormElement | undefined>(undefined);

  function open(next: string): void {
    missing = {};
    inviteRefusal = undefined;
    grantRefusal = undefined;
    isInviting = next === 'invite';
    isGranting = next === 'grant';
    isAddingManaged = next === 'managed';
  }

  function close(): void {
    missing = {};
    isInviting = false;
    isGranting = false;
    inviteRefusal = undefined;
    grantRefusal = undefined;
  }

  const inviteGuard = new DiscardGuard(() => inviteEmail.trim() !== '' || inviteName.trim() !== '' || inviteRole !== '', () => {
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
    inviteWhere = WORKSPACE_PLACE;
    close();
  });
  const grantGuard = new DiscardGuard(() => chosenSubject !== '' || chosenRole !== '', () => {
    chosenSubject = '';
    chosenRole = '';
    close();
  });

  /** The workspace itself. One scope, named once, so the read and the writes cannot disagree. */
  const TENANT = { scopeType: 'TENANT' } as const;
  /** The invitation's choice that means the whole workspace rather than a hub. */
  const WORKSPACE_PLACE = '';

  let failure = $state<string | undefined>(undefined);
  let isWriting = $state(false);
  let chosenRole = $state('');
  let chosenSubject = $state('');
  let inviteEmail = $state('');
  let inviteName = $state('');
  let inviteRole = $state('');
  /**
   * Where the invited person is given their role: the whole workspace, one hub or one collection
   * (UC-WRK-21 check 5, UC-ID-15) - a child, a guest or a requester limited to it from the start
   * rather than given more first. A single entry is chosen from that entry's own *People*.
   */
  let inviteWhere = $state(WORKSPACE_PLACE);
  /** Why the address was refused, said at the address field (UC-ID-14 check 6, WCAG 3.3.1). */
  let inviteRefusal = $state<string | undefined>(undefined);
  /** Why the role could not be given after the invitation went out, said at the form. */
  let grantRefusal = $state<string | undefined>(undefined);
  let inviteAddressField = $state<HTMLElement | undefined>(undefined);
  /**
   * The one answer that carried an invitation's link - an installation that sends no mail - until
   * the reader dismisses it. Held here only: the link is a credential, and nothing keeps it.
   */
  let handedOver = $state<{ readonly who: string; readonly link: string } | undefined>(undefined);

  /** Whether an invitation goes out by mail or as a link, as the manifest says before the press. */
  const way = $derived(inviteWay(manifest.value?.features));

  $effect(() =>
    untrack(() => {
      people.openScope(TENANT);
      // The groups' names, for the rows that name a group rather than a person. Any member may
      // read them, which is the contract's own reasoning: a grant to a group is unreadable until
      // the group can be shown as the people it reaches.
      return groups.open();
    }),
  );

  const holders = $derived(people.holders({}, TENANT));

  // The workspace's sign-in rule, for one switch: whether accounts without an address may be given
  // here at all (ADR-0074 §7). Read rather than assumed, and the form is absent - not disabled -
  // where the rule says no or cannot be read (P-05).
  $effect(() => untrack(() => signInPolicy.open()));
  const managedAllowed = $derived(signInPolicy.policy?.managed_accounts?.value === true);

  /** The row a revoke is being confirmed for; the dialog is open while there is one. */
  let revoking = $state<Holder | undefined>(undefined);
  const revokingOwnership = $derived(
    revoking ? ownershipOf(holders, revoking.membershipId, actor.account?.id, (id) => people.membersOf(id)) : 'other',
  );

  function nameOf(holder: Holder): string {
    if (holder.groupId) {
      return t('app.people.group_named', { name: groups.nameOf(holder.groupId) ?? t('app.people.unnamed_group') });
    }
    return accounts.nameOf(holder.accountId) ?? t('app.people.unnamed');
  }

  // A managed person's two actions (UC-ID-20 checks 5 and 7), on their row and nowhere else: the
  // summary's `managed` is the server's full condition, so a person with an address never sees them.
  /** The row a new start password is being confirmed for. */
  let renewing = $state<Holder | undefined>(undefined);
  /** The one answer that carried the new start password, until the reader dismisses it. */
  let renewed = $state<{ signInName: string; startPassword: string; who: string } | undefined>(undefined);
  /** The row a second factor's reset is being confirmed for (fifth answers 6). */
  let resettingFactor = $state<Holder | undefined>(undefined);
  /** The person an address is being added for. */
  let addressing = $state<{ accountId: string; who: string } | undefined>(undefined);

  function isManaged(holder: Holder): boolean {
    return holder.groupId === undefined && accounts.isManaged(holder.accountId);
  }

  /** Whether a new way into the row's account is no certain refusal (identity.md §13, P-05). */
  function mayWayIn(holder: Holder): boolean {
    return mayIssueWayIn([holder.role], people.grantableAt(TENANT));
  }

  // An address is added only where the password is a way in (identity.md §13): elsewhere it would
  // take a managed person's one way in. The sign-in rules say whether it is, fallback included; the
  // action is offered once they have answered, and its reason stands under the table otherwise.
  $effect(() => signInRules.read());
  const addressOffered = $derived(signInRules.isSettled && signInRules.hasPassword);
  const addressWithheld = $derived(
    signInRules.isSettled && !signInRules.hasPassword && holders.some((holder) => isManaged(holder)),
  );

  async function issueStartPassword(): Promise<void> {
    const holder = renewing;
    renewing = undefined;
    if (!holder?.accountId) return;
    const who = nameOf(holder);
    await attempt(async () => {
      const issued = await people.issueStartPassword(holder.accountId as string);
      renewed = { ...issued, who };
    }, t('app.people.renewed_announced', { who }));
  }

  async function resetFactor(): Promise<void> {
    const holder = resettingFactor;
    resettingFactor = undefined;
    if (!holder?.accountId) return;
    const who = nameOf(holder);
    await attempt(
      () => people.resetSecondFactor(holder.accountId as string),
      t('app.people.factor_reset_announced', { who }),
    );
  }

  async function revoke(): Promise<void> {
    const holder = revoking;
    revoking = undefined;
    if (!holder) return;
    await attempt(() => people.revoke(holder.membershipId, TENANT), t('app.people.revoked_announced'));
  }

  /** The installation's roles, in the order it reports them — never a list compiled in here. */
  const roles = $derived(
    (manifest.value?.roles ?? [])
      .map((description) => description.role)
      .filter((role): role is MembershipRole => role !== undefined),
  );

  /**
   * Of those, the ones the server says this reader may give at the workspace: an administrator is
   * not offered Owner (UC-ID-14 check 1). With none - or before the answer - the invitation and the
   * grant are absent, since every press would be refused (P-05).
   */
  const grantable = $derived(grantableOf(roles, people.grantableAt(TENANT)));

  const roleOptions = $derived(
    grantable.map((role) => ({ value: role, label: t(`app.people.role.${role.toLowerCase()}`) })),
  );

  /**
   * The invitation's place, and the roles offered there: what the reader may give is decided along
   * that place's path (domain-model.md §3.2), so a hub's answer is read when the hub is chosen and
   * nothing is offered until it arrives.
   */
  const invitePlace = $derived(inviteWhere === WORKSPACE_PLACE ? TENANT : invitePlaceOf(inviteWhere));
  $effect(() => {
    const place = invitePlace;
    if (place.scopeType !== 'TENANT') untrack(() => people.openScope(place));
  });
  // The workspace's own roles beside the shipped ones, as the role-grant screens offer them: a
  // requester is invited with a role holding Raise requests at the desk alone (UC-SVC-05).
  $effect(() => untrack(() => workspaceRoles.open()));
  const inviteRoleOptions = $derived(
    inviteRoleChoices(
      grantableOf(roles, people.grantableAt(invitePlace)).map((role) => ({
        value: role,
        label: t(`app.people.role.${role.toLowerCase()}`),
      })),
      workspaceRoles.catalogue?.custom,
    ),
  );
  // A role chosen for one place is not carried to a place that does not offer it.
  $effect(() => {
    if (inviteRole && !inviteRoleOptions.some((option) => option.value === inviteRole)) inviteRole = '';
  });
  $effect(() => untrack(() => containers.start()));
  // Each hub's collections too, under the hub's name: a desk's requester is placed at the desk.
  $effect(() => {
    const ids = containers.hubs.map((hub) => hub.id);
    return untrack(() => {
      const stops = ids.map((id) => containers.openLevel(id));
      return () => stops.forEach((stop) => stop());
    });
  });
  const inviteWhereOptions = $derived([
    { value: WORKSPACE_PLACE, label: t('app.people.managed_where_workspace') },
    ...containers.hubs.flatMap((hub) => [
      { value: hub.id, label: hub.name },
      ...containers.collectionsOf(hub.id).map((collection) => ({
        value: collectionChoice(collection.id),
        label: t('app.people.invite_where_collection', { hub: hub.name, collection: collection.name }),
      })),
    ]),
  ]);

  /** Who could be granted something: the people this workspace already knows about. */
  const candidates = $derived(
    people
      .candidates({})
      .map((id) => ({ value: id, label: accounts.nameOf(id) ?? t('app.people.unnamed') })),
  );

  const columns = $derived<DataColumn[]>([
    { id: 'who', label: t('app.people.name_column'), isSortable: true, priority: 1 },
    ...(mayManage ? [{ id: 'email', label: t('app.people.email_column'), isSortable: true, priority: 2 as const }] : []),
    { id: 'role', label: t('app.people.role_column'), isSortable: true, priority: 2 },
    { id: 'groups', label: t('app.people.groups_column'), isSortable: true, priority: 3 },
    { id: 'status', label: t('app.people.status'), isSortable: true, isStatus: true },
  ]);

  // What narrows the list (PS-7). `GET /memberships` has no search or filter of its own and answers
  // the workspace's people whole, so these narrow what is loaded - which is everything.
  let search = $state('');
  let roleChoice = $state('');
  let standingChoice = $state('');
  const roleChoices = $derived([...new Set(holders.map((holder) => holder.role))]);
  const standingChoices = $derived(
    [...new Set(holders.map((holder) => statusOf(holder.accountId)).filter((status): status is string => status !== undefined))],
  );
  const standingWord = (status: string) => (status === 'INVITED' ? t('app.people.invited') : t(`app.people.status_${status.toLowerCase()}`));
  const roleWord = (role: string) => t(`app.people.role.${role.toLowerCase()}`);
  const shown = $derived(
    holders.filter(
      (holder) =>
        matchesChoice(roleChoice || undefined, holder.role) &&
        matchesChoice(standingChoice || undefined, statusOf(holder.accountId)) &&
        matchesSearch(search, [nameOf(holder), holder.accountId ? (signIn.details[holder.accountId]?.email ?? undefined) : undefined]),
    ),
  );
  const chips = $derived<Filter[]>([
    { id: 'role', label: t('app.people.filter_role', { role: roleChoice ? roleWord(roleChoice) : t('app.page.any') }), isActive: roleChoice !== '' },
    { id: 'standing', label: t('app.people.filter_standing', { standing: standingChoice ? standingWord(standingChoice) : t('app.page.any') }), isActive: standingChoice !== '' },
  ]);
  const activeFilters = $derived(chips.filter((filter) => filter.isActive).length);
  function clearFilters(): void {
    search = '';
    roleChoice = '';
    standingChoice = '';
  }
  // The list on this screen is the one that actually grows - a workspace of two hundred people is
  // two hundred rows, and the server answers `GET /memberships` in one response with no cursor.
  // It is sorted here, over the list the client is already holding whole, and it is **not** paged:
  // a client-side pager would give the product a second answer to "where am I in this list"
  // beside `LoadMore`'s, and one of the two would always be the wrong guess.
  //
  // The role sorts by the **word** rather than by the enum's own order: a reader sorting this
  // column is looking for "every administrator together", and `ADMIN` before `MEMBER` in a
  // language where the words run the other way would be an order that looks arbitrary.
  const order = sorting({
    rows: () => shown,
    read: (holder, columnId) => {
      if (columnId === 'role') return t(`app.people.role.${holder.role.toLowerCase()}`);
      if (columnId === 'status') return statusOf(holder.accountId);
      if (columnId === 'email') return (holder.accountId && signIn.details[holder.accountId]?.email) || '';
      if (columnId === 'groups') return groupsOf(holder);
      return nameOf(holder);
    },
    initialSort: { columnId: 'who', direction: 'ascending' },
  });

  // The groups each person is in, for the column: each group's members are read once, as the
  // group's own page reads them.
  $effect(() => {
    const ids = groups.all.map((group) => group.id);
    untrack(() => ids.forEach((groupId) => void groups.read(groupId).catch(() => undefined)));
  });
  function groupsOf(holder: Holder): string {
    if (!holder.accountId) return '';
    return groups.all
      .filter((group) => groups.detail(group.id)?.members.includes(holder.accountId as string))
      .map((group) => group.name)
      .join(', ');
  }

  /** How a standing reads: active as success, an invitation still out as waiting (rule 3: words too). */
  function standingTone(status: string | undefined): 'success' | 'warning' | 'neutral' {
    if (status === 'ACTIVE') return 'success';
    if (status === 'INVITED') return 'warning';
    return 'neutral';
  }

  /**
   * The workspace is the only scope here, so every badge says the same thing about reach. Derived
   * rather than read once: the reader's catalogue arrives after the first render, and a label
   * fixed before it would stay in the source language beside translated sentences.
   */
  const scopeLabel = $derived(t('app.people.at_workspace'));

  function statusOf(accountId: string | undefined): string | undefined {
    return accountId ? accounts.statusOf(accountId) : undefined;
  }

  /**
   * How the last mail to each person on the list went (user-experience.md §8.3) - the invitation,
   * or a newer mail of any kind -, for whoever may invite, where invitations go by mail. Read again
   * after a renewal; a refusal leaves the rows without the line rather than failing the list.
   */
  let deliveries = $state<Readonly<Record<string, InvitationDelivery>>>({});
  const invitedIds = $derived(
    way === 'mail' && holdsHere('MANAGE_MEMBERS') === true
      ? [...new Set(holders.map((holder) => holder.accountId).filter((id): id is string => !!id))].sort().slice(0, 200)
      : [],
  );
  const invitedKey = $derived(invitedIds.join(','));
  function readDeliveries(ids: readonly string[]): void {
    void people.invitationDeliveries(ids)
      .then((answer) => (deliveries = Object.fromEntries(answer.map((delivery) => [delivery.account_id, delivery]))))
      .catch(() => (deliveries = {}));
  }
  $effect(() => {
    void invitedKey;
    const ids = invitedIds;
    untrack(() => readDeliveries(ids));
  });

  /**
   * Each person's address and authenticator app, for whoever manages members: the address column,
   * and *Reset second factor* only where an app stands. Until the read answers the reset is not
   * offered; where it failed the reset stays, and the server answers a person without an app.
   */
  let signIn = $state<{ readonly state: 'reading' | 'ready' | 'failed'; readonly details: Readonly<Record<string, SignInDetail>> }>({ state: 'reading', details: {} });
  const peopleIds = $derived(
    mayManage
      ? [...new Set(holders.map((holder) => holder.accountId).filter((id): id is string => !!id))].sort().slice(0, 200)
      : [],
  );
  const peopleKey = $derived(peopleIds.join(','));
  function readSignIn(ids: readonly string[]): void {
    void people.signInDetails(ids)
      .then((answer) => (signIn = { state: 'ready', details: Object.fromEntries(answer.map((detail) => [detail.account_id, detail])) }))
      .catch(() => (signIn = { state: 'failed', details: {} }));
  }
  $effect(() => {
    void peopleKey;
    const ids = peopleIds;
    untrack(() => readSignIn(ids));
  });
  function hasApp(accountId: string | undefined): boolean | undefined {
    if (signIn.state === 'failed') return undefined;
    return accountId ? signIn.details[accountId]?.authenticator_app === true : false;
  }

  /** Whether the account counts as an owner who can act: active or restricted, as the server counts. */
  function canAct(accountId: string): boolean {
    const status = statusOf(accountId);
    return status === 'ACTIVE' || status === 'RESTRICTED';
  }

  async function attempt(work: () => Promise<unknown>, said?: string): Promise<void> {
    failure = undefined;
    isWriting = true;
    try {
      await work();
      // Said out loud on success (4.1.3): what changed is on the screen, and a reader who cannot
      // see the screen is told the same thing once, through the one live region.
      if (said) announcer.say(said);
    } catch (error) {
      // The server's own sentence — including the step-up refusal, which is not a failure but a
      // question the prompt has already asked and the reader declined.
      failure = renderProblem(error as never, messages).message;
    } finally {
      isWriting = false;
    }
  }

  /**
   * Invites somebody and gives them the role at once.
   *
   * The role is not optional here, and that is the contract's doing rather than a preference: an
   * invitation carries no role, and there is no accounts listing — so an invitation on its own
   * produces somebody this screen could never show again.
   */
  async function invite(event: SubmitEvent): Promise<void> {
    event.preventDefault();
    const address = inviteEmail.trim();
    missing = marked(event, [
      { kind: 'field', key: 'email', code: 'app.forms.required', isFilled: address !== '' },
      { kind: 'field', key: 'inviteRole', code: 'app.forms.choose', isFilled: inviteRole !== '' },
    ]);
    if (!isComplete(missing)) return;
    handedOver = undefined;
    inviteRefusal = undefined;
    grantRefusal = undefined;
    // The form's own, not the page's `attempt`: a refusal drawn at the top of the page is drawn
    // above the screen of whoever pressed the button at the form (UC-ID-14 check 6).
    isWriting = true;
    let answer: Awaited<ReturnType<typeof people.invite>>;
    try {
      answer = await people.invite(address, inviteName.trim() || undefined, inviteRole, invitePlace);
    } catch (error) {
      // Nothing went out: the address stays to be corrected, the sentence is at its field, the
      // reader is taken there, and it is said once.
      inviteRefusal = renderProblem(error as never, messages).message;
      announcer.say(inviteRefusal);
      await tick();
      inviteAddressField?.querySelector('input')?.focus();
      return;
    } finally {
      isWriting = false;
    }
    // The invitation stands even where the role could not be given; the reason is said at the form,
    // which stays open for it.
    if (answer.grantFailure !== undefined) grantRefusal = renderProblem(answer.grantFailure as never, messages).message;
    else isInviting = false;
    if (answer.link !== undefined) {
      // UC-ID-14 check 5: no mail went out, and the screen does not pretend one did.
      isInviting = false;
      await showLink(address, answer.link, t('app.people.invite_link_announced'));
    } else {
      sent(address);
    }
    // An invitation went out, so the form empties: a second press with the same address in it
    // would be a second invitation the reader did not mean to send.
    inviteEmail = '';
    inviteName = '';
    inviteRole = '';
    inviteWhere = WORKSPACE_PLACE;
  }

  /**
   * Gives an invitation not yet accepted a new way to arrive: the mail again, or a new link shown
   * once. The earlier link stops working, which the panel's hint says.
   */
  async function renew(holder: Holder): Promise<void> {
    if (!holder.accountId) return;
    handedOver = undefined;
    const accountId = holder.accountId;
    await attempt(async () => {
      const link = await people.renewInvitation(accountId);
      if (link !== undefined) {
        await showLink(nameOf(holder), link, t('app.people.renew_link_announced'));
      } else {
        sent(nameOf(holder));
        readDeliveries(invitedIds);
      }
    });
  }

  /**
   * Hands an invitation over by link where mail is sent: the mail still waiting is cancelled, so
   * the mailed link never replaces this one.
   */
  async function handOver(holder: Holder): Promise<void> {
    if (!holder.accountId) return;
    handedOver = undefined;
    const accountId = holder.accountId;
    await attempt(async () => {
      const link = await people.handOverInvitation(accountId);
      if (link !== undefined) await showLink(nameOf(holder), link, t('app.people.renew_link_announced'));
      readDeliveries(invitedIds);
    });
  }

  /** Shows the one answer's link in its Dialog S, the only place it is ever drawn. */
  async function showLink(who: string, link: string, said: string): Promise<void> {
    handedOver = { who, link };
    announcer.say(said);
    await tick();
  }

  /** An invitation went out by mail: said once, in a toast and through the live region. */
  function sent(who: string): void {
    toast.show({ text: t('app.people.invited_sent', { email: who }), tone: 'success' });
    announcer.say(t('app.people.invited_announced'));
  }

  async function grant(): Promise<void> {
    missing = check([
      { kind: 'field', key: 'subject', code: 'app.forms.choose', isFilled: chosenSubject !== '' },
      { kind: 'field', key: 'grantRole', code: 'app.forms.choose', isFilled: chosenRole !== '' },
    ]);
    if (!isComplete(missing)) {
      void tick().then(() => focusFirst(document));
      return;
    }
    await attempt(() =>
      people.grant({ accountId: chosenSubject }, chosenRole as MembershipRole, TENANT), t('app.people.granted_announced')
    );
    if (failure !== undefined) return;
    toast.show({ text: t('app.people.granted_announced'), tone: 'success' });
    chosenSubject = '';
    chosenRole = '';
    close();
  }
  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(t('app.people.title')));

  /** The ways this reader may add somebody here: none where nothing can be given (P-05). */
  const ways = $derived(
    grantable.length === 0
      ? []
      : [
          { id: 'invite', label: t('app.people.invite_title') },
          ...(managedAllowed ? [{ id: 'managed', label: t('app.people.managed_title') }] : []),
          { id: 'grant', label: t('app.people.grant_title') },
        ],
  );
  /** A row's ⋯ (PS-6): everything but its one verb, the destructive act last. */
  function rowMenu(holder: Holder): DataRowMenu {
    const status = statusOf(holder.accountId);
    const items: MenuItem[] = [];
    if (holder.accountId) items.push({ id: 'open', label: t('app.page.open') });
    // View as (A6): the administration's answer for this person - their roles at a place, what they
    // may do, and whether and why for a chosen permission.
    if (holder.accountId && mayViewAs) items.push({ id: 'view_as', label: t('app.people.view_as'), icon: 'shield' });
    if (holder.accountId && mayViewScreens && holder.accountId !== actor.account?.id && status === 'ACTIVE') {
      items.push({ id: 'screens', label: viewingScreens === holder.accountId ? t('app.people.working') : t('app.view_as.start'), icon: 'eye' });
    }
    // Where the mail does not arrive: a link to hand over instead, the waiting mail cancelled -
    // never both (identity.md §12).
    if (status === 'INVITED' && way === 'mail' && mayWayIn(holder)) items.push({ id: 'hand_over', label: t('app.people.copy_invitation_link') });
    if (isManaged(holder) && mayWayIn(holder) && addressOffered) items.push({ id: 'address', label: t('app.people.address_add') });
    if (mayResetFactor(holder, actor.account?.id, status, people.grantableAt(TENANT), hasApp(holder.accountId))) {
      items.push({ id: 'factor', label: t('app.people.factor_reset'), icon: 'key' });
    }
    // Absent for a role above the reader's own: taking it away is refused as handing it out is. On
    // the workspace's last owner, which the server keeps, it stays, unavailable, with its reason
    // (UC-WRK-32 check 1).
    if (mayRevoke(holder.role, people.grantableAt(TENANT))) {
      items.push({
        id: 'revoke',
        label: t('app.people.revoke_menu'),
        isDestructive: true,
        hasSeparatorBefore: items.length > 0,
        disabledReason: endsTheLastOwner(holders, holder.membershipId, people.isCompleteAt(TENANT), canAct) ? t('app.people.last_owner') : undefined,
      });
    }
    return { label: t('app.people.more_for', { name: nameOf(holder) }), items };
  }

  function onRowMenu(holder: Holder, id: string): void {
    if (id === 'open' && holder.accountId) onopen?.(`/administration/people/${holder.accountId}`);
    else if (id === 'view_as') onopen?.(`/administration/people?tab=permissions&person=${holder.accountId}`);
    else if (id === 'screens') void viewScreens(holder.accountId as string);
    else if (id === 'hand_over') void handOver(holder);
    else if (id === 'address') addressing = { accountId: holder.accountId as string, who: nameOf(holder) };
    else if (id === 'factor') resettingFactor = holder;
    else if (id === 'revoke') revoking = holder;
  }

  const figure = $derived(
    holders.length === 0
      ? undefined
      : t('app.people.figure', { people: String(holders.filter((holder) => holder.accountId).length), groups: String(groups.all.length) }),
  );

  const addAction = $derived(
    current === 'people' && ways.length > 0
      ? {
          label: t('app.people.add'),
          icon: 'plus' as const,
          opener: 'add-person',
          onclick: () => void open('invite'),
          menu: { label: t('app.people.add_ways'), items: ways, onselect: (id: string) => void open(id) },
        }
      : undefined,
  );
</script>

{#snippet mailLine(delivery: InvitationDelivery | undefined)}
  {#if delivery}
    {@const line = deliveryLine(delivery)}
    <p class="delivery" class:failed={delivery.state === 'FAILED'} data-delivery={delivery.state} data-mail-kind={delivery.category ?? 'INVITATION'}>
      {t(line.code, { at: formatDateTime(delivery.at, messages.locale), kind: line.kind ? t(line.kind) : '' })}{#if line.reason}<br />{t(line.reason.code, line.reason.params)}{/if}
    </p>
  {/if}
{/snippet}

{#snippet strip()}
  <PeopleStrip {current} {onnavigate} />
{/snippet}

{#if current === 'people'}
  <ListPage
    head={{
      title: t('app.people.title'),
      subtitle: figure,
      breadcrumb: {
        trail: [
          { id: 'administration', label: t('app.admin.title'), href: '/administration' },
          { id: 'people', label: t('app.people.title') },
        ],
        label: t('app.admin.trail'),
        expandLabel: t('app.admin.expand_trail'),
      },
      primary: addAction,
    }}
    {strip}
  >
    {#snippet filters()}
      <FilterBar
        searchLabel={t('app.people.search')}
        clearSearchLabel={t('app.page.clear_search')}
        bind:search
        filters={chips}
        moreLabel={t('app.page.more_filters')}
        filtersLabel={t('app.page.filters', { count: String(activeFilters) })}
        sheetTitle={t('app.page.filters', { count: '0' })}
        sheetDismissLabel={t('app.page.sheet_close')}
        sheetGrabLabel={t('app.page.sheet_grab')}
        showLabel={t('app.page.show_results', { count: String(shown.length) })}
        clearLabel={t('app.page.clear')}
        onclear={clearFilters}
        countLabel={holders.length === 0 ? undefined : t('app.page.found', { shown: String(shown.length), count: String(holders.length) })}
      >
        {#snippet picker(filter: Filter)}
          {#if filter.id === 'role'}
            <Radio
              label={t('app.people.role_column')}
              bind:value={roleChoice}
              options={[{ value: '', label: t('app.page.any') }, ...roleChoices.map((role) => ({ value: role, label: roleWord(role) }))]}
            />
          {:else}
            <Radio
              label={t('app.people.status')}
              bind:value={standingChoice}
              options={[{ value: '', label: t('app.page.any') }, ...standingChoices.map((status) => ({ value: status, label: standingWord(status) }))]}
            />
          {/if}
        {/snippet}
      </FilterBar>
    {/snippet}

    {#if failure && !isGranting}
      <FailureNote problem={failure} />
    {/if}

    <DataTable
      label={t('app.people.title')}
      {columns}
      rows={holders.length === 0 ? [] : order.rows}
      key={(holder: Holder) => holder.membershipId}
      href={(holder: Holder) => (holder.accountId ? `/administration/people/${holder.accountId}` : undefined)}
      rowLabel={(holder: Holder) => nameOf(holder)}
      menu={rowMenu}
      onmenu={onRowMenu}
      actionsLabel={t('app.people.actions')}
      sheetDismissLabel={t('app.page.sheet_close')}
      sheetGrabLabel={t('app.page.sheet_grab')}
      sort={order.sort}
      onSort={(next) => order.by(next)}
      isBusy={isWriting}
      shownLabel={holders.length === 0 ? undefined : t('app.page.showing', { count: String(shown.length) })}
    >
      {#snippet cell(holder: Holder, column: DataColumn)}
        {#if column.id === 'who'}
          <!-- A membership granted to a group reaches the people in it; the group's own page is
               where that list lives, and here it is named as what it is. -->
          <span class="who">{nameOf(holder)}</span>
        {:else if column.id === 'email'}
          {@const detail = holder.accountId ? signIn.details[holder.accountId] : undefined}
          {#if detail?.email}
            <span class="address">{detail.email}</span>
          {:else if detail}
            <span class="quiet">{t('app.people.no_address')}</span>
          {/if}
        {:else if column.id === 'role'}
          <Badge>{roleWord(holder.role)}</Badge>
        {:else if column.id === 'groups'}
          {#if groupsOf(holder)}{groupsOf(holder)}{:else}<span class="quiet">—</span>{/if}
        {:else}
          <span class="standing">
            {#if statusOf(holder.accountId) === 'INVITED'}
              <!-- Not a failure and not a person yet: the invitation is out and unredeemed. -->
              <Badge tone={standingTone('INVITED')}>{t('app.people.invited')}</Badge>
            {:else if statusOf(holder.accountId)}
              <Badge tone={standingTone(statusOf(holder.accountId))}>{t(`app.people.status_${statusOf(holder.accountId)?.toLowerCase()}`)}</Badge>
            {/if}
            {@render mailLine(holder.accountId ? deliveries[holder.accountId] : undefined)}
          </span>
        {/if}
      {/snippet}
      {#snippet verb(holder: Holder)}
        {#if statusOf(holder.accountId) === 'INVITED' && (way === 'mail' || mayWayIn(holder))}
          <!-- An invitation not yet accepted gets a new way to arrive: a link lost or run out would
               otherwise leave the address taken for good. A link is a way in for whoever renews
               it, so it is absent above the reader's own role. -->
          <Button size="sm" tone="subtle" isBusy={isWriting} busyLabel={t('app.people.working')} onclick={() => void renew(holder)}>
            {way === 'link' ? t('app.people.renew_link') : t('app.people.renew_mail')}
          </Button>
        {:else if isManaged(holder) && mayWayIn(holder)}
          <Button size="sm" tone="subtle" onclick={() => (renewing = holder)}>{t('app.people.renew')}</Button>
        {/if}
      {/snippet}
      {#snippet empty()}
        {#if holders.length === 0}
          <p class="quiet reading"><Spinner label={t('app.people.reading')} /> <span>{t('app.people.reading')}</span></p>
        {:else}
          <EmptyState kind="filtered" title={t('app.people.filtered_title')}>
            {#snippet action()}<Button onclick={clearFilters}>{t('app.people.filtered_clear')}</Button>{/snippet}
          </EmptyState>
        {/if}
      {/snippet}
    </DataTable>

    {#if addressWithheld}
      <p class="quiet small">{t('app.people.address_needs_password')}</p>
    {/if}
  </ListPage>

  {#if ways.length > 0}
  <!-- Invite somebody: four fields in one group, a Dialog M. -->
  <Dialog
    size="m"
    title={t('app.people.invite_title')}
    bind:isOpen={isInviting}
    dismissLabel={t('app.people.add_close')}
    onDismiss={inviteGuard.dismiss}
    onClose={close}
    primary={{
      label: way === 'link' ? t('app.people.invite_link_button') : t('app.people.invite'),
      isBusy: isWriting,
      busyLabel: t('app.people.working'),
      onclick: () => inviteForm?.requestSubmit(),
    }}
  >
    <!-- What the press will do, said before it: a mail, or - where no mail goes out - a link to hand
         over. Neither until the manifest has said which. -->
    <p class="quiet small">
      {way === 'link' ? t('app.people.invite_hint_link') : way === 'mail' ? t('app.people.invite_hint') : t('app.people.invite_hint_neutral')}
    </p>
    <form bind:this={inviteForm} onsubmit={invite}>
      <Stack gap="150">
        <div bind:this={inviteAddressField}>
          <Input
            label={t('app.people.invite_email')}
            bind:value={inviteEmail}
            type="email"
            autocomplete="off"
            spellcheck={false}
            isRequired
            error={inviteRefusal ?? (missing.email && !inviteEmail.trim() ? t(missing.email) : undefined)}
            oninput={() => (inviteRefusal = undefined)}
          />
        </div>
        <Input label={t('app.people.invite_name')} hint={t('app.people.invite_name_hint')} bind:value={inviteName} autocomplete="off" />
        <Select label={t('app.people.invite_where')} hint={t('app.people.invite_where_hint')} bind:value={inviteWhere} options={inviteWhereOptions} />
        <Select
          label={t('app.people.role_column')}
          hint={t('app.people.invite_role_hint')}
          bind:value={inviteRole}
          placeholder={t('app.people.choose_role')}
          options={inviteRoleOptions}
          error={missing.inviteRole && !inviteRole ? t(missing.inviteRole) : undefined}
        />
        {#if grantRefusal}
          <Banner tone="warning">{grantRefusal}</Banner>
        {/if}
      </Stack>
    </form>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (inviteGuard.dismiss()) close(); }}>{t('app.page.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={inviteGuard} />

  <!-- A role for somebody already here: two fields, a Dialog M. -->
  <Dialog
    size="m"
    title={t('app.people.grant_title')}
    bind:isOpen={isGranting}
    dismissLabel={t('app.people.add_close')}
    onDismiss={grantGuard.dismiss}
    onClose={close}
    primary={{ label: t('app.people.grant'), isBusy: isWriting, busyLabel: t('app.people.working'), onclick: () => void grant() }}
  >
    <Stack gap="150">
      <p class="quiet small">{t('app.people.grant_hint')}</p>
      {#if failure}
        <FailureNote problem={failure} />
      {/if}
      <Select
        label={t('app.people.who')}
        bind:value={chosenSubject}
        placeholder={t('app.people.choose_person')}
        options={candidates}
        error={missing.subject && !chosenSubject ? t(missing.subject) : undefined}
      />
      <Select
        label={t('app.people.role_column')}
        bind:value={chosenRole}
        placeholder={t('app.people.choose_role')}
        options={roleOptions}
        error={missing.grantRole && !chosenRole ? t(missing.grantRole) : undefined}
      />
    </Stack>
    {#snippet actions()}
      <Button tone="subtle" onclick={() => { if (grantGuard.dismiss()) close(); }}>{t('app.page.cancel')}</Button>
    {/snippet}
  </Dialog>
  <DiscardDialog guard={grantGuard} />
  {/if}

  {#if managedAllowed && grantable.length > 0}
    <ManagedAccountDialog bind:isOpen={isAddingManaged} {roles} workspaceRoles={grantable} />
  {/if}

  <!-- The only time an invitation's link exists anywhere but as the server's hash: a Dialog S. -->
  <Dialog size="s" title={t('app.people.invite_link_title')} isOpen={handedOver !== undefined} onClose={() => (handedOver = undefined)}>
    {#if handedOver}
      <div data-invitation-link>
        <OneTimeSecret
          value={handedOver.link}
          label={t('app.people.invite_link', { name: handedOver.who })}
          hint={t('app.people.invite_link_hint')}
          revealLabel={t('app.people.invite_link_reveal')}
          hideLabel={t('app.people.invite_link_hide')}
          copyLabel={t('app.people.managed_copy')}
          copiedLabel={t('app.people.invite_link_copied')}
          acknowledgementLabel={t('app.people.invite_link_kept')}
          notAcknowledgedReason={t('app.people.invite_link_keep_first')}
          dismissLabel={t('app.tokens.done')}
          onDismiss={() => (handedOver = undefined)}
        />
      </div>
    {/if}
  </Dialog>

  <!-- The only time the new start password exists anywhere but the server's hash: a Dialog S. -->
  <Dialog size="s" title={t('app.people.start_password_title')} isOpen={renewed !== undefined} onClose={() => (renewed = undefined)}>
    {#if renewed}
      <Stack gap="150">
        <StartPasswordResult
          signInName={renewed.signInName}
          startPassword={renewed.startPassword}
          label={t('app.people.renewed_start_password', { who: renewed.who, name: renewed.signInName })}
          onDismiss={() => (renewed = undefined)}
        />
      </Stack>
    {/if}
  </Dialog>

  <RenewDialog who={renewing ? nameOf(renewing) : undefined} isBusy={isWriting} onConfirm={() => void issueStartPassword()} onCancel={() => (renewing = undefined)} />
  <ResetFactorDialog who={resettingFactor ? nameOf(resettingFactor) : undefined} isBusy={isWriting} onConfirm={() => void resetFactor()} onCancel={() => (resettingFactor = undefined)} />
  <AddressDialog target={addressing} onClose={() => (addressing = undefined)} />
  <RevokeDialog
    holder={revoking}
    ownership={revokingOwnership}
    who={revoking ? nameOf(revoking) : ''}
    where={scopeLabel}
    isBusy={isWriting}
    onConfirm={() => void revoke()}
    onCancel={() => (revoking = undefined)}
  />
{:else if current === 'groups'}
  <GroupsView {onnavigate} {onopen} />
{:else if current === 'roles'}
  <RolesView {onnavigate} />
{:else}
  <PermissionsView {person} {onnavigate} />
{/if}

<style>
  .delivery { margin: var(--sp-050) 0 0; font-size: var(--fs-075); color: var(--text-secondary); }
  .delivery.failed { color: var(--text-danger); }

  form { margin: 0; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .reading { padding: var(--sp-300); }

  .who { color: var(--text-primary); font-weight: var(--fw-medium); }

  /* An address is one word to the line breaker; it breaks where it must rather than widen the table. */
  .address { overflow-wrap: anywhere; }

  .standing { display: inline-flex; flex-direction: column; align-items: flex-start; }
</style>
