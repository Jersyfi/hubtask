<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Widget from './Widget.svelte';
  import WidgetStrip from './WidgetStrip.svelte';
</script>

<WidgetStrip label="Your day in figures">
  <Widget title="Overdue" tile={{ icon: 'triangle-alert', color: 'red' }} figure="1" figureTone={{ tone: 'danger', word: 'overdue' }} line="since yesterday" />
  <Widget title="Due today" tile={{ icon: 'calendar', color: 'blue' }} figure="4" line="1 of them with a time" />
  <Widget title="Time today" tile={{ icon: 'clock', product: 'time' }} figure="3:10" line="of 7:30 h" />
</WidgetStrip>
