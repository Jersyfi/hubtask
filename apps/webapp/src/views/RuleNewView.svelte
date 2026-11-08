<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // *Write a rule* (page-system §9.1, §7.7): the rule editor on a create page - the trail, the
  // title, *Save the rule* and *Cancel* in the head, the canvas as the page's one region. Saving
  // lands on the new rule's own page, as every create page does (§5.2).
  import { CreatePage } from '@hubtask/design-system/components';

  import RuleEditor from '../lib/automation/RuleEditor.svelte';
  import { t } from '../lib/i18n/i18n.svelte.ts';

  interface Props {
    /** A new rule on a step: the workflow's key and the step's, from the designer's *Then*. */
    workflow?: string;
    step?: string;
    /** Opened from Your settings: a rule of the reader's own. */
    isOwn?: boolean;
    onnavigate: (path: string) => void;
  }

  const { workflow, step, isOwn = false, onnavigate }: Props = $props();
  const base = $derived(isOwn ? '/profile/rules' : '/administration/rules');
  const leave = () => onnavigate(base);
</script>

<div class="rule-page">
  <RuleEditor id="new" {workflow} {step} {isOwn} {onnavigate}>
    {#snippet frame(head, body)}
      <CreatePage
        head={{
          title: t('app.flow.new_title'),
          breadcrumb: {
            label: isOwn ? t('app.you.trail') : t('app.admin.trail'),
            expandLabel: isOwn ? t('app.you.expand_trail') : t('app.admin.expand_trail'),
            trail: [
              isOwn
                ? { id: 'settings', label: t('app.nav.profile'), href: '/profile' }
                : { id: 'administration', label: t('app.admin.title'), href: '/administration' },
              { id: 'rules', label: isOwn ? t('app.rules.mine_title') : t('app.rules.title'), href: base },
              { id: 'rule', label: t('app.flow.new_title') },
            ],
          },
          primary: head.maySave
            ? { label: t('app.flow.save'), icon: 'check', opener: 'save-rule', onclick: head.save, disabledReason: head.isWorking ? t('app.flow.saving') : undefined }
            : undefined,
          secondary: [{ label: t('app.flow.cancel'), isQuiet: true, onclick: leave }],
        }}
        close={{ label: t('app.flow.cancel'), onclose: leave }}
      >
        {@render body()}
      </CreatePage>
    {/snippet}
  </RuleEditor>
</div>

<style>
  /* The canvas fills what the frame gives a filled page (`page.fill()`), so the page kind passes
     the height down to it; its head sits on the canvas with the editor's own padding. */
  .rule-page { display: flex; flex-direction: column; block-size: 100%; min-block-size: 0; }
  .rule-page :global(.create-page), .rule-page :global([data-page-kind]) { flex: 1 1 auto; min-block-size: 0; gap: 0; }
  .rule-page :global([data-page-kind] > header) { padding: var(--sp-200) var(--sp-200) var(--sp-150); }
  /* design-system-lint-ignore: `primitive.breakpoint.medium` (600px); a media query cannot read a custom property. */
  @media (width < 600px) {
    .rule-page :global(.create-page > .close) { margin: var(--sp-150) var(--sp-200) 0; }
  }
</style>
