import test from 'node:test';
import assert from 'node:assert/strict';
import { readFileSync } from 'node:fs';
import { JSDOM } from 'jsdom';

const dom = new JSDOM('');
globalThis.window = dom.window;
globalThis.document = dom.window.document;
const { renderDocumentMarkdown } = await import('../src/lib/documentMarkdown.ts');

function render(source) {
  const root = document.createElement('div');
  root.innerHTML = renderDocumentMarkdown(source);
  return root;
}

test('README renders HTML header, badges, images, Markdown and navigation', () => {
  const root = render(readFileSync(new URL('../../README.md', import.meta.url), 'utf8'));
  assert.equal(root.querySelector('h1[align="center"]').textContent, 'Agent Sessions');
  const icon = root.querySelector('img[data-document-image]');
  assert.equal(icon.dataset.documentImage, 'frontend/assets/Icon-universal.png');
  assert.equal(icon.getAttribute('width'), '112');
  assert.equal(icon.getAttribute('src'), null);
  assert.ok(root.querySelector('img[src^="https://img.shields.io/"]'));
  assert.ok(root.querySelector('a[data-file-link="#install"]'));
  assert.ok(root.querySelector('h2#install'));
  assert.ok(root.querySelector('strong').textContent);
  assert.ok(root.querySelector('table'));
});

test('document HTML preserves inline formatting, details and repeated heading anchors', () => {
  const root = render('<p align="center"><strong>Title</strong><br>Body</p>\n\n<details><summary>More</summary>Text</details>\n\n## Setup\n## Setup\n\n[Guide](docs/guide.md)');
  assert.ok(root.querySelector('p[align="center"] strong'));
  assert.ok(root.querySelector('p br'));
  assert.ok(root.querySelector('details summary'));
  assert.ok(root.querySelector('#setup-1'));
  assert.ok(root.querySelector('a[data-file-link="docs/guide.md"]'));
});

test('document HTML cannot execute code or impersonate app controls', () => {
  const root = render('<script>alert(1)</script><style>body{display:none}</style><iframe src="https://example.com"></iframe><svg onload="alert(1)"></svg>\n\n<p style="position:fixed" onclick="alert(1)" class="mermaid-diagram" id="app">Safe</p><button class="copy-code-btn" data-code="bad">Copy</button><a href="javascript&#58;alert(1)" data-file-link="secret">Bad</a><img src="x" onerror="alert(1)" srcset="https://example.com/x 2x">');
  assert.equal(root.querySelector('script,style,iframe,svg,button,[onclick],[onerror],[style],[class],[id],[data-code],[srcset]'), null);
  assert.equal(root.querySelector('a').getAttribute('href'), null);
  assert.equal(root.querySelector('a').dataset.fileLink, undefined);
  assert.equal(root.querySelector('img').getAttribute('src'), null);
});

test('images allow HTTPS or inert relative paths, not local or executable URLs', () => {
  for (const src of ['javascript:alert(1)', 'data:image/svg+xml,bad', 'file:///etc/passwd', '//example.com/x', '/private/x']) {
    const img = render(`<img src="${src}">`).querySelector('img');
    assert.equal(img.getAttribute('src'), null, src);
    assert.equal(img.dataset.documentImage, undefined, src);
  }
  assert.equal(render('![Logo](images/logo.png)').querySelector('img').dataset.documentImage, 'images/logo.png');
});

test('code remains literal and generated code and Mermaid controls survive sanitizing', () => {
  const root = render('`<img src=x onerror=bad>`\n\n```html\n<img src=x onerror=bad>\n```\n\n```mermaid\ngraph TD; A --> B\n```');
  assert.equal(root.querySelector('img'), null);
  assert.equal(root.querySelectorAll('.copy-code-btn').length, 2);
  assert.ok(root.querySelector('.mermaid-diagram'));
  assert.ok(root.querySelector('code').textContent.includes('<img'));
});
