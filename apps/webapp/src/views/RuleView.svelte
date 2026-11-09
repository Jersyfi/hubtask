<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A rule (page-system §9.1, §7.2): its record page - *‹ Rules*, the kind, the name, on or off, its
  // health in the facts, *Save the rule* as the head's act and the rest in ⋯ - and the editor as
  // the page's one region, filling what is left of the screen (automation.md §1.5).
  import { RecordPage, type MenuItem, type RecordList } from '@hubtask/design-system/components';

  import RuleEditor, { type RuleHead } from '../lib/automation/RuleEditor.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    id: string;
    /** Opened from Your settings: a rule of the reader's own. */
    isOwn?: boolean;
    onnavigate: (path: string) => void;
  }

  const { id, isOwn = false, onnavigate }: Props = $props();
  const base = $derived(isOwn ? '/profile/rules' : '/administration/rules');

  const lists: RecordList[] = [{ id: 'flow', title: t('app.flow.region') }];

  function status(head: RuleHead): { label: string; tone: 'success' | 'danger' | 'neutral' } | undefined {
    if (!head.stored) return undefined;
    if (head.isBroken) return { label: t('app.flow.health_broken'), tone: 'danger' };
    return head.stored.enabled ? { label: t('app.flow.state_on'), tone: 'success' } : { label: t('app.flow.state_off'), tone: 'neutral' };
  }

  /** *Off · Needs attention · Unsaved changes*: what the head's one badge cannot say as well. */
  function facts(head: RuleHead): string | undefined {
    if (!head.stored) return undefined;
    const said = [
      ...(head.isBroken ? [head.stored.enabled ? t('app.flow.state_on') : t('app.flow.state_off')] : []),
      ...(head.needsAttention && !head.isBroken ? [t('app.flow.health_attention')] : []),
      ...(head.isAutomatic ? [t('app.flow.name_generated')] : []),
      ...(head.isDirty ? [t('app.flow.unsaved')] : []),
    ];
    return said.length > 0 ? said.join(' · ') : undefined;
  }

  function menu(head: RuleHead): MenuItem[] {
    const stored = head.stored;
    if (!stored) return [];
    return [
      ...(head.mayToggle
        ? [{ id: 'toggle', label: stored.enabled ? t('app.flow.disable') : t('app.flow.enable'), icon: stored.enabled ? ('pause' as const) : ('play' as const), disabledReason: head.toggleRefusal }]
        : []),
      ...(head.mayStart ? [{ id: 'start', label: t('app.flow.manual_button'), icon: 'play' as const, disabledReason: head.startRefusal }] : []),
      { id: 'rename', label: t('app.flow.name_own'), icon: 'pencil' as const },
      ...(head.mayDelete ? [{ id: 'delete', label: t('app.flow.delete'), icon: 'trash' as const, isDestructive: true, hasSeparatorBefore: true }] : []),
    ];
  }

  function act(head: RuleHead, choice: string): void {
    if (choice === 'toggle') head.toggle();
    else if (choice === 'start') head.start();
    else if (choice === 'rename') head.rename();
    else if (choice === 'delete') head.remove();
  }
</script>

<div class="rule-page">
  <RuleEditor {id} {isOwn} {onnavigate}>
    {#snippet frame(head, body)}
      <RecordPage
        record={{
          kind: isOwn ? t('app.flow.record_kind_own') : t('app.flow.record_kind'),
          title: head.status === 'reading' ? t('app.flow.reading') : head.status === 'missing' ? t('app.flow.not_found') : head.name,
          back: { label: isOwn ? t('app.rules.mine_title') : t('app.rules.title'), href: base, onnavigate: () => onnavigate(base) },
          status: status(head),
          facts: facts(head),
          edit: head.stored && head.maySave
            ? { label: t('app.flow.save'), icon: 'check', opener: 'save-rule', onclick: head.save, disabledReason: head.isWorking ? t('app.flow.saving') : undefined }
            : undefined,
          menu: head.stored ? { label: t('app.page.more_for', { name: head.name }), items: menu(head), onselect: (choice) => act(head, choice) } : undefined,
        }}
        {lists}
      >
        {#snippet list()}
          {@render body()}
        {/snippet}
      </RecordPage>
    {/snippet}
  </RuleEditor>
</div>

<style>
  /* The canvas fills what the frame gives a filled page (`page.fill()`): the page kind and its one
     region pass the height down to the editor, and the head keeps the editor's own padding. */
  .rule-page { display: flex; flex-direction: column; block-size: 100%; min-block-size: 0; }
  .rule-page :global([data-page-kind]) { flex: 1 1 auto; min-block-size: 0; }
  .rule-page :global([data-page-kind] > header) { padding: var(--sp-200) var(--sp-200) 0; }
  .rule-page :global([data-page-kind] > section) { flex: 1 1 auto; min-block-size: 0; gap: var(--sp-100); }
  .rule-page :global([data-page-kind] > section > :first-child) { padding-inline: var(--sp-200); }
</style>
