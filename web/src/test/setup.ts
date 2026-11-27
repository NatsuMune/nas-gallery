// Vitest runs in the node environment; the app code uses localStorage, so we
// install a minimal spec-compliant stub (no jsdom dependency needed).

class LocalStorageStub {
  private store = new Map<string, string>();

  getItem(key: string): string | null {
    return this.store.has(key) ? (this.store.get(key) as string) : null;
  }

  setItem(key: string, value: string): void {
    this.store.set(String(key), String(value));
  }

  removeItem(key: string): void {
    this.store.delete(key);
  }

  clear(): void {
    this.store.clear();
  }

  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null;
  }

  get length(): number {
    return this.store.size;
  }
}

export function installLocalStorageStub(): void {
  const stub = new LocalStorageStub();
  Object.defineProperty(globalThis, 'localStorage', {
    configurable: true,
    writable: true,
    value: stub,
  });
}

installLocalStorageStub();
