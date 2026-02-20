/**
 * Toast notification store. Any component or island can import addToast()
 * to show ephemeral notifications. The Toast component renders them.
 */

export type ToastVariant = 'info' | 'success' | 'warning' | 'danger';

export interface Toast {
  id: string;
  variant: ToastVariant;
  message: string;
  duration: number;
}

// Reactive toast array — Svelte 5 runes work in .ts modules
let toasts = $state<Toast[]>([]);

export function getToasts(): Toast[] {
  return toasts;
}

export function addToast(
  message: string,
  variant: ToastVariant = 'info',
  duration = 5000,
): string {
  const id = `toast-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`;
  toasts.push({ id, variant, message, duration });

  if (duration > 0) {
    setTimeout(() => removeToast(id), duration);
  }

  return id;
}

export function removeToast(id: string): void {
  toasts = toasts.filter((t) => t.id !== id);
}

export function clearToasts(): void {
  toasts = [];
}
