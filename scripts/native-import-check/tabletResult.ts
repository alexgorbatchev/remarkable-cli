export function tabletResult(answer: string | null): "passed" | "failed" | "pending" {
  const normalized = answer?.trim().toLowerCase();
  if (normalized === "y") return "passed";
  if (normalized === "n") return "failed";
  return "pending";
}
