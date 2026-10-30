import test from 'node:test';
import assert from 'node:assert/strict';
import { isBlameableLine, selectFileLine, includesFileLine } from '../src/lib/fileSelection.ts';

test('blame excludes only the synthetic empty EOF row, not saved blank lines', () => {
  for (const text of ['', 'a', 'a\n', 'a\r\n', 'a\n\n', 'a\r\n\r\n', '\n', '\n\n']) {
    const lines = text.split('\n');
    const count = text ? lines.length - (text.endsWith('\n') ? 1 : 0) : 0;
    for (let line = 1; line <= lines.length + 1; line++) {
      assert.equal(isBlameableLine(lines, line), line <= count, `${JSON.stringify(text)} line ${line}`);
    }
    for (const line of [-1, 0, 1.5, NaN]) assert.equal(isBlameableLine(lines, line), false);
  }
});

test('line selection extends from the anchor in either direction and resets on a plain click', () => {
  let selection = selectFileLine(undefined, 5, false);
  assert.deepEqual(selection, { anchor: 5, head: 5 });
  selection = selectFileLine(selection, 2, true);
  assert.deepEqual(selection, { anchor: 5, head: 2 });
  assert.equal(includesFileLine(selection, 2), true);
  assert.equal(includesFileLine(selection, 4), true);
  assert.equal(includesFileLine(selection, 5), true);
  assert.equal(includesFileLine(selection, 1), false);
  assert.equal(includesFileLine(selection, 6), false);
  assert.equal(includesFileLine(undefined, 2), false);
  assert.deepEqual(selectFileLine(selection, 9, false), { anchor: 9, head: 9 });
});
