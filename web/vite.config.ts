import { resolve } from 'path';
import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';

export default defineConfig({
  base: '/static/',
  plugins: [tailwindcss(), svelte()],
  build: {
    manifest: true,
    cssCodeSplit: true,
    rollupOptions: {
      input: {
        auto: resolve(__dirname, 'src/islands/auto.ts'),
        chat: resolve(__dirname, 'src/islands/chat.ts'),
      },
      output: {
        entryFileNames: 'js/[name].[hash].js',
        chunkFileNames: 'js/chunks/[name].[hash].js',
        assetFileNames: 'assets/[name].[hash][extname]',
      },
    },
  },
  server: {
    proxy: {
      '/api': 'http://localhost:8080',
      '/health': 'http://localhost:8080',
    },
  },
});
