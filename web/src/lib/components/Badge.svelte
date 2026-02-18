<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'default' | 'accent' | 'success' | 'warning' | 'danger';
  type Size = 'sm' | 'md';

  interface Props {
    variant?: Variant;
    size?: Size;
    children: Snippet;
    class?: string;
  }

  let {
    variant = 'default',
    size = 'md',
    children,
    class: className = '',
  }: Props = $props();

  const variantClasses: Record<Variant, string> = {
    default: 'bg-[hsl(var(--color-surfaceAlt))] text-[hsl(var(--color-fgMuted))]',
    accent: 'bg-[hsl(var(--color-accentMuted))] text-[hsl(var(--color-accent))]',
    success: 'bg-[hsl(var(--color-success-subtleBg))] text-[hsl(var(--color-success-subtleFg))]',
    warning: 'bg-[hsl(var(--color-warning-subtleBg))] text-[hsl(var(--color-warning-subtleFg))]',
    danger: 'bg-[hsl(var(--color-danger-subtleBg))] text-[hsl(var(--color-danger-subtleFg))]',
  };

  const sizeClasses: Record<Size, string> = {
    sm: 'px-1.5 py-0.5 text-[length:var(--typography-fontSize-xs)]',
    md: 'px-2 py-0.5 text-[length:var(--typography-fontSize-sm)]',
  };
</script>

<span
  class="inline-flex items-center rounded-[var(--border-radius-pill)] font-[var(--typography-fontWeight-medium)] leading-none {variantClasses[variant]} {sizeClasses[size]} {className}"
  data-testid="badge"
>
  {@render children()}
</span>
