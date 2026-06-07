/**
 * Ink component for live report preview.
 *
 * Renders the markdown report output using ink-markdown.
 */

import React from "react";
import { Text, Box } from "ink";
import MarkdownModule from "ink-markdown";

const Markdown = MarkdownModule as unknown as React.ComponentType<{ children: string }>;

// ── Props ────────────────────────────────────────────────────

interface ReportPreviewProps {
  markdown: string;
}

// ── Component ────────────────────────────────────────────────

export default function ReportPreview({ markdown }: ReportPreviewProps) {
  if (!markdown || markdown.trim().length === 0) {
    return (
      <Box flexDirection="column">
        <Text bold>hem report</Text>
        <Text dimColor>Report is empty.</Text>
      </Box>
    );
  }

  return (
    <Box flexDirection="column">
      <Box marginBottom={1}>
        <Text bold>hem report preview</Text>
      </Box>
      <Markdown>
        {markdown}
      </Markdown>
    </Box>
  );
}
