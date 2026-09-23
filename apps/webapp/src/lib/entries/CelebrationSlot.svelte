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

  import { Celebration } from '@hubtask/design-system/components';

  import { celebration, type Current } from '../celebration.svelte.ts';
  import { t } from '../i18n/i18n.svelte.ts';

  interface Props {
    current: Current;
  }

  const { current }: Props = $props();

  const announcement = $derived(
    t(`app.celebration.${current.reason}`, { title: current.item.title }),
  );
</script>

<Celebration tier={current.tier} {announcement} onDone={() => celebration.dismiss(current.token)} />
