<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Three nested planes, the innermost in the signature bordeaux: the mark sketched in the
  // workbench's `Foundations/Tokens` story. `design-system.md` §9 still lists the wordmark as
  // unfinished, and this is the only drawn record of the idea - so the website uses it rather
  // than inventing a second one, which is what would actually be hard to undo later.
  let { size = 'md' }: { size?: 'md' | 'lg' } = $props();
</script>

<svg class="mark" data-size={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
  <rect x="1" y="1" width="22" height="22" rx="6" fill="var(--blue-700)" />
  <rect x="5" y="5" width="14" height="14" rx="4" fill="var(--blue-500)" opacity=".8" />
  <rect x="8.5" y="8.5" width="7" height="7" rx="2.2" fill="var(--ember-400)" />
</svg>
