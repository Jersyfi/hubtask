<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Administration → E-mail (ADR-0094, UC-JUM-10): the workspace's one mail intake.
  //
  // **Readable names, one order.** A message is filed by the first rule that matches: a ticket key
  // (`EINKAUF-42`) makes a comment, a handle (`#einkauf`) a new entry, then the sender rules and
  // the default; anything else waits in the jumble with its reason. The page shows that order in
  // words, the handles of every collection, and what the last messages became.
  //
  // **The connected mailboxes come first** (user-experience.md §7.10): the head counts them in
  // words and connects one, so a failing mailbox is seen before the settings below it.
  //
  // **Who may write is one choice with two lists.** Members by default; members and allowed
  // addresses and domains; or anyone. The block list applies before every level.
  //
  // **One section per address, one save model each** (page-system §7.3, §9.3, PS-9, PS-10):
  // *Mailboxes*, *Intake*, *Who may write*, *Sender rules*, *Handles* and *Recently arrived*, each
  // at `/administration/mail/<section>`. The settings are one record written whole: *Intake* and
  // *Who may write* each have a save bar that sends its own values over what is saved, a sender
  // rule is written by its Dialog M, a mailbox by its dialogs; the rest is read.

  import { untrack } from 'svelte';

  import FailureNote from '../lib/frame/FailureNote.svelte';

  import {
    Button,
    DataTable,
    EmptyState,
    IconButton,
    Input,
    Radio,
    SaveBar,
    Select,
    SettingRow,
    SettingsLayout,
    SettingsSection,
    Spinner,
    Switch,
    Textarea,
    type DataColumn,
    type SectionLink,
  } from '@hubtask/design-system/components';

  import { adminTrail } from '../lib/administration/trail.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import {
    linesOf,
    mailboxFigure,
    mailboxRouteTo,
    readMailReceipts,
    readMailSettings,
    saveMailSettings,
    suggestedName,
    type MailReceipt,
    type MailSettings,
    type SenderLevel,
  } from '../lib/data/mailintake.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { leaveGuard } from '../lib/frame/leave.svelte.ts';
  import { page } from '../lib/frame/page.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import ConnectMailboxDialog from '../lib/mail/ConnectMailboxDialog.svelte';
  import MailboxDialog from '../lib/mail/MailboxDialog.svelte';
  import MailboxSection from '../lib/mail/MailboxSection.svelte';
  import { MailboxList, type SenderRule } from '../lib/mail/mailboxes.svelte.ts';
  import RemoveMailboxDialog from '../lib/mail/RemoveMailboxDialog.svelte';
  import SenderRuleDialog from '../lib/mail/SenderRuleDialog.svelte';
  import TableSection from '../lib/page/TableSection.svelte';

  interface Props {
    /** The section the address names (`/administration/mail/:section`); none is the list. */
    section?: string;
    /** Replaces the address: a move between sections, so the unsaved values of another stay. */
    onreplace?: (path: string) => void;
    /** What a mail provider sent back after connecting a mailbox (`/administration/mail/oauth`). */
    providerAnswer?: Readonly<Record<string, string>>;
    /** Told once the answer is handed on, so the address drops the code and the state. */
    onanswered?: () => void;
  }

  let { section, onreplace, providerAnswer, onanswered }: Props = $props();

  const BASE = '/administration/mail';
  type Scope = 'intake' | 'senders' | 'rules';

  let loaded = $state<MailSettings | undefined>(undefined);
  let readFailure = $state<string | undefined>(undefined);
  let enabled = $state(false);
  let name = $state('');
  let defaultPlace = $state('');
  let level = $state<SenderLevel>('MEMBERS');
  let allowed = $state('');
  let blocked = $state('');
  let isSaving = $state(false);
  let saveFailure = $state<string | undefined>(undefined);
  let receipts = $state<readonly MailReceipt[]>([]);
  /** The connected mailboxes: their section, the head's figure, the addresses through one. */
  const boxes = new MailboxList();
  const mailboxes = $derived(boxes.mailboxes);

  const mayShape = $derived(holdsHere('AUTOMATION') === true);

  /** What is saved becomes the draft of the scopes named: all on a read, one on a section's save. */
  function take(settings: MailSettings, scopes: readonly Scope[] = ['intake', 'senders']) {
    loaded = settings;
    if (scopes.includes('intake')) {
      enabled = settings.enabled;
      name = settings.name ?? '';
      defaultPlace = settings.default_collection_id ?? '';
    }
    if (scopes.includes('senders')) {
      level = settings.senders.level;
      allowed = settings.senders.allowed.join('\n');
      blocked = settings.senders.blocked.join('\n');
    }
  }

  $effect(() => {
    if (!mayShape) return;
    untrack(() => {
      void readMailSettings()
        .then((settings) => settings && take(settings))
        .catch((error) => (readFailure = renderProblem(error as never, messages).message));
      void readMailReceipts().then((rows) => (receipts = rows), () => undefined);
      void boxes.read();
    });
  });

  // Back from the provider: its answer is handed on once, then the address drops the code and the
  // state - on whatever section, and on a phone's list of sections too.
  $effect(() => {
    const answer = providerAnswer;
    if (!answer?.['state']) return;
    untrack(() => void boxes.complete(answer, () => onanswered?.()));
  });

  // A name on the receiving domain is suggested from the workspace's own, until somebody types one.
  const suggested = $derived.by(() => {
    const own = workspace.workspace?.slug || workspace.workspace?.display_name;
    return loaded?.domain && !loaded.name && own ? suggestedName(own) : '';
  });
  $effect(() => {
    if (suggested && name === '') name = suggested;
  });

  /**
   * How many of each section's values differ from what is saved: its save bar's count. A suggested
   * name is not a change of the reader's; it is written with the first save that has one.
   */
  const changed = $derived.by((): Record<'intake' | 'senders', number> => {
    const held = loaded;
    if (!held) return { intake: 0, senders: 0 };
    const count = (flags: boolean[]) => flags.filter(Boolean).length;
    return {
      intake: count([
        enabled !== held.enabled,
        name.trim() !== (held.name ?? suggested),
        defaultPlace !== (held.default_collection_id ?? ''),
      ]),
      senders: count([
        level !== held.senders.level,
        linesOf(allowed).join('\n') !== held.senders.allowed.join('\n'),
        linesOf(blocked).join('\n') !== held.senders.blocked.join('\n'),
      ]),
    };
  });
  /** Every section's unsaved values: what the leave guard holds, as a move between sections keeps them. */
  $effect(() => leaveGuard.hold(() => changed.intake + changed.senders));

  const SECTIONS = [
    { id: 'mailboxes', code: 'app.mail.section_mailboxes' },
    { id: 'intake', code: 'app.mail.section_on' },
    { id: 'senders', code: 'app.mail.section_senders' },
    { id: 'rules', code: 'app.mail.section_routes' },
    { id: 'handles', code: 'app.mail.section_handles' },
    { id: 'arrived', code: 'app.mail.section_log' },
  ] as const;
  const isKnown = (id: string | undefined): boolean => id !== undefined && SECTIONS.some((each) => each.id === id);
  const shown = $derived(isKnown(section) ? (section as string) : 'mailboxes');
  const view = $derived<'list' | 'detail'>(isKnown(section) ? 'detail' : 'list');

  $effect(() => untrack(() => containers.start()));
  $effect(() => {
    const stops = containers.hubs.map((hub) => untrack(() => containers.openLevel(hub.id)));
    return () => stops.forEach((stop) => stop());
  });

  /** Every collection, by hub, with its handle: the pickers and the overview of handles. */
  const collections = $derived(
    containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: collection.id,
        label: `${hub.name} · ${collection.name}`,
        handle: collection.handle ?? '',
      })),
    ),
  );

  const levels = $derived([
    { value: 'MEMBERS', label: t('app.mail.level_members'), hint: t('app.mail.level_members_hint') },
    { value: 'MEMBERS_AND_ALLOWED', label: t('app.mail.level_allowed'), hint: t('app.mail.level_allowed_hint') },
    { value: 'ANYONE', label: t('app.mail.level_anyone'), hint: t('app.mail.level_anyone_hint') },
  ]);

  const figureText = $derived.by(() => {
    if (mailboxes.length === 0) return '';
    const counts = mailboxFigure(mailboxes);
    return [
      ...(counts.reading > 0 ? [t('app.mail.figure_reading', { count: String(counts.reading) })] : []),
      ...(counts.failing > 0 ? [t('app.mail.figure_failing', { count: String(counts.failing) })] : []),
      ...(counts.off > 0 ? [t('app.mail.figure_off', { count: String(counts.off) })] : []),
    ].join(' · ');
  });

  const address = $derived(loaded?.domain && name ? `${name}@${loaded.domain}` : undefined);
  const handled = $derived(collections.filter((collection) => collection.handle));

  function statusOf(id: string): Pick<SectionLink, 'status' | 'statusTone'> {
    const held = loaded;
    if (!held) return {};
    const count = (n: number): Pick<SectionLink, 'status' | 'statusTone'> => (n > 0 ? { status: t('app.mail.status_count', { count: String(n) }), statusTone: 'set' } : { status: t('app.mail.status_none') });
    switch (id) {
      case 'mailboxes':
        return mailboxFigure(mailboxes).failing > 0 ? { status: figureText, statusTone: 'danger' } : count(mailboxes.length);
      case 'intake':
        return held.enabled ? { status: t('app.mail.status_on'), statusTone: 'set' } : { status: t('app.mail.status_off') };
      case 'senders':
        return { status: levels.find((each) => each.value === held.senders.level)?.label ?? '' };
      case 'rules':
        return count(held.sender_routes.length);
      case 'handles':
        return count(handled.length);
      default:
        return count(receipts.length);
    }
  }

  const title = $derived(t('app.mail.title'));
  const links = $derived<SectionLink[]>(SECTIONS.map((each) => ({ id: each.id, label: t(each.code), href: `${BASE}/${each.id}`, ...statusOf(each.id) })));
  const head = $derived({
    title,
    subtitle: t('app.mail.intro'),
    breadcrumb: adminTrail({ id: 'mail', label: title }),
    primary: mayShape && loaded ? { label: t('app.mail.mailbox_add'), icon: 'plus' as const, onclick: () => (boxes.isAdding = true) } : undefined,
    figure: figureText ? figure : undefined,
  });
  $effect(() => page.entitle(title));

  const ruleColumns: readonly DataColumn[] = $derived([
    { id: 'match', label: t('app.mail.route_match'), priority: 1 },
    { id: 'place', label: t('app.mail.route_place'), priority: 2 },
  ]);
  const handleColumns: readonly DataColumn[] = $derived([
    { id: 'collection', label: t('app.mail.route_place'), priority: 1 },
    { id: 'handle', label: t('app.mail.handle_column'), priority: 2 },
    ...(mailboxes[0]?.plus_addresses ? [{ id: 'through', label: t('app.mail.handle_through'), priority: 3 as const }] : []),
  ]);
  const logColumns: readonly DataColumn[] = $derived([
    { id: 'subject', label: t('app.mail.log_subject'), priority: 1 },
    { id: 'from', label: t('app.mail.log_from'), priority: 2 },
    { id: 'when', label: t('app.mail.log_when'), priority: 3 },
    { id: 'became', label: t('app.mail.log_became'), isStatus: true },
  ]);
  const placeOf = (id: string) => collections.find((collection) => collection.value === id)?.label ?? id;

  const rules = $derived<SenderRule[]>((loaded?.sender_routes ?? []).map((route) => ({ match: route.match, collection: route.collection_id })));

  /**
   * Writes one section's values over what is saved, the sender rules as given; the other
   * sections' drafts stay as they are.
   */
  async function save(scope: Scope, nextRules: readonly SenderRule[] = rules): Promise<boolean> {
    const held = loaded;
    if (!held) return false;
    saveFailure = undefined;
    isSaving = true;
    try {
      take(
        await saveMailSettings({
          enabled: scope === 'intake' ? enabled : held.enabled,
          name: (scope === 'intake' ? name.trim() : held.name ?? (suggested || '')) || null,
          default_collection_id: (scope === 'intake' ? defaultPlace : held.default_collection_id ?? '') || null,
          senders: scope === 'senders'
            ? { level, allowed: linesOf(allowed), blocked: linesOf(blocked) }
            : { level: held.senders.level, allowed: [...held.senders.allowed], blocked: [...held.senders.blocked] },
          sender_routes: nextRules.map((rule) => ({ match: rule.match, collection_id: rule.collection })),
        }),
        scope === 'rules' ? [] : [scope],
      );
      announcer.say(t('app.mail.saved'));
      return true;
    } catch (error) {
      saveFailure = renderProblem(error as never, messages).message;
      return false;
    } finally {
      isSaving = false;
    }
  }

  /** The rule a dialog shows: `null` a new one, an index one pressed, `undefined` none. */
  let editing = $state<number | null | undefined>(undefined);
  const editedRule = $derived(editing === null ? null : editing === undefined ? undefined : rules[editing]);

  async function saveRule(next: SenderRule): Promise<void> {
    const at = editing;
    const nextRules = at === null || at === undefined ? [...rules, next] : rules.map((rule, index) => (index === at ? next : rule));
    if (await save('rules', nextRules)) editing = undefined;
  }

  async function removeRule(index: number): Promise<void> {
    const gone = rules[index];
    if (!gone) return;
    if (await save('rules', rules.filter((_, each) => each !== index))) announcer.say(t('app.mail.route_removed', { match: gone.match }));
  }

  async function copy(text: string) {
    try {
      await navigator.clipboard.writeText(text);
      announcer.say(t('app.mail.copied_announced'));
    } catch {
      announcer.say(t('app.mail.copy_failed', { address: text }));
    }
  }

  function outcomeOf(receipt: MailReceipt): string {
    const became = t(`app.mail.outcome.${receipt.outcome}`);
    return receipt.reason ? `${became} · ${t(`app.mail.reason.${receipt.reason}`)}` : became;
  }

  function hrefOf(receipt: MailReceipt): string | undefined {
    if (receipt.item_id) return `/items/${receipt.item_id}`;
    if (receipt.entry_id) return '/jumble';
    return undefined;
  }
</script>

{#snippet figure()}<span class="figure" data-mail-figure>{figureText}</span>{/snippet}

{#snippet saveBar(scope: 'intake' | 'senders')}
  <SaveBar
    label={t('app.save_bar.label')}
    changesLabel={changed[scope] > 0 ? t('app.save_bar.changes', { count: String(changed[scope]) }) : undefined}
    discardLabel={t('app.save_bar.discard')}
    saveLabel={t('app.mail.save')}
    {isSaving}
    savingLabel={t('app.mail.saving')}
    onDiscard={() => {
      saveFailure = undefined;
      if (loaded) take(loaded, [scope]);
    }}
    onSave={() => void save(scope)}
  />
{/snippet}
{#snippet intakeBar()}{@render saveBar('intake')}{/snippet}
{#snippet sendersBar()}{@render saveBar('senders')}{/snippet}

{#snippet orderRows()}
  <ol class="order" data-mail-order>
    <li>{t('app.mail.order_key')}</li>
    <li>{t('app.mail.order_handle')}</li>
    <li>{t('app.mail.order_sender')}</li>
    <li>{t('app.mail.order_default')}</li>
    <li>{t('app.mail.order_jumble')}</li>
  </ol>
{/snippet}

<SettingsLayout
  {head}
  nav={{ label: t('app.settings_page.sections', { name: title }), sections: links, current: shown, onselect: (next) => onreplace?.(`${BASE}/${next}`) }}
  {view}
  back={{ label: title, onback: () => onreplace?.(BASE) }}
>
  <WorkspaceGate permission="AUTOMATION">
    {#snippet actions()}
      {#if readFailure}
        <FailureNote problem={readFailure} />
      {:else if !loaded}
        <p class="quiet"><Spinner label={t('app.mail.reading')} /> {t('app.mail.reading')}</p>
      {:else}
        {#if saveFailure && editing === undefined}<FailureNote problem={saveFailure} />{/if}
        {#if shown === 'mailboxes'}
          <MailboxSection list={boxes} />
        {:else if shown === 'intake'}
          <div data-mail-on>
            <SettingsSection title={t('app.mail.section_on')} save={intakeBar} advanced={{ label: t('app.mail.section_order'), rows: orderRows }}>
              <SettingRow label={t('app.mail.enabled')} help={t('app.mail.enabled_hint')} fieldWidth="xs" isChanged={enabled !== loaded.enabled}>
                {#snippet control()}<Switch label={t('app.mail.enabled')} isLabelHidden bind:checked={enabled} />{/snippet}
              </SettingRow>
              {#if loaded.domain}
                <SettingRow label={t('app.mail.name')} help={t('app.mail.name_hint', { domain: loaded.domain })} fieldWidth="md" isChanged={name.trim() !== (loaded.name ?? suggested)}>
                  {#snippet control(nameId)}
                    <Input label={t('app.mail.name')} isLabelHidden aria-labelledby={nameId} bind:value={name} />
                    {#if address}
                      <span class="value" data-mail-address>
                        <code>{address}</code>
                        <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: address })} onclick={() => void copy(address)} />
                      </span>
                    {/if}
                  {/snippet}
                </SettingRow>
              {:else}
                <SettingRow label={t('app.mail.address')} help={t('app.mail.no_domain')} fieldWidth="md">
                  {#snippet control()}<span class="quiet small" data-mail-no-domain>—</span>{/snippet}
                </SettingRow>
              {/if}
              <SettingRow label={t('app.mail.default_place')} help={t('app.mail.default_place_hint')} fieldWidth="md" isChanged={defaultPlace !== (loaded.default_collection_id ?? '')}>
                {#snippet control(nameId)}
                  <Select label={t('app.mail.default_place')} isLabelHidden aria-labelledby={nameId} placeholder={t('app.mail.to_jumble')} options={collections} bind:value={defaultPlace} />
                {/snippet}
              </SettingRow>
            </SettingsSection>
          </div>
        {:else if shown === 'senders'}
          <div data-mail-senders>
            <SettingsSection title={t('app.mail.section_senders')} save={sendersBar}>
              <!-- One choice of three, each with its sentence: wider than a row's control column. -->
              <div class="pad">
                <Radio label={t('app.mail.level')} options={levels} bind:value={() => level, (next) => (level = next as SenderLevel)} />
              </div>
              {#if level === 'MEMBERS_AND_ALLOWED'}
                <SettingRow label={t('app.mail.allowed')} help={t('app.mail.list_hint')} fieldWidth="lg" isChanged={linesOf(allowed).join('\n') !== loaded.senders.allowed.join('\n')}>
                  {#snippet control(nameId)}<Textarea label={t('app.mail.allowed')} isLabelHidden aria-labelledby={nameId} rows={3} bind:value={allowed} />{/snippet}
                </SettingRow>
              {/if}
              <SettingRow label={t('app.mail.blocked')} help={t('app.mail.blocked_hint')} fieldWidth="lg" isChanged={linesOf(blocked).join('\n') !== loaded.senders.blocked.join('\n')}>
                {#snippet control(nameId)}<Textarea label={t('app.mail.blocked')} isLabelHidden aria-labelledby={nameId} rows={2} bind:value={blocked} />{/snippet}
              </SettingRow>
            </SettingsSection>
          </div>
        {:else if shown === 'rules'}
          <div data-mail-routes>
            <TableSection title={t('app.mail.section_routes')} note={String(rules.length)} description={t('app.mail.routes_hint')}>
              {#snippet acts()}
                <Button icon="plus" data-list-add onclick={() => (editing = null)}>{t('app.mail.route_add')}</Button>
              {/snippet}
              <DataTable
                label={t('app.mail.section_routes')}
                columns={ruleColumns}
                rows={rules}
                key={(rule: SenderRule) => rule.match}
                onopen={(rule: SenderRule) => (editing = rules.indexOf(rule))}
                rowLabel={(rule: SenderRule) => rule.match}
                menu={(rule: SenderRule) => ({
                  label: t('app.page.more_for', { name: rule.match }),
                  items: [
                    { id: 'edit', label: t('app.page.edit') },
                    { id: 'remove', label: t('app.mail.route_remove_menu'), isDestructive: true, hasSeparatorBefore: true },
                  ],
                })}
                onmenu={(rule: SenderRule, id: string) => (id === 'remove' ? void removeRule(rules.indexOf(rule)) : (editing = rules.indexOf(rule)))}
                actionsLabel={t('app.page.row_actions')}
                sheetDismissLabel={t('app.page.sheet_close')}
                sheetGrabLabel={t('app.page.sheet_grab')}
              >
                {#snippet cell(rule: SenderRule, column: DataColumn)}
                  {#if column.id === 'match'}<span class="name" data-mail-rule={rule.match}>{rule.match}</span>{:else}<span class="quiet">{placeOf(rule.collection)}</span>{/if}
                {/snippet}
                {#snippet empty()}<EmptyState kind="unused" title={t('app.mail.routes_empty')} />{/snippet}
              </DataTable>
            </TableSection>
          </div>
        {:else if shown === 'handles'}
          <div data-mail-handles>
            <TableSection title={t('app.mail.section_handles')} note={String(handled.length)} description={t('app.mail.handles_hint')}>
              <DataTable
                label={t('app.mail.section_handles')}
                columns={handleColumns}
                rows={handled}
                key={(collection: { value: string }) => collection.value}
                rowLabel={(collection: { label: string }) => collection.label}
              >
                {#snippet cell(collection: { value: string; label: string; handle: string }, column: DataColumn)}
                  {#if column.id === 'collection'}
                    <span class="name">{collection.label}</span>
                  {:else if column.id === 'handle'}
                    <span class="value" data-handle={collection.handle}>
                      <code>#{collection.handle}</code>
                      <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: `#${collection.handle}` })} onclick={() => void copy(`#${collection.handle}`)} />
                    </span>
                  {:else if mailboxes[0]}
                    {@const through = mailboxRouteTo(collection.handle, mailboxes[0])}
                    <span class="value" data-handle-through={collection.handle}>
                      <code data-plus-address>{through}</code>
                      <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: through })} onclick={() => void copy(through)} />
                    </span>
                  {/if}
                {/snippet}
                {#snippet empty()}<EmptyState kind="unused" title={t('app.mail.handles_empty')} />{/snippet}
              </DataTable>
            </TableSection>
          </div>
        {:else if shown === 'arrived'}
          <div data-mail-log>
            <TableSection title={t('app.mail.section_log')} note={String(receipts.length)}>
              <DataTable
                label={t('app.mail.section_log')}
                columns={logColumns}
                rows={receipts}
                key={(receipt: MailReceipt) => receipt.id}
                rowLabel={(receipt: MailReceipt) => receipt.subject || receipt.sender}
                shownLabel={receipts.length === 0 ? undefined : t('app.page.showing', { count: String(receipts.length) })}
              >
                {#snippet cell(receipt: MailReceipt, column: DataColumn)}
                  {@const href = hrefOf(receipt)}
                  {#if column.id === 'subject'}
                    <span class="name" data-receipt={receipt.outcome}>{receipt.subject || '—'}</span>
                  {:else if column.id === 'from'}
                    <span class="quiet">{receipt.sender}</span>
                  {:else if column.id === 'when'}
                    <span class="quiet">{formatDateTime(receipt.received_at, messages.locale)}</span>
                  {:else if href}
                    <a {href}>{outcomeOf(receipt)}</a>
                  {:else}
                    <span>{outcomeOf(receipt)}</span>
                  {/if}
                {/snippet}
                {#snippet empty()}<EmptyState kind="unused" title={t('app.mail.log_empty')} />{/snippet}
              </DataTable>
            </TableSection>
          </div>
        {/if}

        <SenderRuleDialog
          rule={editedRule}
          {collections}
          {defaultPlace}
          isWorking={isSaving}
          failure={editing === undefined ? undefined : saveFailure}
          onClose={() => { editing = undefined; saveFailure = undefined; }}
          onsave={(next) => void saveRule(next)}
        />
      {/if}
    {/snippet}
  </WorkspaceGate>
</SettingsLayout>

<!-- Outside the layout: the head's act opens it while a phone shows only the list of sections. -->
<ConnectMailboxDialog
  bind:isOpen={boxes.isAdding}
  presets={boxes.presets}
  oauthProviders={boxes.oauthProviders}
  redirectUrl={boxes.redirectUrl}
  oauthFailure={boxes.oauthFailure}
  onconnected={(box) => boxes.add(box)}
/>
<MailboxDialog
  box={boxes.opened}
  isChecking={boxes.working !== undefined && boxes.working === boxes.openedId}
  onClose={() => (boxes.openedId = undefined)}
  oncheck={(box) => void boxes.check(box)}
  onremove={(box) => boxes.askRemoval(box)}
/>
<RemoveMailboxDialog
  box={boxes.removing}
  isWorking={boxes.working !== undefined && boxes.working === boxes.removing?.id}
  failure={boxes.rowFailure}
  onClose={() => (boxes.removing = undefined)}
  onremove={(box) => void boxes.remove(box)}
/>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .pad { padding: var(--sp-200) var(--sp-250); }

  .figure { color: var(--text-secondary); font-variant-numeric: tabular-nums; }

  .small { font-size: var(--fs-075); }

  .name { color: var(--text-primary); font-weight: var(--fw-medium); overflow-wrap: anywhere; }

  .order { display: grid; gap: var(--sp-050); margin: 0; padding-block: var(--sp-100); padding-inline: var(--sp-500) var(--sp-250); }

  .value { display: inline-flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .value code {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--bg-recess);
    overflow-wrap: anywhere;
  }
</style>
