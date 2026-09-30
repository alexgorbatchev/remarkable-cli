import { Command } from "commander";
import { mkdir } from "node:fs/promises";
import { dirname, join, resolve } from "node:path";
import { parseMapping } from "./parseMapping";
import { tabletResult } from "./tabletResult";
import { verifyBytes } from "./verifyBytes";
import type { CheckOptions, Mapping, PageEvidence } from "./types";

const projectDir = resolve(import.meta.dir, "../..");

function field(output: string, name: string): string {
  const line = output.split("\n").find((line) => line.startsWith(`${name}:`));
  if (!line) throw new Error(`CLI output is missing ${name}`);
  return line.slice(name.length + 1).trim();
}

function pageID(output: string, page: number): string {
  const id = output.split("\n").find((line) => line.startsWith(`${page}\t`))?.split("\t")[1];
  if (!id) throw new Error(`Destination has no initialized native page at index ${page}`);
  return id;
}

async function command(options: CheckOptions, args: string[], outputPath?: string): Promise<Uint8Array> {
  const config = options.config ? ["--config", resolve(options.config)] : [];
  const child = Bun.spawn([resolve(options.binary), "--no-cache", ...config, ...args], {
    env: { ...Bun.env, AGENT: "1" }, stdin: "ignore", stdout: "pipe", stderr: "pipe", timeout: 360_000,
  });
  const [stdout, stderr, exitCode] = await Promise.all([
    new Response(child.stdout).bytes(), new Response(child.stderr).text(), child.exited,
  ]);
  if (outputPath) await Bun.write(outputPath, stdout);
  if (exitCode !== 0) throw new Error(`${args.slice(0, 2).join(" ")} failed (${exitCode}): ${stderr.trim()}`);
  return stdout;
}

async function textCommand(options: CheckOptions, args: string[], outputPath?: string): Promise<string> {
  return new TextDecoder().decode(await command(options, args, outputPath));
}

export async function runCheck(destination: string, options: CheckOptions): Promise<number> {
  if (Bun.env.CI && Bun.env.CI !== "false" && Bun.env.CI !== "0") {
    throw new Error("This live-cloud check must be run manually outside CI");
  }
  if (!/^[0-9a-f]{8}(-[0-9a-f]{4}){3}-[0-9a-f]{12}$/i.test(destination)) {
    throw new Error("Supply the UUID of a disposable destination document");
  }
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
  let tabletState: ReturnType<typeof tabletResult> = "pending";
  let documentName: string = "";
  const saveReport = async (error?: string) => Bun.write(reportPath, JSON.stringify({
    destination, documentName, mappingPath, cloud: cloudState, tablet: tabletState,
    pages, error, recordedAt: new Date().toISOString(),
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
    for (const page of pages) page.pageID = pageID(before, page.pageIndex);
    const hasPDF = field(before, "Format") === "pdf";
    let background: Uint8Array | undefined;
    if (hasPDF) {
      background = await command(options, ["doc", "cat", destination, "--format", "pdf"],
        join(outputDir, "background-before.pdf"));
    }
    const snapshotPath = join(outputDir, "mapping.json");
    await Bun.write(snapshotPath, JSON.stringify(snapshotMapping, null, 2));
    await saveReport();
    const imported = await textCommand(options, ["doc", "import", destination, "--mapping", snapshotPath],
      join(outputDir, "import.txt"));
    if (field(imported, "state") !== "verified") throw new Error("Import did not report verified state");
    if (background) {
      const after = await command(options, ["doc", "cat", destination, "--format", "pdf"],
        join(outputDir, "background-after.pdf"));
      if (!verifyBytes(background, after)) throw new Error("Destination background PDF changed during import");
    }
    for (const page of pages) {
      const bytes = await command(options, ["doc", "cat", destination,
        "--page", String(page.pageIndex), "--format", "rm"]);
      await Bun.write(page.downloadedNative, bytes);
      const original = await Bun.file(page.originalSource).bytes();
      const snapshot = await Bun.file(page.sourceSnapshot).bytes();
      if (!verifyBytes(snapshot, original)) throw new Error(`Source changed: ${page.originalSource}`);
      if (!verifyBytes(snapshot, bytes)) throw new Error(`Downloaded native bytes differ for page ${page.pageIndex}`);
      await textCommand(options, ["doc", "render", destination,
        "--page", String(page.pageIndex), "--output", page.destinationPreview]);
    }
    const after = await textCommand(options, ["doc", "inspect", destination, "--pages"]);
    await Bun.write(join(outputDir, "document-after.txt"), after);
    for (const page of pages) {
      if (page.pageID !== pageID(after, page.pageIndex)) throw new Error(`Page association changed at ${page.pageIndex}`);
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
    console.log("Sync the tablet. On every mapped page containing pen strokes, select imported handwriting,");
    console.log("move it, and erase it. Confirm its background is intact. Sync, close, and reopen the");
    console.log("document, then confirm your edits persist. Metadata-only pages require a visual check.");
    console.log(`Report: ${reportPath}`);
    let answer: string | null;
    do {
      answer = prompt("Did every tablet check above pass? [y/n]");
      tabletState = tabletResult(answer);
    } while (answer !== null && tabletState === "pending");
    await saveReport();
    console.log(`cloud: ${cloudState}\ntablet: ${tabletState}\nreport: ${reportPath}`);
    return tabletState === "passed" ? 0 : 1;
  } catch (error) {
    const message = error instanceof Error ? error.message : String(error);
    if (cloudState !== "passed") cloudState = "failed";
    await saveReport(message);
    throw error;
  }
}

if (import.meta.main) {
  const cli = new Command().name("native-import-check")
    .description("Import into a disposable cloud document, verify downloads, and record a tablet y/n check")
    .argument("<destination-uuid>", "Disposable destination document UUID")
    .requiredOption("--mapping <path>", "JSON source-to-page mapping")
    .option("--binary <path>", "remarkable executable", join(projectDir, "bin/remarkable"))
    .option("--config <path>", "Credentials file passed to remarkable")
    .option("--output-dir <path>", "Parent for a unique evidence directory", join(projectDir, ".tmp/native-import-check"))
    .action(async (destination: string, options: CheckOptions) => {
      process.exitCode = await runCheck(destination, options);
    });
  try {
    await cli.parseAsync();
  } catch (error) {
    console.error(`ERR: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}
