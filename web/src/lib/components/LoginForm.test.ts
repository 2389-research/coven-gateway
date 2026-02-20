import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import LoginForm from './LoginForm.svelte';

describe('LoginForm', () => {
  it('renders username and password fields', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    expect(screen.getByLabelText('Username')).toBeTruthy();
    expect(screen.getByLabelText('Password')).toBeTruthy();
  });

  it('renders form with method POST and action /login', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    const form = screen.getByTestId('login-password-form') as HTMLFormElement;
    expect(form.method).toBe('post');
    expect(form.action).toContain('/login');
  });

  it('includes hidden CSRF token field with correct value', () => {
    render(LoginForm, { props: { csrfToken: 'my-csrf-token' } });
    const form = screen.getByTestId('login-password-form') as HTMLFormElement;
    const hidden = form.querySelector('input[name="csrf_token"]') as HTMLInputElement;
    expect(hidden).toBeTruthy();
    expect(hidden.type).toBe('hidden');
    expect(hidden.value).toBe('my-csrf-token');
  });

  it('shows error alert when error prop is provided', () => {
    render(LoginForm, { props: { csrfToken: 'tok123', error: 'Bad credentials' } });
    const alert = screen.getByRole('alert');
    expect(alert).toBeTruthy();
    expect(alert.textContent).toContain('Bad credentials');
  });

  it('hides error alert when no error', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    expect(screen.queryByRole('alert')).toBeNull();
  });

  it('renders passkey button', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    const btn = screen.getByTestId('passkey-button');
    expect(btn).toBeTruthy();
  });

  it('username and password inputs have required attribute', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    const username = screen.getByLabelText('Username') as HTMLInputElement;
    const password = screen.getByLabelText('Password') as HTMLInputElement;
    expect(username.required).toBe(true);
    expect(password.required).toBe(true);
  });

  it('password field has type password', () => {
    render(LoginForm, { props: { csrfToken: 'tok123' } });
    const password = screen.getByLabelText('Password') as HTMLInputElement;
    expect(password.type).toBe('password');
  });
});
