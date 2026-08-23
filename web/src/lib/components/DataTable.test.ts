// ABOUTME: Tests for DataTable — data-driven table that composes existing table primitives.
// ABOUTME: Covers column headers, row rendering, and empty-state behavior.
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DataTable from './DataTable.svelte';

const rows = [
  { id: 'a1', name: 'alpha', status: 'online' },
  { id: 'b2', name: 'beta', status: 'offline' },
];
const columns = [
  { key: 'name', header: 'Name' },
  { key: 'status', header: 'Status', align: 'right' as const },
];

describe('DataTable', () => {
  it('renders a header per column and a row per item', () => {
    render(DataTable, { props: { columns, rows, rowKey: (r: any) => r.id } });
    expect(screen.getByText('Name')).toBeTruthy();
    expect(screen.getByText('Status')).toBeTruthy();
    expect(screen.getAllByTestId('table-row')).toHaveLength(2);
    expect(screen.getByText('alpha')).toBeTruthy();
    expect(screen.getByText('offline')).toBeTruthy();
  });

  it('renders empty tbody for zero rows', () => {
    render(DataTable, { props: { columns, rows: [], rowKey: (r: any) => r.id } });
    expect(screen.queryAllByTestId('table-row')).toHaveLength(0);
  });
});
