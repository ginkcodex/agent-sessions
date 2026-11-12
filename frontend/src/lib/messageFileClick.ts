import { handleCopyCodeClick } from './copycode';

/**
 * File anchors stay inert unless a transcript owner handles the click; a
 * code block's Run button calls onRunCommand with its command.
 */
export function handleMessageClick(
  event: MouseEvent,
  onFileLink?: (value: string) => void,
  onRunCommand?: (command: string) => void,
): void {
  const target = event.target;
  const anchor = target instanceof Element ? target.closest<HTMLAnchorElement>('a[data-file-link]') : null;
  if (anchor && event.currentTarget instanceof Element && event.currentTarget.contains(anchor)) {
    event.preventDefault();
    event.stopPropagation();
    const value = anchor.dataset.fileLink;
    if (value) onFileLink?.(value);
    return;
  }
  const run = target instanceof Element ? target.closest<HTMLElement>('.run-code-btn') : null;
  if (run && event.currentTarget instanceof Element && event.currentTarget.contains(run)) {
    event.stopPropagation();
    const command = run.dataset.command;
    if (command) onRunCommand?.(command);
    return;
  }
  handleCopyCodeClick(event);
}
