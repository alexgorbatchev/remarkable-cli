export type Mapping = { source: string; page: number };

export type CheckOptions = {
  mapping: string;
  binary: string;
  outputDir: string;
  config?: string;
};

export type PageEvidence = {
  pageIndex: number;
  tabletPage: number;
  pageID?: string;
  originalSource: string;
  sourceSnapshot: string;
  downloadedNative: string;
  sourcePreview: string;
  destinationPreview: string;
  lines: number;
  sha256: string;
};
