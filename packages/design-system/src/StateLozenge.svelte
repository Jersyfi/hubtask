<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A state, in one shape for every vocabulary (work-surface concept §5): a status and its
  // category, a deadline, a goal, a configuration item's health, an agent, a rule, a release, the
  // device's sync. The word is the caller's; the tone and glyph come from `lozenge.ts`, so one
  // state never looks two ways. A square-cornered sibling of `Badge`, which stays the system's own
  // vocabulary about itself; this one is the vocabulary of the work.
  import Icon from './Icon.svelte';
  import type { IconName } from './icons/index.ts';
  import { lozengeLook, type LozengeTone, type LozengeVocabulary } from './lozenge.ts';

  interface Props {
    /** The state's word, resolved: *In progress*, *At risk*. */
    label: string;
    /** The vocabulary and the state in it; together they choose tone and glyph. */
    vocabulary?: LozengeVocabulary;
    state?: string;
    /** A tone of the caller's own, for a status whose category the caller resolved. */
    tone?: LozengeTone;
    /** A glyph that overrides the vocabulary's: an entry type's own mark. */
    icon?: IconName;
    /** `bold` for the one state on a screen that must be seen first. */
    emphasis?: 'subtle' | 'bold';
    /** `sm` in a row or a chip line; `md` stands on its own - in an object's head, the status button's shape. */
    size?: 'sm' | 'md';
  }

  const { label, vocabulary, state, tone, icon, emphasis = 'subtle', size = 'sm' }: Props = $props();

  const look = $derived(vocabulary && state !== undefined ? lozengeLook(vocabulary, state) : undefined);
  const drawnTone = $derived<LozengeTone>(tone ?? look?.tone ?? 'neutral');
  const mark = $derived<IconName | undefined>(icon ?? look?.icon);
</script>

<span class="lozenge" data-tone={drawnTone} data-emphasis={emphasis} data-size={size}>
  {#if mark}<Icon name={mark} size="sm" />{/if}
  <span class="word">{label}</span>
</span>

<style>
  .lozenge {
    --lozenge-surface: var(--status-neutral-surface);
    --lozenge-border: var(--status-neutral-border);
    --lozenge-text: var(--status-neutral-text);
    --lozenge-accent: var(--status-neutral-accent);
    display: inline-flex;
    align-items: center;
    gap: var(--sp-050);
    max-inline-size: 100%;
    padding: var(--bw-hairline) calc(var(--sp-050) + var(--sp-025));
    border: var(--bw-hairline) solid var(--lozenge-border);
    border-radius: var(--r-sm);
    background: var(--lozenge-surface);
    color: var(--lozenge-text);
    font-size: var(--fs-075);
    font-weight: var(--fw-semibold);
    line-height: var(--lh-snug);
    white-space: nowrap;
    vertical-align: middle;
  }

  .lozenge :global(.icon) {
    inline-size: var(--fs-075);
    block-size: var(--fs-075);
    flex: none;
  }

  .word { overflow: hidden; text-overflow: ellipsis; }

  /* The status button's shape and height (`StatusButton`), read only: a head's status that is not changed there. */
  .lozenge[data-size='md'] {
    /* Measured as the button's border box: content-box drew it a padding taller beside it (H-2). */
    box-sizing: border-box;
    min-block-size: var(--density-control-sm-min);
    padding: var(--sp-050) var(--sp-150);
    border-radius: var(--r-md);
    font-size: var(--fs-090);
  }

  .lozenge[data-size='md'] :global(.icon) {
    inline-size: var(--fs-090);
    block-size: var(--fs-090);
  }

  .lozenge[data-tone='info'] {
    --lozenge-surface: var(--status-info-surface);
    --lozenge-border: var(--status-info-border);
    --lozenge-text: var(--status-info-text);
    --lozenge-accent: var(--status-info-accent);
  }

  .lozenge[data-tone='success'] {
    --lozenge-surface: var(--status-success-surface);
    --lozenge-border: var(--status-success-border);
    --lozenge-text: var(--status-success-text);
    --lozenge-accent: var(--status-success-accent);
  }

  .lozenge[data-tone='warning'] {
    --lozenge-surface: var(--status-warning-surface);
    --lozenge-border: var(--status-warning-border);
    --lozenge-text: var(--status-warning-text);
    --lozenge-accent: var(--status-warning-accent);
  }

  .lozenge[data-tone='danger'] {
    --lozenge-surface: var(--status-danger-surface);
    --lozenge-border: var(--status-danger-border);
    --lozenge-text: var(--status-danger-text);
    --lozenge-accent: var(--status-danger-accent);
  }

  /* The reader's own work (*Mine*): the brand's tint, no status meaning, so no border either. */
  .lozenge[data-tone='brand'] {
    --lozenge-surface: var(--accent-primary-subtle);
    --lozenge-border: var(--accent-primary-subtle);
    --lozenge-text: var(--text-brand);
    --lozenge-accent: var(--accent-primary);
  }

  .lozenge[data-emphasis='bold'] {
    background: var(--lozenge-accent);
    border-color: var(--lozenge-accent);
    color: var(--text-inverse);
  }
</style>
