<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The site's signature device (market-analysis.md §4.1): every substantial claim carries a link
  // to the document, the decision record or the CI gate that proves it. A claim with no proof
  // chip beside it is a claim somebody has to justify keeping.
  let { href, label }: { href: string; label: string } = $props();
</script>

<a class="proof" {href} rel="noopener">
  <span class="proof-glyph" aria-hidden="true">&rarr;</span><span class="proof-label">{label}</span>
</a>
