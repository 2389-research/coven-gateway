<script lang="ts">
  type Size = 'sm' | 'md' | 'lg';

  interface Props {
    size?: Size;
    label?: string;
    class?: string;
  }

  let {
    size = 'md',
    label = 'Loading',
    class: className = '',
  }: Props = $props();

  const sizeClasses: Record<Size, string> = {
    sm: 'h-4 w-4 border-2',
    md: 'h-6 w-6 border-2',
    lg: 'h-8 w-8 border-[3px]',
  };
</script>

<span
  class="inline-block animate-spin rounded-full border-accent border-t-transparent {sizeClasses[size]} {className}"
  role="status"
  aria-label={label}
  data-testid="spinner"
>
  <span class="sr-only">{label}</span>
</span>
