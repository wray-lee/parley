#!/usr/bin/env bun
// Run the Conformance Suite against an already-running host.
//
//   bun spec/conformance/run.mjs --url ws://127.0.0.1:8787/ws
//
// The host must run in Demo Mode (mock Providers), as the Bun smoke and the
// future Go demo server do.

import { runConformance } from './cases.mjs';

const index = process.argv.indexOf('--url');
const url = index === -1 ? '' : process.argv[index + 1] ?? '';
if (!url) {
  console.error('usage: bun spec/conformance/run.mjs --url ws://HOST:PORT/ws');
  process.exit(2);
}

try {
  await runConformance({ url, log: console.log });
  console.log('conformance passed');
} catch (error) {
  console.error(`conformance failed: ${error.message}`);
  process.exit(1);
}
