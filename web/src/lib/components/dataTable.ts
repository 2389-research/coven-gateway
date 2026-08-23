// ABOUTME: Exported types for the DataTable component, kept in a separate module so generic
// ABOUTME: component scripts can import them without triggering svelte-check modifier errors.
// Consumers import this as './dataTable.js' — the .js extension is how Vite/ESM resolves .ts modules.
import type { Snippet } from 'svelte';

export interface DataColumn<T> {
  /** Row property rendered by the default cell. A synthetic key not present on T
      (e.g. 'actions') is allowed but must supply `cell` — the default cell renders '' for it. */
  key: string;
  header: string;
  align?: 'left' | 'center' | 'right';
  cell?: Snippet<[T]>;
}
