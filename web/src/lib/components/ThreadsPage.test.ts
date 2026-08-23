// ABOUTME: Characterization tests for ThreadsPage — assert rendered content per row.
// ABOUTME: Written before DataTable migration; must pass unchanged after migration.
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ThreadsPage from './ThreadsPage.svelte';

const threads = [
  {
    ID: 'thread-001',
    FrontendName: 'matrix',
    ExternalID: 'ext-abc',
    AgentID: 'agent-xyz',
    CreatedAt: '2026-01-15T10:00:00Z',
    UpdatedAt: '2026-01-15T11:30:00Z',
  },
  {
    ID: 'thread-002',
    FrontendName: 'http',
    ExternalID: 'ext-def',
    AgentID: 'agent-pqr',
    CreatedAt: '2026-01-16T08:00:00Z',
    UpdatedAt: '2026-01-16T09:00:00Z',
  },
];

describe('ThreadsPage', () => {
  it('renders column headers', () => {
    render(ThreadsPage, { props: { threads, csrfToken: 't' } });
    expect(screen.getByText('Thread')).toBeTruthy();
    expect(screen.getByText('Agent')).toBeTruthy();
    expect(screen.getByText('Frontend')).toBeTruthy();
    expect(screen.getByText('Updated')).toBeTruthy();
    expect(screen.getByText('Actions')).toBeTruthy();
  });

  it('renders thread IDs and agent IDs as code text', () => {
    render(ThreadsPage, { props: { threads, csrfToken: 't' } });
    // IDs ≤ 12 chars display verbatim; truncateId only appends ... for longer IDs
    expect(screen.getByText('thread-001')).toBeTruthy();
    expect(screen.getByText('agent-xyz')).toBeTruthy();
  });

  it('renders frontend name for each row', () => {
    render(ThreadsPage, { props: { threads, csrfToken: 't' } });
    expect(screen.getByText('matrix')).toBeTruthy();
    expect(screen.getByText('http')).toBeTruthy();
  });

  it('renders View link for each thread', () => {
    render(ThreadsPage, { props: { threads, csrfToken: 't' } });
    const links = screen.getAllByText('View') as HTMLAnchorElement[];
    expect(links).toHaveLength(2);
    expect(links[0].getAttribute('href')).toBe('/admin/threads/thread-001');
    expect(links[1].getAttribute('href')).toBe('/admin/threads/thread-002');
  });

  it('shows empty state with no threads', () => {
    render(ThreadsPage, { props: { threads: [], csrfToken: 't' } });
    expect(screen.getByText('No threads yet')).toBeTruthy();
  });
});
