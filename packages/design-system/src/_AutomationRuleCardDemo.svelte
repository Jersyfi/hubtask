<!-- SPDX-License-Identifier: BUSL-1.1
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  // The workbench's wrapper. Every label is resolved text, so the demo is where the wording lives —
  // and where the six trigger kinds get said in words rather than as their tokens.

  import AutomationRuleCard from './AutomationRuleCard.svelte';
  import Stack from './Stack.svelte';

  const { mode = 'one' }: { mode?: 'one' | 'failing' | 'list' } = $props();

  const labels = { trigger: 'Starts on', actions: 'Actions', runAs: 'Runs as' };
</script>

{#if mode === 'failing'}
  <AutomationRuleCard
    name="Escalate overdue reviews"
    href="#rule"
    trigger={{ label: labels.trigger, value: 'An entry becomes overdue' }}
    actions={{ label: labels.actions, value: '3' }}
    runAs={{ label: labels.runAs, value: 'Ada Lovelace' }}
    isEnabled
    stateLabel="On"
    failureLabel="3 runs failed in a row. Two more and this rule switches itself off."
  />
{:else if mode === 'list'}
  <Stack gap="150">
    <AutomationRuleCard
      name="Escalate overdue reviews"
      href="#rule"
      trigger={{ label: labels.trigger, value: 'An entry becomes overdue' }}
      actions={{ label: labels.actions, value: '3' }}
      runAs={{ label: labels.runAs, value: 'Ada Lovelace' }}
      isEnabled
      stateLabel="On"
    />
    <AutomationRuleCard
      name="Weekly tidy of the done column"
      href="#rule"
      trigger={{ label: labels.trigger, value: 'Every Monday at 07:00, Europe/Berlin' }}
      actions={{ label: labels.actions, value: '1' }}
      runAs={{ label: labels.runAs, value: 'Automation service account' }}
      isEnabled={false}
      stateLabel="Off"
    />
    <AutomationRuleCard
      name="File what arrives in the inbox"
      href="#rule"
      trigger={{ label: labels.trigger, value: 'Something arrives in the jumble inbox' }}
      actions={{ label: labels.actions, value: '2' }}
      runAs={{ label: labels.runAs, value: 'Grace Hopper' }}
      isEnabled
      stateLabel="On"
      failureLabel="1 run failed. Four more in a row and this rule switches itself off."
    />
  </Stack>
{:else}
  <AutomationRuleCard
    name="Escalate overdue reviews"
    href="#rule"
    trigger={{ label: labels.trigger, value: 'An entry becomes overdue' }}
    actions={{ label: labels.actions, value: '3' }}
    runAs={{ label: labels.runAs, value: 'Ada Lovelace' }}
    isEnabled
    stateLabel="On"
  />
{/if}
