import test from 'node:test';
import assert from 'node:assert/strict';
import { handleMessageClick } from '../src/lib/messageFileClick.ts';

class FakeElement {
  constructor(anchor = null) { this.anchor = anchor; }
  closest(selector) { return selector === 'a[data-file-link]' ? this.anchor : null; }
  contains(anchor) { return anchor === this.anchor; }
}

test('message file clicks handle nested labels and suppress external navigation', () => {
  const previous = globalThis.Element;
  globalThis.Element = FakeElement;
  try {
    const anchor = { dataset: { fileLink: 'src/main.ts:42' } };
    let prevented = 0, stopped = 0;
    const values = [];
    const event = { target: new FakeElement(anchor), currentTarget: new FakeElement(anchor), preventDefault() { prevented++; }, stopPropagation() { stopped++; } };
    handleMessageClick(event, value => values.push(value));
    assert.deepEqual(values, ['src/main.ts:42']);
    assert.equal(prevented, 1);
    assert.equal(stopped, 1);
    handleMessageClick(event); // No owner must still leave the link inert.
    assert.equal(prevented, 2);
  } finally {
    if (previous === undefined) delete globalThis.Element;
    else globalThis.Element = previous;
  }
});

test('ordinary web links are left to existing navigation', () => {
  const previous = globalThis.Element;
  globalThis.Element = FakeElement;
  try {
    let prevented = false;
    handleMessageClick({ target: new FakeElement(), currentTarget: new FakeElement(), preventDefault() { prevented = true; } });
    assert.equal(prevented, false);
  } finally {
    if (previous === undefined) delete globalThis.Element;
    else globalThis.Element = previous;
  }
});
