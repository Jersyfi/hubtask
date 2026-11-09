<!-- SPDX-License-Identifier: Apache-2.0
     Copyright (c) 2026 Jérôme Bastian Winkel -->
<script lang="ts">
  import Grid from './Grid.svelte';
  import ResourceCard from './ResourceCard.svelte';
</script>

<Grid as="ul" columns={{ compact: 1, medium: 2, expanded: 3 }}>
  <ResourceCard title="Scrum team" description="Stories and bugs from backlog to done in sprints." mark="columns-3" chips={['Pages', 'Planning']} state={{ label: 'In use · update', tone: 'success' }} href="#scrum" />
  <ResourceCard title="Approvals" description="Purchase, leave and travel requests, each decided." mark="circle-check" chips={['Service desk']} onopen={() => {}} />
  <ResourceCard title="Household" description="The chores of a household." mark="house" />
</Grid>
