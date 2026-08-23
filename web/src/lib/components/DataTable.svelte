<!-- ABOUTME: Data-driven table that composes existing table primitives (Table/TableHead/TableBody/TableRow/TableHeader/TableCell). -->
<!-- ABOUTME: Column defs carry an optional per-column cell Snippet; simple pages pass columns+rows and write snippets only for rich cells. -->
<script lang="ts" generics="T">
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';
  import type { DataColumn } from './dataTable.js';

  interface Props {
    columns: DataColumn<T>[];
    rows: T[];
    rowKey: (row: T) => string;
    class?: string;
  }

  let { columns, rows, rowKey, class: className = '' }: Props = $props();

  function plain(row: T, key: string): string {
    const v = (row as Record<string, unknown>)[key];
    return v == null ? '' : String(v);
  }
</script>

<Table class={className}>
  {#snippet children()}
    <TableHead>
      {#snippet children()}
        <!-- Raw <tr> here: TableRow carries data-testid="table-row"; using it in the header
             would pollute the testid count expected by callers scanning for body rows only. -->
        <tr>
          {#each columns as col (col.key)}
            <TableHeader align={col.align}>
              {#snippet children()}{col.header}{/snippet}
            </TableHeader>
          {/each}
        </tr>
      {/snippet}
    </TableHead>
    <TableBody>
      {#snippet children()}
        {#each rows as row (rowKey(row))}
          <TableRow>
            {#snippet children()}
              {#each columns as col (col.key)}
                <TableCell align={col.align}>
                  {#snippet children()}
                    {#if col.cell}{@render col.cell(row)}{:else}{plain(row, col.key)}{/if}
                  {/snippet}
                </TableCell>
              {/each}
            {/snippet}
          </TableRow>
        {/each}
      {/snippet}
    </TableBody>
  {/snippet}
</Table>
