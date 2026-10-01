import { verifyBytes } from "./verifyBytes";

type Entry = { hash: string; type: string; id: string; count: number; size: number };
type Root = { hash: string; generation: number };
type EncodedManifest = { bytes: Uint8Array; hash: string; size: number };
type CloudFile = { name: string; bytes: Uint8Array };

function hash(bytes: Uint8Array): string {
  return new Bun.CryptoHasher("sha256").update(bytes).digest("hex");
}

function parseManifest(bytes: Uint8Array): Entry[] {
  const [version, ...lines] = new TextDecoder().decode(bytes).trimEnd().split("\n");
  if (version !== "3" && version !== "4") throw new Error(`Unsupported cloud manifest version ${version}`);
  return lines.filter((line) => !line.startsWith("0:.:")).map((line) => {
    const [entryHash, type, id, count, size] = line.split(":");
    if (!entryHash || !/^[a-f0-9]{64}$/.test(entryHash) || !type || !id || !count || !size
      || !/^\d+$/.test(count) || !/^\d+$/.test(size)) throw new Error("Invalid cloud manifest entry");
    return { hash: entryHash, type, id, count: Number(count), size: Number(size) };
  });
}

function encodeManifest(entries: Entry[], isRoot: boolean): EncodedManifest {
  const ordered = entries.toSorted((a, b) => a.id < b.id ? -1 : a.id > b.id ? 1 : 0);
  const size = ordered.reduce((total, entry) => total + entry.size, 0);
  const header = isRoot ? `4\n0:.:${ordered.length}:${size}\n` : "3\n";
  const bytes = new TextEncoder().encode(header + ordered.map((entry) =>
    `${entry.hash}:${isRoot ? "0" : entry.type}:${entry.id}:${entry.count}:${entry.size}\n`).join(""));
  const hasher = new Bun.CryptoHasher("sha256");
  for (const entry of ordered) hasher.update(Buffer.from(entry.hash, "hex"));
  return { bytes, hash: isRoot ? hash(bytes) : hasher.digest("hex"), size };
}

function checksum(bytes: Uint8Array): string {
  // Reflected Castagnoli polynomial, CRC-32C (RFC 3720 Appendix B.4).
  let crc = 0xffffffff;
  for (const byte of bytes) {
    crc ^= byte;
    for (let bit = 0; bit < 8; bit++) crc = (crc >>> 1) ^ ((crc & 1) ? 0x82f63b78 : 0);
  }
  const result = new Uint8Array(4);
  new DataView(result.buffer).setUint32(0, (crc ^ 0xffffffff) >>> 0, false);
  return Buffer.from(result).toString("base64");
}

export class CloudSession {
  constructor(private readonly token: string, private readonly host: string) {}

  private async request(path: string, name: string, init: RequestInit = {}): Promise<Response> {
    const headers = new Headers(init.headers);
    headers.set("Authorization", `Bearer ${this.token}`);
    headers.set("rm-filename", name);
    const response = await fetch(`${this.host}${path}`, {
      ...init, headers, redirect: "error", signal: AbortSignal.timeout(360_000),
    });
    if (!response.ok) throw new Error(`Cloud ${init.method ?? "GET"} ${path}: HTTP ${response.status}`);
    return response;
  }

  private async root(): Promise<Root> {
    const value: unknown = await (await this.request("/sync/v3/root", "")).json();
    if (typeof value !== "object" || value === null || !("hash" in value) || !("generation" in value)
      || typeof value.hash !== "string" || !/^[a-f0-9]{64}$/.test(value.hash)
      || typeof value.generation !== "number" || !Number.isSafeInteger(value.generation)) {
      throw new Error("Invalid cloud root state");
    }
    return { hash: value.hash, generation: value.generation };
  }

  private async blob(entryHash: string, name: string): Promise<Uint8Array> {
    return (await this.request(`/sync/v3/files/${entryHash}`, name)).bytes();
  }

  public async documentHash(id: string): Promise<string> {
    const root = await this.root();
    const entry = parseManifest(await this.blob(root.hash, "root.docSchema")).find((entry) => entry.id === id);
    if (!entry) throw new Error(`Cloud document ${id} does not exist`);
    return entry.hash;
  }

  public async documentFile(id: string, documentHash: string, name: string): Promise<Uint8Array> {
    const manifest = parseManifest(await this.blob(documentHash, `${id}.docSchema`));
    const entry = manifest.find((entry) => entry.id === name);
    if (!entry) throw new Error(`Cloud document is missing ${name}`);
    const bytes = await this.blob(entry.hash, name);
    if (hash(bytes) !== entry.hash || bytes.length !== entry.size) throw new Error(`Corrupt cloud file ${name}`);
    return bytes;
  }

  private async upload(entryHash: string, file: CloudFile): Promise<void> {
    await this.request(`/sync/v3/files/${entryHash}`, file.name, {
      method: "PUT", body: new Uint8Array(file.bytes),
      headers: { "Content-Type": "application/octet-stream", "x-goog-hash": `crc32c=${checksum(file.bytes)}` },
    });
    if (!verifyBytes(file.bytes, await this.blob(entryHash, file.name))) {
      throw new Error(`Uploaded bytes differ: ${file.name}`);
    }
  }

  public async createDocument(id: string, files: CloudFile[], reportPath: string): Promise<void> {
    const root = await this.root();
    const entries = parseManifest(await this.blob(root.hash, "root.docSchema"));
    if (entries.some((entry) => entry.id === id)) throw new Error(`Destination ${id} already exists`);
    const manifest = encodeManifest(files.map((file) => ({
      hash: hash(file.bytes), type: "0", id: file.name, count: 0, size: file.bytes.length,
    })), false);
    const nextRoot = encodeManifest([...entries, {
      hash: manifest.hash, type: "0", id, count: files.length, size: manifest.size,
    }], true);
    let state = "staging";
    const save = async () => Bun.write(reportPath, JSON.stringify({
      destination: id, state, documentHash: manifest.hash, rootHash: nextRoot.hash,
      generation: root.generation, recordedAt: new Date().toISOString(),
    }, null, 2));
    await save();
    for (const file of files) await this.upload(hash(file.bytes), file);
    await this.upload(manifest.hash, { name: `${id}.docSchema`, bytes: manifest.bytes });
    await this.upload(nextRoot.hash, { name: "root.docSchema", bytes: nextRoot.bytes });
    state = "commit-unknown";
    await save();
    await this.request("/sync/v3/root", nextRoot.hash, {
      method: "PUT", headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ hash: nextRoot.hash, generation: root.generation, broadcast: true }),
    });
    state = "committed";
    await save();
    if (await this.documentHash(id) !== manifest.hash) throw new Error("Created document association differs");
    for (const file of files) {
      if (!verifyBytes(file.bytes, await this.documentFile(id, manifest.hash, file.name))) {
        throw new Error(`Created document bytes differ: ${file.name}`);
      }
    }
    state = "verified";
    await save();
  }
}
