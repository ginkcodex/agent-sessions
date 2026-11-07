import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { firstVisibleRow, restoreRow, rowAnchor } from '../src/lib/scrollAnchor.ts';

/** Rows of the given heights, stacked from y = 0 in content coordinates. */
function view(heights, { scrollTop = 0, viewTop = 50 } = {}) {
  const el = { scrollTop, getBoundingClientRect: () => ({ top: viewTop }) };
  el.rows = heights.map((height, i) => {
    const row = {
      height,
      getBoundingClientRect() {
        const top = viewTop + el.rows.slice(0, i).reduce((sum, r) => sum + r.height, 0) - el.scrollTop;
        return { top, bottom: top + row.height, height: row.height };
      },
    };
    return row;
  });
  el.scale = (k) => el.rows.forEach((row) => { row.height *= k; });
  return el;
}

test('the first visible row is the first whose bottom is below the top of the view', () => {
  const el = view([40, 60, 80], { scrollTop: 70 });
  assert.equal(firstVisibleRow(el, el.rows), el.rows[1]);
  assert.deepEqual(rowAnchor(el, el.rows[1]), { offset: -30, height: 60 });
  assert.equal(firstVisibleRow(el, []), null);
  assert.equal(firstVisibleRow(view([40], { scrollTop: 40 }), view([40], { scrollTop: 40 }).rows), null);
});

test('a row keeps its place, or its share above the view, when every row grows or shrinks', () => {
  for (const k of [2, 0.5]) {
    // Row 1 starts 30px into the view.
    const inView = view([40, 60, 80], { scrollTop: 10 });
    const a = rowAnchor(inView, inView.rows[1]);
    inView.scale(k);
    restoreRow(inView, inView.rows[1], a);
    assert.equal(inView.rows[1].getBoundingClientRect().top - 50, 30);
    // Row 1 has half of itself above the view.
    const above = view([40, 60, 80], { scrollTop: 70 });
    const b = rowAnchor(above, above.rows[1]);
    above.scale(k);
    restoreRow(above, above.rows[1], b);
    assert.equal(above.scrollTop, (40 + 30) * k);
  }
});

test('the file viewer keeps the first source line or Markdown block in place on a size change', () => {
  const source = readFileSync(new URL('../src/lib/components/common/go-to-file/FileViewer.svelte', import.meta.url), 'utf8');
  const effect = source.match(/\$effect\.pre\(\(\) => \{\s*void preferences\.editorFont; void preferences\.editorFontSize; void preferences\.docsFontSize;[\s\S]*?\n  \}\);/)?.[0];
  assert.ok(effect, 'an effect taken before the editor and docs sizes lay out');
  assert.match(effect, /if \(!el\?\.isConnected \|\| editing\) return;/, 'the editor keeps its own place');
  assert.match(effect, /querySelector\('\.markdown-doc'\)\?\.children/);
  assert.match(effect, /\[data-source-line\]/);
  // Put back after the update, so after App sets the root properties.
  assert.match(effect, /tick\(\)\.then\(\(\) => \{\s*if \(kept\.isConnected && el\.isConnected\) restoreRow\(el, kept, anchor\);/);
});
