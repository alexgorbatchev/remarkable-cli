import { mkdir } from "node:fs/promises";
import { join, resolve } from "node:path";
import { PDFDocument } from "pdf-lib";
import { CloudSession } from "./CloudSession";
import { runCommand } from "./runCommand";
import type { CheckOptions, SourceOptions } from "./types";

type JsonObject = Record<string, unknown>;
type PreparedCheck = { destination: string; options: CheckOptions; cloud: CloudSession; sourceHash: string };

function object(value: unknown, name: string): JsonObject {
  if (typeof value !== "object" || value === null || Array.isArray(value)) throw new Error(`Invalid ${name}`);
  return Object.fromEntries(Object.entries(value));
}

export async function createCheckDocument(source: string, options: SourceOptions): Promise<PreparedCheck> {
  if (Bun.env.CI && Bun.env.CI !== "false" && Bun.env.CI !== "0") throw new Error("Live check is outside CI only");
  if (!/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(source)) throw new Error("Invalid source UUID");
  if (!/^\d+$/.test(options.page)) throw new Error("--page requires a 0-based page index");
  const pageIndex = Number(options.page);
  if (!Number.isSafeInteger(pageIndex)) throw new Error("Invalid page index");
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
  const contentBytes = await cloud.documentFile(source, sourceHash, `${source}.content`);
  const content = object(JSON.parse(new TextDecoder().decode(contentBytes)), "source content");
  if (content.fileType !== "pdf") throw new Error("Self-contained check requires a PDF source document");
  const cPages = object(content.cPages, "source cPages");
  if (!Array.isArray(cPages.pages)) throw new Error("Source has no initialized native pages");
  const page = object(cPages.pages[pageIndex], "source page");
  if (typeof page.id !== "string") throw new Error("Source page has no native ID");
  const redir = object(page.redir, "source PDF page association");
  if (typeof redir.value !== "number" || !Number.isSafeInteger(redir.value) || redir.value < 0) {
    throw new Error("Source page has no valid PDF background association");
  }
  const sourceNative = await cloud.documentFile(source, sourceHash, `${source}/${page.id}.rm`);
  const sourcePath = join(outputDir, "source.rm");
  await Bun.write(sourcePath, sourceNative);
  const stats = new TextDecoder().decode(await runCommand(checkOptions, ["stroke", "inspect", sourcePath]));
  const lines = Number(stats.split("\n").find((line) => line.startsWith("Lines:"))?.slice(6).trim());
  if (!Number.isSafeInteger(lines) || lines <= 0) throw new Error("Source page must contain pen strokes");
  const sourcePDF = await cloud.documentFile(source, sourceHash, `${source}.pdf`);
  await Bun.write(join(outputDir, "source.pdf"), sourcePDF);
  const pdf = await PDFDocument.load(sourcePDF);
  if (redir.value >= pdf.getPageCount()) throw new Error("Source PDF page association is out of bounds");
  const background = await PDFDocument.create();
  const [copiedPage] = await background.copyPages(pdf, [redir.value]);
  if (!copiedPage) throw new Error("PDF page extraction failed");
  background.addPage(copiedPage);
  const backgroundBytes = await background.save();
  await Bun.write(join(outputDir, "background.pdf"), backgroundBytes);
  const destination = Bun.randomUUIDv7();
  const destinationPage = Bun.randomUUIDv7();
  const title = `Native import check - source page ${pageIndex + 1} - ${new Date().toISOString()}`;
  const metadata = object(JSON.parse(new TextDecoder().decode(
    await cloud.documentFile(source, sourceHash, `${source}.metadata`))), "source metadata");
  if (metadata.type !== "DocumentType") throw new Error("Source metadata is not a document");
  const now = String(Date.now());
  const destinationMetadata = {
    ...metadata, visibleName: title, parent: "", deleted: false, pinned: false,
    createdTime: now, lastModified: now, lastOpened: "0", lastOpenedPage: 0,
  };
  const destinationContent = {
    ...content, pageCount: 1, pageTags: [], tags: [], sizeInBytes: String(backgroundBytes.length),
    cPages: {
      ...cPages, original: { ...object(cPages.original, "original PDF count"), value: 1 },
      lastOpened: { ...object(cPages.lastOpened, "last opened page"), value: destinationPage },
      pages: [{ ...page, id: destinationPage, redir: { ...redir, value: 0 },
        verticalScroll: { ...object(page.verticalScroll, "page scroll"), value: 0 } }],
    },
  };
  const template = object(page.template, "page template");
  if (typeof template.value !== "string") throw new Error("Invalid source page template");
  const encode = (value: unknown) => new TextEncoder().encode(JSON.stringify(value, null, 2));
  await Bun.write(checkOptions.mapping, JSON.stringify([{ source: sourcePath, page: 0 }], null, 2));
  await Bun.write(join(outputDir, "setup.json"), JSON.stringify({
    source, sourceHash, sourcePageIndex: pageIndex, sourcePageID: page.id, sourcePDFPageIndex: redir.value,
    destination, destinationPageID: destinationPage, title, lines,
  }, null, 2));
  console.log(`Creating disposable document: ${title} (${destination})`);
  await cloud.createDocument(destination, [
    { name: `${destination}.metadata`, bytes: encode(destinationMetadata) },
    { name: `${destination}.content`, bytes: encode(destinationContent) },
    { name: `${destination}.pdf`, bytes: backgroundBytes },
    { name: `${destination}.pagedata`, bytes: new TextEncoder().encode(`${template.value}\n`) },
  ], join(outputDir, "creation.json"));
  if (await cloud.documentHash(source) !== sourceHash) throw new Error("Source cloud document changed during setup");
  return { destination, options: checkOptions, cloud, sourceHash };
}
