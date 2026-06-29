package plugins

import "github.com/qyinm/gandalf/internal/gandalfcore/scan"

func init() {
	scan.SetDefaultPluginFactory(func() []scan.ScannerPlugin {
		return []scan.ScannerPlugin{
			ClaudeCodeScanner{},
			CodexScanner{},
		}
	})
}
