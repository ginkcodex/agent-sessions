/** A transcript file destination, without resolving it against a session's workspace. */
export interface MessageFileLink {
  path: string;
  line?: number;
  column?: number;
  endLine?: number;
}

const controls = /[\u0000-\u001f\u007f-\u009f�]/;

/**
 * Parse explicit file destinations, decoding URI escapes exactly once. This is
 * deliberately not a URL parser: schemes (including file:) and network shares
 * must never become workspace file actions. Paths remain platform-native.
 */
export function parseMessageFileLink(value: string): MessageFileLink | null {
  if (controls.test(value)) return null;
  let path: string;
  try {
    path = decodeURIComponent(value.trim()).trim();
  } catch {
    return null;
  }
  if (!path || controls.test(path) || /^[\\/]{2}|^\\/.test(path)) return null;
  // A drive must be absolute. Scheme-like bare names are intentionally
  // ambiguous (foo:10), so require a path separator or extension before :line.
  const drive = /^[a-z]:[\\/]/i.test(path);
  if (!drive && /^[a-z][a-z\d+.-]*:/i.test(path) &&
      !/^[^:]*[.\\/][^:]*:\d+(?::\d+)?$/.test(path)) return null;

  let location: Omit<MessageFileLink, 'path'> = {};
  const fragment = path.match(/#L(\d+)(?:-L?(\d+))?$/);
  const suffix = fragment ?? path.match(/:(\d+)(?::(\d+))?$/);
  if (suffix) {
    const line = Number(suffix[1]);
    const second = suffix[2] === undefined ? undefined : Number(suffix[2]);
    if (!Number.isSafeInteger(line) || line < 1 ||
        (second !== undefined && (!Number.isSafeInteger(second) || second < 1)) ||
        (fragment && second !== undefined && second < line)) return null;
    location = fragment
      ? { line, ...(second === undefined ? {} : { endLine: second }) }
      : { line, ...(second === undefined ? {} : { column: second }) };
    path = path.slice(0, suffix.index);
  }
  // Ambiguous fragments, queries, colons, and shell/markup syntax are not paths
  // we recognize. A drive's colon is the sole exception.
  const tail = drive ? path.slice(2) : path;
  if (!tail || /[?#:<>|*`]/.test(tail) || !path.trim() ||
      path === '.' || path === '..' || /^[\\/]+$/.test(tail)) return null;
  return { path, ...location };
}

/** Whole inline-code spans only; never guess that a command or prose is a path. */
export function isMessageFileLink(value: string): boolean {
  const parsed = parseMessageFileLink(value);
  if (!parsed || /\s/.test(value) || /\s/.test(parsed.path) || /["'=$;&(){}\[\]!]/.test(parsed.path)) return false;
  return /[\\/]/.test(parsed.path) || /^\.[\w.-]+$/.test(parsed.path) || /\.[\p{L}\p{N}][\p{L}\p{N}._-]*$/u.test(parsed.path);
}
