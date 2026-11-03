// Date and time formatting helpers for agent sessions.

export function formatRelativeTime(dateStr: string): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '';

  const now = Date.now();
  const diffMs = now - date.getTime();

  if (diffMs < 0) return 'just now';

  const diffSec = Math.floor(diffMs / 1000);
  if (diffSec < 45) return 'just now';

  const diffMin = Math.floor(diffSec / 60);
  if (diffMin < 60) return `${diffMin}m`;

  const diffHour = Math.floor(diffMin / 60);
  if (diffHour < 24) return `${diffHour}h`;

  const diffDay = Math.floor(diffHour / 24);
  if (diffDay < 30) return `${diffDay}d`;

  const diffMonth = Math.floor(diffDay / 30);
  if (diffMonth < 12) return `${diffMonth}mo`;

  const diffYear = Math.floor(diffDay / 365);
  return `${diffYear}y`;
}

export type TimeZoneMode = 'utc' | 'local';

export const TIME_ZONE_OPTIONS: { value: TimeZoneMode; label: string }[] = [
  { value: 'utc', label: 'UTC' },
  { value: 'local', label: 'Local time (this computer)' },
];

export function isTimeZoneMode(value: unknown): value is TimeZoneMode {
  return value === 'utc' || value === 'local';
}

// Fixed English names, like the rest of the fixed numeric format.
const WEEKDAYS = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];

// Local times carry their offset, e.g. "+07:00", so a timestamp always says
// which zone it is in.
export function formatAbsoluteTime(dateStr: string, zone: TimeZoneMode = 'utc'): string {
  if (!dateStr) return '';
  const date = new Date(dateStr);
  if (isNaN(date.getTime())) return '';

  const pad = (n: number) => n.toString().padStart(2, '0');
  const utc = zone === 'utc';
  const year = utc ? date.getUTCFullYear() : date.getFullYear();
  const month = pad((utc ? date.getUTCMonth() : date.getMonth()) + 1);
  const day = pad(utc ? date.getUTCDate() : date.getDate());
  const hours = pad(utc ? date.getUTCHours() : date.getHours());
  const minutes = pad(utc ? date.getUTCMinutes() : date.getMinutes());
  const seconds = pad(utc ? date.getUTCSeconds() : date.getSeconds());
  // The weekday follows the chosen zone, whose date can differ from UTC's.
  const weekday = WEEKDAYS[utc ? date.getUTCDay() : date.getDay()];

  return `${weekday} ${year}-${month}-${day} ${hours}:${minutes}:${seconds} ${utc ? 'UTC' : utcOffset(date)}`;
}

const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];

// A message's time as a chat shows it: "14:32" today, "Nov 3, 14:32" this
// year, "2025-11-03 14:32" before. Same zone as formatAbsoluteTime, which
// the caller shows in full on hover.
export function formatShortTime(dateStr: string, zone: TimeZoneMode = 'utc', now: Date = new Date()): string {
  if (!isKnownTime(dateStr)) return '';
  const date = new Date(dateStr);
  const utc = zone === 'utc';
  const parts = (d: Date) => utc
    ? [d.getUTCFullYear(), d.getUTCMonth(), d.getUTCDate(), d.getUTCHours(), d.getUTCMinutes()]
    : [d.getFullYear(), d.getMonth(), d.getDate(), d.getHours(), d.getMinutes()];
  const [year, month, day, hours, minutes] = parts(date);
  const [nowYear, nowMonth, nowDay] = parts(now);
  const pad = (n: number) => n.toString().padStart(2, '0');
  const clock = `${pad(hours)}:${pad(minutes)}`;
  if (year === nowYear && month === nowMonth && day === nowDay) return clock;
  if (year === nowYear) return `${MONTHS[month]} ${day}, ${clock}`;
  return `${year}-${pad(month + 1)}-${pad(day)} ${clock}`;
}

// getTimezoneOffset is minutes behind UTC, so +07:00 reports -420.
function utcOffset(date: Date): string {
  const offset = -date.getTimezoneOffset();
  const abs = Math.abs(offset);
  const pad = (n: number) => n.toString().padStart(2, '0');
  return `${offset < 0 ? '-' : '+'}${pad(Math.floor(abs / 60))}:${pad(abs % 60)}`;
}

// Go serializes an unknown time.Time as year 1; treat that (and invalid or
// empty strings) as absent rather than rendering "0001-01-01".
export function isKnownTime(dateStr: string): boolean {
  if (!dateStr) return false;
  const date = new Date(dateStr);
  return !isNaN(date.getTime()) && date.getUTCFullYear() > 1;
}

// "5m ago", or "just now" without a dangling "ago".
export function formatAgo(dateStr: string): string {
  const rel = formatRelativeTime(dateStr);
  return !rel || rel === 'just now' ? rel : `${rel} ago`;
}
