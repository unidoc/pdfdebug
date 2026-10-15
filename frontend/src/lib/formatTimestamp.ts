const MONTHS = [
  'January',
  'February',
  'March',
  'April',
  'May',
  'June',
  'July',
  'August',
  'September',
  'October',
  'November',
  'December',
];

function pad2(n: number): string {
  return n < 10 ? `0${n}` : String(n);
}

/**
 * Formats a Unix-millisecond timestamp in local time: `HH:MM` (24-hour) when
 * its local date is the same as `now`, otherwise `D Month YYYY HH:MM`.
 */
export function formatTimestamp(ms: number, now: Date = new Date()): string {
  const d = new Date(ms);
  const time = `${pad2(d.getHours())}:${pad2(d.getMinutes())}`;
  const sameDay =
    d.getFullYear() === now.getFullYear() && d.getMonth() === now.getMonth() && d.getDate() === now.getDate();
  if (sameDay) return time;
  return `${d.getDate()} ${MONTHS[d.getMonth()]} ${d.getFullYear()} ${time}`;
}
