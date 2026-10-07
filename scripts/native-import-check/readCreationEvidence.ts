export type CreationEvidence = { destination: string; state: string; nativePages: string; pageIDs: string[] };

const uuidPattern = /^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i;

// readCreationEvidence reads the `doc upload --evidence` JSON the CLI wrote.
export async function readCreationEvidence(path: string): Promise<CreationEvidence> {
  const value: unknown = await Bun.file(path).json();
  if (typeof value !== "object" || value === null || !("result" in value) || !("native_pages" in value)
    || !("page_ids" in value) || !("pages" in value)) {
    throw new Error(`Upload evidence ${path} lacks result, native_pages, pages, or page_ids`);
  }
  const { result, native_pages: nativePages, page_ids: pageIDs, pages } = value;
  if (typeof result !== "object" || result === null || !("id" in result) || !("state" in result)
    || typeof result.id !== "string" || !uuidPattern.test(result.id) || typeof result.state !== "string") {
    throw new Error(`Upload evidence ${path} has no document identity`);
  }
  if (typeof nativePages !== "string" || !Array.isArray(pageIDs) || typeof pages !== "number"
    || pageIDs.length !== pages || !pageIDs.every((id): id is string => typeof id === "string" && uuidPattern.test(id))) {
    throw new Error(`Upload evidence ${path} has no complete native page identity`);
  }
  return { destination: result.id, state: result.state, nativePages, pageIDs };
}
