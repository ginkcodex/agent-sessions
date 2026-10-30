import test from 'node:test';
import assert from 'node:assert/strict';
import { browserLinkDestination } from '../src/lib/linkPreview.ts';

class FakeElement {
  constructor(href, file = false) { this.href = href; this.file = file; }
  closest() { return this.anchor ?? this; }
  contains(anchor) { return anchor === this.child; }
  hasAttribute() { return this.file; }
  getAttribute() { return this.href; }
}

test('browser link preview shows the actual destination, including nested labels', () => {
  const previous = globalThis.Element;
  globalThis.Element = FakeElement;
  try {
    const container = new FakeElement();
    const anchor = new FakeElement(' https://example.test/path?x=1&y=2 ');
    container.child = anchor;
    const label = new FakeElement(); label.anchor = anchor;
    assert.equal(browserLinkDestination(label, container), 'https://example.test/path?x=1&y=2');
    anchor.href = 'HTTP://example.test';
    assert.equal(browserLinkDestination(anchor, container), 'HTTP://example.test');
    for (const href of ['#', 'src/main.ts', 'mailto:user@example.test', 'javascript:alert(1)', 'file:///tmp/a']) {
      anchor.href = href;
      assert.equal(browserLinkDestination(anchor, container), '');
    }
    anchor.href = 'https://example.test'; anchor.file = true;
    assert.equal(browserLinkDestination(anchor, container), '');
    anchor.file = false; container.child = null;
    assert.equal(browserLinkDestination(anchor, container), '');
    assert.equal(browserLinkDestination(null, container), '');
  } finally {
    if (previous === undefined) delete globalThis.Element;
    else globalThis.Element = previous;
  }
});
