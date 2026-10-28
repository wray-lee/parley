import { test } from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';

import { createRegistry, selectGlossary } from '../core/packs-registry.js';
import { buildTranslatorRequest } from '../core/translator-request.js';

function packFile(name) {
  return { name, text: readFileSync(new URL(`../packs/${name}`, import.meta.url), 'utf8') };
}

function systemFor(registry, src, tgt, packs) {
  const direction = registry.directions.find((d) => d.src === src && d.tgt === tgt);
  const glossary = selectGlossary(registry, { src, tgt, packs });
  return buildTranslatorRequest({ prompt: direction.prompt, glossary }).system;
}

test('system prompt filenames register Directions and derive Language Pairs', () => {
  const registry = createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'zh2ja prompt' },
      { name: 'system_ja2zh.txt', text: 'ja2zh prompt' },
      { name: 'system_zh2fr.txt', text: 'zh2fr prompt' },
    ],
    packFiles: [],
  });

  assert.deepEqual(
    registry.directions.map(({ src, tgt, prompt }) => ({ src, tgt, prompt })),
    [
      { src: 'zh', tgt: 'ja', prompt: 'zh2ja prompt' },
      { src: 'ja', tgt: 'zh', prompt: 'ja2zh prompt' },
      { src: 'zh', tgt: 'fr', prompt: 'zh2fr prompt' },
    ],
  );

  const zhja = registry.pairs.find((p) => p.directions.some((d) => d.src === 'zh'));
  assert.equal(zhja.converse, true);
  assert.deepEqual(zhja.directions, [{ src: 'zh', tgt: 'ja' }, { src: 'ja', tgt: 'zh' }]);

  const zhfr = registry.pairs.find((p) => p.directions.some((d) => d.tgt === 'fr'));
  assert.equal(zhfr.converse, false, 'a one-way Direction only supports Listen');
});

test('a malformed prompt filename is ignored with a log line', () => {
  const warnings = [];
  const registry = createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: 'ok' },
      { name: 'system_broken.txt', text: 'bad' },
      { name: 'system_.txt', text: 'bad' },
    ],
    packFiles: [],
    warn: (line) => warnings.push(line),
  });

  assert.equal(registry.directions.length, 1);
  assert.equal(registry.directions[0].src, 'zh');
  assert.equal(warnings.length, 2);
  assert.match(warnings[0], /system_broken\.txt/);
  assert.match(warnings[1], /system_\.txt/);
});

test('pack filenames register Domain Packs per Direction', () => {
  const warnings = [];
  const registry = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: 'ok' }],
    packFiles: ['conference_zh2ja.tsv', 'conference_ja2zh.tsv', 'broken.tsv'],
    warn: (line) => warnings.push(line),
  });

  assert.deepEqual(registry.packs, [
    { name: 'conference', src: 'zh', tgt: 'ja', file: 'conference_zh2ja.tsv', glossary: [] },
    { name: 'conference', src: 'ja', tgt: 'zh', file: 'conference_ja2zh.tsv', glossary: [] },
  ]);
  assert.equal(warnings.length, 1);
  assert.match(warnings[0], /broken\.tsv/);
});

test('an optional base glossary is injected into the translator request', () => {
  const registry = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: '术语表：\n{{GLOSSARY}}' }],
    glossaryFiles: [
      { name: 'glossary_zh2ja.tsv', text: '预算\t予算\n议事录\t議事録\n' },
    ],
    packFiles: [],
  });

  assert.equal(systemFor(registry, 'zh', 'ja', []), '术语表：\n预算 → 予算\n议事录 → 議事録');
});

test('conference applies to both Directions only when selected', () => {
  const registry = createRegistry({
    promptFiles: [
      { name: 'system_zh2ja.txt', text: '术语表：\n{{GLOSSARY}}' },
      { name: 'system_ja2zh.txt', text: '用語表：\n{{GLOSSARY}}' },
    ],
    packFiles: [packFile('conference_zh2ja.tsv'), packFile('conference_ja2zh.tsv')],
  });

  const zh2ja = systemFor(registry, 'zh', 'ja', ['conference']);
  assert.match(zh2ja, /预算 → 予算/);
  assert.match(zh2ja, /议事录 → 議事録/);

  const ja2zh = systemFor(registry, 'ja', 'zh', ['conference']);
  assert.match(ja2zh, /予算 → 预算/);
  assert.match(ja2zh, /議事録 → 议事录/);

  assert.equal(systemFor(registry, 'zh', 'ja', []), '术语表：\n');
  assert.equal(systemFor(registry, 'ja', 'zh', []), '用語表：\n');
});

test('a Domain Pack entry overrides a base glossary entry for the same source term', () => {
  const registry = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: '{{GLOSSARY}}' }],
    glossaryFiles: [{ name: 'glossary_zh2ja.tsv', text: '预算\t預算\n季度\t四半期\n' }],
    packFiles: [packFile('conference_zh2ja.tsv')],
  });

  assert.deepEqual(selectGlossary(registry, { src: 'zh', tgt: 'ja', packs: ['conference'] }), [
    { src: '预算', tgt: '予算' },
    { src: '季度', tgt: '四半期' },
    { src: '议程', tgt: 'アジェンダ' },
    { src: '决议', tgt: '決議' },
    { src: '议事录', tgt: '議事録' },
    { src: '截止日期', tgt: '締め切り' },
  ]);
});

test('malformed glossary rows are skipped with a log line naming file and line', () => {
  const warnings = [];
  const registry = createRegistry({
    promptFiles: [{ name: 'system_zh2ja.txt', text: '{{GLOSSARY}}' }],
    glossaryFiles: [
      { name: 'glossary_zh2ja.tsv', text: '预算\t予算\nbroken\nonly\tone\textra\n\n' },
    ],
    packFiles: [{ name: 'conference_zh2ja.tsv', text: '议事录\n' }],
    warn: (line) => warnings.push(line),
  });

  assert.deepEqual(registry.directions[0].glossary, [{ src: '预算', tgt: '予算' }]);
  assert.deepEqual(registry.packs[0].glossary, []);
  assert.equal(warnings.length, 3);
  assert.match(warnings[0], /glossary_zh2ja\.tsv:2/);
  assert.match(warnings[1], /glossary_zh2ja\.tsv:3/);
  assert.match(warnings[2], /conference_zh2ja\.tsv:1/);
});
