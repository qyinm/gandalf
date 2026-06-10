import { useState } from 'react';
import { Copy, Check } from 'lucide-react';

interface Props {
  text: string;
  label?: string;
}

export default function CopyButton({ text, label = 'Copy command' }: Props) {
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(text);
    } catch {
      // Fallback for older browsers
      const ta = document.createElement('textarea');
      ta.value = text;
      ta.style.position = 'fixed';
      ta.style.opacity = '0';
      document.body.appendChild(ta);
      ta.select();
      document.execCommand('copy');
      document.body.removeChild(ta);
    }
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  return (
    <button
      className="copy-btn"
      onClick={copy}
      aria-label={label}
    >
      {copied ? <Check size={16} /> : <Copy size={16} />}
    </button>
  );
}