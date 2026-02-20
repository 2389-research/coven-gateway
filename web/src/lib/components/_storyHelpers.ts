/**
 * Helpers for Storybook stories that need Svelte 5 Snippet props.
 *
 * Storybook 8.6 passes CSF3 args as raw values, but Svelte 5 components
 * expect Snippet functions for slot-like props. createRawSnippet bridges
 * the gap by creating real Snippets from plain strings/HTML.
 */
import { createRawSnippet } from 'svelte';

/** Create a Snippet that renders a text string (HTML-escaped). */
export function textSnippet(text: string) {
  return createRawSnippet(() => ({
    render: () => `<span>${text.replace(/&/g, '&amp;').replace(/</g, '&lt;').replace(/>/g, '&gt;')}</span>`,
  }));
}

/**
 * Create a Snippet that renders raw HTML (for icons, rich content).
 * The HTML must be a single root element (e.g. an <svg>).
 * No wrapper is added, preserving DOM structure for CSS child selectors.
 */
export function htmlSnippet(html: string) {
  return createRawSnippet(() => ({
    render: () => html,
  }));
}
