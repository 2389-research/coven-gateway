<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Button from './Button.svelte';
  import Card from './Card.svelte';
  import Dialog from './Dialog.svelte';
  import EmptyState from './EmptyState.svelte';
  import RevealField from './RevealField.svelte';
  import Select from './Select.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';

  interface SecretItem {
    ID: string;
    Key: string;
    Value: string;
    AgentID: string;
    AgentName: string;
    Scope: string;
    UpdatedAt: string;
  }

  interface Agent {
    ID: string;
    Name: string;
  }

  interface Props {
    secrets?: SecretItem[];
    agents?: Agent[];
    userName?: string;
    csrfToken: string;
  }

  let { secrets = [] as SecretItem[], agents = [] as Agent[], userName = '', csrfToken }: Props = $props();

  let scopeFilter = $state('');
  let loading = $state(false);

  // Create form state
  let newKey = $state('');
  let newValue = $state('');
  let newAgentId = $state('');
  let creating = $state(false);
  let createError = $state('');

  // Delete confirmation
  let deleteTarget = $state<SecretItem | null>(null);
  let showDeleteDialog = $state(false);

  // Reveal state: track which secret IDs have their value fetched
  let revealedValues = $state<Record<string, string>>({});

  const scopeOptions = [
    { value: 'global', label: 'Global Only' },
    { value: 'agent', label: 'Agent-Specific' },
  ];

  let agentOptions = $derived(
    agents.map((a) => ({ value: a.ID, label: `${a.Name} (${a.ID})` })),
  );

  let filtered = $derived(
    secrets.filter((s) => {
      if (scopeFilter === 'global' && s.Scope !== 'Global') return false;
      if (scopeFilter === 'agent' && s.Scope === 'Global') return false;
      return true;
    }),
  );

  async function refresh() {
    loading = true;
    try {
      const url = '/api/admin/secrets';
      const res = await fetch(url);
      if (res.ok) {
        secrets = await res.json();
      }
    } finally {
      loading = false;
    }
  }

  async function createSecret() {
    createError = '';
    if (!newKey.trim() || !newValue.trim()) {
      createError = 'Key and value are required.';
      return;
    }

    creating = true;
    try {
      const form = new FormData();
      form.set('key', newKey.trim());
      form.set('value', newValue);
      if (newAgentId) {
        form.set('agent_id', newAgentId);
      }

      const res = await fetch('/admin/secrets', {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken },
        body: form,
      });

      if (res.ok) {
        newKey = '';
        newValue = '';
        newAgentId = '';
        await refresh();
      } else {
        const text = await res.text();
        createError = text || 'Failed to create secret.';
      }
    } finally {
      creating = false;
    }
  }

  async function revealValue(secret: SecretItem) {
    if (revealedValues[secret.ID]) return; // already fetched
    const res = await fetch(`/admin/secrets/${secret.ID}/value`, {
      headers: { 'X-CSRF-Token': csrfToken },
    });
    if (res.ok) {
      const data = await res.json();
      revealedValues = { ...revealedValues, [secret.ID]: data.value };
    }
  }

  function confirmDelete(s: SecretItem) {
    deleteTarget = s;
    showDeleteDialog = true;
  }

  async function executeDelete() {
    if (!deleteTarget) return;
    const res = await fetch(`/admin/secrets/${deleteTarget.ID}`, {
      method: 'DELETE',
      headers: { 'X-CSRF-Token': csrfToken },
    });
    if (res.ok) {
      showDeleteDialog = false;
      deleteTarget = null;
      await refresh();
    }
  }

  function formatTime(iso: string): string {
    if (!iso) return '\u2014';
    const d = new Date(iso);
    return d.toLocaleDateString('en-US', { month: 'short', day: '2-digit' }) +
      ' ' + d.toLocaleTimeString('en-US', { hour: '2-digit', minute: '2-digit', hour12: false });
  }
</script>

<AdminLayout activePage="secrets" {userName} {csrfToken}>
<div data-testid="secrets-page" class="space-y-6 p-6">
  <!-- Create Secret Form -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center gap-3">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Add Secret
        </h3>
      </div>
      <div class="p-6">
        {#if createError}
          <div class="mb-4 px-4 py-2 bg-[var(--cg-danger-subtleBg)] border border-[var(--cg-danger-subtleBorder)] rounded-[var(--border-radius-md)] text-[var(--cg-danger-subtleFg)] text-[length:var(--typography-fontSize-sm)]">
            {createError}
          </div>
        {/if}
        <div class="grid grid-cols-1 md:grid-cols-4 gap-4">
          <div>
            <label for="secret-key" class="block text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg mb-1">Key</label>
            <input
              type="text"
              id="secret-key"
              bind:value={newKey}
              placeholder="ANTHROPIC_API_KEY"
              class="w-full px-3 py-2 bg-surface border border-border rounded-[var(--border-radius-md)] text-fg text-[length:var(--typography-fontSize-sm)] focus:border-ring focus:ring-1 focus:ring-ring outline-none"
            />
          </div>
          <div>
            <label for="secret-value" class="block text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg mb-1">Value</label>
            <input
              type="password"
              id="secret-value"
              bind:value={newValue}
              placeholder="sk-..."
              class="w-full px-3 py-2 bg-surface border border-border rounded-[var(--border-radius-md)] text-fg text-[length:var(--typography-fontSize-sm)] focus:border-ring focus:ring-1 focus:ring-ring outline-none"
            />
          </div>
          <div>
            <label for="secret-scope" class="block text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg mb-1">Scope</label>
            <Select
              options={agentOptions}
              placeholder="Global (all agents)"
              bind:value={newAgentId}
            />
          </div>
          <div class="flex items-end">
            <Button variant="primary" onclick={createSecret} disabled={creating} class="w-full">
              {#snippet children()}{creating ? 'Adding...' : 'Add Secret'}{/snippet}
            </Button>
          </div>
        </div>
      </div>
    {/snippet}
  </Card>

  <!-- Secrets List -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex flex-col sm:flex-row sm:items-center sm:justify-between gap-4">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Secrets Registry
        </h3>
        <div class="flex gap-3">
          <Select
            options={scopeOptions}
            placeholder="All Scopes"
            bind:value={scopeFilter}
          />
        </div>
      </div>

      <div class="p-6">
        {#if filtered.length === 0}
          <EmptyState
            heading="No secrets found"
            description={scopeFilter
              ? 'Try adjusting your scope filter.'
              : 'Secrets will appear here when you add them.'}
          />
        {:else}
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Key{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Scope{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Value{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Updated{/snippet}</TableHeader>
                      <TableHeader align="right">{#snippet children()}Actions{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each filtered as s (s.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <span class="font-mono text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fg">{s.Key}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <Badge variant={s.Scope === 'Global' ? 'accent' : 'default'} fill="outline" size="sm">
                              {#snippet children()}{s.Scope === 'Global' ? 'Global' : s.AgentName || s.AgentID}{/snippet}
                            </Badge>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            {#if revealedValues[s.ID]}
                              <RevealField value={revealedValues[s.ID]} />
                            {:else}
                              <button
                                type="button"
                                class="text-[length:var(--typography-fontSize-sm)] text-fgMuted hover:text-fg underline"
                                onclick={() => revealValue(s)}
                              >
                                Reveal
                              </button>
                            {/if}
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{formatTime(s.UpdatedAt)}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell align="right">
                          {#snippet children()}
                            <Button variant="danger" size="sm" onclick={() => confirmDelete(s)}>
                              {#snippet children()}Delete{/snippet}
                            </Button>
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
        Delete Secret
      </h3>
      <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">
        Are you sure you want to delete <strong class="font-mono">{deleteTarget?.Key}</strong>? This action cannot be undone.
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
</AdminLayout>
