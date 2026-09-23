<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The slot a screen mounts the moment in (F6-13): the component at the tier the table said,
  // with the one sentence resolved here in voice-and-tone.md §7's voice - what happened, no
  // exclamation mark doing the work. Mounted only while the switch is on and a moment stands;
  // `onDone` lets it go, and the screen's own token keeps two screens from showing one moment.
  //
  // Where it sits is the caller's: the completed row for tier 1, the parent row for tier 2, and
  // the level itself for tier 3. A caller that cannot find the parent row puts it on the completed
  // one, which is still the right row for the sentence.

  import { untrack } from 'svelte';

  import { Celebration } from '@hubtask/design-system/components';

  import { celebration, type Current } from '../celebration.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    current: Current;
  }

  const { current }: Props = $props();

  /**
   * The token this slot was mounted for, read once.
   *
   * `onDone` fires from an `animationend`, and two slots can stand for one moment - the entry's
   * head and its row in a list beside it. The first to end dismisses the moment, which takes both
   * of them out of the tree; the second's event still arrives, and a prop read during that
   * teardown answers `undefined` rather than the moment, which threw where the screen sat still
   * long enough to see it. The token cannot move under this slot: a moment is always let go before
   * the next one is shown, so the `{#if}` around this component unmounts it in between.
   *
   * `untrack` says that reading it once is the point rather than an oversight.
   */
  const token = untrack(() => current.token);

  const announcement = $derived(
    t(`app.celebration.${current.reason}`, { title: current.item.title }),
  );
</script>

<Celebration tier={current.tier} {announcement} onDone={() => celebration.dismiss(token)} />
