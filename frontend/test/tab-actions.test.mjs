import test from 'node:test';
import assert from 'node:assert/strict';
import { dropIndex, gapAt, tabMoveShortcut, tabShortcut, tabsToClose } from '../src/lib/tabActions.ts';

test('tab close actions use the clicked tab and preserve tab order', () => {
  const ids = ['a', 'b', 'c', 'd'];
  assert.deepEqual(tabsToClose(ids, 'b', 'close'), ['b']);
  assert.deepEqual(tabsToClose(ids, 'b', 'others'), ['a', 'c', 'd']);
  assert.deepEqual(tabsToClose(ids, 'b', 'left'), ['a']);
  assert.deepEqual(tabsToClose(ids, 'b', 'right'), ['c', 'd']);
  assert.deepEqual(tabsToClose(ids, 'a', 'left'), []);
  assert.deepEqual(tabsToClose(ids, 'd', 'right'), []);
  assert.deepEqual(tabsToClose(['a'], 'a', 'others'), []);
  assert.deepEqual(tabsToClose(ids, 'missing', 'others'), []);
  assert.deepEqual(ids, ['a', 'b', 'c', 'd']);
});

test('tab shortcut reads Cmd+1–9 on macOS and Ctrl+1–9 elsewhere', () => {
  const key = (code, mods) => ({ code, ...mods });
  for (const mods of [{ altKey: false, ctrlKey: false, metaKey: true, shiftKey: false }, { altKey: false, ctrlKey: true, metaKey: false, shiftKey: false }]) {
    const mac = mods.metaKey;
    assert.equal(tabShortcut(key('Digit3', mods), mac), 3);
    assert.equal(tabShortcut(key('Numpad7', mods), mac), 7);
  }
  // The wrong modifier chord for the platform never matches.
  assert.equal(tabShortcut(key('Digit1', { ctrlKey: true, metaKey: false, altKey: false, shiftKey: false }), true), null);
  assert.equal(tabShortcut(key('Digit1', { ctrlKey: false, metaKey: true, altKey: false, shiftKey: false }), false), null);
  // Modifiers that must not ride along, digits out of range and other keys.
  assert.equal(tabShortcut(key('Digit1', { ctrlKey: true, metaKey: false, altKey: true, shiftKey: false }), false), null);
  assert.equal(tabShortcut(key('Digit1', { ctrlKey: true, metaKey: false, altKey: false, shiftKey: true }), false), null);
  assert.equal(tabShortcut(key('Digit0', { ctrlKey: true, metaKey: false, altKey: false, shiftKey: false }), false), null);
  assert.equal(tabShortcut(key('KeyA', { ctrlKey: true, metaKey: false, altKey: false, shiftKey: false }), false), null);
});

test('tab move shortcut is Ctrl+Shift+PageUp/PageDown on every platform, as in VS Code', () => {
  const key = (key, mods = {}) => ({ key, ctrlKey: true, metaKey: false, altKey: false, shiftKey: true, ...mods });
  assert.equal(tabMoveShortcut(key('PageUp')), -1);
  assert.equal(tabMoveShortcut(key('PageDown')), 1);
  assert.equal(tabMoveShortcut(key('PageUp', { shiftKey: false })), null, 'Ctrl+PageUp is left alone');
  assert.equal(tabMoveShortcut(key('PageUp', { ctrlKey: false })), null, 'Shift+PageUp scrolls terminals');
  assert.equal(tabMoveShortcut(key('PageUp', { metaKey: true })), null);
  assert.equal(tabMoveShortcut(key('PageUp', { altKey: true })), null);
  assert.equal(tabMoveShortcut(key('ArrowLeft')), null);
});

test('a dragged tab lands in the gap under the pointer, and nowhere for its own gaps', () => {
  const tabs = [{ left: 0, width: 100 }, { left: 100, width: 50 }, { left: 150, width: 100 }];
  assert.equal(gapAt(-20, tabs), 0, 'left of the bar');
  assert.equal(gapAt(49, tabs), 0);
  assert.equal(gapAt(50, tabs), 1, 'past the middle of the first tab');
  assert.equal(gapAt(124, tabs), 1);
  assert.equal(gapAt(125, tabs), 2);
  assert.equal(gapAt(260, tabs), 3, 'right of the last tab');
  assert.equal(gapAt(5, []), 0);

  // Tab 1 of 3: the gaps on either side of it leave the order as it is.
  assert.equal(dropIndex(1, 1), null);
  assert.equal(dropIndex(1, 2), null);
  assert.equal(dropIndex(1, 0), 0);
  assert.equal(dropIndex(1, 3), 2);
  assert.equal(dropIndex(0, 3), 2, 'first to last');
  assert.equal(dropIndex(2, 0), 0, 'last to first');
});
