export interface SnapError {
  code: string;
  problem: string;
  cause: string;
  fix: string;
  path?: string;
}

export function formatSnapError(error: SnapError): string {
  const lines = [
    error.code,
    `Problem: ${error.problem}`,
    `Cause: ${error.cause}`,
    `Fix: ${error.fix}`
  ];

  if (error.path) {
    lines.push(`Path: ${error.path}`);
  }

  return `${lines.join("\n")}\n`;
}
