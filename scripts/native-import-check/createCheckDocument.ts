import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import { PDFDocument } from "pdf-lib";
import { CloudSession } from "./CloudSession";
import { field, lastField } from "./cliOutput";
import { readCreationEvidence } from "./readCreationEvidence";
import { runCommand } from "./runCommand";
import type { CheckOptions, Mapping, PreparedCheck, SelectedPage, SourceOptions } from "./types";

type JsonObject = Record<string, unknown>;

function object(value: unknown, name: string): JsonObject {
  if (typeof value !== "object" || value === null || Array.isArray(value)) throw new Error(`Invalid ${name}`);
  return Object.fromEntries(Object.entries(value));
}

// pageIndexes requires at least two pages so the check covers handwriting
// imported on several pages of a multi-page destination.
function pageIndexes(values: string[]): number[] {
  if (values.length < 2) throw new Error("--page requires at least two 0-based page indexes");
  const indexes = values.map((value) => {
    if (!/^\d+$/.test(value) || !Number.isSafeInteger(Number(value))) throw new Error("--page requires 0-based page indexes");
    return Number(value);
  });
  if (new Set(indexes).size !== indexes.length) throw new Error("--page indexes must be distinct");
  return indexes;
}

// sourcePageIDs requires the source's native page i to show PDF page i, so the
// uploaded copy of its PDF associates every page index with the same background.
function sourcePageIDs(content: JsonObject, pdfPages: number): string[] {
  if (content.fileType !== "pdf") throw new Error("The check requires a PDF source document");
  const cPages = object(content.cPages, "source cPages");
  if (!Array.isArray(cPages.pages) || cPages.pages.length !== pdfPages) {
    throw new Error("Source native pages must match its PDF pages one to one");
  }
  return cPages.pages.map((value: unknown, index: number) => {
    const page = object(value, `source page ${index}`);
    const redir = page.redir === undefined ? undefined : object(page.redir, `source page ${index} redirection`);
    if (typeof page.id !== "string" || page.deleted !== undefined || redir?.value !== index) {
      throw new Error(`Source page ${index} is deleted, inserted, or moved; choose a source whose pages follow its PDF`);
    }
    return page.id;
  });
}

// createCheckDocument uploads the source's PDF with `doc upload --initialize-pages`
// as a disposable destination and prepares mappings for the source's selected pages.
export async function createCheckDocument(source: string, options: SourceOptions): Promise<PreparedCheck> {
  if (Bun.env.CI && Bun.env.CI !== "false" && Bun.env.CI !== "0") throw new Error("Live check is outside CI only");
  if (!/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(source)) throw new Error("Invalid source UUID");
  const indexes = pageIndexes(options.page);
  const outputDir = join(resolve(options.outputDir), Bun.randomUUIDv7());
  await mkdir(outputDir, { recursive: true });
  console.log(`Setup evidence directory: ${outputDir}`);
  const checkOptions: CheckOptions = { ...options, outputDir, mapping: join(outputDir, "source-mapping.json") };
  const token = new TextDecoder().decode(await runCommand(checkOptions, ["auth", "token"])).trim();
  if (!token || /\s/.test(token)) throw new Error("CLI did not return a bearer token");
  const host = Bun.env.REMARKABLE_HOST ?? "https://internal.cloud.remarkable.com";
  if (new URL(host).protocol !== "https:") throw new Error("Live-cloud setup requires HTTPS");
  const cloud = new CloudSession(token, host.replace(/\/$/, ""));
  const sourceHash = await cloud.documentHash(source);
  const content = object(JSON.parse(new TextDecoder().decode(
    await cloud.documentFile(source, sourceHash, `${source}.content`))), "source content");
  const sourcePDF = await cloud.documentFile(source, sourceHash, `${source}.pdf`);
  const sourcePDFPath = join(outputDir, "source.pdf");
  await Bun.write(sourcePDFPath, sourcePDF);
  const pdfPages = (await PDFDocument.load(sourcePDF)).getPageCount();
  const sourceIDs = sourcePageIDs(content, pdfPages);
  const mapping: Mapping[] = [];
  const selected: SelectedPage[] = [];
  for (const index of indexes) {
    const pageID = sourceIDs[index];
    if (!pageID) throw new Error(`Source has no page index ${index}`);
    const sourcePath = join(outputDir, `source-${String(index).padStart(3, "0")}.rm`);
    await Bun.write(sourcePath, await cloud.documentFile(source, sourceHash, `${source}/${pageID}.rm`));
    const lines = Number(field(new TextDecoder().decode(await runCommand(checkOptions, ["stroke", "inspect", sourcePath])), "Lines"));
    if (!Number.isSafeInteger(lines) || lines < 0) throw new Error("Invalid stroke count from CLI");
    mapping.push({ source: sourcePath, page: index });
    selected.push({ pageIndex: index, tabletPage: index + 1, sourcePageID: pageID, lines });
  }
  if (!selected.some((page) => page.lines > 0)) throw new Error("At least one selected source page must contain pen strokes");
  await Bun.write(checkOptions.mapping, JSON.stringify(mapping, null, 2));
  const settingsMapping = join(outputDir, "settings-map.json");
  await Bun.write(settingsMapping, JSON.stringify(sourceIDs.map((_, index) => (
    { source_page: index, destination_page: index })), null, 2));
  const title = `Native import check - ${pdfPages} pages - ${new Date().toISOString()}`;
  const creationEvidence = join(outputDir, "creation.json");
  await Bun.write(join(outputDir, "setup.json"), JSON.stringify({
    source, sourceHash, pdfPages, selected, title, creationEvidence,
  }, null, 2));
  console.log(`Creating disposable document with initialized pages: ${title}`);
  const upload = new TextDecoder().decode(await runCommand(checkOptions, [
    "doc", "upload", sourcePDFPath, "--title", title, "--evidence", creationEvidence, "--initialize-pages",
  ], join(outputDir, "upload.txt")));
  const creation = await readCreationEvidence(creationEvidence);
  if (lastField(upload, "state") !== "verified" || creation.state !== "verified" || creation.nativePages !== "initialized"
    || field(upload, "document_id") !== creation.destination || creation.pageIDs.length !== pdfPages) {
    throw new Error(`Upload did not report verified initialized pages; see ${creationEvidence}`);
  }
  if (await cloud.documentHash(source) !== sourceHash) throw new Error("Source cloud document changed during setup");
  return {
    source, sourceHash, destination: creation.destination, options: checkOptions, cloud,
    creationEvidence, pageIDs: creation.pageIDs, settingsMapping,
  };
}
