<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // Three nested planes, the innermost in the signature colour: the old sketch in the workbench's
  // `Foundations/Tokens` story, kept until the provisional mark of `design-system.md` §9 replaces
  // it. Its fills are semantic tokens, because application code never binds to a primitive.
  let { size = 'md' }: { size?: 'md' | 'lg' } = $props();
</script>

<svg class="site-mark" data-size={size} viewBox="0 0 24 24" aria-hidden="true" focusable="false">
  <rect x="1" y="1" width="22" height="22" rx="6" fill="var(--accent-primary-hover)" />
  <rect x="5" y="5" width="14" height="14" rx="4" fill="var(--accent-primary)" opacity=".8" />
  <rect x="8.5" y="8.5" width="7" height="7" rx="2.2" fill="var(--accent-signature)" />
</svg>
