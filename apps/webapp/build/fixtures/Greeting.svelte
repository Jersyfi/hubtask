<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // A screen for lazyview.test.js: it draws the one prop it is handed.
  let { name = '' }: { name?: string } = $props();
</script>

<p data-greeting>{name}</p>
