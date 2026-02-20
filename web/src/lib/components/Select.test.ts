import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import Select from './Select.svelte';

const options = [
  { value: 'agent', label: 'Agent' },
  { value: 'human', label: 'Human' },
  { value: 'system', label: 'System', disabled: true },
];

describe('Select', () => {
  it('renders all options', () => {
    render(Select, { props: { options } });
    const select = screen.getByTestId('select').querySelector('select')!;
    const opts = select.querySelectorAll('option');
    expect(opts).toHaveLength(3);
    expect(opts[0].textContent).toBe('Agent');
    expect(opts[1].textContent).toBe('Human');
    expect(opts[2].textContent).toBe('System');
  });

  it('renders placeholder as first option', () => {
    render(Select, { props: { options, placeholder: 'All Types' } });
    const select = screen.getByTestId('select').querySelector('select')!;
    const opts = select.querySelectorAll('option');
    expect(opts).toHaveLength(4);
    expect(opts[0].textContent).toBe('All Types');
    expect(opts[0].value).toBe('');
  });

  it('associates label with select via for/id', () => {
    render(Select, { props: { options, label: 'Principal Type' } });
    const label = screen.getByText('Principal Type');
    const select = screen.getByTestId('select').querySelector('select')!;
    expect(label.getAttribute('for')).toBe(select.id);
  });

  it('uses provided id over auto-generated', () => {
    render(Select, { props: { options, id: 'my-select', label: 'Type' } });
    const select = screen.getByTestId('select').querySelector('select')!;
    expect(select.id).toBe('my-select');
  });

  it('shows error with aria-invalid and aria-describedby', () => {
    render(Select, { props: { options, error: 'Required field' } });
    const select = screen.getByTestId('select').querySelector('select')!;
    expect(select.getAttribute('aria-invalid')).toBe('true');
    const errorEl = screen.getByRole('alert');
    expect(errorEl.textContent).toBe('Required field');
    expect(select.getAttribute('aria-describedby')).toBe(errorEl.id);
  });

  it('does not set aria-invalid when no error', () => {
    render(Select, { props: { options } });
    const select = screen.getByTestId('select').querySelector('select')!;
    expect(select.getAttribute('aria-invalid')).toBeNull();
    expect(select.getAttribute('aria-describedby')).toBeNull();
  });

  it('disables the select element', () => {
    render(Select, { props: { options, disabled: true } });
    const select = screen.getByTestId('select').querySelector('select')!;
    expect(select.disabled).toBe(true);
  });

  it('marks disabled options', () => {
    render(Select, { props: { options } });
    const select = screen.getByTestId('select').querySelector('select')!;
    const opts = select.querySelectorAll('option');
    expect(opts[2].disabled).toBe(true);
  });
});
