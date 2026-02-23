import { render, screen } from '@testing-library/svelte';
import { describe, it, expect } from 'vitest';
import { createRawSnippet } from 'svelte';
import Table from './Table.svelte';
import TableHead from './TableHead.svelte';
import TableBody from './TableBody.svelte';
import TableRow from './TableRow.svelte';
import TableHeader from './TableHeader.svelte';
import TableCell from './TableCell.svelte';

describe('Table', () => {
  it('renders with overflow wrapper', () => {
    const children = createRawSnippet(() => ({
      render: () => '<thead><tr><th>Name</th></tr></thead>',
    }));
    render(Table, { props: { children } });
    const wrapper = screen.getByTestId('table');
    expect(wrapper.className).toContain('overflow-x-auto');
    expect(wrapper.querySelector('table')).toBeTruthy();
  });

  it('applies custom class to table', () => {
    const children = createRawSnippet(() => ({
      render: () => '<tbody></tbody>',
    }));
    render(Table, { props: { children, class: 'striped' } });
    const table = screen.getByTestId('table').querySelector('table')!;
    expect(table.className).toContain('striped');
  });
});

describe('TableHead', () => {
  it('renders with background and border', () => {
    const children = createRawSnippet(() => ({
      render: () => '<tr><th>Col</th></tr>',
    }));
    render(TableHead, { props: { children } });
    const thead = screen.getByTestId('table-head');
    expect(thead.className).toContain('bg-surfaceAlt');
    expect(thead.className).toContain('border-b');
  });
});

describe('TableBody', () => {
  it('renders with divide-y dividers', () => {
    const children = createRawSnippet(() => ({
      render: () => '<tr><td>Cell</td></tr>',
    }));
    render(TableBody, { props: { children } });
    const tbody = screen.getByTestId('table-body');
    expect(tbody.className).toContain('divide-y');
  });
});

describe('TableRow', () => {
  it('has hover state class', () => {
    const children = createRawSnippet(() => ({
      render: () => '<td>Cell</td>',
    }));
    render(TableRow, { props: { children } });
    const tr = screen.getByTestId('table-row');
    expect(tr.className).toContain('hover:bg-surfaceHover');
  });
});

describe('TableHeader', () => {
  it('renders with uppercase typography', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Name</span>',
    }));
    render(TableHeader, { props: { children } });
    const th = screen.getByTestId('table-header');
    expect(th.className).toContain('uppercase');
    expect(th.className).toContain('tracking-wider');
  });

  it('supports align prop', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Price</span>',
    }));
    render(TableHeader, { props: { children, align: 'right' } });
    const th = screen.getByTestId('table-header');
    expect(th.className).toContain('text-right');
  });

  it('defaults to left alignment', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Name</span>',
    }));
    render(TableHeader, { props: { children } });
    const th = screen.getByTestId('table-header');
    expect(th.className).toContain('text-left');
  });
});

describe('TableCell', () => {
  it('supports align prop', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>$49</span>',
    }));
    render(TableCell, { props: { children, align: 'right' } });
    const td = screen.getByTestId('table-cell');
    expect(td.className).toContain('text-right');
  });

  it('supports colspan', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>No data</span>',
    }));
    render(TableCell, { props: { children, colspan: 3 } });
    const td = screen.getByTestId('table-cell');
    expect(td.getAttribute('colspan')).toBe('3');
  });

  it('renders with correct typography', () => {
    const children = createRawSnippet(() => ({
      render: () => '<span>Value</span>',
    }));
    render(TableCell, { props: { children } });
    const td = screen.getByTestId('table-cell');
    expect(td.className).toContain('text-fg');
    expect(td.className).toContain('px-4');
  });
});
