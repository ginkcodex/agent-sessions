import DOMPurify from 'dompurify';
import { Marked } from 'marked';
import { keepsCellOnOneLine, renderMarkdown } from './markdown';

/** Document HTML is untrusted; app-owned code controls are restored only after sanitizing. */
export function renderDocumentMarkdown(source: string): string {
  const blocks: string[] = [];
  const prefix = `as-code-${crypto.randomUUID()}-`;
  const parser = new Marked({
    gfm: true,
    breaks: true,
    renderer: {
      code({ text, lang }) {
        const token = `${prefix}${blocks.length}-end`;
        blocks.push(renderMarkdown('```' + (lang ?? '') + '\n' + text + '\n```'));
        return token;
      },
    },
  });
  const fragment = DOMPurify.sanitize(parser.parse(source, { async: false }), {
    USE_PROFILES: { html: true },
    FORBID_TAGS: ['style', 'input', 'button', 'textarea', 'select', 'form'],
    FORBID_ATTR: ['style', 'class', 'id', 'name'],
    ALLOW_DATA_ATTR: false,
    RETURN_DOM_FRAGMENT: true,
  });
  const slugs = new Map<string, number>();
  for (const heading of fragment.querySelectorAll('h1,h2,h3,h4,h5,h6')) {
    const base = (heading.textContent ?? '').toLowerCase().replace(/[^\p{L}\p{N}\s_-]/gu, '').trim().replace(/\s/g, '-') || 'heading';
    const count = slugs.get(base) ?? 0;
    slugs.set(base, count + 1);
    heading.id = count ? `${base}-${count}` : base;
  }
  for (const anchor of fragment.querySelectorAll('a')) {
    const href = anchor.getAttribute('href') ?? '';
    if (href && !/[\u0000- \u007f]/.test(href) && !/^[a-z][a-z\d+.-]*:|^[\\/]/i.test(href)) {
      anchor.setAttribute('data-file-link', href);
      anchor.setAttribute('href', '#');
    } else {
      anchor.setAttribute('target', '_blank');
      anchor.setAttribute('rel', 'noopener noreferrer');
    }
  }
  // Added after sanitizing, which strips every class from the document.
  for (const cell of fragment.querySelectorAll('th,td')) {
    if (keepsCellOnOneLine(cell.textContent ?? '')) cell.classList.add('nowrap');
  }
  for (const image of fragment.querySelectorAll('img')) {
    const src = image.getAttribute('src') ?? '';
    image.removeAttribute('srcset');
    image.setAttribute('referrerpolicy', 'no-referrer');
    if (/^https?:\/\//i.test(src)) continue;
    image.removeAttribute('src');
    if (src && !/^[a-z][a-z\d+.-]*:|^[\\/]|[\u0000-\u001f\u007f]/i.test(src)) {
      image.setAttribute('data-document-image', src);
    }
  }
  const container = document.createElement('div');
  container.append(fragment);
  return container.innerHTML.replace(new RegExp(`${prefix}(\\d+)-end`, 'g'), (_, index) => blocks[Number(index)]);
}
