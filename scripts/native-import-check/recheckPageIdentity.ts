import { Command } from "commander";
import { join, resolve } from "node:path";
import { inspectedPageIDs, samePageIDs } from "./cliOutput";
import { readCreationEvidence } from "./readCreationEvidence";
import { runCommand } from "./runCommand";
import type { CheckOptions } from "./types";

const projectDir = resolve(import.meta.dir, "../..");

type RecheckOptions = Pick<CheckOptions, "binary" | "config">;

// recheckPageIdentity compares the destination's native page IDs after the
// tablet check with those `doc upload` recorded. It reads the cloud only.
export async function recheckPageIdentity(evidenceDir: string, options: RecheckOptions): Promise<number> {
  const dir = resolve(evidenceDir);
  const report: unknown = await Bun.file(join(dir, "report.json")).json();
  if (typeof report !== "object" || report === null || !("creationEvidence" in report) || !("cloud" in report)
    || typeof report.creationEvidence !== "string" || report.cloud !== "passed") {
    throw new Error(`${dir}/report.json does not record a passed cloud check`);
  }
  const creation = await readCreationEvidence(report.creationEvidence);
  const inspected = new TextDecoder().decode(await runCommand(
    options, ["doc", "inspect", creation.destination, "--pages"],
    join(dir, "document-after-tablet.txt")));
  const pageIDs = inspectedPageIDs(inspected);
  const pageIdentity = samePageIDs(creation.pageIDs, pageIDs) ? "matched" : "changed";
  const resultPath = join(dir, "page-identity-after-tablet.json");
  await Bun.write(resultPath, JSON.stringify({
    destination: creation.destination, pageIdentity, recordedPages: creation.pageIDs.length,
    inspectedPages: pageIDs.length, recordedAt: new Date().toISOString(),
  }, null, 2) + "\n");
  console.log(`page_identity: ${pageIdentity}\nresult: ${resultPath}`);
  return pageIdentity === "matched" ? 0 : 1;
}

if (import.meta.main) {
  const cli = new Command().name("native-import-recheck")
    .description("After the tablet check, compare the destination's native page IDs with the upload evidence")
    .argument("<evidence-dir>", "Run evidence directory printed by native-import-check")
    .option("--binary <path>", "remarkable executable", join(projectDir, "bin/remarkable"))
    .option("--config <path>", "Credentials file passed to remarkable")
    .action(async (evidenceDir: string, options: RecheckOptions) => {
      process.exitCode = await recheckPageIdentity(evidenceDir, options);
    });
  try {
    await cli.parseAsync();
  } catch (error) {
    console.error(`ERR: ${error instanceof Error ? error.message : String(error)}`);
    process.exitCode = 1;
  }
}
