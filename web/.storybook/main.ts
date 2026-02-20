import type { StorybookConfig } from '@storybook/svelte-vite';

const config: StorybookConfig = {
  stories: ['../src/**/*.stories.@(ts|svelte)'],
  addons: ['@storybook/addon-a11y', '@storybook/addon-docs'],
  framework: {
    name: '@storybook/svelte-vite',
    options: {},
  },
  viteFinal: async (config) => {
    // esbuild 0.25.x has a resolution bug where it tries to read
    // `node_modules/react` as a file instead of resolving the package
    // entry point. Pre-bundling react/react-dom fixes this.
    config.optimizeDeps = config.optimizeDeps || {};
    config.optimizeDeps.include = [
      ...(config.optimizeDeps.include || []),
      'react',
      'react-dom',
    ];
    return config;
  },
};

export default config;
