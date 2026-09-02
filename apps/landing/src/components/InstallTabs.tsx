import * as React from "react";
import { Check, Copy } from "lucide-react";
import { Tabs, TabsList, TabsTrigger, TabsContent } from "./ui/Tabs";
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from "./ui/Tooltip";

const methods = [
	{
		id: "homebrew",
		label: "Homebrew",
		cmd: "brew install qyinm/tap/gandalf",
		hint: "Installs the pre-built binary to Homebrew's bin directory with auto-updates.",
	},
	{
		id: "curl",
		label: "curl",
		cmd: "curl -fsSL https://raw.githubusercontent.com/qyinm/gandalf/main/install.sh | bash",
		hint: "Directly downloads and installs the latest binary to ~/.local/bin/gandalf.",
	},
	{
		id: "go",
		label: "Go",
		cmd: "go install github.com/qyinm/gandalf/cmd/gandalf@latest",
		hint: "Builds from source using your local Go 1.22+ compiler into $GOPATH/bin.",
	},
] as const;

export default function InstallTabs() {
	const [activeTab, setActiveTab] = React.useState<string>("homebrew");
	const [copied, setCopied] = React.useState(false);

	const activeMethod = methods.find((m) => m.id === activeTab) ?? methods[0];

	const handleCopy = async () => {
		try {
			await navigator.clipboard.writeText(activeMethod.cmd);
			setCopied(true);
			setTimeout(() => setCopied(false), 2000);
		} catch {
			/* ignore */
		}
	};

	return (
		<TooltipProvider delayDuration={150}>
			<div className="install-tabs">
				<Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
					<div className="install-tabs__bar">
						<TabsList className="bg-transparent p-0 gap-1 h-auto">
							{methods.map((m) => (
								<TabsTrigger
									key={m.id}
									value={m.id}
									className="install-tabs__tab"
								>
									{m.label}
								</TabsTrigger>
							))}
						</TabsList>

						<Tooltip open={copied}>
							<TooltipTrigger asChild>
								<button
									type="button"
									className="install-tabs__copy"
									onClick={handleCopy}
									aria-label="Copy install command"
								>
									{copied ? (
										<>
											<Check size={13} className="text-emerald-400" />
											<span>Copied</span>
										</>
									) : (
										<>
											<Copy size={13} />
											<span>Copy</span>
										</>
									)}
								</button>
							</TooltipTrigger>
							<TooltipContent side="top">
								Command copied to clipboard!
							</TooltipContent>
						</Tooltip>
					</div>

					{methods.map((m) => (
						<TabsContent key={m.id} value={m.id} className="m-0 focus-visible:outline-none">
							<pre className="install-tabs__pre">
								<code>
									<span className="install-tabs__sigil">$</span> {m.cmd}
								</code>
							</pre>
							<p className="install-tabs__hint">{m.hint}</p>
						</TabsContent>
					))}
				</Tabs>
			</div>
		</TooltipProvider>
	);
}
