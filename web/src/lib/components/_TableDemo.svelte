<script lang="ts">
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';
  import Badge from './Badge.svelte';

  type DemoVariant = 'basic' | 'actions' | 'empty' | 'aligned';

  interface Props {
    variant?: DemoVariant;
  }

  let { variant = 'basic' }: Props = $props();
</script>

{#if variant === 'basic'}
  <Table>
    {#snippet children()}
      <TableHead>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableHeader>
                {#snippet children()}Name{/snippet}
              </TableHeader>
              <TableHeader>
                {#snippet children()}Type{/snippet}
              </TableHeader>
              <TableHeader>
                {#snippet children()}Status{/snippet}
              </TableHeader>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableHead>
      <TableBody>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}coven-agent-1{/snippet}</TableCell>
              <TableCell>{#snippet children()}Agent{/snippet}</TableCell>
              <TableCell>
                {#snippet children()}
                  <Badge variant="success" size="sm">{#snippet children()}Online{/snippet}</Badge>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}admin-user{/snippet}</TableCell>
              <TableCell>{#snippet children()}Human{/snippet}</TableCell>
              <TableCell>
                {#snippet children()}
                  <Badge variant="default" size="sm">{#snippet children()}Idle{/snippet}</Badge>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}matrix-bridge{/snippet}</TableCell>
              <TableCell>{#snippet children()}System{/snippet}</TableCell>
              <TableCell>
                {#snippet children()}
                  <Badge variant="danger" size="sm">{#snippet children()}Error{/snippet}</Badge>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableBody>
    {/snippet}
  </Table>

{:else if variant === 'actions'}
  <Table>
    {#snippet children()}
      <TableHead>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableHeader>{#snippet children()}Principal{/snippet}</TableHeader>
              <TableHeader>{#snippet children()}Scope{/snippet}</TableHeader>
              <TableHeader align="right">{#snippet children()}Actions{/snippet}</TableHeader>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableHead>
      <TableBody>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}admin-key{/snippet}</TableCell>
              <TableCell>
                {#snippet children()}
                  <Badge variant="accent" fill="outline" size="sm">{#snippet children()}admin{/snippet}</Badge>
                {/snippet}
              </TableCell>
              <TableCell align="right">
                {#snippet children()}
                  <button type="button" class="text-danger-subtleFg hover:underline text-[length:var(--typography-fontSize-sm)]">Revoke</button>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}read-only{/snippet}</TableCell>
              <TableCell>
                {#snippet children()}
                  <Badge variant="success" fill="outline" size="sm">{#snippet children()}read{/snippet}</Badge>
                {/snippet}
              </TableCell>
              <TableCell align="right">
                {#snippet children()}
                  <button type="button" class="text-danger-subtleFg hover:underline text-[length:var(--typography-fontSize-sm)]">Revoke</button>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableBody>
    {/snippet}
  </Table>

{:else if variant === 'empty'}
  <Table>
    {#snippet children()}
      <TableHead>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableHeader>{#snippet children()}Name{/snippet}</TableHeader>
              <TableHeader>{#snippet children()}Status{/snippet}</TableHeader>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableHead>
      <TableBody>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableCell colspan={2}>
                {#snippet children()}
                  <span class="block py-8 text-center text-fgMuted">No data available</span>
                {/snippet}
              </TableCell>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableBody>
    {/snippet}
  </Table>

{:else if variant === 'aligned'}
  <Table>
    {#snippet children()}
      <TableHead>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableHeader>{#snippet children()}Item{/snippet}</TableHeader>
              <TableHeader align="center">{#snippet children()}Quantity{/snippet}</TableHeader>
              <TableHeader align="right">{#snippet children()}Price{/snippet}</TableHeader>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableHead>
      <TableBody>
        {#snippet children()}
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}Agent License{/snippet}</TableCell>
              <TableCell align="center">{#snippet children()}5{/snippet}</TableCell>
              <TableCell align="right">{#snippet children()}$49.00{/snippet}</TableCell>
            {/snippet}
          </TableRow>
          <TableRow>
            {#snippet children()}
              <TableCell>{#snippet children()}Support Plan{/snippet}</TableCell>
              <TableCell align="center">{#snippet children()}1{/snippet}</TableCell>
              <TableCell align="right">{#snippet children()}$199.00{/snippet}</TableCell>
            {/snippet}
          </TableRow>
        {/snippet}
      </TableBody>
    {/snippet}
  </Table>
{/if}
