import { Command } from "commander";
import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { field, inspectedPageIDs, lastField, samePageIDs } from "./cliOutput";
import { createCheckDocument } from "./createCheckDocument";
import { runCommand } from "./runCommand";
import { parseMapping } from "./parseMapping";
import { verifyBytes } from "./verifyBytes";
import type { CheckOptions, Mapping, PageEvidence, PreparedCheck, SourceOptions } from "./types";

const projectDir = resolve(import.meta.dir, "../..");

async function textCommand(options: CheckOptions, args: string[], outputPath?: string): Promise<string> {
  return new TextDecoder().decode(await runCommand(options, args, outputPath));
}

// runCheck imports the mapped strokes into the freshly created destination, with
// no tablet step in between, then transfers the source's tags and view settings.
export async function runCheck(prepared: PreparedCheck): Promise<number> {
  if (Bun.env.CI && Bun.env.CI !== "false" && Bun.env.CI !== "0") {
    throw new Error("This live-cloud check must be run manually outside CI");
  }
  const { destination, options } = prepared;
  const mappingPath = resolve(options.mapping);
  const mapping = parseMapping(await Bun.file(mappingPath).json());
  const outputDir = join(resolve(options.outputDir), Bun.randomUUIDv7());
  await mkdir(outputDir, { recursive: true });
  console.log(`Evidence directory: ${outputDir}`);
  console.log(`Import will write mapped pages in disposable destination ${destination}.`);
  const pages: PageEvidence[] = [];
  const snapshotMapping: Mapping[] = [];
  const reportPath = join(outputDir, "report.json");
  let cloudState: string = "pending";
  let documentName: string = "";
  let settingsState: string = "pending";
  const saveReport = async (error?: string) => Bun.write(reportPath, JSON.stringify({
    source: prepared.source, destination, documentName, creationEvidence: prepared.creationEvidence, mappingPath,
    settingsMapping: prepared.settingsMapping, nativePages: prepared.pageIDs.length, settings: settingsState,
    cloud: cloudState, tablet: "pending", pages, error, recordedAt: new Date().toISOString(),
  }, null, 2) + "\n");

  try {
    for (const row of mapping) {
      const originalSource = resolve(dirname(mappingPath), row.source);
      const stem = `page-${String(row.page).padStart(3, "0")}`;
      const sourceSnapshot = join(outputDir, `${stem}.source.rm`);
      const sourceBytes = await Bun.file(originalSource).bytes();
      await Bun.write(sourceSnapshot, sourceBytes);
      const stats = await textCommand(options, ["stroke", "inspect", sourceSnapshot]);
      const lines = Number(field(stats, "Lines"));
      if (!Number.isSafeInteger(lines) || lines < 0) throw new Error("Invalid stroke count from CLI");
      const sourcePreview = join(outputDir, `${stem}.source.svg`);
      await textCommand(options, ["stroke", "export", sourceSnapshot, "--output", sourcePreview]);
      pages.push({ pageIndex: row.page, tabletPage: row.page + 1, originalSource, sourceSnapshot,
        sourcePreview, downloadedNative: join(outputDir, `${stem}.downloaded.rm`),
        destinationPreview: join(outputDir, `${stem}.destination.png`), lines,
        sha256: new Bun.CryptoHasher("sha256").update(sourceBytes).digest("hex"),
      });
      snapshotMapping.push({ source: sourceSnapshot, page: row.page });
    }
    if (!pages.some((page) => page.lines > 0)) {
      throw new Error("At least one mapped source must contain pen strokes for the tablet editability check");
    }
    const before = await textCommand(options, ["doc", "inspect", destination, "--pages"]);
    documentName = field(before, "Name");
    await Bun.write(join(outputDir, "document-before.txt"), before);
    const beforeIDs = inspectedPageIDs(before);
    if (!samePageIDs(prepared.pageIDs, beforeIDs)) {
      throw new Error("Fresh doc inspect --pages differs from the page IDs recorded by doc upload");
    }
    for (const page of pages) page.pageID = beforeIDs[page.pageIndex];
    const background = await runCommand(options, ["doc", "cat", destination, "--format", "pdf"],
      join(outputDir, "background-before.pdf"));
    const snapshotPath = join(outputDir, "mapping.json");
    await Bun.write(snapshotPath, JSON.stringify(snapshotMapping, null, 2));
    await saveReport();
    const imported = await textCommand(options, ["doc", "import", destination, "--mapping", snapshotPath],
      join(outputDir, "import.txt"));
    if (field(imported, "state") !== "verified") throw new Error("Import did not report verified state");
    const settings = await textCommand(options, ["doc", "settings", "transfer", prepared.source, destination,
      "--mapping", prepared.settingsMapping, "--replace-viewport"], join(outputDir, "settings.txt"));
    settingsState = lastField(settings, "state");
    if (settingsState !== "verified") throw new Error("Settings transfer did not report verified state");
    const after = await runCommand(options, ["doc", "cat", destination, "--format", "pdf"],
      join(outputDir, "background-after.pdf"));
    if (!verifyBytes(background, after)) throw new Error("Destination background PDF changed during import or settings transfer");
    for (const page of pages) {
      const bytes = await runCommand(options, ["doc", "cat", destination,
        "--page", String(page.pageIndex), "--format", "rm"]);
      await Bun.write(page.downloadedNative, bytes);
      const original = await Bun.file(page.originalSource).bytes();
      const snapshot = await Bun.file(page.sourceSnapshot).bytes();
      if (!verifyBytes(snapshot, original)) throw new Error(`Source changed: ${page.originalSource}`);
      if (!verifyBytes(snapshot, bytes)) throw new Error(`Downloaded native bytes differ for page ${page.pageIndex}`);
      await textCommand(options, ["doc", "render", destination,
        "--page", String(page.pageIndex), "--output", page.destinationPreview]);
    }
    const afterInspect = await textCommand(options, ["doc", "inspect", destination, "--pages"]);
    await Bun.write(join(outputDir, "document-after.txt"), afterInspect);
    if (!samePageIDs(prepared.pageIDs, inspectedPageIDs(afterInspect))) {
      throw new Error("Destination page IDs changed during import or settings transfer");
    }
    if (await prepared.cloud.documentHash(prepared.source) !== prepared.sourceHash) {
      throw new Error("Source cloud document changed during verification");
    }
    cloudState = "passed";
    await saveReport();
    const isAgent = ["1", "true", "yes"].includes(Bun.env.AGENT ?? "");
    console.log(`Cloud round trip passed. On the tablet, open "${documentName}" (${destination}).`);
    for (const page of pages) {
      if (isAgent) {
        console.log(`page: ${page.pageIndex}\ntablet_page: ${page.tabletPage}\npage_id: ${page.pageID}`);
        console.log(`source: ${page.originalSource}\nsource_preview: ${page.sourcePreview}`);
        console.log(`destination_preview: ${page.destinationPreview}\nnative_download: ${page.downloadedNative}`);
        continue;
      }
      console.log(`Page ${page.tabletPage} (CLI index ${page.pageIndex}, ${page.lines} strokes):`);
      console.log(`  Original source: ${page.originalSource}`);
      console.log(`  Open source preview: ${page.sourcePreview}`);
      console.log(`  Open imported-page preview: ${page.destinationPreview}`);
      console.log(`  Downloaded native file: ${page.downloadedNative}`);
    }
    console.log("Sync the tablet and open the document; confirm it opens without a crash and that PDF");
    console.log("navigation, including links, reaches the expected pages. On every mapped page containing");
    console.log("pen strokes, select imported handwriting, move it, and erase it. Confirm its background");
    console.log("is intact. Sync, close, and reopen the document, then confirm your edits persist.");
    console.log("Metadata-only pages require a visual check. After the final tablet sync, record page");
    console.log(`identity with: just native-import-recheck ${outputDir}`);
    console.log(`Report: ${reportPath}`);
    console.log(`cloud: ${cloudState}\ntablet: pending\nreport: ${reportPath}`);
    console.log("Agent: ask the user to perform the printed tablet checks and report the outcome in chat.");
    return 0;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (cloudState !== "passed") cloudState = "failed";
    await saveReport(message);
    throw error;
  }
}

if (import.meta.main) {
  const cli = new Command().name("native-import-check")
    .description("Verify a live native import into an initialized upload and print the evidence for an agent-led tablet check")
    .argument("<source-uuid>", "Source PDF document UUID; its PDF is uploaded as a new disposable destination")
    .requiredOption("--page <index...>", "0-based source page indexes to import; at least one contains pen strokes")
    .option("--binary <path>", "remarkable executable", join(projectDir, "bin/remarkable"))
    .option("--config <path>", "Credentials file passed to remarkable")
    .option("--output-dir <path>", "Parent for a unique evidence directory", join(projectDir, ".tmp/native-import-check"))
    .action(async (source: string, options: SourceOptions) => {
      process.exitCode = await runCheck(await createCheckDocument(source, options));
    });
  try {
    await cli.parseAsync();
  } catch (error) {
    console.error(`ERR: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}
