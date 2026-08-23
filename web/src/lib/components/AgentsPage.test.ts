// ABOUTME: Characterization tests for AgentsPage — verifies row-per-agent rendering,
// ABOUTME: status badge (Online/Offline), details links, and empty-state fallback.
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import AgentsPage from './AgentsPage.svelte';

const agents = [
  { id: 'agent-1', name: 'scout', connected: true },
  { id: 'agent-2', name: 'sleeper', connected: false },
];

describe('AgentsPage', () => {
  it('renders one row per agent with status badge and details link', () => {
    render(AgentsPage, { props: { agents, csrfToken: 't' } });
    expect(screen.getByText('scout')).toBeTruthy();
    expect(screen.getByText('agent-1')).toBeTruthy();
    expect(screen.getByText('Online')).toBeTruthy();
    expect(screen.getByText('Offline')).toBeTruthy();
    const links = screen.getAllByText('Details');
    expect(links).toHaveLength(2);
    expect((links[0] as HTMLAnchorElement).getAttribute('href')).toBe('/admin/agents/agent-1');
  });

  it('shows empty state with no agents', () => {
    render(AgentsPage, { props: { agents: [], csrfToken: 't' } });
    expect(screen.getByText('No agents connected')).toBeTruthy();
  });
});
