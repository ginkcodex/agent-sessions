import { handleCopyCodeClick } from './copycode';

/** File anchors stay inert unless a transcript owner handles the click. */
export function handleMessageClick(event: MouseEvent, onFileLink?: (value: string) => void): void {
  const target = event.target;
  const anchor = target instanceof Element ? target.closest<HTMLAnchorElement>('a[data-file-link]') : null;
  if (anchor && event.currentTarget instanceof Element && event.currentTarget.contains(anchor)) {
    event.preventDefault();
    event.stopPropagation();
    const value = anchor.dataset.fileLink;
    if (value) onFileLink?.(value);
    return;
  }
  handleCopyCodeClick(event);
}
