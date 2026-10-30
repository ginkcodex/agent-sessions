/** Only destinations handled by the app's external browser navigation. */
export function browserLinkDestination(target: EventTarget | null, container: EventTarget | null): string {
  if (!(target instanceof Element) || !(container instanceof Element)) return '';
  const anchor = target.closest('a');
  if (!anchor || !container.contains(anchor) || anchor.hasAttribute('data-file-link')) return '';
  const href = anchor.getAttribute('href')?.trim() ?? '';
  return /^https?:\/\//i.test(href) ? href : '';
}
