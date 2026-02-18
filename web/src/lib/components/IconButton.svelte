<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLButtonAttributes } from 'svelte/elements';

  type Variant = 'primary' | 'secondary' | 'ghost' | 'danger';
  type Size = 'sm' | 'md' | 'lg';

  interface Props extends HTMLButtonAttributes {
    variant?: Variant;
    size?: Size;
    icon: Snippet;
    'aria-label': string;
    class?: string;
  }

  let {
    variant = 'ghost',
    size = 'md',
    disabled = false,
    icon,
    'aria-label': ariaLabel,
    class: className = '',
    ...rest
  }: Props = $props();

  const baseClasses =
    'inline-flex items-center justify-center rounded-[var(--border-radius-md)] transition-colors duration-[var(--motion-duration-fast)] focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring disabled:opacity-50 disabled:cursor-not-allowed';

  const variantClasses: Record<Variant, string> = {
    primary:
      'bg-accent text-fgOnAccent hover:bg-accentHover',
    secondary:
      'border border-border bg-surface text-fg hover:bg-surfaceHover',
    ghost:
      'text-fgMuted hover:bg-surfaceHover hover:text-fg',
    danger:
      'bg-danger-solidBg text-fgOnAccent hover:bg-danger-subtleFg',
  };

  const sizeClasses: Record<Size, string> = {
    sm: 'h-[var(--sizing-control-sm)] w-[var(--sizing-control-sm)] [&>svg]:h-4 [&>svg]:w-4',
    md: 'h-[var(--sizing-control-md)] w-[var(--sizing-control-md)] [&>svg]:h-5 [&>svg]:w-5',
    lg: 'h-[var(--sizing-control-lg)] w-[var(--sizing-control-lg)] [&>svg]:h-5 [&>svg]:w-5',
  };
</script>

<button
  class="{baseClasses} {variantClasses[variant]} {sizeClasses[size]} {className}"
  {disabled}
  aria-label={ariaLabel}
  data-testid="icon-button"
  {...rest}
>
  {@render icon()}
</button>
