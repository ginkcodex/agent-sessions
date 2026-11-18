import test from 'node:test';
import assert from 'node:assert/strict';
import { Text } from '@codemirror/state';
import { IDENTIFIER, LIKELY_TRIGGERS, answerStart, caretAfter, completionChanges, completionDetail, completionType, identifierStart, positionOffset, serverBoosts, snippetText } from '../src/lib/editorCompletion.ts';

// Language-server suggestions in the editor (CC-06): the pure parts.

const doc = (s) => Text.of(s.split('\n'));
const at = (line, character) => ({ line, character });
const range = (l1, c1, l2, c2) => ({ start: at(l1, c1), end: at(l2, c2) });
const apply = (d, changes) => {
  let out = d.toString();
  for (const c of [...changes].sort((a, b) => b.from - a.from)) out = out.slice(0, c.from) + c.insert + out.slice(c.to);
  return out;
};

test('kinds map to CodeMirror icons; the detail line prefers label details and stays one short line', () => {
  assert.equal(completionType(2), 'method');
  assert.equal(completionType(3), 'function');
  assert.equal(completionType(7), 'class');
  assert.equal(completionType(14), 'keyword');
  assert.equal(completionType(undefined), undefined);
  assert.equal(completionType(99), undefined);
  assert.equal(completionDetail({ label: 'f', labelDetail: '(a int)', labelDescription: 'pkg', detail: 'func f(a int)' }), '(a int) pkg');
  assert.equal(completionDetail({ label: 'f', labelDescription: 'pkg', detail: 'x' }), 'pkg');
  assert.equal(completionDetail({ label: 'f', detail: 'func f(\n\ta int,\n)' }), 'func f( a int, )');
  assert.equal(completionDetail({ label: 'f' }), undefined);
  const long = completionDetail({ label: 'f', detail: 'x'.repeat(300) });
  assert.equal(long.length, 120);
  assert.ok(long.endsWith('…'));
});

test('boosts keep the server order by sortText, then its own, all above words', () => {
  const items = [{ label: 'b', sortText: '2' }, { label: 'a', sortText: '1' }, { label: 'c', sortText: '2' }, { label: 'd' }];
  const boosts = serverBoosts(items);
  assert.ok(boosts[1] > boosts[0] && boosts[0] > boosts[2] && boosts[2] > boosts[3], `${boosts}`);
  assert.ok(boosts.every(b => b >= 1 && b <= 99));
  const many = serverBoosts(Array.from({ length: 1000 }, (_, i) => ({ label: `x${i}`, sortText: String(i).padStart(4, '0') })));
  assert.ok(many.every(b => b >= 1 && b <= 99));
  assert.equal(many[0], 99);
  assert.ok(many.every((b, i) => i === 0 || b <= many[i - 1]), 'never out of order');
});

test('positions, identifier starts and where an answer filters from', () => {
  const d = doc('fmt.Pri\n  @na\nαβ x');
  assert.equal(positionOffset(d, at(0, 7)), 7);
  assert.equal(positionOffset(d, at(1, 0)), 8);
  assert.equal(positionOffset(d, at(0, 8)), undefined, 'past the line end');
  assert.equal(positionOffset(d, at(3, 0)), undefined, 'past the last line');
  assert.equal(identifierStart(d, 7), 4);
  assert.equal(identifierStart(d, 4), 4, 'nothing typed after the dot');
  assert.equal(identifierStart(d, 8 + 5), 8 + 3, 'a sigil is not part of the word');
  assert.equal(identifierStart(d, 14 + 2), 14, 'letters of any script');
  assert.ok(IDENTIFIER.test('@name') && IDENTIFIER.test('$x') && IDENTIFIER.test('') && IDENTIFIER.test('αβ'));
  assert.ok(!IDENTIFIER.test('a.b') && !IDENTIFIER.test('a b') && !IDENTIFIER.test('a@'));
  for (const c of '.:>/@<#"\'`$(') assert.ok(LIKELY_TRIGGERS.includes(c), c);

  const edit = (c1, c2) => ({ label: 'x', edit: { range: range(1, c1, 1, c2), newText: 'x' } });
  assert.equal(answerStart(d, 13, [edit(2, 5), edit(2, 5), { label: 'y' }]), 10, "the edits' shared start: ruby-lsp's @ivar");
  assert.equal(answerStart(d, 13, [edit(2, 5), edit(3, 5)]), 11, 'edits disagree: the word');
  assert.equal(answerStart(d, 13, [{ label: 'y' }]), 11, 'no edits: the word');
  assert.equal(answerStart(d, 13, [{ label: 'x', edit: { range: range(0, 0, 0, 1), newText: 'x' } }]), 11, 'a start on another line: the word');
  assert.equal(answerStart(d, 7, []), 4);
});

test('snippets read as plain text with the caret at the first tab stop', () => {
  assert.deepEqual(snippetText('Println(${1:a ...any})$0'), { text: 'Println(a ...any)', caret: 8 });
  assert.deepEqual(snippetText('f($1, $2)'), { text: 'f(, )', caret: 2 });
  assert.deepEqual(snippetText('end$0'), { text: 'end', caret: 3 });
  assert.deepEqual(snippetText('if ${1:cond} {\n\t$0\n}'), { text: 'if cond {\n\t\n}', caret: 3 });
  assert.deepEqual(snippetText('${1|one,two|} x'), { text: 'one x', caret: 0 });
  assert.deepEqual(snippetText('${1:outer ${2:inner}}'), { text: 'outer inner', caret: 0 });
  assert.deepEqual(snippetText('\\$1 costs \\}'), { text: '$1 costs }', caret: 10 });
  assert.deepEqual(snippetText('$TM_FILENAME ${TM_SELECTED_TEXT:none}'), { text: ' none', caret: 5 });
  assert.deepEqual(snippetText('plain'), { text: 'plain', caret: 5 });
  assert.deepEqual(snippetText('${2:b} ${1:a}'), { text: 'b a', caret: 2 }, '$1 first, wherever it is');
  assert.deepEqual(snippetText('$'), { text: '$', caret: 1 });
});

test('accepting applies the server edit and its imports where they were asked', () => {
  const asked = doc('package main\n\nfunc f() {\n\tfmt.Pr\n}');
  const pos = asked.line(4).to, start = pos - 2;
  const item = {
    label: 'Println', snippet: true,
    edit: { range: range(3, 5, 3, 7), newText: 'Println(${1:a ...any})' },
    extraEdits: [{ range: range(1, 0, 1, 0), newText: 'import "fmt"\n' }],
  };
  // Typed one more letter since asking: the filter text grew to "Pri".
  const now = doc('package main\n\nfunc f() {\n\tfmt.Pri\n}');
  const out = completionChanges(asked, pos, start, now, start, pos + 1, item);
  assert.equal(apply(now, out.changes), 'package main\nimport "fmt"\n\nfunc f() {\n\tfmt.Println(a ...any)\n}');
  assert.deepEqual(out.changes.map(c => c.from), [13, start], 'sorted');
  assert.equal(caretAfter(out.changes, out.main, out.caret), start + 'import "fmt"\n'.length + 'Println('.length);

  // Text after the caret the edit replaces moves with the caret.
  const tail = doc('x.Prfoo()');
  const repl = { label: 'Print', edit: { range: range(0, 2, 0, 7), newText: 'Print' } };
  const moved = completionChanges(tail, 4, 2, doc('x.Prifoo()'), 2, 5, repl);
  assert.equal(apply(doc('x.Prifoo()'), moved.changes), 'x.Print()');
});

test('an extra edit whose text changed, or that touches the main edit, is dropped; other changes give the plain label', () => {
  const asked = doc('import a\nx = fo');
  const item = { label: 'foo', extraEdits: [
    { range: range(0, 0, 0, 8), newText: 'import a, foo' },
    { range: range(1, 4, 1, 4), newText: 'bad' },
    { range: range(5, 0, 5, 0), newText: 'outside' },
  ] };
  const out = completionChanges(asked, asked.length, asked.length - 2, asked, asked.length - 2, asked.length, item);
  assert.equal(apply(asked, out.changes), 'import a, foo\nx = foo', 'the import applies; the overlapping and invalid ones do not');

  // Something else changed (another edit above): only the label replaces the typed text.
  const edited = doc('import b\nx = foo');
  const from = edited.length - 3, to = edited.length;
  const plain = completionChanges(asked, asked.length, asked.length - 2, edited, from, to, { ...item, edit: { range: range(1, 2, 1, 6), newText: '= foo' } });
  assert.deepEqual(plain.changes, [{ from, to, insert: 'foo' }]);
  assert.equal(plain.main, 0);
  assert.equal(caretAfter(plain.changes, plain.main, plain.caret), from + 3);

  // An edit starting inside the typed text guesses at text the server never saw.
  const inside = completionChanges(asked, asked.length, asked.length - 2, doc('import a\nx = foo'), asked.length - 2, asked.length + 1,
    { label: 'oo', edit: { range: range(1, 5, 1, 6), newText: 'o()' } });
  assert.deepEqual(inside.changes, [{ from: asked.length - 2, to: asked.length + 1, insert: 'oo' }], 'its label then');
  // insertText, then the label, when there is no edit.
  const insert = completionChanges(asked, asked.length, asked.length - 2, asked, asked.length - 2, asked.length, { label: 'foo', insertText: 'foo()' });
  assert.equal(apply(asked, insert.changes), 'import a\nx = foo()');
});
