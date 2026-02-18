<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
  type Size = 'sm' | 'md' | 'lg';

  interface Props extends HTMLButtonAttributes {
    variant?: Variant;
    size?: Size;
    loading?: boolean;
    children: Snippet;
    class?: string;
  }

  let {
    variant = 'primary',
    size = 'md',
    loading = false,
    disabled = false,
    children,
    class: className = '',
    ...rest
  }: Props = $props();

  const baseClasses =
    'inline-flex items-center justify-center font-[var(--typography-fontWeight-medium)] rounded-[var(--border-radius-md)] transition-colors duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-[hsl(var(--color-ring))] disabled:opacity-50 disabled:cursor-not-allowed';

  const variantClasses: Record<Variant, string> = {
    primary:
      'bg-[hsl(var(--color-accent))] text-[hsl(var(--color-fgOnAccent))] hover:bg-[hsl(var(--color-accentHover))]',
    secondary:
      'border border-[hsl(var(--color-border))] bg-[hsl(var(--color-surface))] text-[hsl(var(--color-fg))] hover:bg-[hsl(var(--color-surfaceHover))] hover:border-[hsl(var(--color-borderHover))]',
    ghost:
      'text-[hsl(var(--color-fg))] hover:bg-[hsl(var(--color-surfaceHover))]',
    danger:
      'bg-[hsl(var(--color-danger-solidBg))] text-[hsl(var(--color-fgOnAccent))] hover:bg-[hsl(var(--color-danger-subtleFg))]',
  };

  const sizeClasses: Record<Size, string> = {
    sm: 'h-[var(--sizing-control-sm)] px-3 text-[length:var(--typography-fontSize-sm)] gap-1.5',
    md: 'h-[var(--sizing-control-md)] px-4 text-[length:var(--typography-fontSize-sm)] gap-2',
    lg: 'h-[var(--sizing-control-lg)] px-5 text-[length:var(--typography-fontSize-base)] gap-2',
  };
</script>

<button
  class="{baseClasses} {variantClasses[variant]} {sizeClasses[size]} {className}"
  disabled={disabled || loading}
  aria-busy={loading}
  data-testid="button"
  {...rest}
>
  {#if loading}
    <span class="inline-block h-4 w-4 animate-spin rounded-full border-2 border-current border-t-transparent" aria-hidden="true"></span>
  {/if}
  {@render children()}
</button>
