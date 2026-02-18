import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

export default defineConfig({
  plugins: [svelte({ hot: !process.env.VITEST })],
  resolve: {
    // Ensure Svelte resolves the client (browser) export, not the server one.
    // Without this, @testing-library/svelte's mount() fails with
    // "lifecycle_function_unavailable" because jsdom doesn't set the browser condition.
    conditions: ['browser'],
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.{test,spec}.{ts,js}'],
    setupFiles: ['./test/setup.ts'],
    globals: true,
  },
});
