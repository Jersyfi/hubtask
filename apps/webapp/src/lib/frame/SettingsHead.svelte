<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The head every screen of Your settings wears (ADR-0065 decision 3).
  //
  // Eight screens, one head: the title is the row's own word, the trail is *Your settings › this
  // screen*, and the bar carries the title on a phone. Written once because eight copies of a
  // trail is eight places for it to say something the column does not — the row and the screen it
  // opens have to agree, and here they cannot disagree: both read the same row of the same list.
  //
  // A component of the frame rather than of a view, for the same reason `SectionNav` is: what a
  // section's screen is called is the section's business, and the section is the frame's.

  import { PageHeader } from '@hubtask/design-system/components';

  import { page } from './page.svelte.ts';
  import { viewport } from './viewport.svelte.ts';

  import { t } from '../i18n/i18n.svelte.ts';
  import { SETTINGS } from '../navigation.ts';

  interface Props {
    /** Which row of the section this screen is, by the id the list gives it. */
    row: string;
    /** Lines under the title, where a screen has one: a refusal, a notice. */
    notices?: import('svelte').Snippet;
  }

  const { row, notices }: Props = $props();

  const found = $derived(SETTINGS.flatMap((group) => group.rows).find((each) => each.id === row));
  // The heading's own word where the row has one, because a column has less room than a heading:
  // the row says "Signed in" and the screen says "Where you are signed in".
  const title = $derived(t(found?.titleCode ?? found?.code ?? 'app.nav.profile'));

  // The bar carries the page's title on a phone (ADR-0061 decision 1's table); the head then
  // reads its heading rather than drawing it, so the screen keeps one heading.
  $effect(() => page.entitle(title));
</script>

<PageHeader
  {title}
  isTitleInBar={viewport.isCompact}
  breadcrumb={{
    trail: [
      // The section, then the screen. The first crumb's id is the section's own rather than the
      // row's: on the first screen the two would otherwise be one id twice, and a trail keyed by
      // id draws each crumb once.
      { id: 'settings', label: t('app.nav.profile'), href: '/profile' },
      { id: row, label: title },
    ],
    label: t('app.you.trail'),
    expandLabel: t('app.you.expand_trail'),
  }}
  {notices}
/>
