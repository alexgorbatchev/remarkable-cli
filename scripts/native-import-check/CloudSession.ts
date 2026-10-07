type Entry = { hash: string; type: string; id: string; count: number; size: number };
type Root = { hash: string; generation: number };

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
}
