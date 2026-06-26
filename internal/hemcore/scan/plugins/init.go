package plugins

import "github.com/qyinm/hem/internal/hemcore/scan"

func init() {
	scan.SetDefaultPluginFactory(func() []scan.ScannerPlugin {
		return []scan.ScannerPlugin{
			ClaudeCodeScanner{},
			CodexScanner{},
			CursorScanner{},
			OpenCodeScanner{},
			PiAgentScanner{},
			ProjectScanner{},
		}
	})
}