import { test } from 'node:test';
import assert from 'node:assert/strict';

import { buildTranslatorRequest } from '../core/translator-request.js';

test('builds the fixed system + user message shape with no context', () => {
  const request = buildTranslatorRequest({
    prompt: 'You translate. Glossary:\n{{GLOSSARY}}',
    context: [],
    text: 'hello',
  });

  assert.equal(request.system, 'You translate. Glossary:\n');
  assert.equal(request.user, '[已译上下文]\n[新段落]\nhello');
});

test('{{GLOSSARY}} renders one source → target per line', () => {
  const request = buildTranslatorRequest({
    prompt: '术语表：\n{{GLOSSARY}}',
    glossary: [
      { src: '预算', tgt: '予算' },
      { src: '议事录', tgt: '議事録' },
    ],
    text: 'x',
  });

  assert.equal(request.system, '术语表：\n预算 → 予算\n议事录 → 議事録');
});

test('rolling context labels each previous Segment, newest last', () => {
  const request = buildTranslatorRequest({
    prompt: 'P',
    context: [
      { src: 'a', tgt: 'A' },
      { src: 'b', tgt: 'B' },
    ],
    text: 'c',
  });

  assert.equal(
    request.user,
    '[已译上下文]\n原文：a\n译文：A\n原文：b\n译文：B\n[新段落]\nc',
  );
});
