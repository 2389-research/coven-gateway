// ABOUTME: Characterization tests for TodosPage — assert rendered content per row.
// ABOUTME: Written before DataTable migration; must pass unchanged after migration.
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import TodosPage from './TodosPage.svelte';

const todos = [
  {
    ID: 'todo-001',
    AgentID: 'agent-a',
    Description: 'Write unit tests',
    Status: 'in_progress',
    Priority: 'high',
    Notes: 'focus on edge cases',
    DueDate: null,
    CreatedAt: '2026-01-15T10:00:00Z',
    UpdatedAt: '2026-01-15T10:00:00Z',
  },
  {
    ID: 'todo-002',
    AgentID: '',
    Description: 'Deploy to staging',
    Status: 'done',
    Priority: '',
    Notes: '',
    DueDate: '2026-01-20T00:00:00Z',
    CreatedAt: '2026-01-16T08:00:00Z',
    UpdatedAt: '2026-01-16T08:00:00Z',
  },
];

describe('TodosPage', () => {
  it('renders column headers', () => {
    render(TodosPage, { props: { todos, csrfToken: 't' } });
    expect(screen.getByText('Description')).toBeTruthy();
    expect(screen.getByText('Agent')).toBeTruthy();
    expect(screen.getByText('Status')).toBeTruthy();
    expect(screen.getByText('Priority')).toBeTruthy();
    expect(screen.getByText('Due')).toBeTruthy();
    expect(screen.getByText('Created')).toBeTruthy();
  });

  it('renders description and notes for each row', () => {
    render(TodosPage, { props: { todos, csrfToken: 't' } });
    expect(screen.getByText('Write unit tests')).toBeTruthy();
    expect(screen.getByText('focus on edge cases')).toBeTruthy();
    expect(screen.getByText('Deploy to staging')).toBeTruthy();
  });

  it('renders status badge for each todo', () => {
    render(TodosPage, { props: { todos, csrfToken: 't' } });
    expect(screen.getByText('in_progress')).toBeTruthy();
    expect(screen.getByText('done')).toBeTruthy();
  });

  it('renders priority badge when set, em-dash when not', () => {
    render(TodosPage, { props: { todos, csrfToken: 't' } });
    expect(screen.getByText('high')).toBeTruthy();
    // empty Priority shows em-dash (U+2014); multiple em-dashes may appear (due date, agent, priority)
    expect(screen.getAllByText('—').length).toBeGreaterThanOrEqual(1);
  });

  it('renders agent ID or em-dash when empty', () => {
    render(TodosPage, { props: { todos, csrfToken: 't' } });
    expect(screen.getByText('agent-a')).toBeTruthy();
  });

  it('shows empty state with no todos', () => {
    render(TodosPage, { props: { todos: [], csrfToken: 't' } });
    expect(screen.getByText('No todos')).toBeTruthy();
  });
});
