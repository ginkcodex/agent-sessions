import test from 'node:test';
import assert from 'node:assert/strict';
import { createMeasure, fitToWidth, transcriptNeed } from '../src/lib/layoutSizes.ts';

// A stand-in for canvas measurement: every character is 10px wide.
const measure = (text) => text.length * 10;

test('the need is the widest row plus gaps and padding', () => {
  const rows = [
    [
      { text: 'aaaaaa', font: '12px sans' }, // 60
      { text: 'bb', font: '12px sans' }, // 20
    ],
    [{ text: 'cccccccccc', font: '12px sans' }], // 100
  ];
  // 100 (widest row) + 24 padding; the 12px gap only adds to the first row.
  assert.equal(transcriptNeed(rows, measure, { gapPx: 12, paddingPx: 24, min: 0 }), 124);
});

test('shrinkable items count only their minimum width', () => {
  const rows = [
    [
      { text: 'a-very-long-path-that-would-take-over', font: '12px mono', flex: true, minWidth: 140 },
      { text: 'msgs', font: '12px sans' }, // 40
    ],
  ];
  // 140 + 12 gap + 40 + 24 padding = 216, not the path's 370.
  assert.equal(transcriptNeed(rows, measure, { gapPx: 12, paddingPx: 24, min: 0 }), 216);
});

test('the need never falls below the floor or above the cap', () => {
  const tiny = [[{ text: 'x', font: '12px sans' }]];
  assert.equal(transcriptNeed(tiny, measure, { min: 380, max: 800 }), 380);
  const huge = [[{ text: 'x'.repeat(200), font: '12px sans' }]];
  assert.equal(transcriptNeed(huge, measure, { min: 380, max: 800 }), 800);
});

test('a wide window leaves the panes untouched', () => {
  assert.deepEqual(
    fitToWidth(1500, { sidebar: 260, sessionList: 340 }, { sidebar: 200, sessionList: 260, transcript: 700 }),
    { sidebar: 260, sessionList: 340 },
  );
});

test('a narrow window takes width from the pane further above its floor', () => {
  // 1200 window, 700 for the transcript leaves 500; the panes want 260+380.
  // The session list sits 120 above its floor and gives that up first, and
  // the sidebar covers the remaining 20.
  const got = fitToWidth(1200, { sidebar: 260, sessionList: 380 }, { sidebar: 200, sessionList: 260, transcript: 700 });
  assert.deepEqual(got, { sidebar: 240, sessionList: 260 });
});

test('floors hold when the window cannot fit them all', () => {
  const got = fitToWidth(800, { sidebar: 260, sessionList: 340 }, { sidebar: 200, sessionList: 260, transcript: 700 });
  // Everything gives up its slack and stops at the floors; the transcript
  // then simply gets less than its floor.
  assert.deepEqual(got, { sidebar: 200, sessionList: 260 });
});

test('the exact fit boundary leaves the panes as they are', () => {
  // 1240 window exactly fits 260 + 340 + 640.
  const widths = { sidebar: 260, sessionList: 340 };
  assert.deepEqual(fitToWidth(1240, widths, { sidebar: 200, sessionList: 260, transcript: 640 }), widths);
});

test('without a DOM every text measures nothing and the minimum rules', () => {
  const rows = [[{ text: 'anything', font: '12px sans' }]];
  assert.equal(transcriptNeed(rows, createMeasure(), { min: 380, max: 800 }), 380);
});
