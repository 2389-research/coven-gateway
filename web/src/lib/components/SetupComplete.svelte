<!--
  ABOUTME: Setup completion island — success state with optional API token display.
  ABOUTME: Mounted via data-island="setup-complete" in setup_complete.html Go template.
-->
<script lang="ts">
  import Card from './Card.svelte';
  import Alert from './Alert.svelte';
  import Stack from './Stack.svelte';
  import CopyButton from './CopyButton.svelte';

  interface Props {
    displayName: string;
    apiToken?: string;
    hasToken: boolean;
    grpcAddress: string;
  }

  let { displayName, apiToken = '', hasToken, grpcAddress }: Props = $props();
</script>

<div class="min-h-screen flex items-center justify-center p-4" data-testid="setup-complete">
  <div class="w-full max-w-lg">
    <!-- Logo/Brand -->
    <div class="text-center mb-8">
      <div class="inline-flex items-center gap-3 mb-4">
        <div
          class="w-10 h-10 border border-success-subtleBorder bg-success-subtleBg flex items-center justify-center"
        >
          <svg
            class="w-5 h-5 text-success-subtleFg"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="2"
              d="M5 13l4 4L19 7"
            />
          </svg>
        </div>
        <span class="font-mono text-2xl font-bold tracking-tight">COVEN</span>
      </div>
      <p class="text-success-subtleFg font-mono text-xs tracking-widest uppercase">
        Setup Complete
      </p>
    </div>

    <!-- Success Card -->
    <Card>
      {#snippet header()}
        <div class="flex items-center gap-2">
          <div class="w-2 h-2 rounded-full bg-success-solidBg"></div>
          <span class="font-mono text-xs text-text-secondary uppercase tracking-wider">Ready</span>
        </div>
      {/snippet}

      <Stack gap="4">
        <p class="text-fg">
          Welcome, <strong>{displayName}</strong>! Your admin account has been created and
          you're now logged in.
        </p>

        {#if hasToken}
          <Alert variant="warning">
            {#snippet children()}
              Save this token — it won't be shown again.
            {/snippet}
          </Alert>

          <div class="rounded-[var(--border-radius-md)] border border-border bg-bgMuted p-4">
            <div class="flex items-center justify-between mb-2">
              <span class="text-xs font-medium text-fgMuted uppercase tracking-wider">API Token</span>
              <CopyButton value={apiToken} label="Copy Token" />
            </div>
            <pre
              class="text-xs font-mono bg-surface p-3 rounded-[var(--border-radius-sm)] border border-border overflow-x-auto whitespace-pre-wrap break-all"
              data-testid="api-token">{apiToken}</pre>
          </div>

          <div class="rounded-[var(--border-radius-md)] border border-border bg-bgMuted p-4">
            <span class="text-xs font-medium text-fgMuted uppercase tracking-wider block mb-2">CLI Usage</span>
            <pre
              class="text-xs font-mono bg-surface p-3 rounded-[var(--border-radius-sm)] border border-border overflow-x-auto whitespace-pre-wrap"
>export COVEN_TOKEN="{apiToken}"
export COVEN_GATEWAY_GRPC="{grpcAddress}"
coven-admin me</pre>
          </div>
        {/if}

        <a
          href="/"
          class="block w-full text-center rounded-[var(--border-radius-md)] bg-accent px-4 py-2.5 text-[length:var(--typography-fontSize-sm)] font-[var(--typography-fontWeight-medium)] text-fgOnAccent transition-colors duration-[var(--motion-duration-fast)] hover:bg-accentHover focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-ring"
          data-testid="start-chatting"
        >
          Start Chatting
        </a>
      </Stack>
    </Card>

    <!-- Footer -->
    <p class="text-center text-text-secondary font-mono text-xs mt-6">
      coven-gateway <span class="opacity-50">|</span> agent control plane
    </p>
  </div>
</div>
