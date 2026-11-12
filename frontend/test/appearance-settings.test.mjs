import test from 'node:test';
import assert from 'node:assert/strict';
import { loadAppearanceSettings, renderWithFixture } from './helpers.mjs';

const DEFAULTS = { editorFont: '', editorFontSize: null, docsFontSize: null, sessionFontSize: null };
const LISTED = { fonts: ['Fira Code'], loading: false, error: '' };

async function render(preferences = {}) {
  const html = await renderWithFixture(await loadAppearanceSettings(), { preferences: { ...DEFAULTS, ...preferences }, installedFonts: LISTED });
  return html.replace(/<!--[\s\S]*?-->/g, '');
}

/** The opening tag of the element labelled `label`. */
const tag = (html, label) => html.match(new RegExp(`<[^>]*aria-label="${label}"[^>]*>`))?.[0] ?? '';

/** The stepper group named `label`: its value and which buttons are disabled. */
function stepper(html, label) {
  const group = html.match(new RegExp(`<div class="stepper[^"]*" role="group" aria-label="${label}">([\\s\\S]*?)</div>`))?.[1];
  assert.ok(group, label);
  const button = (name) => group.match(new RegExp(`<button[^>]*aria-label="${name} ${label.toLowerCase()}"[^>]*>`))[0];
  return {
    value: group.match(/aria-live="polite">([^<]*)</)[1],
    decrease: /disabled/.test(button('Decrease')),
    increase: /disabled/.test(button('Increase')),
    reset: /disabled/.test(button('Reset')),
  };
}

test('Appearance shows the editor, docs and session groups at their defaults', async () => {
  const html = await render();
  for (const heading of ['Appearance', 'Editor', 'Project docs preview', 'Session content']) assert.match(html, new RegExp(`<h3[^>]*>${heading}</h3>`));
  assert.match(html, /headings, code and labels keep their ratio to it/);
  assert.deepEqual(stepper(html, 'Editor font size'), { value: '14px (default)', decrease: false, increase: false, reset: true });
  assert.deepEqual(stepper(html, 'Project docs text size'), { value: '14.5px (default)', decrease: false, increase: false, reset: true });
  assert.deepEqual(stepper(html, 'Session text size'), { value: '14.5px (default)', decrease: false, increase: false, reset: true });
  assert.match(html, /aria-label="Editor font"[\s\S]*?class="trigger-label[^"]*">App default \(JetBrains Mono\)</);
  // The samples use the same custom properties and type scale as the views.
  assert.match(tag(html, 'Editor font preview'), /font-size: var\(--editor-font-size, 0\.78rem\)/);
  assert.match(tag(html, 'Project docs text preview'), /style="--sample-body: var\(--docs-font-size, 14\.5px\);"/);
  assert.match(tag(html, 'Session text preview'), /style="--sample-body: 14\.5px;"/);
});

test('Appearance shows chosen values and disables steps at the limits', async () => {
  const html = await render({ editorFont: 'Fira Code', editorFontSize: 10, docsFontSize: 28, sessionFontSize: 18.5 });
  assert.deepEqual(stepper(html, 'Editor font size'), { value: '10px', decrease: true, increase: false, reset: false });
  assert.deepEqual(stepper(html, 'Project docs text size'), { value: '28px', decrease: false, increase: true, reset: false });
  assert.deepEqual(stepper(html, 'Session text size'), { value: '18.5px', decrease: false, increase: false, reset: false });
  assert.match(html, /aria-label="Editor font"[\s\S]*?class="trigger-label[^"]*">Fira Code</);
  assert.match(tag(html, 'Session text preview'), /style="--sample-body: 18\.5px;"/);
  const top = await render({ sessionFontSize: 24, docsFontSize: 11 });
  assert.equal(stepper(top, 'Session text size').increase, true);
  assert.equal(stepper(top, 'Project docs text size').decrease, true);
});

test('Appearance offers the editor font for session content, off by default', async () => {
  const off = await render({ editorFont: 'Fira Code' });
  const box = (html) => html.match(/<input[^>]*type="checkbox"[^>]*>/)?.[0] ?? '';
  assert.doesNotMatch(box(off), /checked/);
  assert.match(off, /Use the editor font for monospace text in session content/);
  assert.doesNotMatch(tag(off, 'Session text preview'), /--font-mono/);
  const on = await render({ editorFont: 'Fira Code', sessionUsesEditorFont: true });
  assert.match(box(on), /checked/);
  assert.match(tag(on, 'Session text preview'), /--font-mono: &quot;Fira Code&quot;|--font-mono: "Fira Code"/);
});

test('the type sample sizes every text from its body with the type scale', async () => {
  const { readFileSync } = await import('node:fs');
  const style = readFileSync(new URL('../src/lib/components/common/TypeSample.svelte', import.meta.url), 'utf8').match(/<style>([\s\S]*)<\/style>/)[1];
  const sizes = [...style.matchAll(/font-size:\s*([^;]+);/g)].map((m) => m[1]);
  assert.ok(sizes.length >= 4);
  for (const size of sizes) assert.match(size, /^calc\(var\(--sample-body\) \* var\(--type-[\w-]+\)\)$|^calc\(var\(--type-inline-code\) \* 1em\)$/);
});

test('session content takes every monospace font from --font-mono, so the editor font reaches all of it', async () => {
  const { readFileSync, readdirSync } = await import('node:fs');
  const root = new URL('../src/lib/components/', import.meta.url);
  const files = ['transcript', 'transcript/parts', 'chat'].flatMap((dir) =>
    readdirSync(new URL(`${dir}/`, root)).filter((f) => f.endsWith('.svelte')).map((f) => `${dir}/${f}`));
  files.push('../../style.css');
  const uses = (source) => source.replace(/@font-face\s*\{[^}]*\}/g, '');
  assert.ok(files.length > 20);
  for (const file of files) {
    const source = readFileSync(new URL(file, root), 'utf8');
    for (const [, value] of uses(source).matchAll(/font-family:\s*([^;}]+)/g)) {
      assert.match(value.trim(), /^(var\(--font-(mono|sans)\)|inherit)$/, `${file}: font-family: ${value.trim()}`);
    }
  }
});

test('every code, pre, kbd and samp in session content has a font rule, so none falls back to the browser monospace', async () => {
  // Browsers give these elements `font-family: monospace` of their own, so
  // without an author rule they ignore --font-mono and the editor font.
  const { readFileSync, readdirSync } = await import('node:fs');
  const root = new URL('../src/lib/components/', import.meta.url);
  const files = ['transcript', 'transcript/parts', 'chat'].flatMap((dir) =>
    readdirSync(new URL(`${dir}/`, root)).filter((f) => f.endsWith('.svelte')).map((f) => `${dir}/${f}`));
  const MONO_TAGS = /<(code|pre|kbd|samp)\b([^>]*)>/g;
  let checked = 0;
  for (const file of files) {
    const source = readFileSync(new URL(file, root), 'utf8');
    const style = source.match(/<style[^>]*>([\s\S]*)<\/style>/)?.[1] ?? '';
    const markup = source.replace(/<style[^>]*>[\s\S]*<\/style>/, '').replace(/<script[^>]*>[\s\S]*?<\/script>/g, '');
    // The last compound selector of each rule that sets the font family.
    const fontRules = [...style.replace(/\/\*[\s\S]*?\*\//g, '').matchAll(/([^{}]+)\{([^}]*)\}/g)]
      .filter(([, , body]) => /(^|[;\s])font(-family)?\s*:/.test(body))
      .flatMap(([, selectors]) => selectors.split(','))
      .map((selector) => selector.trim().replace(/:global\(([^)]*)\)/g, '$1').split(/[\s>+~]+/).pop());
    for (const [, tag, attrs] of markup.matchAll(MONO_TAGS)) {
      const classes = attrs.match(/\bclass="([^"]*)"/)?.[1].split(/\s+/).filter(Boolean) ?? [];
      const matched = fontRules.some((compound) => {
        const [, ruleTag, ruleClasses = ''] = compound.match(/^([a-z]*)((?:\.[\w-]+)*)/) ?? [];
        if (!ruleTag && !ruleClasses) return false;
        if (ruleTag && ruleTag !== tag) return false;
        return ruleClasses.split('.').filter(Boolean).every((c) => classes.includes(c));
      });
      assert.ok(matched, `${file}: <${tag}${attrs}> has no font rule`);
      checked++;
    }
  }
  assert.ok(checked >= 8, `checked ${checked} elements`);
});
