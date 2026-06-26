import { useState } from "react";

const tabs = [
	{ label: "npm", cmd: "npm install -g @qxinm/gandalf" },
] as const;

export default function InstallTabs() {
	const [active, setActive] = useState(0);
	const [copied, setCopied] = useState(false);

	const onCopy = async () => {
		try {
			await navigator.clipboard.writeText(tabs[active].cmd);
			setCopied(true);
			setTimeout(() => setCopied(false), 1600);
		} catch {
			/* ignore */
		}
	};

	return (
		<div className="install-tabs">
			<div
				className="install-tabs__bar"
				role="tablist"
				aria-label="Install method"
			>
				{tabs.map((t, i) => (
					<button
						key={t.label}
						role="tab"
						aria-selected={i === active}
						className={`install-tabs__tab ${i === active ? "is-active" : ""}`}
						onClick={() => setActive(i)}
						type="button"
					>
						{t.label}
					</button>
				))}
				<button
					type="button"
					className="install-tabs__copy"
					onClick={onCopy}
					aria-label="Copy command"
				>
					{copied ? "Copied" : "Copy"}
				</button>
			</div>
			<pre className="install-tabs__pre">
				<code>
					<span className="install-tabs__sigil">$</span> {tabs[active].cmd}
				</code>
			</pre>
		</div>
	);
}
