<script lang="ts">
  import type { Snippet } from 'svelte';

  type Variant = 'default' | 'accent' | 'success' | 'warning' | 'danger';
  type Size = 'sm' | 'md';
  type Fill = 'solid' | 'outline';

  interface Props {
    variant?: Variant;
    size?: Size;
    fill?: Fill;
    children: Snippet;
    class?: string;
  }

  let {
    variant = 'default',
    size = 'md',
    fill = 'solid',
    children,
    class: className = '',
  }: Props = $props();

  const solidClasses: Record<Variant, string> = {
    default: 'bg-surfaceAlt text-fgMuted',
    accent: 'bg-accentMuted text-accent',
    success: 'bg-success-subtleBg text-success-subtleFg',
    warning: 'bg-warning-subtleBg text-warning-subtleFg',
    danger: 'bg-danger-subtleBg text-danger-subtleFg',
  };

  const outlineClasses: Record<Variant, string> = {
    default: 'border border-border bg-transparent text-fgMuted',
    accent: 'border border-accent bg-transparent text-accent',
    success: 'border border-success-subtleFg bg-transparent text-success-subtleFg',
    warning: 'border border-warning-subtleFg bg-transparent text-warning-subtleFg',
    danger: 'border border-danger-subtleFg bg-transparent text-danger-subtleFg',
  };

  const fillClasses: Record<Fill, Record<Variant, string>> = {
    solid: solidClasses,
    outline: outlineClasses,
  };

  const sizeClasses: Record<Size, string> = {
    sm: 'px-1.5 py-0.5 text-[length:var(--typography-fontSize-xs)]',
    md: 'px-2 py-0.5 text-[length:var(--typography-fontSize-sm)]',
  };
</script>

<span
  class="inline-flex items-center rounded-[var(--border-radius-pill)] font-[var(--typography-fontWeight-medium)] leading-none {fillClasses[fill][variant]} {sizeClasses[size]} {className}"
  data-testid="badge"
>
  {@render children()}
</span>
