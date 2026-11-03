/**
 * Shows a startup failure above the app, for when the desktop bridge is
 * broken and no backend call can succeed. Shown once; later calls replace the
 * message.
 */
export function showStartupError(err: unknown, doc: Document = document): HTMLElement {
  const message = err instanceof Error ? err.message : String(err);
  let box = doc.getElementById('startup-error');
  if (!box) {
    box = doc.createElement('div');
    box.id = 'startup-error';
    box.setAttribute('role', 'alert');
    box.style.cssText = [
      'position:fixed', 'inset:0', 'z-index:2147483647', 'display:flex', 'flex-direction:column',
      'align-items:center', 'justify-content:center', 'gap:12px', 'padding:32px', 'text-align:center',
      'background:var(--bg-primary,#1e1e1e)', 'color:var(--text-primary,#eee)', 'font:14px/1.5 sans-serif',
    ].join(';');
    doc.body.appendChild(box);
  }
  const title = doc.createElement('strong');
  title.textContent = 'Agent Sessions could not start';
  const body = doc.createElement('span');
  body.textContent = message;
  const hint = doc.createElement('span');
  hint.textContent = 'Restart the app. If this keeps happening, reinstall it.';
  box.replaceChildren(title, body, hint);
  return box;
}
