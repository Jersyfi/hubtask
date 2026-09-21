<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rule editor: a rule drawn as a path (F8-04, `milestone-F8.md` decisions 1-4, 10, 11).
  //
  // **The view holds one draft**, and everything else draws from it: the head with the name and
  // the sentence, the canvas, the inspector. An edit is a function over the draft; nothing edits
  // in place, so the three surfaces cannot disagree.
  //
  // **The name is generated unless it is owned** (decision 4). A stored name equal to the one the
  // rule generates for itself counts as automatic and follows every edit; any other is the
  // person's and is left alone; clearing it returns to automatic.
  //
  // **Every list is read rather than compiled in.** The triggers, the action kinds, their fields
  // and the event types come from `/meta/capabilities`, so an installation that serves one more
  // action gets one more card without a release of this client.
  //
  // **Nothing is pre-empted.** Writing a rule needs the automation permission *and* the rights the
  // rule's own actions need, which no client can compute; the server refuses and this renders the
  // refusal at the field it names.

  import { untrack } from 'svelte';

  import { Badge, Banner, Button, Dialog, Drawer, Icon, OneTimeSecret, Spinner, Tabs } from '@hubtask/design-system/components';

  import RuleCanvas from '../lib/automation/RuleCanvas.svelte';
  import BlocksPanel from '../lib/automation/BlocksPanel.svelte';
  import RuleInspector from '../lib/automation/RuleInspector.svelte';
  import RuleProbe from '../lib/automation/RuleProbe.svelte';
  import RuleRuns from '../lib/automation/RuleRuns.svelte';
  import { framesOfRun, framesOfTest, type Frame, type Outcome, type Verdict } from '../lib/automation/probe.ts';
  import type { Choice } from '../lib/automation/ActionForm.svelte';
  import { addRung, canPlace, emptyDraft, endsRun, fromRule, insertAt, isAutomatic, listAt, moveStep, newStep, nudge, removeAt, replaceAt, stepAt, toRuleDraft, walk, type Draft, type Step } from '../lib/automation/model.ts';
  import { DRAG_TYPE, dragHint, type Drag, type Selection } from '../lib/automation/selection.ts';
  import { REFERENCE, eventWords, generatedName, sentence, usageOf, type Names } from '../lib/automation/words.ts';
  import { findingWords, marksOf } from '../lib/automation/findings.ts';
  import { accounts } from '../lib/data/accounts.svelte.ts';
  import { buckets } from '../lib/data/buckets.svelte.ts';
  import { manifest } from '../lib/data/capabilities.svelte.ts';
  import { containers } from '../lib/data/containers.svelte.ts';
  import { groups } from '../lib/data/groups.svelte.ts';
  import { labels } from '../lib/data/labels.svelte.ts';
  import { people } from '../lib/data/people.svelte.ts';
  import { rules, type InboundToken } from '../lib/data/rules.svelte.ts';
  import { inboundAddressOf } from '../lib/data/rules.ts';
  import { runs, type Run } from '../lib/data/runs.svelte.ts';
  import { serviceAccounts } from '../lib/data/serviceaccounts.svelte.ts';
  import { templates } from '../lib/data/templates.svelte.ts';
  import { webhooks } from '../lib/data/webhooks.svelte.ts';
  import { announcer } from '../lib/announce.svelte.ts';
  import { formatDateTime } from '../lib/i18n/datetime.ts';
  import { messages, t } from '../lib/i18n/i18n.svelte.ts';
  import { renderProblem } from '../lib/problem.ts';

  interface Props {
    /** The rule's identifier, or `new`. */
    id: string;
    onnavigate: (path: string) => void;
  }

  const { id, onnavigate }: Props = $props();

  const TENANT = { scopeType: 'TENANT' } as const;
  const isNew = $derived(id === 'new');

  // What the first paint needs, and nothing more (issue 818): the rules, the hubs and their
  // collections for the scope's name, the service accounts for the runner's. Everything a form or
  // a name might need later - the memberships, the groups, the templates, the webhooks, each
  // collection's labels and buckets - is opened when a field of the rule or the panel names its
  // kind (`needed`, below), so that a deep link never meets the credential's burst on one screen.
  $effect(() => untrack(() => rules.open()));
  $effect(() => untrack(() => containers.start()));
  $effect(() => untrack(() => serviceAccounts.open()));
  $effect(() => {
    for (const hub of containers.hubs) untrack(() => containers.openLevel(hub.id));
  });

  const stored = $derived(isNew ? undefined : rules.all.find((rule) => rule.id === id));
  $effect(() => {
    if (isNew) return;
    return untrack(() => runs.open({ ruleId: id }));
  });
  const ruleRuns = $derived(isNew ? [] : runs.of({ ruleId: id }));
  const reading = $derived(rules.state);

  /** The draft, taken once from the stored rule when it arrives; edits never re-read it. */
  let draft = $state<Draft>(emptyDraft());
  let taken = $state(false);
  $effect(() => {
    if (taken) return;
    if (isNew) {
      const first = manifest.value?.event_types?.[0] ?? '';
      draft = emptyDraft(first);
      taken = true;
    } else if (stored) {
      draft = fromRule(stored);
      taken = true;
    }
  });
  let dirty = $state(false);
  const update = (change: (draft: Draft) => Draft): void => {
    draft = change(draft);
    dirty = true;
  };

  let selection = $state<Selection>({ kind: 'rule' });
  /** The panel's five tabs (decision 16): Rule, Blocks, Details, Probe, Runs. */
  let tab = $state('rule');
  const RULE_TAB: ReadonlySet<Selection['kind']> = new Set(['rule', 'scope', 'runas', 'guardrails']);
  const TABS = [
    { id: 'rule', code: 'app.flow.tab_rule', icon: 'settings' },
    { id: 'blocks', code: 'app.flow.tab_blocks', icon: 'plus' },
    { id: 'piece', code: 'app.flow.tab_piece', icon: 'pencil' },
    { id: 'probe', code: 'app.flow.tab_probe', icon: 'play' },
    { id: 'runs', code: 'app.flow.tab_runs', icon: 'clock' },
  ] as const;

  /* ---------- Narrow: the inspector as a sheet, one arm at a time (F8-05, decision 8) ---------- */

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px) less one; a media query cannot read a custom property. */
  const narrowQuery = typeof matchMedia === 'function' ? matchMedia('(max-width: 904px)') : undefined;
  let narrow = $state(narrowQuery?.matches ?? false);
  $effect(() => {
    if (!narrowQuery) return;
    const onchange = (event: MediaQueryListEvent) => (narrow = event.matches);
    narrowQuery.addEventListener('change', onchange);
    return () => narrowQuery.removeEventListener('change', onchange);
  });
  let sheetOpen = $state(false);
  const select = (next: Selection): void => {
    selection = next;
    // The canvas shows, the panel sets (decision 16): what was clicked opens where it is edited.
    tab = RULE_TAB.has(next.kind) ? 'rule' : 'piece';
    if (narrow) sheetOpen = true;
  };
  let armChoice = $state<Map<string, 'then' | 'else'>>(new Map());
  const pickArm = (path: string, arm: 'then' | 'else'): void => {
    armChoice = new Map([...armChoice, [path, arm]]);
  };

  /* ---------- Drag and drop (decision 7) ---------- */

  let drag = $state<Drag | undefined>(undefined);
  let refusal = $state<string | undefined>(undefined);
  let refusalTimer: ReturnType<typeof setTimeout> | undefined;
  function lift(event: DragEvent, piece: Drag): void {
    if (!event.dataTransfer) return;
    event.dataTransfer.setData(DRAG_TYPE, JSON.stringify(piece));
    event.dataTransfer.effectAllowed = 'copy';
    drag = piece;
  }
  function dropped(list: string, index: number, piece: Drag): void {
    if (piece.src === 'action') insert(list, index, piece.kind);
    else if (piece.src === 'step') {
      const moved = moveStep(draft.actions, piece.path, list, index);
      if (moved) {
        update((current) => ({ ...current, actions: moved }));
        select({ kind: 'step', path: list ? `${list}/${index}` : String(index) });
      } else {
        refuse(piece, list);
      }
    }
    drag = undefined;
  }
  /** Why a piece may not go where it was let go, said in the hint line (decision 20). */
  function refuse(piece: Drag, list?: string): void {
    const stop = (piece.src === 'action' && piece.kind === 'STOP') || (piece.src === 'step' && stepAt(draft.actions, piece.path)?.kind === 'STOP');
    const ended = list !== undefined && endsRun(listAt(draft.actions, list) ?? []);
    refusal = stop ? t('app.flow.refused_stop') : ended ? t('app.flow.refused_after_end') : t(`app.flow.refused_${piece.src}`);
    clearTimeout(refusalTimer);
    refusalTimer = setTimeout(() => (refusal = undefined), 6000);
  }

  /** + Else if: a rung under the ladder, selected so the panel opens on its condition (decision 19). */
  function addElseIf(path: string): void {
    const before = stepAt(draft.actions, path);
    if (!before || before.kind !== 'BRANCH') return;
    update((current) => ({ ...current, actions: addRung(current.actions, path) }));
    // The new rung is the last of the ladder: follow the else arms down to it.
    let at = path;
    let step = stepAt(draft.actions, at);
    while (step && step.else?.length === 1 && step.else[0]?.kind === 'BRANCH') {
      at = `${at}/else/0`;
      step = step.else[0];
    }
    select({ kind: 'step', path: at });
  }
  let sentenceOpen = $state(false);
  try {
    sentenceOpen = localStorage.getItem('hubtask.rule.sentence') === 'open';
  } catch {
    // A private window, or storage blocked: the sentence starts collapsed.
  }
  function toggleSentence(): void {
    sentenceOpen = !sentenceOpen;
    try {
      localStorage.setItem('hubtask.rule.sentence', sentenceOpen ? 'open' : 'closed');
    } catch {
      // The choice lives for this view then.
    }
  }

  /* ---------- What the manifest and the workspace offer ---------- */

  const triggers = $derived(manifest.value?.automation?.triggers ?? []);
  const actionKinds = $derived(manifest.value?.automation?.actions ?? []);
  const actionFields = $derived((manifest.value?.automation?.action_fields ?? {}) as Readonly<Record<string, readonly { name: string; kind: string; required: boolean; enum?: readonly string[]; description?: string }[]>>);
  const eventTypes = $derived(manifest.value?.event_types ?? []);
  const itemTypes = $derived<Choice[]>(
    (manifest.value?.item_types ?? []).flatMap((entry) => (entry.type ? [{ value: String(entry.type), label: String(entry.type) }] : [])),
  );

  const runners = $derived<Choice[]>([
    ...serviceAccounts.all.map((account) => ({ value: account.id, label: t('app.rules.service_account_named', { name: account.display_name }) })),
    ...people.candidates({}).map((accountId) => ({ value: accountId, label: accounts.nameOf(accountId) ?? t('app.people.unnamed') })),
  ]);

  const scopes = $derived<Choice[]>([
    { value: 'TENANT', label: t('app.rules.scope_tenant') },
    ...containers.hubs.map((hub) => ({ value: `HUB:${hub.id}`, label: t('app.rules.scope_hub', { name: hub.name }) })),
    ...containers.hubs.flatMap((hub) =>
      containers.collectionsOf(hub.id).map((collection) => ({
        value: `COLLECTION:${collection.id}`,
        label: t('app.rules.scope_collection', { name: `${hub.name} · ${collection.name}` }),
      })),
    ),
  ]);

  /** The collections the rule's scope sees: what a label or bucket picker draws from. */
  const collectionsInScope = $derived.by(() => {
    if (draft.scope.type === 'COLLECTION' && draft.scope.id) {
      const found = containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id)).find((each) => each.id === draft.scope.id);
      return found ? [found] : [];
    }
    const hubs = draft.scope.type === 'HUB' && draft.scope.id ? containers.hubs.filter((hub) => hub.id === draft.scope.id) : containers.hubs;
    return hubs.flatMap((hub) => containers.collectionsOf(hub.id));
  });
  /**
   * The kinds of thing the rule names or the panel is about to ask for: what decides which stores
   * are read (issue 818). A step's declared fields name their kinds through the reference table;
   * a condition anywhere may name a label, a bucket or an account; the composer and the runner
   * picker need theirs open while they are shown.
   */
  const needed = $derived.by(() => {
    const kinds = new Set<string>();
    const conditions: string[] = [...draft.conditions];
    walk(draft.actions, (step) => {
      for (const field of actionFields[step.kind] ?? []) {
        const reference = REFERENCE[field.name];
        if (reference) kinds.add(reference);
      }
      if (step.kind === 'BRANCH') conditions.push(String(step.params.condition ?? ''));
    });
    for (const expr of conditions) {
      if (expr.includes('item.labels')) kinds.add('label');
      if (expr.includes('item.bucket_id')) kinds.add('bucket');
      if (expr.includes('assignee_id') || expr.includes('actor.id')) kinds.add('account');
    }
    if (selection.kind === 'gate' || selection.kind === 'condition' || (selection.kind === 'step' && stepAt(draft.actions, selection.path)?.kind === 'BRANCH')) {
      kinds.add('label');
      kinds.add('bucket');
      kinds.add('account');
    }
    if (tab === 'rule' || selection.kind === 'runas' || (draft.runAs && !serviceAccounts.all.some((account) => account.id === draft.runAs))) kinds.add('account');
    return kinds;
  });
  $effect(() => {
    if (!needed.has('label') && !needed.has('bucket')) return;
    for (const collection of collectionsInScope) {
      untrack(() => {
        if (needed.has('label')) labels.open(collection.id);
        if (needed.has('bucket')) buckets.open(collection.id);
      });
    }
  });
  $effect(() => {
    if (needed.has('account')) untrack(() => people.openScope(TENANT));
  });
  $effect(() => {
    if (needed.has('group')) untrack(() => groups.open());
  });
  $effect(() => {
    if (needed.has('template')) untrack(() => templates.open());
  });
  $effect(() => {
    if (needed.has('subscription')) untrack(() => webhooks.open());
  });

  const pickers = $derived<Readonly<Record<string, readonly Choice[]>>>({
    label: collectionsInScope.flatMap((collection) => labels.of(collection.id).map((label) => ({ value: label.id, label: collectionsInScope.length > 1 ? `${collection.name} · ${label.name}` : label.name }))),
    bucket: collectionsInScope.flatMap((collection) => buckets.of(collection.id).map((bucket) => ({ value: bucket.id, label: collectionsInScope.length > 1 ? `${collection.name} · ${bucket.name}` : bucket.name }))),
    container: [
      ...containers.hubs.map((hub) => ({ value: hub.id, label: hub.name })),
      ...containers.hubs.flatMap((hub) => containers.collectionsOf(hub.id).map((collection) => ({ value: collection.id, label: `${hub.name} · ${collection.name}` }))),
    ],
    template: templates.of().map((template) => ({ value: template.id, label: template.name })),
    subscription: webhooks.all.map((subscription) => ({ value: subscription.id, label: subscription.target_url })),
    group: groups.all.map((group) => ({ value: group.id, label: group.name })),
    account: runners,
  });

  /* ---------- Words ---------- */

  const words = { t, has: (code: string) => messages.has(code) };
  const names = $derived<Names>({
    scope: (scope) => scopes.find((choice) => choice.value === (scope.id ? `${scope.type}:${scope.id}` : scope.type))?.label ?? scope.type,
    account: (accountId) => runners.find((choice) => choice.value === accountId)?.label ?? accounts.nameOf(accountId) ?? t('app.rules.choose_runner'),
    event: (type) => eventWords(words, type).clause,
    bucket: (bucketId) => pickers.bucket?.find((choice) => choice.value === bucketId)?.label,
    label: (labelId) => pickers.label?.find((choice) => choice.value === labelId)?.label,
  });
  const generated = $derived(generatedName(words, names, draft));
  const automatic = $derived(isAutomatic(draft.name, generated));
  const shownName = $derived(automatic ? generated : draft.name);
  const said = $derived(sentence(words, names, { ...draft, name: shownName }));

  const triggerMeta = $derived.by(() => {
    const trigger = draft.trigger;
    switch (trigger.kind) {
      case 'EVENT':
        return trigger.event_type ? eventWords(words, trigger.event_type).said : t('app.rules.choose_event');
      case 'SCHEDULE':
        return [trigger.rrule, trigger.timezone].filter(Boolean).join(' · ');
      case 'RELATIVE_DATE':
        return `${trigger.offset ?? ''} · ${t(`app.flow.anchor_${(trigger.anchor ?? 'DUE_DATE').toLowerCase()}`)}`;
      case 'INBOUND_WEBHOOK':
        return stored?.inbound_rotated_at ? t('app.rules.inbound_minted', { moment: formatDateTime(stored.inbound_rotated_at, messages.locale) }) : t('app.flow.no_address');
      case 'MANUAL':
        return t('app.flow.manual_hint');
      default:
        return t('app.flow.jumble_hint');
    }
  });

  /** One line under a step's title: its settings, identifiers replaced by their names. */
  function describe(step: Step): string {
    const named = (value: unknown): string => {
      for (const choices of Object.values(pickers)) {
        const found = choices.find((choice) => choice.value === value);
        if (found) return found.label;
      }
      return typeof value === 'object' ? JSON.stringify(value) : String(value);
    };
    return Object.entries(step.params)
      .filter(([, value]) => value !== undefined && value !== '' && value !== null)
      .map(([key, value]) => `${key.replace(/_/g, ' ')}: ${named(value)}`)
      .join(' · ');
  }

  /* ---------- Edits ---------- */

  function insert(list: string, index: number, kind: string): void {
    if (!canPlace(draft.actions, list, index, kind)) {
      // An end anywhere but an arm's last place, or anything after an end (decision 19): said,
      // never silently dropped.
      refuse({ src: 'action', kind }, list);
      return;
    }
    update((current) => ({ ...current, actions: insertAt(current.actions, list, index, newStep(kind)) }));
    select({ kind: 'step', path: list ? `${list}/${index}` : String(index) });
  }

  function nudgeStep(path: string, direction: -1 | 1): void {
    const { list, index } = { list: path.includes('/') ? path.slice(0, path.lastIndexOf('/')) : '', index: Number(path.slice(path.lastIndexOf('/') + 1)) };
    const target = index + direction;
    update((current) => ({ ...current, actions: nudge(current.actions, path, direction) }));
    if (target >= 0) selection = { kind: 'step', path: list ? `${list}/${target}` : String(target) };
  }

  function remove(path: string): void {
    update((current) => ({ ...current, actions: removeAt(current.actions, path) }));
    selection = { kind: 'gate' };
  }

  function replaceTrigger(kind: string): void {
    update((current) => ({ ...current, trigger: { kind, ...(kind === 'EVENT' ? { event_type: current.trigger.event_type ?? eventTypes[0] ?? '' } : {}) } }));
    select({ kind: 'trigger' });
  }

  function fold(path: string): void {
    const step = stepAt(draft.actions, path);
    if (!step) return;
    draft = { ...draft, actions: replaceAt(draft.actions, path, { ...step, collapsed: !step.collapsed }) };
  }

  function addCondition(): void {
    update((current) => ({ ...current, conditions: [...current.conditions, "item.type == 'TASK'"] }));
    select({ kind: 'condition', index: draft.conditions.length - 1 });
  }

  function removeCondition(index: number): void {
    update((current) => ({ ...current, conditions: current.conditions.filter((_, at) => at !== index) }));
    selection = { kind: 'gate' };
  }

  /* ---------- Saving and the switches ---------- */

  let failure = $state<ReturnType<typeof renderProblem> | undefined>(undefined);
  let isWorking = $state(false);
  let isDeleting = $state(false);
  let minted = $state<InboundToken | undefined>(undefined);

  const errors = $derived(failure?.fields ?? new Map<string, string>());

  /** What the check found (ADR-0060), and whether the rule can run at all. */
  const findings = $derived(stored?.findings ?? []);
  const isBroken = $derived(findings.some((finding) => finding.level === 'BROKEN'));

  /** A finding or refusal drawn at a card: the check's findings first, a refusal over them. */
  const marks = $derived.by(() => {
    const found = marksOf(words, findings);
    for (const [pointer, message] of errors) {
      if (pointer.startsWith('/trigger')) found.set('trigger', message);
      const condition = /^\/conditions\/(\d+)/.exec(pointer);
      if (condition) found.set(`conditions/${condition[1]}`, message);
      const step = /^\/actions\/(.+?)(?:\/kind|\/params\/(?!then|else).*)?$/.exec(pointer);
      if (step?.[1]) found.set(step[1].replace(/\/params\/(then|else)/g, '/$1'), message);
    }
    return found;
  });

  async function attempt(work: () => Promise<unknown>, announced?: string): Promise<boolean> {
    failure = undefined;
    isWorking = true;
    try {
      await work();
      if (announced) announcer.say(announced);
      return true;
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
      return false;
    } finally {
      isWorking = false;
    }
  }

  async function save(): Promise<void> {
    if (!draft.runAs) {
      selection = { kind: 'runas' };
      return;
    }
    const body = toRuleDraft(draft, shownName);
    if (isNew) {
      let created: { id: string } | undefined;
      const ok = await attempt(async () => {
        created = await rules.write(body);
      }, t('app.flow.saved_off_announced'));
      if (ok && created) {
        dirty = false;
        onnavigate(`/administration/rules/${created.id}`);
      }
      return;
    }
    if (!stored) return;
    const ok = await attempt(() => rules.change(stored.id, body, stored.version), t('app.flow.saved_announced'));
    if (ok) {
      dirty = false;
      // The write leaves the rule unchecked; the check right after it is what tells the writer
      // whether the repair held, at the card it concerns, rather than at the next opening of the
      // list. Nothing to undo if it fails: the list checks again when it opens.
      await rules.check().catch(() => undefined);
    }
  }

  async function toggle(): Promise<void> {
    if (!stored) return;
    if (stored.enabled) {
      await attempt(() => rules.disable(stored.id), t('app.rules.disabled_announced'));
    } else {
      await attempt(() => rules.enable(stored.id), t('app.rules.enabled_announced'));
    }
  }

  async function remove_rule(): Promise<void> {
    if (!stored) return;
    const ok = await attempt(() => rules.remove(stored.id), t('app.rules.removed_announced'));
    isDeleting = false;
    if (ok) onnavigate('/administration/rules');
  }

  async function mint(): Promise<void> {
    if (!stored) return;
    await attempt(async () => {
      minted = await rules.rotateInbound(stored.id);
    }, t('app.rules.rotated_announced'));
  }

  async function start(): Promise<void> {
    if (!stored) return;
    await attempt(() => runs.trigger(stored.id), t('app.flow.manual_started'));
  }

  /** Each kind's sentence from the manifest (F8-15), and how often this workspace's rules use it (decision 17). */
  const summaries = $derived((manifest.value?.automation?.action_summaries ?? {}) as Readonly<Record<string, string>>);
  const usage = $derived(usageOf(rules.all));

  /* ---------- The probe, drawn onto the canvas (decision 9) ---------- */

  let verdicts = $state<Map<string, Verdict>>(new Map());
  let drawn = $state(false);
  let outcome = $state<Outcome | undefined>(undefined);
  let isProbing = $state(false);
  let drawing: ReturnType<typeof setTimeout> | undefined;

  /** One frame every beat, the arm a branch takes shown, the outcome last. Reduced motion: all at once. */
  function draw(frames: readonly Frame[], result: Outcome): void {
    clearTimeout(drawing);
    verdicts = new Map();
    drawn = false;
    outcome = undefined;
    const reduced = typeof matchMedia === 'function' && (matchMedia('(prefers-reduced-motion: reduce)').matches || document.documentElement.dataset.motion === 'reduced');
    const step = (at: number): void => {
      if (at >= frames.length) {
        drawn = true;
        outcome = result;
        announcer.say(t('app.flow.probe_announced'));
        return;
      }
      const frame = frames[at]!;
      verdicts = new Map([...verdicts, [frame.key, frame.verdict]]);
      if (frame.verdict.code === 'app.flow.verdict_then' || frame.verdict.code === 'app.flow.verdict_otherwise') {
        pickArm(frame.key, frame.verdict.state === 'yes' ? 'then' : 'else');
      }
      if (reduced) step(at + 1);
      else drawing = setTimeout(() => step(at + 1), 260);
    };
    step(0);
  }

  function clearDrawing(): void {
    clearTimeout(drawing);
    verdicts = new Map();
    drawn = false;
    outcome = undefined;
  }

  async function probe(sample: { type: string; subject?: string; payload?: Record<string, unknown> }): Promise<void> {
    isProbing = true;
    clearDrawing();
    try {
      const result = await runs.dryRunDraft(toRuleDraft(draft, shownName), { type: sample.type, subject: sample.subject }, sample.payload);
      failure = undefined;
      const { frames, outcome: ended } = framesOfTest(draft.actions, result);
      draw(frames, ended);
    } catch (cause) {
      failure = renderProblem(cause as never, messages);
    } finally {
      isProbing = false;
    }
  }

  function drawRun(run: Run): void {
    const { frames, outcome: ended } = framesOfRun(draft.actions, run);
    draw(frames, ended);
    if (narrow) sheetOpen = false;
  }
</script>

<div class="editor">
  {#if !isNew && (reading.status === 'loading' || reading.status === 'idle')}
    <div class="head">
      <h1 class="plain">{t('app.flow.reading')}</h1>
      <p class="waiting"><Spinner label={t('app.flow.reading')} /> <span>{t('app.flow.reading')}</span></p>
    </div>
  {:else if !isNew && !stored}
    <div class="head">
      <h1 class="plain">{t('app.flow.not_found')}</h1>
      <Banner tone="danger" title={t('app.flow.not_found')} />
    </div>
  {:else}
    <header class="head">
      <div class="row">
        <a class="back" href="/administration/rules"><Icon name="arrow-left" size="sm" />{t('app.flow.back')}</a>
        <div class="spacer"></div>
        {#if stored}
          <Badge tone={stored.enabled ? 'success' : 'neutral'}>{stored.enabled ? t('app.flow.state_on') : t('app.flow.state_off')}</Badge>
        {:else}
          <Badge tone="neutral">{t('app.flow.state_new')}</Badge>
        {/if}
        {#if isBroken}<Badge tone="danger" icon="circle-alert">{t('app.flow.health_broken')}</Badge>{:else if findings.length > 0}<Badge tone="warning" icon="triangle-alert">{t('app.flow.health_attention')}</Badge>{/if}
        {#if dirty}<Badge tone="warning">{t('app.flow.unsaved')}</Badge>{/if}
        <Button tone="primary" size="sm" isBusy={isWorking} busyLabel={t('app.flow.saving')} onclick={save}>{t('app.flow.save')}</Button>
        {#if stored}
          <Button size="sm" tone={stored.enabled ? 'secondary' : 'primary'} isBusy={isWorking} busyLabel={t('app.rules.working')} onclick={toggle} disabledReason={!stored.enabled && isBroken ? t('app.flow.enable_refused_broken') : undefined}>
            {stored.enabled ? t('app.flow.disable') : t('app.flow.enable')}
          </Button>
          <Button size="sm" tone="subtle" icon="trash" onclick={() => (isDeleting = true)}>{t('app.flow.delete')}</Button>
        {/if}
      </div>

      <div class="row">
        <button class="name" class:automatic type="button" onclick={() => select({ kind: 'rule' })} title={t('app.flow.name_own')}>
          <h1>{shownName || t('app.flow.new_title')}</h1>
          {#if automatic}<span class="auto" title={t('app.flow.name_automatic_hint')}>{t('app.flow.name_automatic')}</span>{/if}
          <Icon name="pencil" size="sm" />
        </button>
        <div class="chips">
          <button class="chip" type="button" onclick={() => select({ kind: 'scope' })}>
            <Icon name="hub" size="sm" /><span>{t('app.flow.applies_in')}</span><b>{names.scope(draft.scope)}</b>
          </button>
          <button class="chip" class:flagged={marks.has('run_as')} type="button" onclick={() => select({ kind: 'runas' })}>
            <Icon name="shield" size="sm" /><span>{t('app.flow.runs_as')}</span><b>{names.account(draft.runAs)}</b>
            {#if marks.get('run_as')}<span class="chip-flag"><Icon name="triangle-alert" size="sm" />{marks.get('run_as')}</span>{/if}
          </button>
        </div>
      </div>

      <div class="sentence-row">
        <p class="sentence" class:open={sentenceOpen}>{said}</p>
        <button class="fold" type="button" aria-expanded={sentenceOpen} onclick={toggleSentence} aria-label={sentenceOpen ? t('app.flow.sentence_hide') : t('app.flow.sentence_show')}>
          <Icon name={sentenceOpen ? 'chevron-up' : 'chevron-down'} size="sm" />
        </button>
      </div>

      {#if findings.length > 0}
        <Banner tone={isBroken ? 'danger' : 'warning'} title={isBroken ? t('app.flow.enable_refused_broken') : t('app.flow.health_attention')}>
          {findings.map((finding) => findingWords(words, finding)).join(' · ')}
          {#if stored?.checked_at}<span class="quiet small"> — {t('app.flow.checked_at', { moment: formatDateTime(stored.checked_at, messages.locale) })}</span>{/if}
        </Banner>
      {/if}
      {#if failure && !failure.fields.size}
        <Banner tone="danger" title={failure.message}>
          {#if failure.reference}{t('app.error_reference', { request_id: failure.reference })}{/if}
        </Banner>
      {:else if failure}
        <Banner tone="danger" title={failure.message} />
      {/if}

      {#if minted && minted.rule_id === stored?.id}
        <OneTimeSecret
          value={inboundAddressOf(window.location.origin, minted.token)}
          label={t('app.rules.inbound_address')}
          hint={t('app.rules.inbound_minted_hint')}
          revealLabel={t('app.jumble.reveal')}
          hideLabel={t('app.jumble.hide')}
          copyLabel={t('app.jumble.copy')}
          copiedLabel={t('app.jumble.copied')}
          acknowledgementLabel={t('app.jumble.kept')}
          notAcknowledgedReason={t('app.jumble.keep_first')}
          dismissLabel={t('app.jumble.done')}
          onDismiss={() => (minted = undefined)}
        />
      {/if}
    </header>

    <!-- Where a lifted piece may go, and why a drop was refused (decision 20): a line that takes
         no room - it is zero height and its words overlay the canvas's top padding - so nothing
         moves under the pointer when a piece is lifted, and sticky, so it is read while scrolling. -->
    <p class="dragline" role="status" class:lifted={drag !== undefined} class:refused={drag === undefined && refusal !== undefined}>
      {#if drag || refusal}<span>{drag ? dragHint(drag, draft.actions, t) : refusal}</span>{/if}
    </p>

    <div class="bench">

      <section class="canvas" aria-label={t('app.rules.title')}>
        <RuleCanvas
          {draft}
          {selection}
          kinds={actionKinds}
          {summaries}
          {usage}
          {names}
          {triggerMeta}
          {marks}
          {describe}
          onselect={select}
          oninsert={insert}
          onremove={remove}
          onfold={fold}
          onaddcondition={addCondition}
          onnudge={nudgeStep}
          onaddrung={addElseIf}
          {drag}
          ondragchange={(next) => (drag = next)}
          ondrop={dropped}
          onreplacetrigger={replaceTrigger}
          onrefuse={refuse}
          segmented={narrow}
          {armChoice}
          onpickarm={pickArm}
          {verdicts}
          dimUnvisited={drawn}
        />
      </section>

      {#if narrow}
        <!-- Below the expanded breakpoint the details come to the canvas rather than the reader
             scrolling to them: a sheet over it, one glass surface at a time (rule 2). -->
        <Drawer bind:isOpen={sheetOpen} edge="block-end" title={t('app.flow.inspector')} dismissLabel={t('app.flow.sheet_close')}>
          {@render inspector()}
        </Drawer>
        <div class="sheetbar" role="tablist" aria-label={t('app.flow.inspector')}>
          {#each TABS as each (each.id)}
            <button type="button" role="tab" aria-selected={tab === each.id} onclick={() => { tab = each.id; sheetOpen = true; if (each.id === 'runs' && !isNew) void runs.reload({ ruleId: id }); }}>
              <Icon name={each.icon} size="sm" /><span>{t(each.code)}</span>
            </button>
          {/each}
        </div>
      {:else}
        <aside class="inspector" aria-label={t('app.flow.inspector')}>
          {@render inspector()}
        </aside>
      {/if}
    </div>

    {#snippet inspector()}
        <Tabs
          label={t('app.flow.inspector')}
          selected={tab}
          onselect={(next) => {
            tab = next;
            // What the rule has done since the editor opened: no record announces a run.
            if (next === 'runs' && !isNew) void runs.reload({ ruleId: id });
          }}
          tabs={TABS.map((each) => ({ id: each.id, label: t(each.code) }))}
        />
        {#if tab === 'rule'}
          <RuleInspector
            {draft}
            selection={{ kind: 'rule' }}
            section="rule"
            ruleId={stored?.id}
            generatedName={generated}
            {triggers}
            {eventTypes}
            {actionFields}
            {scopes}
            {runners}
            {pickers}
            {itemTypes}
            {errors}
            onupdate={update}
            onremovestep={remove}
            onremovecondition={removeCondition}
            onaddcondition={addCondition}
          />
        {:else if tab === 'blocks'}
          <div class="panel">
            <BlocksPanel
              {triggers}
              kinds={actionKinds}
              {summaries}
              {usage}
              onpick={(kind) => insert('', draft.actions.length, kind)}
              onpicktrigger={replaceTrigger}
              onlift={(event, piece) => lift(event, piece)}
              ondrop={() => (drag = undefined)}
            />
          </div>
        {:else if tab === 'piece'}
          <RuleInspector
            {draft}
            {selection}
            ruleId={stored?.id}
            generatedName={generated}
            {triggers}
            {eventTypes}
            {actionFields}
            {scopes}
            {runners}
            {pickers}
            {itemTypes}
            {errors}
            onupdate={update}
            onremovestep={remove}
            onremovecondition={removeCondition}
            onaddcondition={addCondition}
            inbound={stored?.trigger.kind === 'INBOUND_WEBHOOK' ? { rotatedAt: stored.inbound_rotated_at ?? undefined, onmint: mint, isWorking } : undefined}
            onstart={stored?.trigger.kind === 'MANUAL' ? start : undefined}
          />
        {:else if tab === 'probe'}
          <RuleProbe
            {eventTypes}
            defaultType={draft.trigger.event_type ?? ''}
            takesPayload={draft.trigger.kind === 'INBOUND_WEBHOOK'}
            isRunning={isProbing}
            {outcome}
            onrun={probe}
            onclear={clearDrawing}
          />
        {:else if stored}
          <RuleRuns ruleId={stored.id} enabled={stored.enabled} findings={stored.findings ?? []} runs={ruleRuns} ondraw={drawRun} />
        {:else}
          <p class="quiet panel">{t('app.flow.runs_none')}</p>
        {/if}
    {/snippet}

    {#if isDeleting && stored}
      <Dialog bind:isOpen={isDeleting} title={t('app.flow.delete_title')} dismissLabel={t('app.flow.cancel')}>
        <p class="quiet">{t('app.flow.delete_body')}</p>
        {#snippet actions()}
          <Button tone="secondary" onclick={() => (isDeleting = false)}>{t('app.flow.cancel')}</Button>
          <Button tone="danger" isBusy={isWorking} busyLabel={t('app.rules.working')} onclick={remove_rule}>{t('app.flow.delete_confirm')}</Button>
        {/snippet}
      </Dialog>
    {/if}
  {/if}
</div>

<style>
  .editor { display: flex; flex-direction: column; min-height: 100%; }

  .waiting { margin: 0; display: flex; align-items: center; gap: var(--sp-100); color: var(--text-secondary); }

  .plain { margin: 0; font-family: var(--font-display); font-size: var(--fs-400); font-weight: var(--fw-semibold); line-height: var(--lh-tight); }

  .head { display: flex; flex-direction: column; gap: var(--sp-100); padding: var(--sp-150) var(--sp-200); background: var(--bg-surface); border-block-end: var(--bw-hairline) solid var(--border-subtle); }

  .row { display: flex; flex-wrap: wrap; align-items: center; gap: var(--sp-100) var(--sp-150); }

  .spacer { flex: 1 1 auto; }

  .back { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-subtle); font-size: var(--fs-075); text-decoration: none; }

  .back:hover { color: var(--text-primary); }

  .name { display: flex; align-items: center; gap: var(--sp-100); flex: 1 1 24ch; min-width: 0; padding: 0; border: 0; background: none; color: var(--text-primary); text-align: start; cursor: text; }

  .name h1 { margin: 0; font-family: var(--font-display); font-size: var(--fs-400); font-weight: var(--fw-semibold); line-height: var(--lh-tight); overflow-wrap: anywhere; }

  .name.automatic h1 { color: var(--text-secondary); }

  .auto { flex: 0 0 auto; padding: 0 var(--sp-100); border-radius: var(--r-full); background: var(--label-slate-bg); color: var(--label-slate-fg); font-size: var(--fs-050); font-weight: var(--fw-medium); }

  .name:focus-visible, .chip:focus-visible, .fold:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); border-radius: var(--r-xs); }

  .chips { display: flex; flex-wrap: wrap; gap: var(--sp-100); }

  .chip { display: inline-flex; align-items: center; gap: var(--sp-050); padding: var(--sp-025) var(--sp-100); border: var(--bw-hairline) solid var(--border-subtle); border-radius: var(--r-full); background: var(--bg-surface-sunken); color: var(--text-secondary); font-size: var(--fs-075); cursor: pointer; }

  .chip:hover { border-color: var(--border-default); }

  .chip b { font-weight: var(--fw-medium); color: var(--text-primary); }
  .chip.flagged { border-color: var(--status-warning-border); }
  .chip-flag { display: inline-flex; align-items: center; gap: var(--sp-050); color: var(--text-warning); }

  .sentence-row { display: flex; align-items: flex-start; gap: var(--sp-050); }

  /* Collapsed to one line, expanded on request, the choice kept in this browser (decision 4). */
  .sentence { margin: 0; flex: 1 1 auto; min-width: 0; font-size: var(--fs-075); color: var(--text-secondary); display: -webkit-box; -webkit-line-clamp: 1; line-clamp: 1; -webkit-box-orient: vertical; overflow: hidden; }

  .sentence.open { display: block; max-width: 80ch; }

  .fold { flex: 0 0 auto; width: var(--sp-300); height: var(--sp-300); padding: 0; display: grid; place-items: center; border: 0; border-radius: var(--r-sm); background: transparent; color: var(--text-subtle); }

  .fold:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  /* The canvas and the panel, nothing else (decision 16): the panel reaches the bottom and
     scrolls on its own, the canvas takes every other pixel. */
  .bench { display: grid; grid-template-columns: minmax(0, 1fr) minmax(0, 44ch); flex: 1 1 auto; min-height: 0; }

  .dragline { position: sticky; inset-block-start: 0; z-index: var(--z-sticky); margin: 0; max-width: none; align-self: stretch; height: 0; overflow: visible; text-align: center; pointer-events: none; }

  .dragline span { display: inline-block; max-width: 100%; padding: var(--sp-050) var(--sp-200); border-end-start-radius: var(--r-md); border-end-end-radius: var(--r-md); font-size: var(--fs-075); font-weight: var(--fw-medium); color: var(--text-inverse); background: var(--accent-primary); box-shadow: var(--shadow-raised); }

  .dragline.refused span { background: var(--status-warning-text); }

  .canvas { padding: var(--sp-400) var(--sp-200) var(--sp-1000); overflow-x: auto; background: radial-gradient(circle at var(--sp-025) var(--sp-025), var(--border-subtle) var(--sp-025), transparent 0) 0 0 / var(--sp-250) var(--sp-250); }

  .inspector { display: flex; flex-direction: column; border-inline-start: var(--bw-hairline) solid var(--border-subtle); background: var(--bg-surface); position: sticky; inset-block-start: 0; align-self: start; height: 100vh; overflow: auto; }

  .quiet { margin: 0; color: var(--text-secondary); }

  .small { font-size: var(--fs-075); }

  .panel { padding: var(--sp-200); font-size: var(--fs-075); }

  /* Below the expanded breakpoint the palette is gone (the + is the way in) and the inspector
     follows the canvas; F8-05 makes it a sheet over it. */
  .sheetbar { display: none; }

  /* design-system-lint-ignore: `primitive.breakpoint.expanded` (905px) less one; a media query cannot read a custom property. */
  @media (max-width: 904px) {
    .bench { grid-template-columns: minmax(0, 1fr); }
    .canvas { padding-block-end: calc(var(--sp-1600) + var(--layout-bottombar-height)); }
    /* Above the shell's bottom bar (F9), not behind it. */
    .sheetbar { position: fixed; inset-inline: var(--sp-200); inset-block-end: calc(var(--layout-bottombar-height) + env(safe-area-inset-bottom, 0) + var(--sp-100)); z-index: var(--z-sticky); display: flex; border: var(--bw-hairline) solid var(--border-default); border-radius: var(--r-full); background: var(--bg-surface); box-shadow: var(--shadow-overlay); overflow: hidden; }
    .sheetbar button { flex: 1 1 0; min-width: 0; display: inline-flex; flex-direction: column; align-items: center; gap: var(--sp-025); padding: var(--sp-050) var(--sp-025); border: 0; background: transparent; color: var(--text-secondary); font-size: var(--fs-050); }
    .sheetbar button[aria-selected='true'] { color: var(--accent-primary); }
    .sheetbar button:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: calc(-1 * var(--sp-025)); }
  }
</style>
