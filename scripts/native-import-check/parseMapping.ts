import type { Mapping } from "./types";

export function parseMapping(value: unknown): Mapping[] {
  if (!Array.isArray(value) || value.length === 0) throw new Error("Mapping must be a nonempty array");
  const rows: unknown[] = value;
  const pages = new Set<number>();
  return rows.map((row) => {
    if (typeof row !== "object" || row === null || !("source" in row) || !("page" in row)
      || typeof row.source !== "string" || row.source.trim() === ""
      || typeof row.page !== "number" || !Number.isSafeInteger(row.page) || row.page < 0
      || Object.keys(row).length !== 2) {
      throw new Error("Each mapping requires only source and a nonnegative integer page");
    }
    if (pages.has(row.page)) throw new Error(`Destination page ${row.page} is mapped more than once`);
    pages.add(row.page);
    return { source: row.source, page: row.page };
  });
}
