import test from 'node:test';
import assert from 'node:assert/strict';
import { EditorState, Text } from '@codemirror/state';
import { hasNextSnippetField, nextSnippetField, snippet } from '@codemirror/autocomplete';
import { IDENTIFIER, LIKELY_TRIGGERS, answerStart, caretAfter, completionChanges, completionDetail, completionType, identifierStart, positionOffset, serverBoosts, snippetTemplate, snippetText, withExtraEdits } from '../src/lib/editorCompletion.ts';

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

test('extra edits are all or none: one that does not fit, or touches an edit, drops them; other changes give the plain label', () => {
  const asked = doc('import a\nx = fo');
  const imp = { range: range(0, 0, 0, 8), newText: 'import a, foo' };
  const item = { label: 'foo', extraEdits: [imp] };
  const out = completionChanges(asked, asked.length, asked.length - 2, asked, asked.length - 2, asked.length, item);
  assert.equal(apply(asked, out.changes), 'import a, foo\nx = foo', 'the import applies');
  for (const bad of [
    { range: range(1, 4, 1, 4), newText: 'touches the main edit' },
    { range: range(5, 0, 5, 0), newText: 'outside' },
    { range: range(0, 2, 0, 2), newText: 'inside the import' },
  ]) {
    const none = completionChanges(asked, asked.length, asked.length - 2, asked, asked.length - 2, asked.length, { ...item, extraEdits: [imp, bad] });
    assert.equal(apply(asked, none.changes), 'import a\nx = foo', bad.newText);
    assert.equal(none.main, 0);
  }

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

test('resolved imports join the accepted edit, in the current text; one that does not fit or touches an edit drops them all', () => {
  const now = doc('export {};\nhelperVa');
  const from = now.length - 8, to = now.length;
  const accept = completionChanges(now, to, from, now, from, to, { label: 'helperValue' });
  const imp = { range: range(0, 0, 0, 0), newText: 'import { helperValue } from "./helper.js";\n' };
  const out = withExtraEdits(accept, now, [imp]);
  assert.equal(apply(now, out.changes), 'import { helperValue } from "./helper.js";\nexport {};\nhelperValue');
  assert.equal(out.main, 1, 'the main edit, after the import');
  assert.equal(caretAfter(out.changes, out.main, out.caret), apply(now, out.changes).length);

  for (const bad of [
    { range: range(1, 2, 1, 2), newText: 'inside' },
    { range: range(4, 0, 4, 0), newText: 'outside' },
    { range: range(0, 0, 0, 0), newText: 'same place' },
  ]) {
    const dropped = withExtraEdits(accept, now, [imp, bad]);
    assert.equal(apply(now, dropped.changes), 'export {};\nhelperValue', bad.newText);
    assert.equal(dropped.main, 0);
  }
  assert.deepEqual(withExtraEdits(accept, now, []), { ...accept, changes: accept.changes });
});

// What CodeMirror makes of a template: the text, then each field's selections as Tab moves through them.
function fill(template, line = '') {
  let state = EditorState.create({ doc: line, extensions: [EditorState.allowMultipleSelections.of(true)] });
  snippet(template)({ state, dispatch: tr => { state = tr.state; } }, null, line.length, line.length);
  const text = state.doc.toString();
  const stops = [state.selection.ranges.map(r => text.slice(r.from, r.to) || `@${r.from}`)];
  while (hasNextSnippetField(state)) {
    nextSnippetField({ state, dispatch: tr => { state = tr.state; } });
    stops.push(state.selection.ranges.map(r => text.slice(r.from, r.to) || `@${r.from}`));
  }
  return { text, stops };
}

test('snippet templates: tab stops become CodeMirror fields, ending at $0', () => {
  assert.equal(snippetTemplate('Println(${1:a ...any})$0'), 'Println(${1:a ...any})${0}');
  assert.deepEqual(fill(snippetTemplate('Println(${1:a ...any})$0')), { text: 'Println(a ...any)', stops: [['a ...any'], ['@17']] });
  assert.deepEqual(fill(snippetTemplate('f($1, ${2})')), { text: 'f(, )', stops: [['@2'], ['@4'], ['@5']] }, 'no $0: the end');
  assert.deepEqual(fill(snippetTemplate('${2:b} ${1:a}')), { text: 'b a', stops: [['a'], ['b'], ['@3']] }, 'in number order');
  assert.deepEqual(fill(snippetTemplate('x := ${1:v}; use($1, $1)')), { text: 'x := v; use(v, v)', stops: [['v', 'v', 'v'], ['@17']] }, 'mirrors show and select the text');
  assert.deepEqual(fill(snippetTemplate('${1|one,two|} ${2|\\,a,b|}')), { text: 'one ,a', stops: [['one'], [',a'], ['@6']] }, 'the first choice');
  assert.deepEqual(fill(snippetTemplate('${0:done}')), { text: 'done', stops: [['done']] });
});

test('snippet templates keep braces, escapes, variables and other scripts as text', () => {
  // Braces in the text are no fields: Go bodies, Ruby interpolation, JS templates.
  assert.deepEqual(fill(snippetTemplate('func() {\n\t$0\n}')), { text: 'func() {\n  \n}', stops: [['@11']] }, 'tabs after a new line are indent units');
  assert.deepEqual(fill(snippetTemplate('"#{${1:name}}"')), { text: '"#{name}"', stops: [['name'], ['@9']] });
  assert.deepEqual(fill(snippetTemplate('`${x}` ${1}')).text, '`` ', 'an unknown ${x} variable is nothing');
  assert.equal(fill(snippetTemplate('\\${not} \\$1 \\} \\\\')).text, '${not} $1 } \\', 'escapes');
  assert.equal(fill(snippetTemplate('a\\{b} #{c}')).text, 'a\\{b} #{c}', 'text that only looks like syntax');
  assert.equal(snippetText('a\\{b} #{c} ${d').text, 'a\\{b} #{c} ${d');
  assert.equal(snippetTemplate('a\\{b} #{c} ${d'), undefined, 'three escapes before the final stop: plain text');
  assert.deepEqual(fill(snippetTemplate('$TM_FILENAME ${TM_SELECTED_TEXT:${1:sel}} ${CLIPBOARD/(.*)/x/}')), { text: ' sel ', stops: [['sel'], ['@5']] }, 'variables give their default');
  assert.deepEqual(fill(snippetTemplate('${1/(.*)/$1/} x')).text, ' x', 'a transform inserts nothing');
  assert.deepEqual(fill(snippetTemplate('名前(${1:値}) 😀$0')), { text: '名前(値) 😀', stops: [['値'], ['@8']] }, 'UTF-16 offsets');

  assert.equal(snippetTemplate('#{${1:a}} \\\\{x}'), '#\\{${1:a}} \\\\{x}${0}', 'only braces CodeMirror would misread are escaped');
  assert.deepEqual(fill(snippetTemplate('$${1}{x} #{${2:y}}')), { text: '${x} #{y}', stops: [['@1'], ['y'], ['@9']] }, 'a brace after an empty field');
  assert.deepEqual(fill(snippetTemplate('#{a} #{b} ${1:x}')), { text: '#{a} #{b} x', stops: [['x'], ['@11']] }, 'two escapes before a field are fine');
  assert.equal(snippetTemplate('#{a} #{b} #{c} ${1:x}'), undefined, "a third one would put CodeMirror's fields in the wrong place");
  assert.ok(snippetTemplate('#{a} #{b} #{c}\n${1:x}'), 'on another line it does not');

  // Field text CodeMirror cannot hold: the stop goes before it, nested stops keep their own fields.
  assert.deepEqual(fill(snippetTemplate('${1:outer ${2:inner}}')), { text: 'outer inner', stops: [['@0'], ['inner'], ['@11']] });
  assert.deepEqual(fill(snippetTemplate('${1:{}} ${2:a\nb}')), { text: '{} a\nb', stops: [['@0'], ['@3'], ['@6']] });
});

test('malformed snippets read as plain text and never throw', () => {
  for (const bad of ['${', '${1', '${1:abc', '${1|a,b', '$}', '}}{{', '${1:${2:${3', '\\', '${a b}', '$$$$', '${1/x', '#{', '${}']) {
    const { text: plain } = snippetText(bad);
    const template = snippetTemplate(bad);
    assert.equal(template === undefined ? plain : fill(template).text, plain, bad);
  }
  assert.equal(snippetText('${1:abc').text, 'abc');
  assert.equal(snippetText('${}').text, '${}');
});

test('accepting a snippet item gives its template, unless only the label fits', () => {
  const asked = doc('fmt.Pr');
  const item = { label: 'Println', snippet: true, edit: { range: range(0, 4, 0, 6), newText: 'Println(${1:a})$0' } };
  assert.equal(completionChanges(asked, 6, 4, asked, 4, 6, item).template, 'Println(${1:a})${0}');
  assert.equal(completionChanges(asked, 6, 4, doc('xfmt.Pr'), 5, 7, item).template, 'Println(${1:a})${0}', 'the typed word only: still the snippet');
  const other = { ...item, edit: { range: range(0, 0, 0, 6), newText: 'fmt.Println($1)' } };
  assert.equal(completionChanges(asked, 6, 4, doc('xfmt.Pr'), 5, 7, other).template, undefined, 'the label instead');
  assert.equal(completionChanges(asked, 6, 4, asked, 4, 6, { label: 'Println', insertText: 'Println($1)' }).template, undefined, 'not a snippet');
});

test('a snippet CodeMirror would misplace goes in as plain text, the caret at its first stop', () => {
  const asked = doc('x = ');
  const item = { label: 'h', snippet: true, insertText: '"#{a} #{b} #{c} ${1:name}"' };
  const out = completionChanges(asked, 4, 4, asked, 4, 4, item);
  assert.equal(out.template, undefined);
  assert.equal(apply(asked, out.changes), 'x = "#{a} #{b} #{c} name"');
  assert.equal(caretAfter(out.changes, out.main, out.caret), 4 + '"#{a} #{b} #{c} '.length);
});
