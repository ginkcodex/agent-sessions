// A chat's question card, driven in a DOM: one question or tabs with a
// Review, every kind of answer, answers that change until Submit, required
// questions, Skip, and nothing chosen for the user. Forms are made up, shaped
// like each adapter's (docs/plan/chat-questions-adapters.md).
import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { JSDOM } from 'jsdom';
import { compile } from 'svelte/compiler';

const dom = new JSDOM('<!doctype html><body></body>', { pretendToBeVisual: true });
globalThis.window = dom.window;
for (const key of Object.getOwnPropertyNames(dom.window)) {
  if (key in globalThis || !/^[A-Z]|^(document|getComputedStyle|requestAnimationFrame|cancelAnimationFrame)$/.test(key)) continue;
  globalThis[key] = dom.window[key];
}
Object.defineProperty(globalThis, 'navigator', { value: dom.window.navigator, configurable: true });

const svelte = (path) => JSON.stringify(new URL(`../node_modules/svelte/src/${path}`, import.meta.url).href);
const { mount, unmount, flushSync } = await import(new URL('../node_modules/svelte/src/index-client.js', import.meta.url).href);
const { proxy } = await import(new URL('../node_modules/svelte/src/internal/client/index.js', import.meta.url).href);

async function loadCard() {
  const url = new URL('../src/lib/components/chat/QuestionCard.svelte', import.meta.url);
  const filename = fileURLToPath(url);
  const lib = (rel) => JSON.stringify(new URL(rel, url).href);
  let code = compile(readFileSync(filename, 'utf8'), { generate: 'client', filename, css: 'external' }).js.code;
  code = code
    .replace("'svelte/internal/disclose-version'", svelte('internal/disclose-version.js'))
    .replace("'svelte/internal/client'", svelte('internal/client/index.js'))
    .replace("from 'svelte';", `from ${svelte('index-client.js')};`)
    .replace("import { chats } from '../../stores/chats.svelte';", 'const chats = globalThis.__card.chats;')
    .replace("'../../chatQuestions'", lib('../../chatQuestions.ts'))
    .replace("'../../link'", lib('../../link.ts'))
    .replace("'../../manage'", lib('../../manage.ts'))
    .replace("import Icon from '../common/Icon.svelte';", 'const Icon = () => {};');
  return (await import(`data:text/javascript,${encodeURIComponent(code)}`)).default;
}

const answers = [];
let answer = async (...args) => answers.push(args);
const late = [];
globalThis.__card = {
  chats: {
    answer: (...args) => answer(...args),
    sendLate: async (...args) => late.push(['send', ...args]),
    dismissLate: (...args) => late.push(['dismiss', ...args]),
  },
};
const Card = await loadCard();

const opt = (title, description) => ({ const: title, title, ...(description && { description }) });

const CLAUDE = {
  id: 'q-claude',
  message: 'Please answer the following questions.',
  fields: [
    {
      key: 'question_0',
      type: 'string',
      title: 'Implementation approach',
      description: 'Which approach should I take?',
      oneOf: [opt('Extend existing module (Recommended)', 'Keep settings in one place.'), opt('New module', 'Its own file.')],
    },
    { key: 'question_0_custom', type: 'string', title: 'Other', description: 'Type your own answer…' },
    { key: 'question_1', type: 'array', title: 'Extras to include', description: 'Which extras?', items: { anyOf: [opt('Unit tests'), opt('Documentation'), opt('Changelog entry')] } },
    { key: 'question_1_custom', type: 'string', title: 'Other', description: 'Type your own answer…' },
    { key: 'question_2', type: 'string', title: 'Confirm destructive action', description: 'This will overwrite src/config.ts. Continue?', oneOf: [opt('Yes, overwrite'), opt('No, keep it')] },
    { key: 'question_2_custom', type: 'string', title: 'Other', description: 'Type your own answer…' },
  ],
};

const ONE = {
  id: 'q-one',
  message: 'Please answer the following question.',
  fields: [CLAUDE.fields[4], CLAUDE.fields[5]],
};

const CODEX = {
  id: 'q-codex',
  message: 'Codex needs your input to continue.',
  fields: [
    { key: 'approach', type: 'string', title: 'Which approach should I take?', description: 'Approach', oneOf: [opt('Extend'), opt('Rewrite'), opt('Other')], _meta: { codex: { isOther: true } } },
    { key: 'approach_note', type: 'string', title: 'Additional answer or note', _meta: { codex: { questionId: 'approach', role: 'user_note' } } },
    { key: 'token', type: 'string', title: 'Which token should I use?', description: 'Token', _meta: { codex: { isSecret: true } } },
  ],
  required: ['approach', 'token'],
  meta: { codex: { autoResolutionMs: 60000 } },
};

const OPENCODE = {
  id: 'q-opencode',
  message: 'Answer to continue.',
  fields: [
    { key: 'approach', type: 'string', title: 'Approach', description: 'Which approach?', oneOf: [opt('Extend'), opt('Rewrite')], default: 'Extend' },
    { key: 'overwrite', type: 'boolean', title: 'Overwrite', description: 'Overwrite src/config.ts?', default: false },
    { key: 'retries', type: 'integer', title: 'Retries', description: 'How many retries?', minimum: 0, maximum: 5 },
  ],
  required: ['approach', 'overwrite'],
};

function open(question, agentName = 'Claude Code', phase = 'working') {
  const target = document.createElement('div');
  document.body.appendChild(target);
  // Reactive props, so a test can change them under the mounted card.
  const props = proxy({ chat: { chatId: 'c1', key: 'k1', phase }, question, agentName });
  const component = mount(Card, { target, props });
  flushSync();
  const card = () => target.querySelector('[data-question-card]');
  const buttons = () => [...target.querySelectorAll('button')];
  const text = (el) => el.textContent.trim().replace(/\s+/g, ' ');
  const button = (name) => buttons().find((b) => text(b).startsWith(name));
  const click = (name) => {
    const b = button(name);
    assert.ok(b, `a button ${name}`);
    b.click();
    flushSync();
  };
  const tab = (key) => target.querySelector(`[data-tab="${key}"]`);
  const shown = () => target.querySelector('[data-question]')?.dataset.question ?? (target.querySelector('[data-review]') ? 'review' : undefined);
  const checked = () => [...target.querySelectorAll('[aria-checked="true"]')].map(text);
  const key = (k, el = card()) => {
    el.dispatchEvent(new KeyboardEvent('keydown', { key: k, bubbles: true, cancelable: true }));
    flushSync();
  };
  const type = (input, value) => {
    input.value = value;
    input.dispatchEvent(new window.Event('input', { bubbles: true }));
    flushSync();
  };
  const close = () => {
    unmount(component);
    target.remove();
  };
  const set = (fields) => {
    Object.assign(props, fields);
    flushSync();
  };
  return { target, card, button, click, tab, shown, checked, key, type, close, set };
}

const settle = () => new Promise((resolve) => setTimeout(resolve, 0));

test('several questions: a tab each and Review; a choice moves on, and nothing is chosen at first', () => {
  answers.length = 0;
  const c = open(CLAUDE);
  try {
    assert.match(c.target.textContent, /Claude Code asks/);
    assert.match(c.target.textContent, /Please answer the following questions\./);
    const tabs = [...c.target.querySelectorAll('[role="tab"]')].map((t) => t.textContent.trim());
    assert.deepEqual(tabs, ['1 Implementation approach', '2 Extras to include', '3 Confirm destructive action', 'Review']);
    assert.equal(c.shown(), 'question_0');
    assert.deepEqual(c.checked(), [], 'no option is preselected');
    assert.equal(document.activeElement, document.body, 'the card takes no focus');
    assert.match(c.target.textContent, /Keep settings in one place\./, 'options show their descriptions');
    assert.match(c.target.textContent, /Extend existing module \(Recommended\)/, '"(Recommended)" stays as sent');

    c.click('2. New module');
    assert.equal(c.shown(), 'question_1', 'a single choice moves to the next tab');
    assert.ok(c.tab('question_0').classList.contains('done'), 'the answered tab has a check');
    assert.equal(c.tab('question_1').getAttribute('aria-selected'), 'true');

    c.click('1. Unit tests');
    c.click('2. Documentation');
    assert.equal(c.shown(), 'question_1', 'a multi-select stays open for more picks');
    assert.deepEqual(c.checked(), ['1. Unit tests', '2. Documentation']);
    c.click('1. Unit tests');
    assert.deepEqual(c.checked(), ['2. Documentation'], 'a pick can be taken back');

    c.click('Next');
    c.click('1. Yes, overwrite');
    assert.equal(c.shown(), 'review', 'the last choice moves to Review');
    assert.equal(answers.length, 0, 'choosing sends nothing');
  } finally {
    c.close();
  }
});

test('Review shows each answer with Change, and answers change until Submit', async () => {
  answers.length = 0;
  const c = open(CLAUDE);
  try {
    c.click('Review');
    const rows = () => [...c.target.querySelectorAll('[data-review-row]')].map((r) => r.textContent.trim().replace(/\s+/g, ' '));
    assert.deepEqual(rows(), [
      'Implementation approach Not answered Change',
      'Extras to include Not answered Change',
      'Confirm destructive action Not answered Change',
    ]);

    c.key('ArrowLeft');
    c.key('ArrowLeft');
    c.key('ArrowLeft');
    assert.equal(c.shown(), 'question_0', '← goes back a tab');
    c.key('2');
    assert.equal(c.shown(), 'question_1', '2 picks the second option, which moves on');
    c.key('3');
    c.type(c.target.querySelector('input[data-own="question_1"]'), 'Benchmarks');
    c.key('ArrowRight');
    c.key('2');
    assert.equal(c.shown(), 'review');
    assert.deepEqual(rows(), [
      'Implementation approach New module Change',
      'Extras to include Changelog entry, Benchmarks Change',
      'Confirm destructive action No, keep it Change',
    ]);

    // Change the first answer to the user's own.
    [...c.target.querySelectorAll('[data-review-row="question_0"] button')].find((b) => b.textContent === 'Change').click();
    flushSync();
    assert.equal(c.shown(), 'question_0');
    assert.deepEqual(c.checked(), ['2. New module Its own file.'], 'the earlier answer shows');
    c.key('3');
    assert.equal(document.activeElement, c.target.querySelector('input[data-own="question_0"]'), 'the own answer number goes to its box');
    c.key('1', document.activeElement);
    assert.equal(c.shown(), 'question_0', 'digits typed in a box are text, not choices');
    c.type(document.activeElement, 'Split the module in two');
    assert.deepEqual(c.checked(), ['3.'], 'the own answer is the choice');
    c.click('Review');
    assert.match(rows()[0], /Split the module in two/);

    c.click('Submit');
    await settle();
    assert.deepEqual(answers, [
      ['c1', 'q-claude', 'accept', { question_0_custom: 'Split the module in two', question_1: ['Changelog entry'], question_1_custom: 'Benchmarks', question_2: 'No, keep it' }],
    ]);
  } finally {
    c.close();
  }
});

test('one question: no tabs, Submit right there, and Enter does not submit', async () => {
  answers.length = 0;
  const c = open(ONE);
  try {
    assert.equal(c.target.querySelector('[role="tablist"]'), null);
    assert.match(c.target.textContent, /This will overwrite src\/config\.ts\. Continue\?/);
    c.click('1. Yes, overwrite');
    assert.equal(c.shown(), 'question_2', 'one question stays open');
    const submit = c.button('Submit');
    const enter = new KeyboardEvent('keydown', { key: 'Enter', bubbles: true, cancelable: true });
    submit.dispatchEvent(enter);
    assert.equal(enter.defaultPrevented, true, 'Enter on Submit is held back');
    c.click('2. No, keep it');
    c.click('Submit');
    await settle();
    assert.deepEqual(answers, [['c1', 'q-one', 'accept', { question_2: 'No, keep it' }]]);
  } finally {
    c.close();
  }
});

test('required questions keep Submit off and say which; a secret is typed hidden', async () => {
  answers.length = 0;
  const c = open(CODEX, 'Codex');
  try {
    assert.match(c.target.textContent, /Codex goes on without an answer after about 60 seconds\./);
    assert.deepEqual(
      [...c.target.querySelectorAll('[role="radio"]')].map((b) => b.textContent.trim()),
      ['1. Extend', '2. Rewrite', '3.'],
      'Codex’s Other is the own answer, not an option',
    );
    c.click('Review');
    assert.ok(c.button('Submit').disabled);
    assert.match(c.target.textContent, /2 questions need an answer: Approach, Token\./);
    assert.match(c.target.textContent, /Not answered — needs an answer/);

    c.key('ArrowLeft');
    const token = c.target.querySelector('input[data-field="token"]');
    assert.equal(token.type, 'password');
    c.type(token, 'abc');
    c.key('ArrowRight', c.card());
    assert.match(c.target.textContent, /Approach needs an answer\./);
    c.button('Submit').click();
    await settle();
    assert.deepEqual(answers, [], 'a disabled Submit sends nothing');

    c.tab('approach').click();
    flushSync();
    c.type(c.target.querySelector('input[data-own="approach"]'), 'Both, in two steps');
    c.click('Review');
    assert.match(c.target.textContent, /\(hidden\)/, 'the secret is not read back');
    assert.ok(!c.button('Submit').disabled);
    c.click('Submit');
    await settle();
    assert.deepEqual(answers, [['c1', 'q-codex', 'accept', { approach: 'Other', approach_note: 'Both, in two steps', token: 'abc' }]]);
  } finally {
    c.close();
  }
});

test('a default is marked, not chosen; Yes/No and numbers; Clear takes an answer back', async () => {
  answers.length = 0;
  const c = open(OPENCODE, 'OpenCode');
  try {
    assert.match(c.button('1. Extend').textContent, /Default/);
    assert.deepEqual(c.checked(), []);
    c.key('1');
    assert.equal(c.shown(), 'overwrite');
    assert.match(c.target.textContent, /Default: No/);
    assert.deepEqual(c.checked(), [], 'the default No is not chosen');
    c.key('2');
    assert.equal(c.shown(), 'retries');
    const retries = c.target.querySelector('input[data-field="retries"]');
    assert.match(c.target.textContent, /A whole number from 0 to 5\./);
    c.type(retries, '9');
    assert.match(c.target.querySelector('[role="alert"]').textContent, /At most 5\./);
    assert.ok(!c.tab('retries').classList.contains('done'), 'an answer with a problem gets no check');
    c.click('Clear answer');
    assert.equal(c.target.querySelector('input[data-field="retries"]').value, '');

    c.tab('overwrite').click();
    flushSync();
    assert.deepEqual(c.checked(), ['2. No']);
    c.click('1. Yes');
    c.click('Submit');
    await settle();
    assert.deepEqual(answers, [['c1', 'q-opencode', 'accept', { approach: 'Extend', overwrite: true }]]);
  } finally {
    c.close();
  }
});

test('Skip declines, and a refused answer keeps the card with why', async () => {
  answers.length = 0;
  const c = open(CODEX, 'Codex');
  try {
    c.click('Skip');
    await settle();
    assert.deepEqual(answers, [['c1', 'q-codex', 'decline', undefined]]);

    answer = async () => {
      throw new Error('the question is no longer pending: q-codex');
    };
    c.click('Skip');
    await settle();
    flushSync();
    assert.match(c.target.querySelector('[role="alert"]').textContent, /no longer pending/);
    assert.ok(!c.button('Skip').disabled, 'the card can be answered again');
  } finally {
    c.close();
    answer = async (...args) => answers.push(args);
  }
});

test('a question the agent went on without stays, late, with its draft; its answers go as a message once the turn ends', async () => {
  answers.length = 0;
  late.length = 0;
  const c = open(CODEX, 'Codex');
  try {
    c.click('1. Extend');
    assert.equal(c.shown(), 'token');
    c.type(c.target.querySelector('input[data-field="token"]'), 'abc');

    // The agent withdraws it; the turn is still streaming.
    c.set({ question: { ...CODEX, late: true } });
    assert.ok(c.card().dataset.late !== undefined);
    assert.match(c.target.textContent, /Codex asked/);
    assert.match(c.target.textContent, /Codex went on without your answer\. You can still send it as a message\. Hidden answers are not put in a message\./);
    assert.doesNotMatch(c.target.textContent, /goes on without an answer after/, 'no countdown once it went on');
    assert.equal(c.target.querySelector('input[data-field="token"]').value, 'abc', 'the draft is kept');
    c.click('Review');
    assert.match(c.target.textContent, /Extend/);
    assert.doesNotMatch(c.target.textContent, /needs an answer/, 'required no longer applies');
    const send = c.button('Send as message');
    assert.ok(send.disabled, 'nothing is sent while the agent works');
    assert.match(c.target.textContent, /Codex is still working\. Send your answers once it finishes\./);

    c.set({ chat: { chatId: 'c1', key: 'k1', phase: 'idle' } });
    assert.ok(!c.button('Send as message').disabled);
    c.click('Send as message');
    await settle();
    assert.deepEqual(late, [['send', 'c1', 'q-codex', 'My answer to your question:\n- Which approach should I take? — Extend']]);
    assert.deepEqual(answers, [], 'the agent is not answered through the withdrawn request');
  } finally {
    c.close();
  }
});

test('a late card is dismissed, and a reply refused as it went late gives way to the note', async () => {
  late.length = 0;
  answer = async () => {
    throw new Error('the question is no longer pending: q-one');
  };
  const c = open(ONE, 'Claude Code', 'idle');
  try {
    c.click('2. No, keep it');
    c.click('Submit');
    await settle();
    flushSync();
    assert.match(c.target.querySelector('[role="alert"]').textContent, /no longer pending/);
    c.set({ question: { ...ONE, late: true } });
    assert.equal(c.target.querySelector('[role="alert"]'), null, 'the late note replaces the error');
    c.click('Dismiss');
    assert.deepEqual(late, [['dismiss', 'c1', 'q-one']]);
  } finally {
    c.close();
    answer = async (...args) => answers.push(args);
  }
});
