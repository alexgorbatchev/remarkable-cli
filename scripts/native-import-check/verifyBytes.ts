export function verifyBytes(expected: Uint8Array, actual: Uint8Array): boolean {
  return Bun.deepEquals(expected, actual);
}
