// ABOUTME: Characterization tests for LogsPage — assert rendered content per row.
// ABOUTME: Written before DataTable migration; must pass unchanged after migration.
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import LogsPage from './LogsPage.svelte';

const entries = [
  {
    ID: 'log-001',
    AgentID: 'agent-a',
    Message: 'Tool execution started',
    Tags: ['tool', 'start'],
    CreatedAt: '2026-01-15T10:00:00Z',
  },
  {
    ID: 'log-002',
    AgentID: '',
    Message: 'System event',
    Tags: [],
    CreatedAt: '2026-01-15T11:00:00Z',
  },
];

describe('LogsPage', () => {
  it('renders column headers', () => {
    render(LogsPage, { props: { entries, csrfToken: 't' } });
    expect(screen.getByText('Time')).toBeTruthy();
    expect(screen.getByText('Agent')).toBeTruthy();
    expect(screen.getByText('Message')).toBeTruthy();
    expect(screen.getByText('Tags')).toBeTruthy();
  });

  it('renders message text for each row', () => {
    render(LogsPage, { props: { entries, csrfToken: 't' } });
    expect(screen.getByText('Tool execution started')).toBeTruthy();
    expect(screen.getByText('System event')).toBeTruthy();
  });

  it('renders agent ID or em-dash for empty', () => {
    render(LogsPage, { props: { entries, csrfToken: 't' } });
    expect(screen.getByText('agent-a')).toBeTruthy();
    // empty AgentID shows em-dash (U+2014)
    expect(screen.getByText('—')).toBeTruthy();
  });

  it('renders badge for each tag', () => {
    render(LogsPage, { props: { entries, csrfToken: 't' } });
    expect(screen.getByText('tool')).toBeTruthy();
    expect(screen.getByText('start')).toBeTruthy();
  });

  it('shows empty state with no entries', () => {
    render(LogsPage, { props: { entries: [], csrfToken: 't' } });
    expect(screen.getByText('No log entries')).toBeTruthy();
  });
});
