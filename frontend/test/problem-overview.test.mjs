// The overview ruler and the viewer's hover: which problems are drawn, where,
// and which ones a word touches. Pure functions only; placement in pixels is
// measured in the browser.
import test from 'node:test';
import assert from 'node:assert/strict';
import { overviewMarks, problemsAt, sourceProblems } from '../src/lib/problemOverview.ts';
import { projectSourceDiagnostics } from '../src/lib/sourceDiagnostics.ts';

const range = (a, b, c = a, d = b) => ({ start: { line: a, character: b }, end: { line: c, character: d } });
const saved = { dirty: false, revision: 'r1' };
function finding(r, changes = {}, providers = ['gopls']) {
  const diagnostic = { path: 'a.go', message: 'undefined: time.Seconds', source: 'compiler', severity: 'error', freshness: 'current', basis: { kind: 'saved', revision: 'r1' }, ...(r ? { range: r } : {}), ...changes };
  return { diagnostic, entries: providers.map(providerId => ({ providerId, diagnostic })) };
}
const problem = (r, severity = 'error', message = severity) => ({ range: r, severity, message, source: '' });

test('only problems painted in the source reach the ruler, with their checker and code', () => {
  const p = projectSourceDiagnostics('package a\nvar x = time.Seconds\n', [
    finding(range(1, 13, 1, 20), { code: 'UndeclaredImportedName' }),
    finding(undefined, { message: 'file-level' }),
    finding(range(1, 0, 1, 3), { freshness: 'stale', message: 'stale' }),
    finding(range(0, 0, 0, 7), { severity: 'warning', message: 'w', source: '' }),
  ], saved);
  assert.deepEqual(sourceProblems(p), [
    { range: range(1, 13, 1, 20), severity: 'error', message: 'undefined: time.Seconds', source: 'compiler · UndeclaredImportedName' },
    { range: range(0, 0, 0, 7), severity: 'warning', message: 'w', source: '' },
  ]);
});

test('one mark per line, in line order, coloured by its worst problem', () => {
  const marks = overviewMarks([
    problem(range(9, 0), 'hint'),
    problem(range(2, 4), 'warning'),
    problem(range(2, 1), 'error'),
    problem(range(2, 0), 'information'),
  ]);
  assert.deepEqual(marks.map(m => [m.line, m.severity, m.problems.map(p => p.severity)]), [
    [2, 'error', ['error', 'warning', 'information']],
    [9, 'hint', ['hint']],
  ]);
  assert.deepEqual(overviewMarks([]), []);
});

test('a word finds the problems over it, worst first; exclusive ends and zero-width points hold', () => {
  const problems = [
    problem(range(0, 4, 0, 8), 'warning'),
    problem(range(0, 6, 0, 10), 'error'),
    problem(range(0, 12), 'hint'),
    problem(range(1, 2, 3, 0), 'information'),
  ];
  assert.deepEqual(problemsAt(problems, 0, 5, 7).map(p => p.severity), ['error', 'warning']);
  assert.deepEqual(problemsAt(problems, 0, 0, 4).map(p => p.severity), [], 'a range starting where the word ends does not touch it');
  assert.deepEqual(problemsAt(problems, 0, 8, 9).map(p => p.severity), ['error'], 'an exclusive end stops before the word');
  assert.deepEqual(problemsAt(problems, 0, 11, 15).map(p => p.severity), ['hint'], 'a zero-width problem inside the word');
  assert.deepEqual(problemsAt(problems, 0, 12, 15).map(p => p.severity), ['hint'], 'a zero-width problem at the word start');
  assert.deepEqual(problemsAt(problems, 2, 0, 3).map(p => p.severity), ['information'], 'a line inside a multi-line range');
  assert.deepEqual(problemsAt(problems, 3, 0, 3).map(p => p.severity), [], 'an end at column 0 belongs to the lines before');
});
