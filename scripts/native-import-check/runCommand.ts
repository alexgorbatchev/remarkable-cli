import { resolve } from "node:path";
import type { CheckOptions } from "./types";

export async function runCommand(options: CheckOptions, args: string[], outputPath?: string): Promise<Uint8Array> {
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
