import test from 'node:test';
import assert from 'node:assert/strict';
import { parseMessageFileLink, isMessageFileLink } from '../src/lib/messageFileLinks.ts';

test('message file destinations preserve paths and parse source locations', () => {
  const cases = [
    ['src/main.ts', { path: 'src/main.ts' }],
    ['../notes/read me.md', { path: '../notes/read me.md' }],
    ['/tmp/source.go:10:2', { path: '/tmp/source.go', line: 10, column: 2 }],
    ['main.ts:10', { path: 'main.ts', line: 10 }],
    ['src/main.ts#L10-L20', { path: 'src/main.ts', line: 10, endLine: 20 }],
    ['src/main.ts#L10', { path: 'src/main.ts', line: 10 }],
    ['notes%20one.md%3A10%3A2', { path: 'notes one.md', line: 10, column: 2 }],
    [String.raw`C:\Users\Gink\main.go:10:2`, { path: String.raw`C:\Users\Gink\main.go`, line: 10, column: 2 }],
    ['D:/project/main.ts#L10-L20', { path: 'D:/project/main.ts', line: 10, endLine: 20 }],
    ['src\\main.ts', { path: 'src\\main.ts' }],
    ['%252Fetc/passwd', { path: '%2Fetc/passwd' }],
  ];
  for (const [value, expected] of cases) assert.deepEqual(parseMessageFileLink(value), expected, value);
});

test('message file parser rejects schemes, authorities, controls, and malformed locations', () => {
  for (const value of [
    '', '.', '..', '/', 'C:/', '#heading', 'notes.md#heading', 'notes.md?raw=1',
    'javascript:alert', 'data:text/html,hi', 'https://example.com/a.ts', 'mailto:a@b.com',
    'file:///tmp/a.ts', 'vscode://file/tmp/a.ts', 'C:relative.ts',
    '//server/share/a.ts', String.raw`\\server\share\a.ts`, String.raw`\rooted\a.ts`, '/\\server/a.ts',
    '%2f%2fserver/a.ts', '%5c%5cserver/a.ts', 'javascript%3Aalert',
    'src/%GG.ts', 'src/%.ts', '%C0%AFetc/passwd', 'src/a\u0000.ts', 'src/a\n.ts',
    'src/%00.ts', 'src/%0a.ts', 'src/%7F.ts', 'src/%C2%85.ts',
    'a.ts:0', 'a.ts:1:0', 'a.ts:9007199254740992', 'a.ts#L20-L10',
    'a.ts#L0', 'a.ts#L1-L0', 'a.ts:abc', 'a.ts:1:2:3',
  ]) assert.equal(parseMessageFileLink(value), null, JSON.stringify(value));
});

test('inline code recognition requires a whole conservative path', () => {
  for (const value of ['src/main.ts:10:2', '/tmp/a.go', String.raw`C:\src\a.ts`, './README', '../src', '.gitignore', 'README.md']) {
    assert.equal(isMessageFileLink(value), true, value);
  }
  for (const value of ['hello', 'npm run build', 'cat src/main.ts', 'src/a.ts && echo x', 'foo(bar).ts', 'src/a*.ts', 'src/read me.md', 'https://example.com/a.ts']) {
    assert.equal(isMessageFileLink(value), false, value);
  }
});
