// ABOUTME: Exported types for the DataTable component, kept in a separate module so generic
// ABOUTME: component scripts can import them without triggering svelte-check modifier errors.
import type { Snippet } from 'svelte';

export interface DataColumn<T> {
  key: string;
  header: string;
  align?: 'left' | 'center' | 'right';
  cell?: Snippet<[T]>;
}
