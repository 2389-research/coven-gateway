import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { createRawSnippet } from 'svelte';
import AdminLayout from './AdminLayout.svelte';

function renderLayout(overrides: Record<string, unknown> = {}) {
  return render(AdminLayout, {
    props: {
      activePage: 'dashboard',
      userName: 'alice',
      csrfToken: 'tok123',
      children: createRawSnippet(() => ({
        render: () => '<div data-testid="child-content">page content</div>',
      })),
      ...overrides,
    },
  });
}

describe('AdminLayout', () => {
  it('renders children content', () => {
    renderLayout();
    expect(screen.getByTestId('child-content')).toBeTruthy();
  });

  it('displays user name in header', () => {
    renderLayout({ userName: 'bob' });
    expect(screen.getByTestId('user-name').textContent).toBe('bob');
  });

  it('renders logout form with CSRF token', () => {
    renderLayout({ csrfToken: 'secret-token' });
    const form = screen.getByTestId('logout-form') as HTMLFormElement;
    expect(form.getAttribute('action')).toBe('/admin/logout');
    expect(form.getAttribute('method')).toBe('POST');
    const hidden = form.querySelector('input[type="hidden"]') as HTMLInputElement;
    expect(hidden.value).toBe('secret-token');
  });

  it('highlights active page in sidebar nav', () => {
    renderLayout({ activePage: 'agents' });
    const navItem = screen.getByTestId('nav-item-agents');
    expect(navItem.getAttribute('aria-current')).toBe('page');
  });

  it('does not highlight inactive pages', () => {
    renderLayout({ activePage: 'dashboard' });
    const navItem = screen.getByTestId('nav-item-agents');
    expect(navItem.getAttribute('aria-current')).toBeNull();
  });

  it('renders Chat link in sidebar', () => {
    renderLayout();
    const chatLink = screen.getByTestId('chat-link') as HTMLAnchorElement;
    expect(chatLink.getAttribute('href')).toBe('/');
    expect(chatLink.textContent).toContain('Chat');
  });

  it('renders version footer', () => {
    renderLayout();
    expect(screen.getByTestId('sidebar-footer').textContent).toContain('coven-gateway v0.1');
  });

  it('renders all admin nav items', () => {
    renderLayout();
    for (const id of ['dashboard', 'agents', 'principals', 'secrets', 'tools', 'threads']) {
      expect(screen.getByTestId(`nav-item-${id}`)).toBeTruthy();
    }
  });

  it('renders all activity nav items', () => {
    renderLayout();
    for (const id of ['logs', 'todos', 'board']) {
      expect(screen.getByTestId(`nav-item-${id}`)).toBeTruthy();
    }
  });
});
