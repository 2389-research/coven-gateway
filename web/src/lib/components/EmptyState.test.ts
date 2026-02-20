import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { createRawSnippet } from 'svelte';
import EmptyState from './EmptyState.svelte';

describe('EmptyState', () => {
  it('renders heading', () => {
    render(EmptyState, { props: { heading: 'No agents found' } });
    expect(screen.getByText('No agents found')).toBeTruthy();
  });

  it('renders description when provided', () => {
    render(EmptyState, {
      props: { heading: 'Empty', description: 'Nothing to show here.' },
    });
    expect(screen.getByText('Nothing to show here.')).toBeTruthy();
  });

  it('does not render description when omitted', () => {
    render(EmptyState, { props: { heading: 'Empty' } });
    const container = screen.getByTestId('empty-state');
    expect(container.querySelectorAll('p')).toHaveLength(0);
  });

  it('renders icon snippet', () => {
    const icon = createRawSnippet(() => ({
      render: () => '<svg data-testid="test-icon"></svg>',
    }));
    render(EmptyState, { props: { heading: 'Empty', icon } });
    expect(screen.getByTestId('test-icon')).toBeTruthy();
  });

  it('renders action snippet', () => {
    const action = createRawSnippet(() => ({
      render: () => '<button type="button">Create</button>',
    }));
    render(EmptyState, { props: { heading: 'Empty', action } });
    expect(screen.getByText('Create')).toBeTruthy();
  });

  it('applies custom class', () => {
    render(EmptyState, { props: { heading: 'Empty', class: 'my-custom' } });
    const container = screen.getByTestId('empty-state');
    expect(container.className).toContain('my-custom');
  });
});
