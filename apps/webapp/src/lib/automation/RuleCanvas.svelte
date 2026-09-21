<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The rule drawn as a path (F8-04, `milestone-F8.md` decision 1).
  //
  // **The canvas is a vertical flow, in this order and no other:** the trigger card; the gate,
  // one block holding every condition; the chain of steps, each a card, with `BRANCH` drawn as a
  // fork into *then* and *otherwise* that rejoins, `WAIT` as a pause with its duration written on
  // the line below it, and `STOP` as a terminus that draws no line onward; the guardrails last.
  // Every gap between two cards is exactly one line with one insertion point in its middle.
  //
  // **It draws, it does not decide.** A click selects into the inspector, a `+` inserts, the
  // tools remove or fold; every change goes back through a callback and the parent holds the
  // draft. The connectors are elements rather than pseudo-elements so that a later step can light
  // them as a run travels down (F8-06).
  //
  // **A branch's arms are equal in height**, so that both tails reach the join; an arm that ends
  // in a stop draws no tail, because the run does not continue from there.

  import { Icon, type IconName } from '@hubtask/design-system/components';

  import InsertMenu from './InsertMenu.svelte';
  import RuleCanvasList from './RuleCanvasList.svelte';
  import type { Draft, Step } from './model.ts';
  import type { Drag, Selection } from './selection.ts';
  import { TRIGGER_ICONS, conditionWords, type Names } from './words.ts';
  import type { Verdict } from './probe.ts';
  import { messages, t } from '../i18n/i18n.svelte.ts';

  interface Props {
    draft: Draft;
    selection: Selection;
    /** The action kinds this installation serves. */
    kinds: readonly string[];
    names: Names;
    /** The text under the trigger's title: the event, the schedule, the address. */
    triggerMeta: string;
    /** A finding or refusal at a card, by the card's path (`trigger`, `conditions/1`, a step path). */
    marks?: ReadonlyMap<string, string>;
    /** One line under a step's title: its settings in words. */
    describe?: (step: Step) => string;
    onselect: (selection: Selection) => void;
    oninsert: (list: string, index: number, kind: string) => void;
    onremove: (path: string) => void;
    onfold: (path: string) => void;
    onaddcondition: () => void;
    onnudge: (path: string, direction: -1 | 1) => void;
    /** The drag in flight (F8-05, decision 7), and where it may land. */
    drag?: Drag;
    ondragchange: (drag: Drag | undefined) => void;
    ondrop: (list: string, index: number, drag: Drag) => void;
    /** A trigger let go on the trigger card, a condition on the gate. */
    onreplacetrigger: (kind: string) => void;
    /** A piece let go where nothing takes it, with the drag that was refused. */
    onrefuse: (drag: Drag) => void;
    segmented: boolean;
    armChoice: ReadonlyMap<string, 'then' | 'else'>;
    onpickarm: (path: string, arm: 'then' | 'else') => void;
    /** A run drawn onto the canvas (F8-06): what each card says, by its key; cards not in it fade once `dimUnvisited`. */
    verdicts?: ReadonlyMap<string, Verdict>;
    dimUnvisited?: boolean;
  }

  const {
    draft, selection, kinds, names, triggerMeta, marks, describe, onselect, oninsert, onremove, onfold, onaddcondition,
    onnudge, drag, ondragchange, ondrop, onreplacetrigger, onrefuse, segmented, armChoice, onpickarm, verdicts, dimUnvisited = false,
  }: Props = $props();

  const verdictWord = (verdict: Verdict): string => t(verdict.code, verdict.params);

  const triggerTakes = $derived(drag?.src === 'trigger');
  const gateTakes = $derived(drag?.src === 'condition');
  let overTrigger = $state(false);
  let overGate = $state(false);

  const hint = $derived.by(() => {
    switch (drag?.src) {
      case 'trigger':
        return t('app.flow.drag_trigger');
      case 'condition':
        return t('app.flow.drag_condition');
      case 'action':
        return t('app.flow.drag_action');
      case 'step':
        return t('app.flow.drag_step');
      default:
        return '';
    }
  });

  function allow(event: DragEvent, takes: boolean): void {
    if (!takes) return;
    event.preventDefault();
    if (event.dataTransfer) event.dataTransfer.dropEffect = 'copy';
  }

  /** A drop on the canvas that no target took: refused with a reason, never silently nothing. */
  function refuse(event: DragEvent): void {
    if (!drag) return;
    event.preventDefault();
    onrefuse(drag);
    ondragchange(undefined);
  }

  const words = { t, has: (code: string) => messages.has(code) };

  const TRIGGER_ICON: Record<string, IconName> = TRIGGER_ICONS as Record<string, IconName>;

  const isSelected = (kind: Selection['kind'], index?: number): boolean =>
    selection.kind === kind && (kind !== 'condition' || (selection as { index: number }).index === index);

  function onkey(event: KeyboardEvent, select: () => void): void {
    // A key on a tool inside the card is the tool's, not the card's: the card's own handler
    // would otherwise swallow the Enter that presses "move down".
    if (event.target !== event.currentTarget) return;
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      select();
    }
  }
</script>

<!-- svelte-ignore a11y_no_static_element_interactions -->
<div class="flow" data-canvas ondragover={(event) => { if (drag) event.preventDefault(); }} ondrop={refuse}>
  {#if drag}<p class="draghint" role="status">{hint}</p>{/if}
  <!-- The trigger: the one card in the signature colour, because it is where the run comes from. -->
  <div
    class="card trigger"
    class:selected={isSelected('trigger')}
    class:target={triggerTakes}
    class:over={overTrigger}
    class:inert={drag !== undefined && !triggerTakes}
    class:lit={verdicts?.has('trigger')}
    class:yes={verdicts?.get('trigger')?.state === 'yes'}
    data-card="trigger"
    role="button"
    tabindex="0"
    onclick={() => onselect({ kind: 'trigger' })}
    onkeydown={(event) => onkey(event, () => onselect({ kind: 'trigger' }))}
    ondragover={(event) => { allow(event, triggerTakes); overTrigger = triggerTakes; }}
    ondragleave={() => (overTrigger = false)}
    ondrop={(event) => {
      overTrigger = false;
      if (!triggerTakes || drag?.src !== 'trigger') return;
      event.preventDefault();
      event.stopPropagation();
      onreplacetrigger(drag.kind);
      ondragchange(undefined);
    }}
  >
    {#if triggerTakes}<span class="dropword">{t('app.flow.drop_trigger')}</span>{/if}
    {#if verdicts?.get('trigger')}<span class="verdict yes"><Icon name="zap" size="sm" />{verdictWord(verdicts.get('trigger')!)}</span>{/if}
    <span class="mark trigger-mark"><Icon name={TRIGGER_ICON[draft.trigger.kind] ?? 'zap'} size="sm" /></span>
    <span class="body">
      <span class="kind">{t('app.flow.card_starts_on')}</span>
      <span class="title">{t(`app.rules.trigger_${draft.trigger.kind.toLowerCase()}`)}</span>
      <span class="meta">{triggerMeta}</span>
      {#if marks?.get('trigger')}<span class="flag"><Icon name="triangle-alert" size="sm" />{marks.get('trigger')}</span>{/if}
    </span>
  </div>

  <span class="stub"></span>

  <!-- The gate: one block, every condition inside it, through which the run has to pass. -->
  <div
    class="gate"
    class:selected={isSelected('gate')}
    class:empty={draft.conditions.length === 0}
    class:target={gateTakes}
    class:over={overGate}
    class:inert={drag !== undefined && !gateTakes}
    data-card="gate"
    role="button"
    tabindex="0"
    onclick={() => onselect({ kind: 'gate' })}
    onkeydown={(event) => onkey(event, () => onselect({ kind: 'gate' }))}
    ondragover={(event) => { allow(event, gateTakes); overGate = gateTakes; }}
    ondragleave={() => (overGate = false)}
    ondrop={(event) => {
      overGate = false;
      if (!gateTakes) return;
      event.preventDefault();
      event.stopPropagation();
      onaddcondition();
      ondragchange(undefined);
    }}
  >
    {#if gateTakes}<span class="dropword">{t('app.flow.drop_condition')}</span>{/if}
    <span class="ghead">
      <span class="mark condition-mark"><Icon name="funnel" size="sm" /></span>
      <span class="title">{t('app.flow.card_only_when')}</span>
      <span class="hint">{draft.conditions.length > 0 ? t('app.flow.card_only_when_all') : t('app.flow.card_only_when_none')}</span>
    </span>
    {#each draft.conditions as expr, index (index)}
      {@const verdict = verdicts?.get(`conditions/${index}`)}
      <div
        class="condition"
        class:selected={isSelected('condition', index)}
        class:lit={verdict !== undefined}
        class:yes={verdict?.state === 'yes'}
        class:no={verdict?.state === 'no'}
        class:faded={dimUnvisited && verdict === undefined}
        data-card={`conditions/${index}`}
        role="button"
        tabindex="0"
        onclick={(event) => {
          event.stopPropagation();
          onselect({ kind: 'condition', index });
        }}
        onkeydown={(event) => {
          event.stopPropagation();
          onkey(event, () => onselect({ kind: 'condition', index }));
        }}
      >
        {#if index > 0}<span class="and">{t('app.flow.sentence_and').trim()}</span>{/if}
        <span class="body">
          <span class="words">{conditionWords(words, names, expr)}</span>
          <code class="expr">{expr}</code>
          {#if marks?.get(`conditions/${index}`)}<span class="flag"><Icon name="triangle-alert" size="sm" />{marks.get(`conditions/${index}`)}</span>{/if}
        </span>
        {#if verdict}<span class="verdict" class:yes={verdict.state === 'yes'} class:no={verdict.state === 'no'}><Icon name={verdict.state === 'yes' ? 'check' : 'x'} size="sm" />{verdictWord(verdict)}</span>{/if}
      </div>
    {/each}
    <button
      class="add"
      type="button"
      onclick={(event) => {
        event.stopPropagation();
        onaddcondition();
      }}
    >
      <Icon name="plus" size="sm" />{t('app.flow.add_condition')}
    </button>
  </div>

  <InsertMenu {kinds} list="" index={0} onpick={oninsert} {drag} {ondrop} />

  <RuleCanvasList steps={draft.actions} prefix="" {kinds} {names} {selection} {marks} {describe} {onselect} {oninsert} {onremove} {onfold} {onnudge} {drag} {ondragchange} {ondrop} {segmented} {armChoice} {onpickarm} {verdicts} {dimUnvisited} />

  <!-- The guardrails: what bounds the rule, drawn as the end of the path. -->
  <div
    class="card guardrails"
    class:selected={isSelected('guardrails')}
    class:inert={drag !== undefined}
    data-card="guardrails"
    role="button"
    tabindex="0"
    onclick={() => onselect({ kind: 'guardrails' })}
    onkeydown={(event) => onkey(event, () => onselect({ kind: 'guardrails' }))}
  >
    <span class="mark settings-mark"><Icon name="settings" size="sm" /></span>
    <span class="body">
      <span class="kind">{t('app.flow.card_guardrails')}</span>
      <span class="title">{t(`app.rules.on_error_${draft.onError.toLowerCase()}`)}</span>
      <span class="meta">
        {draft.throttle.maxRunsPerHour
          ? t('app.flow.card_guardrails_runs', { count: draft.throttle.maxRunsPerHour })
          : t('app.flow.card_guardrails_unbounded')}{#if draft.throttle.dedupeKeyExpr}{' · '}{t('app.flow.card_guardrails_dedupe', { expr: draft.throttle.dedupeKeyExpr })}{/if}
      </span>
    </span>
  </div>
</div>

<style>
  .flow { position: relative; display: flex; flex-direction: column; align-items: center; width: 100%; max-width: 92ch; margin-inline: auto; }

  .stub { width: var(--bw-ring); height: var(--sp-150); background: var(--border-default); border-radius: var(--r-full); flex: 0 0 auto; }

  /* One card shape for every step (design-system.md §6 rule 1: raised = standalone). The trigger
     alone carries the signature colour, and the guardrails are recessed: a bound, not a step. */
  .card {
    position: relative;
    width: min(44ch, 100%);
    display: flex;
    gap: var(--sp-150);
    align-items: flex-start;
    padding: var(--sp-150);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    text-align: start;
    cursor: pointer;
  }

  .card.trigger { border-color: var(--accent-signature); border-width: var(--bw-thick); }

  .card.guardrails { border-style: dashed; box-shadow: none; background: var(--bg-surface-sunken); }

  .card.selected, .gate.selected, .condition.selected { outline: var(--bw-ring) solid var(--accent-primary); outline-offset: var(--sp-025); }

  /* While a piece is lifted (decision 7): what may take it is ringed and labelled, what may not fades. */
  .card.target, .gate.target { outline: var(--bw-ring) dashed var(--accent-primary); outline-offset: var(--sp-050); }

  .card.over, .gate.over { background: var(--accent-primary-subtle); }

  .card.inert, .gate.inert { opacity: 0.35; }

  /* A run drawn on: a lit card carries its word on a badge; what the run never reached fades. */
  .card.lit, .condition.lit { box-shadow: 0 0 0 var(--sp-050) var(--accent-primary-subtle), var(--shadow-raised); }

  .card.lit.yes, .condition.lit.yes { border-color: var(--success-500); }

  .condition.lit.no { border-color: var(--warning-500); }

  .condition.faded { opacity: 0.38; }

  .verdict { position: absolute; inset-block-start: calc(-1 * var(--sp-150)); inset-inline-end: var(--sp-150); display: inline-flex; align-items: center; gap: var(--sp-050); padding: 0 var(--sp-100); min-height: var(--sp-250); border-radius: var(--r-full); font-size: var(--fs-050); font-weight: var(--fw-medium); background: var(--label-slate-bg); color: var(--label-slate-fg); animation: arrive var(--motion-entrance-duration) var(--motion-entrance-easing) both; }

  .verdict.yes { background: var(--label-green-bg); color: var(--label-green-fg); }

  .verdict.no { background: var(--label-amber-bg); color: var(--label-amber-fg); }

  /* Rule 6: opacity and transform only. */
  @keyframes arrive { from { opacity: 0; translate: 0 var(--sp-050); } to { opacity: 1; translate: none; } }

  @media (prefers-reduced-motion: reduce) { .verdict { animation: none; } }
  :global([data-motion='reduced']) .verdict { animation: none; }

  .dropword { position: absolute; inset-block-start: calc(-1 * var(--sp-150)); inset-inline-start: 50%; translate: -50% 0; padding: 0 var(--sp-100); border-radius: var(--r-full); background: var(--accent-primary); color: var(--text-inverse); font-size: var(--fs-050); font-weight: var(--fw-medium); white-space: nowrap; }

  /* Out of the flow, in the canvas's own top padding: a hint that took space would move every
     card under the pointer the moment a piece is lifted. */
  .draghint { position: absolute; inset-block-start: calc(-1 * var(--sp-300)); inset-inline-start: 50%; translate: -50% 0; margin: 0; padding: var(--sp-050) var(--sp-150); border-radius: var(--r-full); background: var(--accent-primary); color: var(--text-inverse); font-size: var(--fs-075); font-weight: var(--fw-medium); max-width: 100%; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; pointer-events: none; }

  .card:focus-visible, .gate:focus-visible, .condition:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }

  .body { flex: 1 1 auto; min-width: 0; display: flex; flex-direction: column; gap: var(--sp-025); }

  .kind { font-size: var(--fs-050); font-weight: var(--fw-medium); text-transform: uppercase; color: var(--text-subtle); }

  .title { font-weight: var(--fw-medium); color: var(--text-primary); overflow-wrap: anywhere; }

  .meta { font-size: var(--fs-075); color: var(--text-secondary); overflow-wrap: anywhere; }

  .mark { flex: 0 0 auto; display: inline-grid; place-items: center; width: var(--sp-300); height: var(--sp-300); border-radius: var(--r-sm); }

  .trigger-mark { width: var(--sp-400); height: var(--sp-400); background: var(--accent-signature-subtle); color: var(--accent-signature); }

  .condition-mark { background: var(--label-amber-bg); color: var(--label-amber-fg); }

  .settings-mark { background: var(--label-slate-bg); color: var(--label-slate-fg); }

  .flag { display: inline-flex; align-items: center; gap: var(--sp-050); font-size: var(--fs-075); color: var(--text-warning); }

  .gate {
    position: relative;
    width: min(44ch, 100%);
    display: flex;
    flex-direction: column;
    gap: var(--sp-100);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--label-amber-fg);
    border-radius: var(--r-lg);
    background: var(--bg-surface);
    box-shadow: var(--shadow-raised);
    cursor: pointer;
    text-align: start;
  }

  .gate.empty { border-style: dashed; box-shadow: none; background: var(--bg-surface-sunken); }

  .ghead { display: flex; align-items: center; gap: var(--sp-100); padding-inline: var(--sp-050); }

  .ghead .hint { font-size: var(--fs-075); color: var(--text-subtle); }

  .condition {
    position: relative;
    display: flex;
    gap: var(--sp-100);
    padding: var(--sp-100);
    border: var(--bw-hairline) solid var(--border-subtle);
    border-radius: var(--r-md);
    background: var(--bg-surface-sunken);
  }

  .condition .words { font-size: var(--fs-100); color: var(--text-primary); }

  .expr { display: block; font-family: var(--font-mono); font-size: var(--fs-050); color: var(--text-subtle); overflow-wrap: anywhere; }

  .and {
    position: absolute;
    inset-inline-start: var(--sp-150);
    inset-block-start: calc(-1 * var(--sp-100));
    padding-inline: var(--sp-050);
    font-size: var(--fs-050);
    font-weight: var(--fw-medium);
   
    text-transform: uppercase;
    background: var(--bg-surface);
    color: var(--text-subtle);
  }

  .add {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    min-height: var(--density-control-sm-min);
    padding: var(--sp-025) var(--sp-100);
    border: 0;
    border-radius: var(--r-sm);
    background: transparent;
    color: var(--text-secondary);
    font-size: var(--fs-075);
    font-weight: var(--fw-medium);
  }

  .add:hover { background: var(--bg-surface-hover); color: var(--text-primary); }

  .add:focus-visible { outline: var(--bw-ring) solid var(--focus-ring); outline-offset: var(--sp-025); }
</style>
