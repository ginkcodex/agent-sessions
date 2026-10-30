import test from 'node:test';
import assert from 'node:assert/strict';
import { firstVisibleSourceLine } from '../src/lib/filePosition.ts';

test('source position finds the first visible wrapped line with logarithmic layout reads', () => {
  let reads = 0;
  const lines = Array.from({ length: 10000 }, (_, i) => ({
    getBoundingClientRect() { reads++; return { bottom: (i + 1) * 40 }; },
  }));
  assert.equal(firstVisibleSourceLine(lines, 399960), 9999);
  assert.ok(reads <= 14, `read ${reads} line rectangles`);
  assert.equal(firstVisibleSourceLine(lines, 0), 0);
  assert.equal(firstVisibleSourceLine(lines, 39), 0);
  assert.equal(firstVisibleSourceLine(lines, 40), 1);
  assert.equal(firstVisibleSourceLine(lines, 400000), lines.length);
  assert.equal(firstVisibleSourceLine([], 0), 0);
});
