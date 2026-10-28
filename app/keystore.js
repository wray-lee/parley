// Thin adapter over the third-party secure-keystore plugin. A failure (the
// plugin is 0.0.3-beta.1) is documented behaviour, not a crash: keys fall back
// to an in-memory store for this session and are never written to WebView
// storage. set/get/deleteItem are injected so the fallback is testable.

const MEMORY_WARNING =
  'secure keystore unavailable; API keys stay in memory for this session only';

export function createKeyStore({ setItem, getItem, deleteItem, warn = () => {} } = {}) {
  const memory = new Map();
  let persistent = typeof setItem === 'function' && typeof getItem === 'function';
  let warned = false;

  const degrade = (error) => {
    persistent = false;
    if (!warned) {
      warned = true;
      warn(`${MEMORY_WARNING} (${error?.message ?? error})`);
    }
  };

  return {
    get persistent() {
      return persistent;
    },
    async get(key) {
      if (!persistent) return memory.get(key) ?? null;
      try {
        const value = await getItem(key);
        if (value != null) memory.set(key, value);
        return value ?? null;
      } catch (error) {
        degrade(error);
        return memory.get(key) ?? null;
      }
    },
    async set(key, value) {
      memory.set(key, value);
      if (!persistent) return;
      try {
        await setItem(key, value);
      } catch (error) {
        degrade(error);
      }
    },
    async remove(key) {
      memory.delete(key);
      if (!persistent || typeof deleteItem !== 'function') return;
      try {
        await deleteItem(key);
      } catch (error) {
        degrade(error);
      }
    },
  };
}

export async function loadKeyStore({ warn = () => {} } = {}) {
  if (!globalThis.__TAURI_INTERNALS__) return createKeyStore({ warn });
  try {
    const module = await import('./vendor/plugin-keystore.js');
    return createKeyStore({
      setItem: module.setItem,
      getItem: module.getItem,
      deleteItem: module.deleteItem,
      warn,
    });
  } catch (error) {
    warn(`${MEMORY_WARNING} (${error?.message ?? error})`);
    return createKeyStore({ warn });
  }
}
