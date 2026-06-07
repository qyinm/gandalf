export interface SnapshotListRow {
  name: string;
  selected: boolean;
}

export interface SnapshotListViewModel {
  title: string;
  emptyMessage?: string;
  emptyAction?: string;
  rows: SnapshotListRow[];
}

export function buildSnapshotListViewModel(input: {
  names: string[];
  selectedIndex?: number;
}): SnapshotListViewModel {
  const selectedIndex = clampIndex(input.selectedIndex ?? 0, input.names.length);

  return {
    title: `Saved setups (${input.names.length} total)`,
    emptyMessage: input.names.length === 0 ? "No saved setups yet." : undefined,
    emptyAction: input.names.length === 0 ? "s save setup" : undefined,
    rows: input.names.map((name, index) => ({
      name,
      selected: index === selectedIndex
    }))
  };
}

function clampIndex(index: number, length: number): number {
  if (length <= 0) return 0;
  return Math.min(Math.max(0, index), length - 1);
}
