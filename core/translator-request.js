// The fixed translator request format. The four bracket labels are protocol
// markers documented for contributors; do not translate them.

const CONTEXT = '[已译上下文]';
const SOURCE = '原文：';
const TARGET = '译文：';
const NEW = '[新段落]';

export function buildTranslatorRequest({ prompt = '', context = [], text = '', glossary = [] }) {
  const terms = glossary.map(({ src, tgt }) => `${src} → ${tgt}`).join('\n');
  const system = String(prompt).replaceAll('{{GLOSSARY}}', terms);
  const lines = [CONTEXT];
  for (const pair of context) {
    lines.push(`${SOURCE}${pair.src}`, `${TARGET}${pair.tgt}`);
  }
  lines.push(NEW, text);
  return { system, user: lines.join('\n') };
}
