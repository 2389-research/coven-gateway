import { render, screen, fireEvent } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import RevealField from './RevealField.svelte';

describe('RevealField', () => {
  it('shows mask by default', () => {
    render(RevealField, { props: { value: 'secret-123' } });
    expect(screen.getByText('••••••••')).toBeTruthy();
  });

  it('uses custom mask', () => {
    render(RevealField, { props: { value: 'secret', mask: '********' } });
    expect(screen.getByText('********')).toBeTruthy();
  });

  it('reveals value on toggle click', async () => {
    render(RevealField, { props: { value: 'my-secret-key' } });
    await fireEvent.click(screen.getByTestId('reveal-toggle'));
    expect(screen.getByText('my-secret-key')).toBeTruthy();
  });

  it('hides value on second toggle click', async () => {
    render(RevealField, { props: { value: 'my-secret-key' } });
    const toggle = screen.getByTestId('reveal-toggle');
    await fireEvent.click(toggle);
    expect(screen.getByText('my-secret-key')).toBeTruthy();
    await fireEvent.click(toggle);
    expect(screen.getByText('••••••••')).toBeTruthy();
  });

  it('toggle has correct aria-label', async () => {
    render(RevealField, { props: { value: 'secret' } });
    const toggle = screen.getByTestId('reveal-toggle');
    expect(toggle.getAttribute('aria-label')).toBe('Reveal value');
    await fireEvent.click(toggle);
    expect(toggle.getAttribute('aria-label')).toBe('Hide value');
  });

  it('does not disclose value length via mask', () => {
    render(RevealField, { props: { value: 'ab' } });
    // Mask is fixed-length, not matching value length
    expect(screen.getByText('••••••••')).toBeTruthy();
  });
});
