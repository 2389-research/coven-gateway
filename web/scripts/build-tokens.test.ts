// ABOUTME: Tests for token-utils.ts — pure functions used by build-tokens.ts.
// ABOUTME: Validates flatten and resolveRefs handle valid refs, unresolved refs, and cyclic refs.

import { describe, it, expect } from 'vitest';
import { resolveRefs, flatten } from './token-utils';

describe('flatten', () => {
  it('flattens nested objects into dot-delimited paths', () => {
    const input = { color: { primary: { base: '210 100% 50%' } } };
    expect(flatten(input)).toEqual({
      'color.primary.base': '210 100% 50%',
    });
  });

  it('returns empty object for non-object input', () => {
    expect(flatten(null)).toEqual({});
    expect(flatten('string')).toEqual({});
  });
});

describe('resolveRefs', () => {
  it('resolves valid references', () => {
    const lookup = {
      'color.blue': '210 100% 50%',
      'color.primary': '{color.blue}',
    };
    const errors: string[] = [];
    const result = resolveRefs('{color.blue}', lookup, errors);
    expect(result).toBe('210 100% 50%');
    expect(errors).toHaveLength(0);
  });

  it('resolves chained references', () => {
    const lookup = {
      'color.blue': '210 100% 50%',
      'color.primary': '{color.blue}',
      'color.accent': '{color.primary}',
    };
    const errors: string[] = [];
    const result = resolveRefs('{color.accent}', lookup, errors);
    expect(result).toBe('210 100% 50%');
    expect(errors).toHaveLength(0);
  });

  it('returns raw value when no references are present', () => {
    const errors: string[] = [];
    const result = resolveRefs('16px', {}, errors);
    expect(result).toBe('16px');
    expect(errors).toHaveLength(0);
  });

  it('collects errors for unresolved references', () => {
    const lookup = { 'color.blue': '210 100% 50%' };
    const errors: string[] = [];
    const result = resolveRefs('{color.missing}', lookup, errors);
    expect(result).toBe('{color.missing}');
    expect(errors).toHaveLength(1);
    expect(errors[0]).toContain('Unresolved token reference');
    expect(errors[0]).toContain('color.missing');
  });

  it('collects errors for cyclic references', () => {
    const lookup = {
      'color.a': '{color.b}',
      'color.b': '{color.a}',
    };
    const errors: string[] = [];
    const result = resolveRefs('{color.a}', lookup, errors);
    // The cycle will be detected when color.a is visited again
    expect(errors.length).toBeGreaterThanOrEqual(1);
    expect(errors.some((e) => e.includes('Cyclic token reference'))).toBe(true);
  });

  it('collects multiple errors from a single value', () => {
    const errors: string[] = [];
    const result = resolveRefs('{a.missing} and {b.missing}', {}, errors);
    expect(result).toBe('{a.missing} and {b.missing}');
    expect(errors).toHaveLength(2);
  });

  it('accumulates errors across multiple resolveRefs calls with shared array', () => {
    const lookup = {
      'valid.ref': 'ok',
      'bad.chain': '{nonexistent}',
    };
    const errors: string[] = [];
    resolveRefs('{valid.ref}', lookup, errors);
    expect(errors).toHaveLength(0);

    resolveRefs('{bad.chain}', lookup, errors);
    expect(errors.length).toBeGreaterThan(0);

    // A second bad call appends to the same array
    resolveRefs('{also.missing}', lookup, errors);
    expect(errors.length).toBeGreaterThan(1);
  });
});
