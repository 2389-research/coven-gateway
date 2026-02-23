import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { createRawSnippet } from 'svelte';
import Breadcrumb from './Breadcrumb.svelte';
import BreadcrumbItem from './BreadcrumbItem.svelte';

describe('Breadcrumb', () => {
  it('renders nav with aria-label', () => {
    const children = createRawSnippet(() => ({
      render: () => '<li>Home</li>',
    }));
    render(Breadcrumb, { props: { children } });
    const nav = screen.getByTestId('breadcrumb');
    expect(nav.getAttribute('aria-label')).toBe('Breadcrumb');
  });

  it('renders an ordered list', () => {
    const children = createRawSnippet(() => ({
      render: () => '<li>Item</li>',
    }));
    render(Breadcrumb, { props: { children } });
    expect(screen.getByTestId('breadcrumb').querySelector('ol')).toBeTruthy();
  });
});

describe('BreadcrumbItem', () => {
  it('renders as link when href is provided', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Dashboard</span>',
    }));
    render(BreadcrumbItem, { props: { href: '/admin', children } });
    const item = screen.getByTestId('breadcrumb-item');
    const link = item.querySelector('a');
    expect(link).toBeTruthy();
    expect(link!.getAttribute('href')).toBe('/admin');
  });

  it('renders as text when no href', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Current Page</span>',
    }));
    render(BreadcrumbItem, { props: { children } });
    const item = screen.getByTestId('breadcrumb-item');
    expect(item.querySelector('a')).toBeNull();
    expect(item.querySelector('span')).toBeTruthy();
  });

  it('shows separator chevron for link items', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Dashboard</span>',
    }));
    render(BreadcrumbItem, { props: { href: '/admin', children } });
    const item = screen.getByTestId('breadcrumb-item');
    expect(item.querySelector('svg')).toBeTruthy();
  });

  it('does not show separator for current (non-link) items', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Current</span>',
    }));
    render(BreadcrumbItem, { props: { children } });
    const item = screen.getByTestId('breadcrumb-item');
    expect(item.querySelector('svg')).toBeNull();
  });
});
