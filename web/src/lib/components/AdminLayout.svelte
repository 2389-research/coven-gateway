<script lang="ts">
  import type { Snippet } from 'svelte';
  import AppShell from './AppShell.svelte';
  import SidebarNav from './SidebarNav.svelte';

  interface Props {
    activePage: string;
    userName: string;
    csrfToken: string;
    children: Snippet;
  }

  let { activePage, userName, csrfToken, children }: Props = $props();

  const navGroups = [
    {
      label: 'Admin',
      items: [
        { id: 'dashboard', label: 'Dashboard', href: '/admin/' },
        { id: 'agents', label: 'Agents', href: '/admin/agents' },
        { id: 'principals', label: 'Principals', href: '/admin/principals' },
        { id: 'secrets', label: 'Secrets', href: '/admin/secrets' },
        { id: 'tools', label: 'Tools', href: '/admin/tools' },
        { id: 'threads', label: 'Threads', href: '/admin/threads' },
        { id: 'usage', label: 'Usage', href: '/admin/usage' },
      ],
    },
    {
      label: 'Activity',
      items: [
        { id: 'logs', label: 'Activity Logs', href: '/admin/logs' },
        { id: 'todos', label: 'Todos', href: '/admin/todos' },
        { id: 'board', label: 'Discussion Board', href: '/admin/board' },
      ],
    },
  ];

  const groups = navGroups.map((g) => ({
    ...g,
    items: g.items.map((i) => ({ ...i, active: i.id === activePage })),
  }));
</script>

<AppShell>
  {#snippet sidebar()}
    <div class="flex h-full flex-col">
      <div class="p-4">
        <a
          href="/"
          class="flex items-center gap-2 rounded-[var(--border-radius-md)] px-3 py-2 text-[length:var(--typography-fontSize-sm)] text-fgMuted transition-colors duration-[var(--motion-duration-fast)] hover:bg-surfaceHover hover:text-fg"
          data-testid="chat-link"
        >
          <svg class="h-4 w-4" fill="none" stroke="currentColor" viewBox="0 0 24 24">
            <path stroke-linecap="round" stroke-linejoin="round" stroke-width="1.5" d="M8 12h.01M12 12h.01M16 12h.01M21 12c0 4.418-4.03 8-9 8a9.863 9.863 0 01-4.255-.949L3 20l1.395-3.72C3.512 15.042 3 13.574 3 12c0-4.418 4.03-8 9-8s9 3.582 9 8z" />
          </svg>
          <span>Chat</span>
        </a>
      </div>
      <SidebarNav {groups} class="flex-1 px-2" />
      <div class="border-t border-border p-4 text-[length:var(--typography-fontSize-xs)] text-fgMuted" data-testid="sidebar-footer">
        coven-gateway v0.1
      </div>
    </div>
  {/snippet}
  {#snippet header()}
    <div class="flex w-full items-center justify-between" data-testid="admin-header">
      <a href="/admin/" class="flex items-center gap-2">
        <span class="font-serif font-[var(--typography-fontWeight-semibold)] text-fg">Coven</span>
        <span class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">Control Plane</span>
      </a>
      <div class="flex items-center gap-4">
        <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted" data-testid="user-name">{userName}</span>
        <form method="POST" action="/admin/logout" data-testid="logout-form">
          <input type="hidden" name="csrf_token" value={csrfToken} />
          <button type="submit" class="text-[length:var(--typography-fontSize-sm)] text-fgMuted transition-colors hover:text-fg">
            Sign Out
          </button>
        </form>
      </div>
    </div>
  {/snippet}
  {@render children()}
</AppShell>
