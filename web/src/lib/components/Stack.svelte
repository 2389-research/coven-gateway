<script lang="ts">
  import type { Snippet } from 'svelte';

  interface Props {
    direction?: 'vertical' | 'horizontal';
    gap?: '0' | '1' | '2' | '3' | '4' | '6' | '8' | '12' | '16';
    align?: 'start' | 'center' | 'end' | 'stretch';
    justify?: 'start' | 'center' | 'end' | 'between';
    wrap?: boolean;
    children: Snippet;
    class?: string;
  }

  let {
    direction = 'vertical',
    gap = '4',
    align = 'stretch',
    justify = 'start',
    wrap = false,
    children,
    class: className = '',
  }: Props = $props();

  const gapMap: Record<string, string> = {
    '0': 'gap-0',
    '1': 'gap-1',
    '2': 'gap-2',
    '3': 'gap-3',
    '4': 'gap-4',
    '6': 'gap-6',
    '8': 'gap-8',
    '12': 'gap-12',
    '16': 'gap-16',
  };

  const alignMap: Record<string, string> = {
    start: 'items-start',
    center: 'items-center',
    end: 'items-end',
    stretch: 'items-stretch',
  };

  const justifyMap: Record<string, string> = {
    start: 'justify-start',
    center: 'justify-center',
    end: 'justify-end',
    between: 'justify-between',
  };
</script>

<div
  class="flex {direction === 'horizontal' ? 'flex-row' : 'flex-col'} {gapMap[gap]} {alignMap[align]} {justifyMap[justify]} {wrap ? 'flex-wrap' : ''} {className}"
  data-testid="stack"
>
  {@render children()}
</div>
