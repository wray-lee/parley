// Provider resolution by convention: kind "asr|translator|tts" plus a plain
// name maps to ./{kind}-{name}.js exporting create(options). No registry edits
// are needed when an adapter file is added.

const KINDS = new Set(['asr', 'translator', 'tts']);
const NAME = /^[a-z][a-z0-9-]*$/;
const factories = new Map();

export async function loadProvider(kind, name) {
  if (!KINDS.has(kind)) throw new Error(`unknown provider kind: ${kind}`);
  if (!NAME.test(String(name))) throw new Error(`unknown provider: ${kind}/${name}`);
  const key = `${kind}-${name}`;
  if (!factories.has(key)) {
    let module;
    try {
      module = await import(`./${key}.js`);
    } catch (error) {
      if (error?.code === 'ERR_MODULE_NOT_FOUND') {
        throw new Error(`unknown provider: ${kind}/${name}`, { cause: error });
      }
      throw error;
    }
    if (typeof module.create !== 'function') {
      throw new Error(`provider ${key} does not export create()`);
    }
    factories.set(key, module.create);
  }
  return factories.get(key);
}

export async function createProvider(kind, name, options = {}) {
  const create = await loadProvider(kind, name);
  return create(options);
}
