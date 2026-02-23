<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    icon?: Snippet;
    heading: string;
    description?: string;
    action?: Snippet;
    class?: string;
  }

  let {
    icon,
    heading,
    description,
    action,
    class: className = '',
  }: Props = $props();
</script>

<div class="py-16 flex flex-col items-center text-center {className}" data-testid="empty-state">
  {#if icon}
    <div class="text-fgMuted opacity-30 [&>svg]:w-12 [&>svg]:h-12 mb-4">
      {@render icon()}
    </div>
  {/if}

  <h3 class="text-[length:var(--typography-fontSize-base)] font-[var(--typography-fontWeight-semibold)] text-fg">
    {heading}
  </h3>

  {#if description}
    <p class="mt-1 text-[length:var(--typography-fontSize-sm)] text-fgMuted">
      {description}
    </p>
  {/if}

  {#if action}
    <div class="mt-4">
      {@render action()}
    </div>
  {/if}
</div>
