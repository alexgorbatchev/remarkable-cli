// Parsers for the remarkable CLI's AGENT=1 output.

function fieldValue(line: string | undefined, name: string): string {
  if (!line) throw new Error(`CLI output is missing ${name}`);
  return line.slice(name.length + 1).trim();
}

export function field(output: string, name: string): string {
  return fieldValue(output.split("\n").find((line) => line.startsWith(`${name}:`)), name);
}

// lastField reads the final value of a field that progress output repeats, such as `state`.
export function lastField(output: string, name: string): string {
  return fieldValue(output.split("\n").findLast((line) => line.startsWith(`${name}:`)), name);
}

// inspectedPageIDs returns the native page IDs of `doc inspect --pages` rows in page order.
export function inspectedPageIDs(output: string): string[] {
  const ids: string[] = [];
  for (const line of output.split("\n")) {
    const [index, id] = line.split("\t");
    if (index === undefined || !/^\d+$/.test(index)) continue;
    if (Number(index) !== ids.length || !id) throw new Error(`Unexpected doc inspect page row: ${line}`);
    ids.push(id);
  }
  return ids;
}

export function samePageIDs(expected: string[], actual: string[]): boolean {
  return expected.length === actual.length && expected.every((id, index) => id === actual[index]);
}
