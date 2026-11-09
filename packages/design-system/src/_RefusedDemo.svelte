<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The refused content of the Pages stories: what a person without the right sees under the head.
  import ErrorState from './ErrorState.svelte';

  const { what }: { what: string } = $props();
</script>

<ErrorState title={`You may not see ${what}`} description="An administrator of this workspace gives this right. Your role is Member." reference="403 · admin.read" referenceLabel="Details for support" />
