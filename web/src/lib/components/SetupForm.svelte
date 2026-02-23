<!--
  ABOUTME: Full-page setup wizard island — creates first admin user + optional API principal.
  ABOUTME: Mounted via data-island="setup-form" in setup.html Go template.
-->
<script lang="ts">
  import Card from './Card.svelte';
  import TextField from './TextField.svelte';
  import Button from './Button.svelte';
  import Alert from './Alert.svelte';
  import Stack from './Stack.svelte';

  interface Props {
    csrfToken: string;
    error?: string;
  }

  let { csrfToken, error }: Props = $props();
</script>

<div class="min-h-screen flex items-center justify-center p-4" data-testid="setup-form">
  <div class="w-full max-w-md">
    <!-- Logo/Brand -->
    <div class="text-center mb-8">
      <div class="inline-flex items-center gap-3 mb-4">
        <div
          class="w-10 h-10 border border-border-bright bg-surface-raised flex items-center justify-center"
        >
          <svg
            class="w-5 h-5 text-accent-primary"
            fill="none"
            stroke="currentColor"
            viewBox="0 0 24 24"
            aria-hidden="true"
          >
            <path
              stroke-linecap="round"
              stroke-linejoin="round"
              stroke-width="1.5"
              d="M9 3v2m6-2v2M9 19v2m6-2v2M5 9H3m2 6H3m18-6h-2m2 6h-2M7 19h10a2 2 0 002-2V7a2 2 0 00-2-2H7a2 2 0 00-2 2v10a2 2 0 002 2zM9 9h6v6H9V9z"
            />
          </svg>
        </div>
        <span class="font-mono text-2xl font-bold tracking-tight">COVEN</span>
      </div>
      <p class="text-text-secondary font-mono text-xs tracking-widest uppercase">
        Initial Setup
      </p>
    </div>

    <!-- Setup Card -->
    <Card>
      {#snippet header()}
        <div class="flex items-center gap-2">
          <div class="w-2 h-2 rounded-full bg-accent-primary animate-pulse"></div>
          <span class="font-mono text-xs text-text-secondary uppercase tracking-wider"
            >Welcome</span
          >
        </div>
      {/snippet}

      <Stack gap="4">
        <p class="text-text-secondary text-sm">
          Create your admin account to get started managing your agent network.
        </p>

        {#if error}
          <Alert variant="danger" role="alert">
            {#snippet children()}
              {error}
            {/snippet}
          </Alert>
        {/if}

        <form method="POST" action="/setup" data-testid="setup-password-form">
          <input type="hidden" name="csrf_token" value={csrfToken} />
          <Stack gap="4">
            <TextField
              label="Username"
              name="username"
              required
              autocomplete="username"
              hint="3-32 characters, letters, numbers, and underscores"
            />
            <TextField
              label="Display Name"
              name="display_name"
              required
              autocomplete="name"
            />
            <TextField
              label="Password"
              name="password"
              type="password"
              required
              autocomplete="new-password"
              hint="Minimum 8 characters"
            />

            <label class="flex items-start gap-3 cursor-pointer pt-1">
              <input
                type="checkbox"
                name="create_principal"
                checked
                class="mt-0.5 w-4 h-4 rounded border-border bg-surface accent-accent"
              />
              <div>
                <span class="text-sm font-medium text-fg">Create API owner principal</span>
                <p class="text-xs text-fgMuted mt-0.5">
                  Required for agents and API clients to connect. A 30-day token will be generated.
                </p>
              </div>
            </label>

            <Button variant="primary" type="submit">
              {#snippet children()}Complete Setup{/snippet}
            </Button>
          </Stack>
        </form>
      </Stack>
    </Card>

    <!-- Footer -->
    <p class="text-center text-text-secondary font-mono text-xs mt-6">
      Already set up? <a href="/login" class="text-accent hover:underline">Sign in</a>
    </p>
  </div>
</div>
