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
    default: 'bg-surfaceAlt text-fgMuted',
    accent: 'bg-accentMuted text-accent',
    success: 'bg-success-subtleBg text-success-subtleFg',
    warning: 'bg-warning-subtleBg text-warning-subtleFg',
    danger: 'bg-danger-subtleBg text-danger-subtleFg',
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
