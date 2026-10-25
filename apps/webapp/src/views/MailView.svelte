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
  // **Who may write is one choice with two lists.** Members by default; members and allowed
  // addresses and domains; or anyone. The block list applies before every level.

  import { untrack } from 'svelte';

  import {
    Banner,
    Button,
    IconButton,
    Input,
    PageHeader,
    Radio,
    Select,
    Spinner,
    Stack,
    Switch,
    Textarea,
  } from '@hubtask/design-system/components';

  import { announcer } from '../lib/announce.svelte.ts';
  import { holdsHere } from '../lib/data/capability.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import {
    linesOf,
    mailboxRouteTo,
    readMailReceipts,
    readMailSettings,
    saveMailSettings,
    suggestedName,
    type Mailbox,
    type MailReceipt,
    type MailSettings,
    type SenderLevel,
  } from '../lib/data/mailintake.svelte.ts';
  import { workspace } from '../lib/data/workspace.svelte.ts';
  import { viewport } from '../lib/frame/viewport.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';
  import WorkspaceGate from '../lib/workspace/WorkspaceGate.svelte';
  import MailboxSection from '../lib/mail/MailboxSection.svelte';

  interface DraftRoute {
    key: string;
    match: string;
    collection: string;
  }

  let loaded = $state<MailSettings | undefined>(undefined);
  let readFailure = $state<string | undefined>(undefined);
  let enabled = $state(false);
  let name = $state('');
  let defaultPlace = $state('');
  let level = $state<SenderLevel>('MEMBERS');
  let allowed = $state('');
  let blocked = $state('');
  let routes = $state<DraftRoute[]>([]);
  let isSaving = $state(false);
  let saveFailure = $state<string | undefined>(undefined);
  let receipts = $state<readonly MailReceipt[]>([]);
  /** The connected mailboxes, for the addresses a collection is reached at through one. */
  let mailboxes = $state<readonly Mailbox[]>([]);

  const mayShape = $derived(holdsHere('AUTOMATION') === true);

  function take(settings: MailSettings) {
    loaded = settings;
    enabled = settings.enabled;
    name = settings.name ?? '';
    defaultPlace = settings.default_collection_id ?? '';
    level = settings.senders.level;
    allowed = settings.senders.allowed.join('\n');
    blocked = settings.senders.blocked.join('\n');
    routes = settings.sender_routes.map((route) => ({ key: crypto.randomUUID(), match: route.match, collection: route.collection_id }));
  }

  $effect(() => {
    if (!mayShape) return;
    untrack(() => {
      void readMailSettings()
        .then((settings) => settings && take(settings))
        .catch((error) => (readFailure = renderProblem(error as never, messages).message));
      void readMailReceipts().then((rows) => (receipts = rows), () => undefined);
    });
  });

  // A name on the receiving domain is suggested from the workspace's own, until somebody types one.
  $effect(() => {
    const own = workspace.workspace?.slug || workspace.workspace?.display_name;
    if (loaded?.domain && !loaded.name && name === '' && own) {
      name = suggestedName(own);
    }
  });

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

  const address = $derived(loaded?.domain && name ? `${name}@${loaded.domain}` : undefined);

  function addRoute() {
    routes = [...routes, { key: crypto.randomUUID(), match: '', collection: defaultPlace || (collections[0]?.value ?? '') }];
  }

  function removeRoute(key: string) {
    routes = routes.filter((route) => route.key !== key);
  }

  async function save() {
    saveFailure = undefined;
    isSaving = true;
    try {
      take(
        await saveMailSettings({
          enabled,
          name: name.trim() || null,
          default_collection_id: defaultPlace || null,
          senders: { level, allowed: linesOf(allowed), blocked: linesOf(blocked) },
          sender_routes: routes
            .filter((route) => route.match.trim() !== '')
            .map((route) => ({ match: route.match.trim(), collection_id: route.collection })),
        }),
      );
      announcer.say(t('app.mail.saved'));
    } catch (error) {
      saveFailure = renderProblem(error as never, messages).message;
    } finally {
      isSaving = false;
    }
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

<Stack gap="300">
  <PageHeader
    title={t('app.mail.title')}
    isTitleInBar={viewport.isCompact}
    breadcrumb={{
      trail: [
        { id: 'administration', label: t('app.admin.title'), href: '/administration' },
        { id: 'mail', label: t('app.mail.title') },
      ],
      label: t('app.admin.trail'),
      expandLabel: t('app.admin.expand_trail'),
    }}
  />
  <p class="quiet">{t('app.mail.intro')}</p>

  <WorkspaceGate permission="AUTOMATION">
    {#snippet actions()}
      {#if readFailure}
        <Banner tone="danger">{readFailure}</Banner>
      {:else if !loaded}
        <p class="quiet"><Spinner label={t('app.mail.reading')} /> {t('app.mail.reading')}</p>
      {:else}
        <Stack gap="400">
          <section class="panel" aria-labelledby="mail-on" data-mail-on>
            <Stack gap="150">
              <h2 class="section" id="mail-on">{t('app.mail.section_on')}</h2>
              <Switch label={t('app.mail.enabled')} hint={t('app.mail.enabled_hint')} bind:checked={enabled} />
              {#if loaded.domain}
                <div class="fields">
                  <Input label={t('app.mail.name')} hint={t('app.mail.name_hint', { domain: loaded.domain })} bind:value={name} />
                </div>
                {#if address}
                  <div class="value" data-mail-address>
                    <span class="value-label">{t('app.mail.address')}</span>
                    <code>{address}</code>
                    <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: address })} onclick={() => void copy(address)} />
                  </div>
                {/if}
              {:else}
                <p class="quiet small" data-mail-no-domain>{t('app.mail.no_domain')}</p>
              {/if}
              <div class="fields">
                <Select label={t('app.mail.default_place')} hint={t('app.mail.default_place_hint')} placeholder={t('app.mail.to_jumble')} options={collections} bind:value={defaultPlace} />
              </div>
            </Stack>
          </section>

          <section class="panel" aria-labelledby="mail-mailboxes" data-mail-mailboxes>
            <Stack gap="150">
              <h2 class="section" id="mail-mailboxes">{t('app.mail.section_mailboxes')}</h2>
              <MailboxSection onchange={(next) => (mailboxes = next)} />
            </Stack>
          </section>

          <section class="panel" aria-labelledby="mail-senders" data-mail-senders>
            <Stack gap="150">
              <h2 class="section" id="mail-senders">{t('app.mail.section_senders')}</h2>
              <Radio label={t('app.mail.level')} options={levels} bind:value={() => level, (next) => (level = next as SenderLevel)} />
              {#if level === 'MEMBERS_AND_ALLOWED'}
                <Textarea label={t('app.mail.allowed')} hint={t('app.mail.list_hint')} rows={3} bind:value={allowed} />
              {/if}
              <Textarea label={t('app.mail.blocked')} hint={t('app.mail.blocked_hint')} rows={2} bind:value={blocked} />
            </Stack>
          </section>

          <section class="panel" aria-labelledby="mail-routes" data-mail-routes>
            <Stack gap="150">
              <h2 class="section" id="mail-routes">{t('app.mail.section_routes')}</h2>
              <p class="quiet small">{t('app.mail.routes_hint')}</p>
              {#each routes as route (route.key)}
                <div class="row" data-mail-route>
                  <Input label={t('app.mail.route_match')} bind:value={route.match} />
                  <Select label={t('app.mail.route_place')} options={collections} bind:value={route.collection} />
                  <IconButton icon="trash" label={t('app.mail.route_remove', { match: route.match || '…' })} onclick={() => removeRoute(route.key)} />
                </div>
              {/each}
              <div><Button tone="secondary" size="sm" icon="plus" onclick={addRoute}>{t('app.mail.route_add')}</Button></div>
            </Stack>
          </section>

          {#if saveFailure}<Banner tone="danger">{saveFailure}</Banner>{/if}
          <div><Button tone="primary" isBusy={isSaving} busyLabel={t('app.mail.saving')} onclick={() => void save()}>{t('app.mail.save')}</Button></div>

          <section class="panel" aria-labelledby="mail-order" data-mail-order>
            <Stack gap="150">
              <h2 class="section" id="mail-order">{t('app.mail.section_order')}</h2>
              <ol class="order">
                <li>{t('app.mail.order_key')}</li>
                <li>{t('app.mail.order_handle')}</li>
                <li>{t('app.mail.order_sender')}</li>
                <li>{t('app.mail.order_default')}</li>
                <li>{t('app.mail.order_jumble')}</li>
              </ol>
            </Stack>
          </section>

          <section class="panel" aria-labelledby="mail-handles" data-mail-handles>
            <Stack gap="150">
              <h2 class="section" id="mail-handles">{t('app.mail.section_handles')}</h2>
              <p class="quiet small">{t('app.mail.handles_hint')}</p>
              <ul class="handles">
                {#each collections.filter((collection) => collection.handle) as collection (collection.value)}
                  <li class="value" data-handle={collection.handle}>
                    <span class="value-label">{collection.label}</span>
                    <code>#{collection.handle}</code>
                    <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: `#${collection.handle}` })} onclick={() => void copy(`#${collection.handle}`)} />
                    {#if mailboxes[0]?.plus_addresses}
                      {@const through = mailboxRouteTo(collection.handle, mailboxes[0])}
                      <code data-plus-address>{through}</code>
                      <IconButton icon="copy" size="sm" label={t('app.mail.copy_named', { name: through })} onclick={() => void copy(through)} />
                    {/if}
                  </li>
                {/each}
              </ul>
            </Stack>
          </section>

          <section class="panel" aria-labelledby="mail-log" data-mail-log>
            <Stack gap="150">
              <h2 class="section" id="mail-log">{t('app.mail.section_log')}</h2>
              {#if receipts.length === 0}
                <p class="quiet small">{t('app.mail.log_empty')}</p>
              {:else}
                <ul class="log" class:compact={viewport.isCompact}>
                  {#each receipts as receipt (receipt.id)}
                    {@const href = hrefOf(receipt)}
                    <li data-receipt={receipt.outcome}>
                      <span class="when">{formatDateTime(receipt.received_at, messages.locale)}</span>
                      <span class="who">{receipt.sender}</span>
                      <span class="what">{receipt.subject || '—'}</span>
                      {#if href}<a {href}>{outcomeOf(receipt)}</a>{:else}<span>{outcomeOf(receipt)}</span>{/if}
                    </li>
                  {/each}
                </ul>
              {/if}
            </Stack>
          </section>
        </Stack>
      {/if}
    {/snippet}
  </WorkspaceGate>
</Stack>

<style>
  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .section { margin: 0; font-family: var(--font-display); font-size: var(--fs-200); font-weight: var(--fw-semibold); }

  .panel {
    padding: var(--sp-200);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
  }

  .fields { max-inline-size: calc(var(--sp-1000) * 6); }

  .row { display: flex; flex-wrap: wrap; align-items: end; gap: var(--sp-100); }

  .row > :global(*:not(:last-child)) { flex: 1 1 calc(var(--sp-1000) * 2.5); min-inline-size: 0; }

  .order { display: grid; gap: var(--sp-050); margin: 0; padding-inline-start: var(--sp-300); }

  .handles, .log { display: grid; gap: var(--sp-100); margin: 0; padding: 0; list-style: none; }

  .value { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100); min-inline-size: 0; }

  .value-label { flex: 0 0 calc(var(--sp-1000) * 2); color: var(--text-secondary); font-size: var(--fs-075); }

  .value code {
    font-family: var(--font-mono);
    font-size: var(--fs-075);
    padding: var(--sp-025) var(--sp-100);
    border-radius: var(--r-sm);
    background: var(--bg-recess);
    overflow-wrap: anywhere;
  }

  .log li {
    display: grid;
    grid-template-columns: auto minmax(0, 1fr) minmax(0, 2fr) auto;
    gap: var(--sp-150);
    align-items: baseline;
    font-size: var(--fs-075);
  }

  .when, .who { color: var(--text-secondary); }

  .who, .what { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

  .log.compact li { grid-template-columns: minmax(0, 1fr); gap: var(--sp-025); }
</style>
