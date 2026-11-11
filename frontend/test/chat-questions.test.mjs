import test from 'node:test';
import assert from 'node:assert/strict';
import {
  answered,
  asksWhileWorking,
  choose,
  chooseOwn,
  clear,
  content,
  lateBlocker,
  lateMessage,
  missing,
  problem,
  questionForm,
  setNumber,
  setOwn,
  setText,
  setYes,
  submitBlocker,
  summary,
  toggle,
} from '../src/lib/chatQuestions.ts';

// Forms as each adapter sends them (docs/plan/chat-questions-adapters.md).
// Made up, not copied from a session.
const opt = (title, description) => ({ const: title, title, ...(description && { description }) });

const CLAUDE = {
  id: 'question-1',
  toolCallId: 'toolu_ask',
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
    { key: 'question_1', type: 'array', title: 'Extras to include', description: 'Which extras?', items: { anyOf: [opt('Unit tests'), opt('Documentation'), opt('Changelog')] } },
    { key: 'question_1_custom', type: 'string', title: 'Other', description: 'Type your own answer…' },
    { key: 'question_2', type: 'string', title: 'Confirm', description: 'This will overwrite src/config.ts. Continue?', oneOf: [opt('Yes, overwrite'), opt('No, keep it')] },
    { key: 'question_2_custom', type: 'string', title: 'Other', description: 'Type your own answer…' },
  ],
};

const codexMeta = (extra) => ({ codex: { isSecret: false, ...extra } });
const OTHER = { const: 'Other', title: 'Other', description: 'Provide a different answer in the note field.' };
const CODEX = {
  id: 'question-2',
  message: 'Codex needs your input to continue.',
  fields: [
    { key: 'approach', type: 'string', title: 'Which approach should I take?', description: 'Approach', oneOf: [opt('Extend'), opt('Rewrite'), OTHER], _meta: codexMeta({ isOther: true }) },
    { key: 'approach_note', type: 'string', title: 'Additional answer or note', _meta: codexMeta({ questionId: 'approach', role: 'user_note' }) },
    { key: 'name', type: 'string', title: 'What should the module be called?', description: 'Name', _meta: codexMeta({ isOther: false }) },
  ],
  required: ['approach', 'name'],
  meta: { codex: { autoResolutionMs: 60000 } },
};

const OPENCODE = {
  id: 'question-3',
  message: 'Answer to continue.',
  fields: [
    { key: 'approach', type: 'string', title: 'Approach', description: 'Which approach?', oneOf: [opt('Extend'), opt('Rewrite')], default: 'Extend' },
    { key: 'extras', type: 'array', title: 'Extras', description: 'Which extras?', items: { anyOf: [opt('Tests'), opt('Docs')] }, maxItems: 2 },
    { key: 'extras_custom', type: 'string', title: 'Extras (other)', description: 'Add your own answer' },
    { key: 'overwrite', type: 'boolean', title: 'Overwrite', description: 'Overwrite src/config.ts?', default: false },
    { key: 'retries', type: 'integer', title: 'Retries', minimum: 0, maximum: 5 },
  ],
  required: ['approach', 'overwrite'],
};

test("Claude Code's form: one question per question_<n>, each with its own answer", () => {
  const form = questionForm(CLAUDE);
  assert.equal(form.message, 'Please answer the following questions.');
  assert.deepEqual(
    form.questions.map((q) => [q.key, q.kind, q.header, q.text, q.ownKey, q.required]),
    [
      ['question_0', 'choice', 'Implementation approach', 'Which approach should I take?', 'question_0_custom', false],
      ['question_1', 'multi', 'Extras to include', 'Which extras?', 'question_1_custom', false],
      ['question_2', 'choice', 'Confirm', 'This will overwrite src/config.ts. Continue?', 'question_2_custom', false],
    ],
  );
  const [approach] = form.questions;
  assert.deepEqual(approach.options, [
    { value: 'Extend existing module (Recommended)', label: 'Extend existing module (Recommended)', description: 'Keep settings in one place.', isDefault: false },
    { value: 'New module', label: 'New module', description: 'Its own file.', isDefault: false },
  ]);
  assert.equal(approach.ownPlaceholder, 'Type your own answer…');
  assert.equal(approach.ownValue, undefined);
  assert.deepEqual(missing(form, {}), [], 'nothing is required');
  assert.equal(submitBlocker(form, {}), undefined);
});

test('answers can change until they are sent, and the content follows the last choice', () => {
  const form = questionForm(CLAUDE);
  const [approach, extras, confirm] = form.questions;
  let d = {};
  d = choose(d, approach.key, 'New module');
  d = choose(d, approach.key, 'Extend existing module (Recommended)');
  d = toggle(d, extras, 'Documentation');
  d = toggle(d, extras, 'Unit tests');
  d = toggle(d, extras, 'Changelog');
  d = toggle(d, extras, 'Changelog');
  d = choose(d, confirm.key, 'Yes, overwrite');
  assert.deepEqual(d[extras.key].picks, ['Unit tests', 'Documentation'], 'picks keep the form order');
  assert.deepEqual(content(form, d), {
    question_0: 'Extend existing module (Recommended)',
    question_1: ['Unit tests', 'Documentation'],
    question_2: 'Yes, overwrite',
  });

  // The own answer replaces a single choice; picking an option again brings it back.
  d = setOwn(d, approach, 'Split the module in two');
  assert.deepEqual(content(form, d).question_0_custom, 'Split the module in two');
  assert.equal(content(form, d).question_0, undefined, 'only the own answer is sent');
  d = choose(d, approach.key, 'New module');
  assert.equal(content(form, d).question_0, 'New module');
  assert.equal(content(form, d).question_0_custom, undefined);
  d = chooseOwn(d, approach.key);
  assert.equal(content(form, d).question_0_custom, 'Split the module in two', 'going back to the own answer keeps its text');
  d = setOwn(d, approach, ' ');
  assert.equal(answered(approach, d[approach.key]), false, 'an own answer with no text is no answer');
  assert.equal(content(form, d).question_0, undefined);

  // A multi-select's own answer goes with its picks; no picks sends no list.
  d = toggle(d, extras, 'Unit tests');
  d = toggle(d, extras, 'Documentation');
  d = setOwn(d, extras, 'Benchmarks');
  assert.deepEqual(content(form, d), { question_1_custom: 'Benchmarks', question_2: 'Yes, overwrite' });
  d = clear(d, confirm.key);
  assert.deepEqual(Object.keys(content(form, d)), ['question_1_custom']);
});

test("Codex's form: the note answers the question in place of the hidden Other", () => {
  const form = questionForm(CODEX);
  const [approach, name] = form.questions;
  assert.equal(form.questions.length, 2, 'the note is part of its question');
  assert.deepEqual([approach.header, approach.text], ['Approach', 'Which approach should I take?'], 'Codex sends the question as the title');
  assert.deepEqual(approach.options.map((o) => o.value), ['Extend', 'Rewrite'], 'Other is not shown');
  assert.equal(approach.ownKey, 'approach_note');
  assert.equal(approach.ownValue, 'Other');
  assert.equal(approach.ownPlaceholder, 'Additional answer or note');
  assert.deepEqual([name.kind, name.header, name.text, name.ownKey], ['text', 'Name', 'What should the module be called?', undefined]);

  let d = {};
  assert.deepEqual(missing(form, d).map((q) => q.key), ['approach', 'name']);
  assert.equal(submitBlocker(form, d), '2 questions need an answer: Approach, Name.');
  d = setOwn(d, approach, 'Both, in two steps');
  d = setText(d, name.key, '   ');
  assert.equal(submitBlocker(form, d), 'Name needs an answer.', 'blank text is no answer');
  d = setText(d, name.key, 'settings');
  assert.equal(submitBlocker(form, d), undefined);
  assert.deepEqual(content(form, d), { approach: 'Other', approach_note: 'Both, in two steps', name: 'settings' });
  assert.equal(summary(form, 'accept', content(form, d)), 'Answered: Both, in two steps · settings');
  d = choose(d, approach.key, 'Rewrite');
  assert.deepEqual(content(form, d), { approach: 'Rewrite', name: 'settings' });
});

test("OpenCode's form: required questions, a default that is not chosen, yes/no and numbers", () => {
  const form = questionForm(OPENCODE);
  const [approach, extras, overwrite, retries] = form.questions;
  assert.equal(form.questions.length, 4);
  assert.deepEqual([approach.required, approach.ownKey], [true, undefined]);
  assert.deepEqual(approach.options.map((o) => o.isDefault), [true, false], 'the default is marked');
  assert.equal(extras.ownKey, 'extras_custom');
  assert.equal(extras.ownPlaceholder, 'Add your own answer');
  assert.deepEqual([overwrite.kind, overwrite.required, overwrite.defaultValue], ['boolean', true, false]);
  assert.deepEqual([retries.kind, retries.integer, retries.min, retries.max], ['number', true, 0, 5]);

  let d = {};
  assert.deepEqual(missing(form, d).map((q) => q.key), ['approach', 'overwrite'], 'nothing is preselected');
  assert.deepEqual(content(form, d), {});
  d = choose(d, approach.key, 'Extend');
  d = setYes(d, overwrite.key, false);
  assert.deepEqual(missing(form, d), [], 'No is an answer');
  d = setNumber(d, retries.key, '3');
  d = setOwn(d, extras, 'Changelog');
  assert.deepEqual(content(form, d), { approach: 'Extend', extras_custom: 'Changelog', overwrite: false, retries: 3 });
  assert.equal(summary(form, 'accept', content(form, d)), 'Answered: Extend · Changelog · No · 3');
});

test('a text field pairs only where the own answer can stand for its question', () => {
  const form = questionForm({
    id: 'q',
    fields: [
      { key: 'color', type: 'string', title: 'Color', oneOf: [opt('Red'), opt('Blue')] },
      { key: 'color_custom', type: 'string', title: 'Color (other)' },
      { key: 'size', type: 'string', title: 'Size', enum: ['S', 'M'] },
      { key: 'size_custom', type: 'string', title: 'Size (other)' },
      { key: 'tags_custom', type: 'string', title: 'Tags (other)' },
    ],
    required: ['color', 'size_custom'],
  });
  assert.deepEqual(
    form.questions.map((q) => [q.key, q.kind, q.ownKey]),
    [
      ['color', 'choice', undefined],
      ['color_custom', 'text', undefined],
      ['size', 'choice', undefined],
      ['size_custom', 'text', undefined],
      ['tags_custom', 'text', undefined],
    ],
    'a required choice, a required own field and a field with no question stay apart',
  );
  assert.deepEqual(form.questions[2].options.map((o) => o.label), ['S', 'M'], 'enum options are titled by their value');
});

test('limits say what is wrong, and only for an answer that was given', () => {
  const form = questionForm({
    id: 'q',
    fields: [
      { key: 'pick', type: 'array', title: 'Pick', items: { enum: ['a', 'b', 'c'] }, minItems: 2, maxItems: 2 },
      { key: 'code', type: 'string', title: 'Code', minLength: 2, maxLength: 4, pattern: '^[a-z]+$' },
      { key: 'odd', type: 'string', title: 'Odd', pattern: '(?<' },
      { key: 'ratio', type: 'number', title: 'Ratio', minimum: 0, maximum: 1 },
      { key: 'count', type: 'integer', title: 'Count' },
    ],
  });
  const [pick, code, odd, ratio, count] = form.questions;
  assert.equal(problem(pick, {}), undefined);
  assert.equal(problem(pick, { picks: ['a'] }), 'Choose at least 2.');
  assert.equal(problem(pick, { picks: ['a', 'b', 'c'] }), 'Choose at most 2.');
  assert.equal(problem(code, { text: 'a' }), 'Needs at least 2 characters.');
  assert.equal(problem(code, { text: 'abcde' }), 'At most 4 characters.');
  assert.equal(problem(code, { text: 'AB' }), 'Does not have the expected form.');
  assert.equal(problem(code, { text: 'ab' }), undefined);
  assert.equal(problem(odd, { text: 'x' }), undefined, 'a pattern JavaScript cannot read is left to the backend');
  assert.equal(problem(ratio, { number: '2' }), 'At most 1.');
  assert.equal(problem(ratio, { number: 'x' }), 'Enter a number.');
  assert.equal(problem(count, { number: '1.5' }), 'Enter a whole number.');
  assert.equal(submitBlocker(form, { pick: { picks: ['a'] } }), 'Pick: Choose at least 2.');
});

test('the summary line for each way a question ends', () => {
  const form = questionForm(CLAUDE);
  assert.equal(
    summary(form, 'accept', { question_0: 'New module', question_1: ['Unit tests'], question_1_custom: 'Benchmarks', question_2_custom: 'Ask me later' }),
    'Answered: New module · Unit tests, Benchmarks · Ask me later',
  );
  assert.equal(summary(form, 'accept', {}), 'Answered without choosing anything');
  assert.equal(summary(form, 'decline'), 'Skipped the question');
  assert.equal(summary(form, 'withdrawn'), 'The agent went on without an answer');
  assert.equal(summary(form, 'cancel'), 'The question was cancelled');
});

test('a late answer: required questions do not block it, one answer does, and a hidden one is left out', () => {
  const form = questionForm(CODEX);
  const [approach, name] = form.questions;
  let d = {};
  assert.equal(lateBlocker(form, d), 'Answer a question to send it.');
  d = choose(d, approach.key, 'Rewrite');
  assert.equal(lateBlocker(form, d), undefined, 'the unanswered required Name does not block a message');
  assert.equal(lateMessage(form, d), 'My answer to your question:\n- Which approach should I take? — Rewrite');
  d = setOwn(d, approach, 'Both, in two steps');
  d = setText(d, name.key, 'settings');
  assert.equal(
    lateMessage(form, d),
    'My answers to your questions:\n- Which approach should I take? — Both, in two steps\n- What should the module be called? — settings',
  );

  const oc = questionForm(OPENCODE);
  const retries = oc.questions.find((q) => q.key === 'retries');
  assert.equal(lateBlocker(oc, setNumber({}, retries.key, '9')), 'Retries: At most 5.', 'a problem still blocks');

  const token = { key: 'token', type: 'string', title: 'Which token?', description: 'Token', _meta: codexMeta({ isSecret: true }) };
  const typed = setText({}, 'token', 'abc');
  assert.equal(lateBlocker(questionForm({ id: 'q', message: '', fields: [token] }), typed), 'Answer a question to send it.', 'a hidden answer alone cannot go as a message');
  const withToken = questionForm({ ...CODEX, fields: [...CODEX.fields, token] });
  assert.equal(lateMessage(withToken, { ...d, ...typed }), lateMessage(form, d), 'the hidden answer is left out');
});

test("Codex's async question keeps the agent working", () => {
  assert.equal(asksWhileWorking({ ...CODEX, meta: { codex: { delivery: 'async' } } }), true);
  assert.equal(asksWhileWorking(CODEX), false);
  assert.equal(asksWhileWorking(CLAUDE), false);
});
