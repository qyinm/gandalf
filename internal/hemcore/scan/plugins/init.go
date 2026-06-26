package plugins

import "github.com/qyinm/hem/internal/hemcore/scan"

func init() {
	scan.SetDefaultPluginFactory(func() []scan.ScannerPlugin {
		return []scan.ScannerPlugin{
			scan.ClaudeCodeScanner{},
			CodexScanner{},
			scan.CursorScanner{},
			scan.OpenCodeScanner{},
			scan.PiAgentScanner{},
			ProjectScanner{},
		}
	})
}