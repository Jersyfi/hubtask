<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Property from './Property.svelte';
</script>

<div class="column">
  <Property label="Story points" value="8" />
  <Property label="Assignee" emptyLabel="Nobody – name someone" requiredLabel="required" />
  <Property label="Priority" value="↑ P2 High" note="from impact × urgency" />
  <Property label="Specification" value="wiki.example/checkout/wallets/apple-pay-and-google-pay-in-the-native-apps" />
</div>

<style>
  .column { max-inline-size: var(--layout-rail); }
</style>
