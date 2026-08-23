// ABOUTME: Shared timestamp formatting for admin pages.
// ABOUTME: Formats ISO strings as "Mon DD HH:MM" (optionally with seconds); empty input renders an em-dash.

const EM_DASH = '—';

export function formatTime(iso: string, opts?: { seconds?: boolean }): string {
  if (!iso) return EM_DASH;
  const d = new Date(iso);
  return (
    d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
    ' ' +
    d.toLocaleTimeString('en-US', {
      hour: '2-digit',
      minute: '2-digit',
      ...(opts?.seconds ? { second: '2-digit' as const } : {}),
      hour12: false,
    })
  );
}
