# Contributing

Parley is Apache-2.0. There is **no CLA**: by opening a pull request you agree to license your contribution under Apache-2.0.

**Open an issue first** for a new Language Pack, Domain Pack, Preset or Provider adapter, so naming, scope and the self-test plan can be agreed before you write it. Small fixes can go straight to a pull request.

Before you start:

- Install **Bun ≥ 1.4**; it runs the server, the tests and the build scripts. Node is not required.
- Never copy code from PolyTalk or any AGPL/GPL/LGPL project. Edge TTS is an independent implementation; keep it that way.
- `prompts/system_zh2ja.txt`, `prompts/system_ja2zh.txt` and `packs/conference_*.tsv` are frozen product assets. Never reword them; add new files instead.
- Keep language-specific logic out of `core/`. No code may branch on a particular language code.
- Never commit keys, `.env`, certificates or keystores.

Run the checks before pushing:

```sh
bun test                      # all tests
bun run lint
bun scripts/demo-smoke.mjs    # keyless Demo Mode over the real Server Host
```

## Testing policy: two seams

Parley tests exactly two seams. Do not add tests that assert private state or internal call order.

1. **Seam 1 — Session core.** Drive a Session with protocol messages, audio frames, a fake clock and scripted fake Providers; assert on emitted protocol events and Segment log records. Covers Cut triggers, Hold/merge/retry, ordering, context and Glossary, Turn routing/Gating, Demo labelling and errors.
2. **Seam 2 — Provider contract.** Replay recorded real wire traffic through one adapter and assert the normalised events it emits. Recordings are captured once from live services, stored under `tests/contract/fixtures/<provider>/`, and **must have keys, request ids and personal endpoints stripped**.

Live checks against real services (`scripts/live-check-*.mjs`) are acceptance scripts, not CI tests. They need keys or local models and write only to gitignored local directories. CI runs lint, the test suite and the keyless demo smoke (`.github/workflows/ci.yml`).

## Adding a Language Pack (a new Direction)

A Language Pack is **one system prompt file** and an optional glossary. No code is required.

| File | Required | Purpose |
|---|---|---|
| `prompts/system_{src}2{tgt}.txt` | Yes | Defines one Direction and carries the prompt. |
| `prompts/glossary_{src}2{tgt}.tsv` | No | Base Glossary for that Direction. |
| `config/pairs.json` entry | No | Per-Direction Cut tuning (clause length/unit). Defaults apply otherwise. |

- `{src}` and `{tgt}` are opaque language codes: lowercase ASCII letters, digits, `-` or `_`. The separator is the literal `2`. For example, `system_en2fr.txt` defines English→French.
- One file = one Direction. Language Pairs are derived automatically: if both `en2fr` and `fr2en` exist, the pair works in Listen and Converse; with one Direction it is listen-only.
- Start from the annotated template below. Keep the prompt itself in the target language's working language; the zh↔ja prompts are written in Chinese.
- Per-language Cut tuning goes in `config/pairs.json`, keyed as `"{src}->{tgt}"`: `{"clauseLength": 12, "unit": "char"}`. `unit` is `char` or `word`; an invalid or missing entry disables the clause-length trigger (silence, accumulation and PTT still cut). Never put thresholds in code.
- **Wiring:** the Bun Server Host loads `prompts/glossary_{src}2{tgt}.tsv` and every `packs/*.tsv`; the Session injects the selected Domain Packs into the translator request.

### The four protocol markers

`core/translator-request.js` builds a fixed request. A prompt must follow this contract:

| Marker | Where | Meaning |
|---|---|---|
| `[已译上下文]` | first line of the user message | Header of the Rolling Context block. |
| `原文：` | before each context source | Context pair source. |
| `译文：` | before each context translation | Context pair translation. |
| `[新段落]` | before the text to translate | The only text that may be translated. |

Two more tokens matter:

- `{{GLOSSARY}}` — a placeholder in the prompt. It is replaced with one `source → target` line per Glossary entry, or with an empty string. Keep it exactly as written, once; if it is absent, Glossary terms are silently unused. Write the surrounding section so it still reads correctly when nothing is injected (the zh↔ja prompts say to ignore the section).
- `⏸` (U+23F8) — the **Hold** answer. It must be the entire trimmed answer and nothing else. The Session merges a Held Segment with the next one and retries; after 2.5 s it appends `prompts/forced_translation.txt` and forces the translation.

Never translate or rename the four markers; the protocol depends on them. The prompt must output only the `[新段落]` translation: no source text, quotes, prefixes or explanations.

### Annotated zh↔ja template

Shared skeleton (keep identical in every Language Pack; adapt the role and the language-specific rules):

```text
# <角色>
<one sentence: you are a {src}→{tgt} interpreter for <this kind of talk>.>
<you receive only the latest short stretch plus earlier context; answer immediately.>

# 输入格式
[已译上下文] <pairs of 原文：/译文：; use only for reference, terminology and tone; never repeat them>
[新段落] <the only thing to translate>

# 输出规则
1. Output only the [新段落] translation in <target language>. No source, quotes, explanation or prefix.
2. If [新段落] is semantically incomplete (missing predicate, unresolved reference), output only: ⏸
3. <register and sentence-length rule>
4. <target-language grammar rule>
5. Numbers, units, names and organisations exactly; never add, drop or rewrite.
6. Use the Glossary below exactly; keep an established rendering for the whole talk.
7. Never lose or reverse negation, condition or causality; do not hide it behind fluency.
8. Delete hesitation fillers (<examples in the source language>).
9. If the previous Segment got a number, name or negation wrong, open this Segment with a short spoken correction;
   otherwise never revise already-played output.

# 术语表
{{GLOSSARY}}
（本节由编排层按当前语言对与所选领域包注入；为空时忽略本节。）
```

Language-specific parts to adapt for a new Direction:

- **Role and audience:** who is listening, domain and formality.
- **Register:** the zh→ja prompt uses です・ます and short spoken sentences; the ja→zh prompt uses simplified spoken Chinese and prefers completeness over style.
- **Grammar:** ja→zh must supply Japanese-omitted subjects/objects; zh→ja must make Chinese predicates and clause order natural Japanese. Translate keigo/授受 by meaning, not form.
- **Correction wording:** the permitted one-line correction at the start of a Segment.
- **Filler examples:** language-specific hesitation words to delete, while keeping genuine uncertainty (for example 「〜かもしれません」 must remain "可能").

Do not copy these rules blindly: a new language may need different rules, but it must keep the shared skeleton, all four markers, `{{GLOSSARY}}` and the `⏸` rule.

## Adding a Domain Pack

A Domain Pack is a data-only glossary, selected per Session (off by default). Put it in:

```text
packs/{name}_{src}2{tgt}.tsv
```

- `{name}` matches `[a-z0-9-]+`; `{src}`/`{tgt}` match `[a-z0-9-]+`. Example: `packs/medical_en2fr.tsv`.
- UTF-8, exactly two tab-separated columns per line: `source term<TAB>target term`.
- No header row and no comment syntax: a line beginning with `#` is treated as a term (or as a malformed row if it has no tab). Blank lines are ignored; cells are trimmed; both cells must be non-empty.
- Malformed rows are ignored with a warning naming `file:line`. A duplicate source term wins with the last occurrence. A selected Pack overrides a base Glossary entry for the same source term.
- Match the target language of the Direction: `conference_zh2ja.tsv` translates Chinese terms into Japanese. A Pack for the reverse Direction is a separate file.
- **Wiring:** the registry parses Packs, `selectGlossary()` merges them over the base Glossary, and the Session passes the result into the translator request. The Bun Server Host loads `glossary_*.tsv` and pack text. Tests: `tests/packs-registry.test.js`, `tests/session.test.js` and `tests/server-host.test.js`.

Example (from `packs/conference_zh2ja.tsv`):

```text
议程	アジェンダ
决议	決議
预算	予算
```

## Adding a Preset

Presets are **data only** in `config/presets.js`. Adding one needs no code and no registry edit; the session loader uses `LLM_PRESET` to find it. You supply only a key (plus a base URL/model for `custom`).

```js
example: {
  id: 'example',
  baseUrl: 'https://api.example.com/v1',
  model: 'example-model',
  trainsOnData: true,             // be conservative and truthful
  region: 'Global / China (mainland-reachable) / EU / local',
  docs: 'https://example.com/docs',
  requiresKey: true,
  note: 'One line shown in Settings.',
},
```

Rules:

- `id` and the object key are the same lowercase slug; `LLM_PRESET=<id>` selects it.
- `baseUrl` is the OpenAI-compatible API root (the adapter appends `/chat/completions`); no trailing slash needed.
- `trainsOnData` must be `true` unless the Provider's current terms clearly say prompts are not used for training. Conservative is correct.
- `requiresKey: false` only when the endpoint genuinely needs no key (for example Ollama).
- `custom` is special: `baseUrl`/`model` are empty and the user must set `LLM_BASE_URL` (and usually `LLM_MODEL`).
- Do not add provider-specific code to the translator adapter. If the API is not OpenAI-compatible, it needs its own adapter (see below).
- Update the Preset table in `README.md` with its region and training status. Free-tier figures are stated as a 2026-10 snapshot; do not promise quotas.

## Self-test for a new Language Pack or Domain Pack

Run these before opening the PR. Use the bundled samples with a real ASR/translator, or the mock pipeline for the structural checks. The sample WAVs are 24 kHz; convert one to 16 kHz mono before feeding it to a live ASR script:

```sh
ffmpeg -i samples/zh-numbers.wav -ar 16000 -ac 1 -c:a pcm_s16le /tmp/zh-numbers-16k.wav
```

| Check | Input | Expected evidence |
|---|---|---|
| **Hold** | `samples/ja-held.wav` or a sentence whose predicate arrives in the next clause | First attempt answers `⏸`; the Segment merges and retries; log shows `held:true`, `merges:1`; a forced translation appears only after the 2.5 s limit. |
| **Numbers** | `samples/zh-numbers.wav` (contains `127.5` and `42`) | Both numbers, units and `n` appear unchanged in the translation; no rounding or rewriting. |
| **Glossary** | zh→ja with the `conference` Domain Pack selected | 预算→予算, 议事录→議事録, 截止日期→締め切り. With the Pack unselected, those mappings are not forced. The reverse Direction needs its own Pack. Also covered by `bun test tests/session.test.js tests/packs-registry.test.js tests/server-host.test.js`. |
| **Context** | `samples/zh-context.wav` / `samples/ja-context.wav` | The second Segment reuses the first Segment's term and pronouns consistently (for example 議事録/预算); context is per Direction and capped at 8 pairs. |
| **Direction / Pair** | New prompt files | One Direction appears as listen-only; both Directions are selectable in Converse. |
| **Regression** | Clean checkout, no `.env` | `bun test`, `bun run lint` and `bun scripts/demo-smoke.mjs` all pass. |

Copy this into the PR description:

```text
Language Pack / Domain Pack: <codes, filenames>
Direction(s): <src2tgt>
Samples used: <ids>
Hold: pass/fail — <evidence>
Numbers: pass/fail — <evidence>
Glossary on/off: pass/fail — <evidence>
Context: pass/fail — <evidence>
Pair availability: Listen/Converse — <evidence>
bun test / lint / demo-smoke: pass/fail
```

For a live check, use the existing scripts with your own key in the shell (never in a file):

```sh
bun scripts/live-check-translator.mjs
bun scripts/live-check-asr.mjs /tmp/zh-numbers-16k.wav zh
```

## Adding an ASR or TTS Provider

Providers are resolved by filename convention; no registry edit is needed.

1. Add `core/providers/asr-{name}.js` or `core/providers/tts-{name}.js`. `{name}` matches `^[a-z][a-z0-9-]*$` and becomes the value of `ASR_PROVIDER` / `TTS_PROVIDER`.
2. Export `create(options)` returning the adapter object. The loader imports `./{kind}-{name}.js` and calls `create`; host options are passed in.
3. Keep it platform-free: inject `fetch`/`WebSocket` through options (as the existing adapters do) and do not import Node built-ins or use Bun-only APIs. The same file must run under Bun and in the app WebView.
4. Emit normalised errors, never throw silently: `{type:'error', provider:'asr'|'tts', kind, message, fallback}` with `kind` one of `unreachable | auth | rate_limit | quota | timeout | protocol | region` and a fallback naming the env variable and a working alternative (for example `ASR_PROVIDER=mock`).

**ASR adapter shape**

```js
export function create({ apiKey = '', WebSocket: WebSocketImpl = globalThis.WebSocket } = {}) {
  return {
    name: 'example',
    acceptsLive: true,            // false for a sample-replay mock
    open(direction) { /* connect; direction.src is the language hint */ },
    push({ pcm, audioMs }) {      // return events for this audio
      return [
        { type: 'partial', text: '…' },                 // display only
        { type: 'stable', text: 'committed prefix' },   // may become a Segment
      ];
    },
    close() {},
  };
}
```

- Use `direction.src`; never auto-detect or hardcode a language.
- Emit Partial for unstable hypotheses and Stable Prefix for confirmed text. If the adapter performs its own Stable Prefix agreement, do **not** set `revisable` (the Session would derive a duplicate prefix).
- `acceptsLive: false` makes the Session reject live microphone audio with a clear error, as the mock does.

**TTS adapter shape**

```js
export function create({ url, apiKey } = {}) {
  return {
    name: 'example',
    async synthesize({ text, seg_id, direction }) {
      // Return independently decodable mp3 for audio, or text for system TTS.
      return { format: 'mp3', data: new Uint8Array() };
      // return { format: 'text', data: null };
    },
  };
}
```

- `direction.tgt` selects the voice/language; `config/voices.json` maps target language → voice (fall back to any voice of that language and log it).
- Each downstream chunk must be independently decodable mp3. Do not rely on the client running a media-source pipeline.

**Tests and wiring**

- Add a Seam 2 test under `tests/contract/` and a stripped fixture under `tests/contract/fixtures/<provider>/`. Run `bun test tests/contract/<file>.test.js`.
- Wire new options into `hosts/node/config.js` and `.env.example`; the app's Settings/provider wiring is in `app/device-host.js` and `app/settings.js` if it should be selectable on-device.
- Add the option to `deploy/README.md` or `README.md` only if users must set it.
- Do not copy a reference implementation from an AGPL/GPL/LGPL project. Write the protocol client from the Provider's public documentation and record its real traffic in the fixture.

## Pull request checklist

- [ ] Scope agreed in an issue (new Pack/Preset/Provider).
- [ ] No frozen asset reworded; no key, `.env`, certificate or keystore committed.
- [ ] Formats and filenames follow this document.
- [ ] `bun test`, `bun run lint`, `bun scripts/demo-smoke.mjs` pass.
- [ ] New non-trivial logic has a Seam 1 or Seam 2 test; live checks are separate.
- [ ] README/Preset table updated if user-facing configuration changed.
- [ ] Contribution is original or clearly licensed for Apache-2.0.
