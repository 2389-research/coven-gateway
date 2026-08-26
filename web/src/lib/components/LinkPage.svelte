<script lang="ts">
  import AdminLayout from './AdminLayout.svelte';
  import Badge from './Badge.svelte';
  import Card from './Card.svelte';
  import CodeText from './CodeText.svelte';
  import EmptyState from './EmptyState.svelte';
  import Table from './Table.svelte';
  import TableHead from './TableHead.svelte';
  import TableBody from './TableBody.svelte';
  import TableRow from './TableRow.svelte';
  import TableHeader from './TableHeader.svelte';
  import TableCell from './TableCell.svelte';
  import { formatTime } from '../utils/time.js';

  interface LinkCodeItem {
    ID: string;
    Code: string;
    Fingerprint: string;
    DeviceName: string;
    Status: string;
    CreatedAt: string;
    ExpiresAt: string;
  }

  interface Props {
    codes?: LinkCodeItem[];
    userName?: string;
    csrfToken: string;
  }

  let { codes = [] as LinkCodeItem[], userName = '', csrfToken }: Props = $props();
  let approving = $state<Record<string, boolean>>({});
  let approved = $state<Record<string, boolean>>({});

  function truncateFingerprint(fp: string): string {
    if (fp.length >= 16) return fp.slice(0, 16) + '...';
    return fp;
  }

  async function approve(id: string) {
    approving = { ...approving, [id]: true };
    try {
      const formData = new URLSearchParams();
      formData.set('csrf_token', csrfToken);
      const res = await fetch(`/admin/link/${id}/approve`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/x-www-form-urlencoded' },
        body: formData.toString(),
      });
      if (res.ok) {
        approved = { ...approved, [id]: true };
      }
    } finally {
      approving = { ...approving, [id]: false };
    }
  }

  async function refresh() {
    try {
      const res = await fetch('/api/admin/link');
      if (res.ok) {
        const data = await res.json();
        codes = data.codes ?? [];
        approved = {};
      }
    } catch {
      // ignore
    }
  }

  interface MintResponse {
    url: string;
    qr: string;
    expires_at: string;
  }

  let minting = $state(false);
  let mintError = $state('');
  let pairURL = $state('');
  let pairQR = $state('');
  let pairExpiresAt = $state<Date | null>(null);
  let remaining = $state(0);
  let ticker: ReturnType<typeof setInterval> | undefined;

  $effect(() => {
    return () => {
      if (ticker) clearInterval(ticker);
    };
  });

  function stopTicker() {
    if (ticker) {
      clearInterval(ticker);
      ticker = undefined;
    }
  }

  function tick() {
    if (!pairExpiresAt) return;
    remaining = Math.max(0, Math.floor((pairExpiresAt.getTime() - Date.now()) / 1000));
    if (remaining <= 0) stopTicker();
  }

  function formatRemaining(s: number): string {
    const m = Math.floor(s / 60);
    const sec = s % 60;
    return `${m}:${String(sec).padStart(2, '0')}`;
  }

  async function mintPairToken() {
    minting = true;
    mintError = '';
    try {
      const res = await fetch('/api/admin/link/pair-token', {
        method: 'POST',
        headers: { 'X-CSRF-Token': csrfToken },
      });
      if (!res.ok) {
        mintError = res.status === 409 ? await res.text() : 'Failed to create pairing code';
        return;
      }
      const data: MintResponse = await res.json();
      pairURL = data.url;
      pairQR = data.qr;
      pairExpiresAt = new Date(data.expires_at);
      stopTicker();
      tick();
      ticker = setInterval(tick, 1000);
    } catch {
      mintError = 'Failed to create pairing code';
    } finally {
      minting = false;
    }
  }
</script>

<AdminLayout activePage="dashboard" {userName} {csrfToken}>
<div data-testid="link-page" class="max-w-4xl mx-auto p-6 space-y-6">
  <!-- Header -->
  <div>
    <h1 class="text-[length:var(--typography-fontSize-2xl)] font-[var(--typography-fontWeight-bold)] text-fg">
      Device Linking
    </h1>
    <p class="text-fgMuted text-[length:var(--typography-fontSize-sm)] mt-1">Approve devices requesting to connect</p>
  </div>

  <!-- Pending Codes Table -->
  <Card>
    {#snippet children()}
      <div class="px-6 py-4 border-b border-border flex items-center justify-between">
        <h3 class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-semibold)] text-fg">
          Pending Requests
        </h3>
        <button
          type="button"
          class="text-[length:var(--typography-fontSize-sm)] text-fgMuted hover:text-fg"
          onclick={refresh}
        >
          Refresh
        </button>
      </div>

      <div class="p-6">
        {#if codes.length === 0}
          <EmptyState
            heading="No Pending Requests"
            description="When a device runs coven-link, it will appear here for approval."
          />
        {:else}
          <Table>
            {#snippet children()}
              <TableHead>
                {#snippet children()}
                  <TableRow>
                    {#snippet children()}
                      <TableHeader>{#snippet children()}Code{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Device{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Fingerprint{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Expires{/snippet}</TableHeader>
                      <TableHeader>{#snippet children()}Action{/snippet}</TableHeader>
                    {/snippet}
                  </TableRow>
                {/snippet}
              </TableHead>
              <TableBody>
                {#snippet children()}
                  {#each codes as code (code.ID)}
                    <TableRow>
                      {#snippet children()}
                        <TableCell>
                          {#snippet children()}
                            <CodeText class="text-[length:var(--typography-fontSize-lg)] font-[var(--typography-fontWeight-bold)]">
                              {#snippet children()}{code.Code}{/snippet}
                            </CodeText>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="font-[var(--typography-fontWeight-medium)] text-fg">{code.DeviceName}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <CodeText class="text-[length:var(--typography-fontSize-xs)] text-fgMuted">
                              {#snippet children()}{truncateFingerprint(code.Fingerprint)}{/snippet}
                            </CodeText>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            <span class="text-fgMuted">{formatTime(code.ExpiresAt, { seconds: true })}</span>
                          {/snippet}
                        </TableCell>
                        <TableCell>
                          {#snippet children()}
                            {#if approved[code.ID]}
                              <Badge variant="success" size="sm">
                                {#snippet children()}Approved{/snippet}
                              </Badge>
                            {:else}
                              <button
                                type="button"
                                onclick={() => approve(code.ID)}
                                disabled={approving[code.ID]}
                                class="px-3 py-1.5 bg-[var(--color-primary)] text-[var(--color-primaryFg)] text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] rounded-[var(--border-radius-md)] hover:opacity-90 transition-opacity disabled:opacity-50"
                              >
                                {approving[code.ID] ? 'Approving...' : 'Approve'}
                              </button>
                            {/if}
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

  <!-- QR Pairing Section -->
  <Card>
    {#snippet children()}
      <div class="p-6">
        <h3 class="font-[var(--typography-fontWeight-semibold)] text-fg mb-3">Pair by QR code</h3>
        <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mb-4">
          Generate a single-use code, then scan it with the Coven app (or paste the link on macOS).
          It expires after 5 minutes.
        </p>
        {#if mintError}
          <p class="text-[length:var(--typography-fontSize-sm)] text-danger-subtleFg mb-4" role="alert">{mintError}</p>
        {/if}
        {#if pairQR && remaining > 0}
          <div class="flex flex-col items-start gap-3">
            <img src={pairQR} alt="Pairing QR code" width="256" height="256" class="rounded-[var(--border-radius-md)] bg-white p-2" />
            <CodeText class="text-[length:var(--typography-fontSize-xs)] break-all">
              {#snippet children()}{pairURL}{/snippet}
            </CodeText>
            <div class="flex items-center gap-3">
              <span class="text-[length:var(--typography-fontSize-sm)] text-fgMuted">
                Expires in {formatRemaining(remaining)}
              </span>
              <button
                type="button"
                onclick={mintPairToken}
                disabled={minting}
                class="text-[length:var(--typography-fontSize-sm)] text-fgMuted hover:text-fg disabled:opacity-50"
              >
                {minting ? 'Generating...' : 'New code'}
              </button>
            </div>
          </div>
        {:else}
          {#if pairExpiresAt && remaining <= 0}
            <p class="text-[length:var(--typography-fontSize-sm)] text-fgMuted mb-4">
              This code has expired — generate another.
            </p>
          {/if}
          <button
            type="button"
            onclick={mintPairToken}
            disabled={minting}
            class="px-3 py-1.5 bg-[var(--color-primary)] text-[var(--color-primaryFg)] text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] rounded-[var(--border-radius-md)] hover:opacity-90 transition-opacity disabled:opacity-50"
          >
            {minting ? 'Generating...' : 'Generate QR code'}
          </button>
        {/if}
      </div>
    {/snippet}
  </Card>

  <!-- How-to Section -->
  <Card>
    {#snippet children()}
      <div class="p-6">
        <h3 class="font-[var(--typography-fontWeight-semibold)] text-fg mb-3">How to link a device</h3>
        <ol class="space-y-2 text-[length:var(--typography-fontSize-sm)] text-fgMuted">
          <li class="flex gap-2">
            <span class="bg-surfaceAlt px-2 py-0.5 rounded-[var(--border-radius-sm)] text-[length:var(--typography-fontSize-xs)] font-mono">1</span>
            Install coven-link on the device
          </li>
          <li class="flex gap-2">
            <span class="bg-surfaceAlt px-2 py-0.5 rounded-[var(--border-radius-sm)] text-[length:var(--typography-fontSize-xs)] font-mono">2</span>
            Run <CodeText class="text-[length:var(--typography-fontSize-xs)]">{#snippet children()}coven-link your-gateway-url{/snippet}</CodeText>
          </li>
          <li class="flex gap-2">
            <span class="bg-surfaceAlt px-2 py-0.5 rounded-[var(--border-radius-sm)] text-[length:var(--typography-fontSize-xs)] font-mono">3</span>
            Enter the displayed code here and click Approve
          </li>
          <li class="flex gap-2">
            <span class="bg-surfaceAlt px-2 py-0.5 rounded-[var(--border-radius-sm)] text-[length:var(--typography-fontSize-xs)] font-mono">4</span>
            The device is now configured!
          </li>
        </ol>
      </div>
    {/snippet}
  </Card>
</div>
</AdminLayout>
