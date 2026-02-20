<script lang="ts">
  import Breadcrumb from './Breadcrumb.svelte';
  import BreadcrumbItem from './BreadcrumbItem.svelte';

  type DemoVariant = 'two-level' | 'three-level' | 'current-only';

  interface Props {
    variant?: DemoVariant;
  }

  let { variant = 'two-level' }: Props = $props();
</script>

{#if variant === 'two-level'}
  <Breadcrumb>
    {#snippet children()}
      <BreadcrumbItem href="/admin">
        {#snippet children()}Dashboard{/snippet}
      </BreadcrumbItem>
      <BreadcrumbItem>
        {#snippet children()}Agent Details{/snippet}
      </BreadcrumbItem>
    {/snippet}
  </Breadcrumb>

{:else if variant === 'three-level'}
  <Breadcrumb>
    {#snippet children()}
      <BreadcrumbItem href="/admin">
        {#snippet children()}Dashboard{/snippet}
      </BreadcrumbItem>
      <BreadcrumbItem href="/admin/agents">
        {#snippet children()}Agents{/snippet}
      </BreadcrumbItem>
      <BreadcrumbItem>
        {#snippet children()}coven-agent-1{/snippet}
      </BreadcrumbItem>
    {/snippet}
  </Breadcrumb>

{:else if variant === 'current-only'}
  <Breadcrumb>
    {#snippet children()}
      <BreadcrumbItem>
        {#snippet children()}Principals{/snippet}
      </BreadcrumbItem>
    {/snippet}
  </Breadcrumb>
{/if}
