/**
 * Lightweight markdown renderer for DeepSeek R1 inference results.
 *
 * Handles the patterns DeepSeek R1 actually outputs:
 *   - Headings (# / ## / ###)
 *   - **bold** and *italic*
 *   - `inline code` and ```code blocks```
 *   - - bullet lists and 1. numbered lists
 *   - > blockquotes
 *   - Plain paragraphs with blank-line separation
 *
 * No external dependencies — pure React elements.
 */

import { Fragment } from 'react';
import { cn } from '@/lib/utils';

type Token =
  | { type: 'heading'; level: 1 | 2 | 3; text: string }
  | { type: 'code_block'; lang: string; code: string }
  | { type: 'blockquote'; text: string }
  | { type: 'ul'; items: string[] }
  | { type: 'ol'; items: string[] }
  | { type: 'paragraph'; text: string }
  | { type: 'blank' };

function tokenize(markdown: string): Token[] {
  const lines = markdown.split('\n');
  const tokens: Token[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];

    // Fenced code block
    if (line.trimStart().startsWith('```')) {
      const lang = line.trim().slice(3).trim();
      const codeLines: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith('```')) {
        codeLines.push(lines[i]);
        i++;
      }
      tokens.push({ type: 'code_block', lang, code: codeLines.join('\n') });
      i++;
      continue;
    }

    // Headings
    const h3 = line.match(/^###\s+(.*)/);
    const h2 = line.match(/^##\s+(.*)/);
    const h1 = line.match(/^#\s+(.*)/);
    if (h1) { tokens.push({ type: 'heading', level: 1, text: h1[1] }); i++; continue; }
    if (h2) { tokens.push({ type: 'heading', level: 2, text: h2[1] }); i++; continue; }
    if (h3) { tokens.push({ type: 'heading', level: 3, text: h3[1] }); i++; continue; }

    // Blockquote
    if (line.startsWith('> ')) {
      const quoteLines: string[] = [];
      while (i < lines.length && lines[i].startsWith('> ')) {
        quoteLines.push(lines[i].slice(2));
        i++;
      }
      tokens.push({ type: 'blockquote', text: quoteLines.join('\n') });
      continue;
    }

    // Unordered list
    if (/^[-*+]\s/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^[-*+]\s/.test(lines[i])) {
        items.push(lines[i].replace(/^[-*+]\s/, ''));
        i++;
      }
      tokens.push({ type: 'ul', items });
      continue;
    }

    // Ordered list
    if (/^\d+\.\s/.test(line)) {
      const items: string[] = [];
      while (i < lines.length && /^\d+\.\s/.test(lines[i])) {
        items.push(lines[i].replace(/^\d+\.\s/, ''));
        i++;
      }
      tokens.push({ type: 'ol', items });
      continue;
    }

    // Blank line
    if (line.trim() === '') {
      tokens.push({ type: 'blank' });
      i++;
      continue;
    }

    // Paragraph — accumulate until blank line or block element
    const paraLines: string[] = [];
    while (
      i < lines.length &&
      lines[i].trim() !== '' &&
      !/^(#{1,3}|```|> |[-*+]\s|\d+\.\s)/.test(lines[i])
    ) {
      paraLines.push(lines[i]);
      i++;
    }
    if (paraLines.length > 0) {
      tokens.push({ type: 'paragraph', text: paraLines.join(' ') });
    }
  }

  return tokens;
}

// Render inline formatting: **bold**, *italic*, `code`
function InlineText({ text }: { text: string }) {
  const parts: React.ReactNode[] = [];
  // Split on bold, italic, inline code
  const re = /(\*\*(.+?)\*\*|\*(.+?)\*|`([^`]+)`)/g;
  let last = 0;
  let m: RegExpExecArray | null;

  while ((m = re.exec(text)) !== null) {
    if (m.index > last) parts.push(text.slice(last, m.index));
    if (m[0].startsWith('**'))      parts.push(<strong key={m.index} className="font-semibold text-white">{m[2]}</strong>);
    else if (m[0].startsWith('*'))  parts.push(<em key={m.index} className="italic text-neutral-300">{m[3]}</em>);
    else if (m[0].startsWith('`'))  parts.push(<code key={m.index} className="rounded bg-neutral-700 px-1 py-0.5 font-mono text-[0.8em] text-emerald-300">{m[4]}</code>);
    last = m.index + m[0].length;
  }
  if (last < text.length) parts.push(text.slice(last));

  return <>{parts}</>;
}

export function MarkdownRenderer({ content, className }: { content: string; className?: string }) {
  const tokens = tokenize(content);

  return (
    <div className={cn('space-y-2 text-sm leading-relaxed text-neutral-100', className)}>
      {tokens.map((tok, idx) => {
        switch (tok.type) {
          case 'heading': {
            const Tag = `h${tok.level}` as 'h1' | 'h2' | 'h3';
            const cls = tok.level === 1
              ? 'text-base font-bold text-white mt-3'
              : tok.level === 2
              ? 'text-sm font-semibold text-white mt-2'
              : 'text-sm font-medium text-neutral-200 mt-1';
            return <Tag key={idx} className={cls}><InlineText text={tok.text} /></Tag>;
          }

          case 'code_block':
            return (
              <div key={idx} className="rounded-lg bg-neutral-900 border border-neutral-700 overflow-x-auto">
                {tok.lang && (
                  <div className="px-3 py-1 text-xs text-neutral-500 border-b border-neutral-700 font-mono">
                    {tok.lang}
                  </div>
                )}
                <pre className="px-3 py-2 text-xs text-emerald-300 font-mono whitespace-pre">
                  {tok.code}
                </pre>
              </div>
            );

          case 'blockquote':
            return (
              <blockquote key={idx} className="border-l-2 border-neutral-600 pl-3 text-neutral-400 italic">
                <InlineText text={tok.text} />
              </blockquote>
            );

          case 'ul':
            return (
              <ul key={idx} className="space-y-0.5 pl-4">
                {tok.items.map((item, j) => (
                  <li key={j} className="flex gap-2">
                    <span className="text-emerald-400 mt-0.5 shrink-0">•</span>
                    <span><InlineText text={item} /></span>
                  </li>
                ))}
              </ul>
            );

          case 'ol':
            return (
              <ol key={idx} className="space-y-0.5 pl-4">
                {tok.items.map((item, j) => (
                  <li key={j} className="flex gap-2">
                    <span className="text-emerald-400 shrink-0 tabular-nums">{j + 1}.</span>
                    <span><InlineText text={item} /></span>
                  </li>
                ))}
              </ol>
            );

          case 'blank':
            return <div key={idx} className="h-1" />;

          case 'paragraph':
            return (
              <p key={idx}>
                <InlineText text={tok.text} />
              </p>
            );

          default:
            return null;
        }
      })}
    </div>
  );
}
