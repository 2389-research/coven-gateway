<script lang="ts">
  import type { Snippet } from 'svelte';
  import type { HTMLInputAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLInputAttributes, 'class' | 'prefix'> {
    label?: string;
    error?: string;
    leading?: Snippet;
    trailing?: Snippet;
    class?: string;
  }

  let {
    label,
    error,
    leading,
    trailing,
    disabled = false,
    id,
    class: className = '',
    ...rest
  }: Props = $props();

  const fallbackId = `tf-${Math.random().toString(36).slice(2, 8)}`;
  let inputId = $derived(id ?? fallbackId);
  let errorId = $derived(`${inputId}-error`);
</script>

<div class="flex flex-col gap-1.5 {className}" data-testid="text-field">
  {#if label}
    <label
      for={inputId}
      class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg"
    >
      {label}
    </label>
  {/if}

  <div
    class="flex items-center h-[var(--sizing-control-md)] rounded-[var(--border-radius-md)] border bg-surface transition-colors duration-[var(--motion-duration-fast)]
      {error
        ? 'border-danger-solidBg'
        : 'border-border focus-within:border-ring'}
      {disabled ? 'opacity-50 cursor-not-allowed' : ''}"
  >
    {#if leading}
      <span class="flex items-center pl-3 text-fgMuted">
        {@render leading()}
      </span>
    {/if}

    <input
      id={inputId}
      class="h-full w-full bg-transparent px-3 text-[length:var(--typography-fontSize-sm)] text-fg placeholder:text-fgMuted focus:outline-none disabled:cursor-not-allowed
        {leading ? 'pl-0' : ''}
        {trailing ? 'pr-0' : ''}"
      {disabled}
      aria-invalid={error ? 'true' : undefined}
      aria-describedby={error ? errorId : undefined}
      {...rest}
    />

    {#if trailing}
      <span class="flex items-center pr-3 text-fgMuted">
        {@render trailing()}
      </span>
    {/if}
  </div>

  {#if error}
    <p id={errorId} class="text-[length:var(--typography-fontSize-sm)] text-danger-subtleFg" role="alert">
      {error}
    </p>
  {/if}
</div>
