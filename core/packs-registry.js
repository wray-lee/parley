// Builds the Direction / Language Pair / Domain Pack registry from file names.
// File reading is the host's job; this module only parses (no Node built-ins).

const DIRECTION = /^system_([A-Za-z0-9_-]+)2([A-Za-z0-9_-]+)\.txt$/;
const GLOSSARY = /^glossary_([A-Za-z0-9_-]+)2([A-Za-z0-9_-]+)\.tsv$/;
const PACK = /^([A-Za-z0-9-]+)_([A-Za-z0-9-]+)2([A-Za-z0-9-]+)\.tsv$/;

export function createRegistry({
  promptFiles = [],
  glossaryFiles = [],
  packFiles = [],
  warn = () => {},
} = {}) {
  const directions = [];
  for (const file of promptFiles) {
    const name = typeof file === 'string' ? file : file.name;
    const match = DIRECTION.exec(name);
    if (!match) {
      warn(`ignoring malformed prompt filename: ${name}`);
      continue;
    }
    directions.push({
      src: match[1],
      tgt: match[2],
      file: name,
      prompt: typeof file === 'string' ? '' : (file.text ?? ''),
      glossary: [],
    });
  }

  for (const file of glossaryFiles) {
    const name = typeof file === 'string' ? file : file.name;
    const match = GLOSSARY.exec(name);
    if (!match) {
      warn(`ignoring malformed glossary filename: ${name}`);
      continue;
    }
    const direction = directions.find((d) => d.src === match[1] && d.tgt === match[2]);
    if (!direction) {
      warn(`glossary ${name} has no matching Direction`);
      continue;
    }
    direction.glossary = parseGlossary(typeof file === 'string' ? '' : file.text, name, warn);
  }

  const packs = [];
  for (const file of packFiles) {
    const name = typeof file === 'string' ? file : file.name;
    const match = PACK.exec(name);
    if (!match) {
      warn(`ignoring malformed pack filename: ${name}`);
      continue;
    }
    packs.push({
      name: match[1],
      src: match[2],
      tgt: match[3],
      file: name,
      glossary: parseGlossary(typeof file === 'string' ? '' : file.text, name, warn),
    });
  }

  return { directions, packs, pairs: derivePairs(directions) };
}

// Base Glossary for a Direction plus the selected Domain Packs, last source term wins.
export function selectGlossary(registry, { src, tgt, packs = [] } = {}) {
  const selected = new Set(packs);
  const merged = new Map();
  const direction = registry.directions.find((d) => d.src === src && d.tgt === tgt);
  for (const entry of direction?.glossary ?? []) merged.set(entry.src, entry.tgt);
  for (const pack of registry.packs) {
    if (pack.src !== src || pack.tgt !== tgt || !selected.has(pack.name)) continue;
    for (const entry of pack.glossary) merged.set(entry.src, entry.tgt);
  }
  return [...merged].map(([source, target]) => ({ src: source, tgt: target }));
}

function parseGlossary(text, file, warn) {
  const entries = new Map();
  String(text ?? '')
    .split('\n')
    .forEach((raw, index) => {
      const line = index === 0 && raw.charCodeAt(0) === 0xfeff ? raw.slice(1) : raw;
      if (!line.trim()) return;
      const fields = line.split('\t').map((field) => field.trim());
      if (fields.length !== 2 || !fields[0] || !fields[1]) {
        warn(`${file}:${index + 1}: malformed glossary row ignored`);
        return;
      }
      entries.set(fields[0], fields[1]);
    });
  return [...entries].map(([src, tgt]) => ({ src, tgt }));
}

function derivePairs(directions) {
  const groups = new Map();
  for (const { src, tgt } of directions) {
    const [a, b] = [src, tgt].sort();
    const key = `${a}\u0000${b}`;
    let pair = groups.get(key);
    if (!pair) {
      pair = { a, b, directions: [] };
      groups.set(key, pair);
    }
    pair.directions.push({ src, tgt });
  }
  return [...groups.values()].map((pair) => ({
    ...pair,
    converse: pair.directions.length > 1,
  }));
}
