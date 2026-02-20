<script lang="ts">
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import Dialog from './Dialog.svelte';
  import EmptyState from './EmptyState.svelte';
  import Select from './Select.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

  interface Principal {
    ID: string;
    Type: string;
    PubkeyFP: string;
    DisplayName: string;
    Status: string;
    CreatedAt: string;
    LastSeen: string | null;
    Metadata: Record<string, any> | null;
  }

  interface Props {
    principals?: Principal[];
    csrfToken: string;
  }

  let { principals = [] as Principal[], csrfToken }: Props = $props();
  let typeFilter = $state('');
  let statusFilter = $state('');
  let loading = $state(false);
  let deleteTarget = $state<Principal | null>(null);
  let showDeleteDialog = $state(false);

  const typeOptions = [
    { value: 'client', label: 'Client' },
    { value: 'agent', label: 'Agent' },
    { value: 'pack', label: 'Pack' },
  ];

  const statusOptions = [
    { value: 'pending', label: 'Pending' },
    { value: 'approved', label: 'Approved' },
    { value: 'revoked', label: 'Revoked' },
  ];

  const typeVariant: Record<string, 'accent' | 'success' | 'warning' | 'danger' | 'default'> = {
    agent: 'accent',
    client: 'success',
    pack: 'default',
  };

  const statusVariant: Record<string, 'accent' | 'success' | 'warning' | 'danger' | 'default'> = {
    approved: 'success',
    pending: 'warning',
    revoked: 'danger',
  };

  let filtered = $derived(
    principals.filter((p) => {
      if (typeFilter && p.Type !== typeFilter) return false;
      if (statusFilter && p.Status !== statusFilter) return false;
      return true;
    }),
  );

  async function refresh() {
    loading = true;
    try {
      const res = await fetch('/api/admin/principals');
      if (res.ok) {
        principals = await res.json();
      }
    } finally {
      loading = false;
    }
  }

  async function action(method: string, url: string) {
    const res = await fetch(url, {
      method,
      headers: { 'X-CSRF-Token': csrfToken },
    });
    if (res.ok) {
      await refresh();
    }
  }

  function approve(p: Principal) {
    action('POST', `/admin/principals/${p.ID}/approve`);
  }

  function revoke(p: Principal) {
    action('POST', `/admin/principals/${p.ID}/revoke`);
  }

  function confirmDelete(p: Principal) {
    deleteTarget = p;
    showDeleteDialog = true;
  }

  async function executeDelete() {
    if (!deleteTarget) return;
    await action('DELETE', `/admin/principals/${deleteTarget.ID}`);
    showDeleteDialog = false;
    deleteTarget = null;
  }

  function formatTime(iso: string | null): string {
    if (!iso) return '—';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }
</script>

<div data-testid="principals-page">
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Principals Registry
        </h3>
        <div class="flex gap-3">
          <Select
            options={typeOptions}
            placeholder="All Types"
            bind:value={typeFilter}
          />
          <Select
            options={statusOptions}
            placeholder="All Statuses"
            bind:value={statusFilter}
          />
        </div>
      </div>

      <div class="p-6">
        {#if filtered.length === 0}
          <EmptyState
            heading="No principals found"
            description={typeFilter || statusFilter
              ? 'Try adjusting your filters.'
              : 'Principals will appear here when agents or clients register.'}
          />
        {:else}
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Name{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Type{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Status{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Last Seen{/snippet}</TableHeader>
                      <TableHeader align="right">{#snippet children()}Actions{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each filtered as p (p.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <div>
                              <div class="font-[var(--typography-fontWeight-medium)] text-fg">{p.DisplayName}</div>
                              <CodeText class="text-[length:var(--typography-fontSize-xs)] text-fgMuted mt-0.5">
                                {#snippet children()}{p.ID}{/snippet}
                              </CodeText>
                            </div>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <Badge variant={typeVariant[p.Type] ?? 'default'} fill="outline" size="sm">
                              {#snippet children()}{p.Type}{/snippet}
                            </Badge>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <Badge variant={statusVariant[p.Status] ?? 'default'} size="sm">
                              {#snippet children()}{p.Status}{/snippet}
                            </Badge>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{formatTime(p.LastSeen)}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell align="right">
                          {#snippet children()}
                            <div class="flex items-center justify-end gap-2">
                              {#if p.Status === 'pending'}
                                <Button variant="primary" size="sm" onclick={() => approve(p)}>
                                  {#snippet children()}Approve{/snippet}
                                </Button>
                              {/if}
                              {#if p.Status !== 'revoked'}
                                <Button variant="secondary" size="sm" onclick={() => revoke(p)}>
                                  {#snippet children()}Revoke{/snippet}
                                </Button>
                              {/if}
                              <Button variant="danger" size="sm" onclick={() => confirmDelete(p)}>
                                {#snippet children()}Delete{/snippet}
                              </Button>
                            </div>
                          {/snippet}
                        </TableCell>
                      {/snippet}
                    </TableRow>
                  {/each}
                {/snippet}
              </TableBody>
            {/snippet}
          </Table>
        {/if}
      </div>
    {/snippet}
  </Card>
</div>

<Dialog
  open={showDeleteDialog}
  onclose={() => { showDeleteDialog = false; deleteTarget = null; }}
>
  {#snippet children()}
    <div class="flex flex-col gap-4">
      <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
        Delete Principal
      </h3>
      <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">
        Are you sure you want to delete <strong>{deleteTarget?.DisplayName}</strong>? This action cannot be undone.
      </p>
      <div class="flex justify-end gap-3">
        <Button variant="secondary" onclick={() => { showDeleteDialog = false; deleteTarget = null; }}>
          {#snippet children()}Cancel{/snippet}
        </Button>
        <Button variant="danger" onclick={executeDelete}>
          {#snippet children()}Delete{/snippet}
        </Button>
      </div>
    </div>
  {/snippet}
</Dialog>
