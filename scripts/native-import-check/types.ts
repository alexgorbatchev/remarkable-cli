import type { CloudSession } from "./CloudSession";

export type Mapping = { source: string; page: number };

export type CheckOptions = {
  mapping: string;
  binary: string;
  outputDir: string;
  config?: string;
};

export type SourceOptions = Omit<CheckOptions, "mapping"> & { page: string[] };

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

export type SelectedPage = { pageIndex: number; tabletPage: number; sourcePageID: string; lines: number };

export type PreparedCheck = {
  source: string;
  sourceHash: string;
  destination: string;
  options: CheckOptions;
  cloud: CloudSession;
  creationEvidence: string;
  pageIDs: string[];
  settingsMapping: string;
};
