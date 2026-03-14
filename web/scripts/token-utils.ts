// ABOUTME: Pure utility functions for token resolution used by build-tokens.ts.
// ABOUTME: Extracted to avoid module-level side effects when imported in tests.

// Flatten a nested object into path→value pairs: { "a.b.c": "value" }
export function flatten(obj: unknown, prefix = ''): Record<string, string> {
  const result: Record<string, string> = {};
  if (typeof obj !== 'object' || obj === null) return result;
  for (const [key, value] of Object.entries(obj)) {
    const path = prefix ? `${prefix}.${key}` : key;
    if (typeof value === 'object' && value !== null) {
      Object.assign(result, flatten(value, path));
    } else {
      result[path] = String(value);
    }
  }
  return result;
}

// Resolve {path.to.value} references against a flat lookup.
// Tracks visited refs to prevent infinite recursion on cyclic references.
// Collects error messages into the provided errors array for strict mode enforcement.
export function resolveRefs(
  value: string,
  lookup: Record<string, string>,
  errors: string[] = [],
  visited = new Set<string>(),
): string {
  return value.replace(/\{([^}]+)\}/g, (_, ref: string) => {
    if (visited.has(ref)) {
      const msg = `Cyclic token reference detected: {${ref}}`;
      console.warn(msg);
      errors.push(msg);
      return `{${ref}}`;
    }
    const resolved = lookup[ref];
    if (resolved === undefined) {
      const msg = `Unresolved token reference: {${ref}}`;
      console.warn(msg);
      errors.push(msg);
      return `{${ref}}`;
    }
    // Recursively resolve in case of chained references
    const next = new Set(visited);
    next.add(ref);
    return resolveRefs(resolved, lookup, errors, next);
  });
}
