<script lang="ts">
  import type { HTMLSelectAttributes } from 'svelte/elements';

  interface Props extends Omit<HTMLSelectAttributes, 'class' | 'value'> {
    label?: string;
    error?: string;
    options: Array<{ value: string; label: string; disabled?: boolean }>;
    placeholder?: string;
    value?: string;
    class?: string;
  }

  let {
    label,
    error,
    options,
    placeholder,
    disabled = false,
    id,
    value = $bindable(''),
    class: className = '',
    ...rest
  }: Props = $props();

  const fallbackId = `sel-${Math.random().toString(36).slice(2, 8)}`;
  let selectId = $derived(id ?? fallbackId);
  let errorId = $derived(`${selectId}-error`);
</script>

<div class="flex flex-col gap-1.5 {className}" data-testid="select">
  {#if label}
    <label
      for={selectId}
      class="text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg"
    >
      {label}
    </label>
  {/if}

  <select
    id={selectId}
    class="h-[var(--sizing-control-md)] w-full appearance-none rounded-[var(--border-radius-md)] border bg-surface px-3 pr-8 text-[length:var(--typography-fontSize-sm)] text-fg transition-colors duration-[var(--motion-duration-fast)] bg-no-repeat bg-[position:right_0.5rem_center] bg-[length:1.25rem_1.25rem] bg-[image:url('data:image/svg+xml;charset=utf-8,%3Csvg%20xmlns%3D%22http%3A%2F%2Fwww.w3.org%2F2000%2Fsvg%22%20viewBox%3D%220%200%2020%2020%22%20fill%3D%22%236b7280%22%3E%3Cpath%20fill-rule%3D%22evenodd%22%20d%3D%22M5.23%207.21a.75.75%200%20011.06.02L10%2011.168l3.71-3.938a.75.75%200%20111.08%201.04l-4.25%204.5a.75.75%200%2001-1.08%200l-4.25-4.5a.75.75%200%2001.02-1.06z%22%20clip-rule%3D%22evenodd%22%2F%3E%3C%2Fsvg%3E')]
      {error ? 'border-danger-solidBg' : 'border-border focus:border-ring'}
      {disabled ? 'opacity-50 cursor-not-allowed' : ''}
      focus:outline-none"
    {disabled}
    bind:value
    aria-invalid={error ? 'true' : undefined}
    aria-describedby={error ? errorId : undefined}
    {...rest}
  >
    {#if placeholder}
      <option value="">{placeholder}</option>
    {/if}
    {#each options as opt}
      <option value={opt.value} disabled={opt.disabled}>{opt.label}</option>
    {/each}
  </select>

  {#if error}
    <p id={errorId} class="text-[length:var(--typography-fontSize-sm)] text-danger-subtleFg" role="alert">
      {error}
    </p>
  {/if}
</div>
