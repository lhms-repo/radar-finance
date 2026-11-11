const TIME_ZONE = "America/Sao_Paulo";

const dateTimeFormat = new Intl.DateTimeFormat("pt-BR", {
  timeZone: TIME_ZONE,
  day: "2-digit",
  month: "2-digit",
  year: "numeric",
  hour: "2-digit",
  minute: "2-digit",
});

const monthFormat = new Intl.DateTimeFormat("pt-BR", { timeZone: TIME_ZONE, month: "long" });

/** Formats an ISO timestamp as dd/mm/aaaa hh:mm in São Paulo time (RNF-16). */
export function formatDateTime(iso: string) {
  return dateTimeFormat.format(new Date(iso)).replace(",", "");
}

/** Returns the month name in Portuguese with an uppercase first letter. */
export function monthName(year: number, month: number) {
  const name = monthFormat.format(new Date(Date.UTC(year, month - 1, 15)));
  return name.charAt(0).toUpperCase() + name.slice(1);
}

/** Formats a byte count as a short human readable size. */
export function formatBytes(bytes: number) {
  const units = ["B", "KB", "MB", "GB"];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return `${value.toLocaleString("pt-BR", { maximumFractionDigits: unit === 0 ? 0 : 1 })} ${units[unit]}`;
}

/** Formats a duration in seconds as days, hours and minutes. */
export function formatUptime(seconds: number) {
  const days = Math.floor(seconds / 86400);
  const hours = Math.floor((seconds % 86400) / 3600);
  const minutes = Math.floor((seconds % 3600) / 60);
  const parts: string[] = [];
  if (days > 0) parts.push(`${days} ${days === 1 ? "dia" : "dias"}`);
  if (hours > 0) parts.push(`${hours} h`);
  parts.push(`${minutes} min`);
  return parts.join(" ");
}

/** Returns the current year and month in São Paulo time. */
export function currentMonth(): { year: number; month: number } {
  const parts = new Intl.DateTimeFormat("en-US", { timeZone: TIME_ZONE, year: "numeric", month: "numeric" }).formatToParts(
    new Date(),
  );
  const year = Number(parts.find((p) => p.type === "year")?.value);
  const month = Number(parts.find((p) => p.type === "month")?.value);
  return { year, month };
}

const moneyFormat = new Intl.NumberFormat("pt-BR", { style: "currency", currency: "BRL" });
const plainMoneyFormat = new Intl.NumberFormat("pt-BR", { minimumFractionDigits: 2, maximumFractionDigits: 2 });

/** Formats integer cents as reais, like R$ 1.234,56 (RN-21: cents only become reais on screen). */
export function formatMoney(cents: number) {
  return moneyFormat.format(cents / 100);
}

/** Formats integer cents without the currency symbol, like 1.234,56. */
export function formatAmount(cents: number) {
  return plainMoneyFormat.format(Math.abs(cents) / 100);
}

/** Formats an ISO date like 2026-11-11 as 11/11/2026 (RNF-16). */
export function formatDate(iso: string) {
  const [y, m, d] = iso.split("-");
  return `${d}/${m}/${y}`;
}

/** Formats an ISO date as a short day and month, like 11/11. */
export function formatDayMonth(iso: string) {
  const [, m, d] = iso.split("-");
  return `${d}/${m}`;
}

const dayParts = new Intl.DateTimeFormat("en-US", { timeZone: TIME_ZONE, year: "numeric", month: "2-digit", day: "2-digit" });

/** Returns today in São Paulo as an ISO date, built from parts because locale formats differ between engines. */
export function todayISO() {
  const parts = dayParts.formatToParts(new Date());
  /** Returns the value of one date part. */
  const get = (type: string) => parts.find((p) => p.type === type)?.value ?? "";
  return `${get("year")}-${get("month")}-${get("day")}`;
}

/** Returns the YYYY-MM form the API uses for a month. */
export function monthParam(year: number, month: number) {
  return `${year}-${String(month).padStart(2, "0")}`;
}
