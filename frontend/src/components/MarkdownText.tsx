import OpenInNewIcon from '@mui/icons-material/OpenInNew';
import Box from '@mui/material/Box';
import Link from '@mui/material/Link';
import Typography, { type TypographyProps } from '@mui/material/Typography';
import type { ReactNode } from 'react';
import { useTranslation } from 'react-i18next';
import { type Block, type Inline, parseInlineMarkdown, parseMarkdown } from '../utils/markdown';

interface MarkdownTextProps {
  /** The stored source, rendered but never altered. */
  text: string;
  variant?: TypographyProps['variant'];
  sx?: TypographyProps['sx'];
}

// The parsed tree is immutable and re-derived on every render, so a node's
// position is its identity.
const positionalKey = (i: number) => `n${i}`;

const visuallyHidden = {
  position: 'absolute',
  width: 1,
  height: 1,
  overflow: 'hidden',
  clipPath: 'inset(50%)',
  whiteSpace: 'nowrap',
} as const;

function renderInline(nodes: Inline[], t: (key: string) => string): ReactNode[] {
  return nodes.map((node, idx) => {
    switch (node.kind) {
      case 'text':
        return node.text;
      case 'code':
        return (
          <Box
            key={positionalKey(idx)}
            component="code"
            sx={{ fontFamily: 'monospace', bgcolor: 'action.hover', px: 0.5, borderRadius: 0.5 }}
          >
            {node.text}
          </Box>
        );
      case 'strong':
        return <strong key={positionalKey(idx)}>{renderInline(node.children, t)}</strong>;
      case 'em':
        return <em key={positionalKey(idx)}>{renderInline(node.children, t)}</em>;
      case 'link':
        return (
          <Link
            key={positionalKey(idx)}
            href={node.href}
            {...(node.external
              ? { target: '_blank', rel: 'noopener noreferrer nofollow' }
              : { rel: 'noopener noreferrer' })}
            sx={{ overflowWrap: 'anywhere' }}
          >
            {renderInline(node.children, t)}
            {node.external && (
              <>
                <OpenInNewIcon
                  aria-hidden="true"
                  sx={{ fontSize: '0.9em', ml: 0.25, verticalAlign: '-0.1em' }}
                />
                <Box component="span" sx={visuallyHidden}>
                  {` ${t('markdown.opensInNewTab')}`}
                </Box>
              </>
            )}
          </Link>
        );
      default:
        return null;
    }
  });
}

function renderBlocks(blocks: Block[], t: (key: string) => string): ReactNode[] {
  return blocks.map((block, idx) => {
    // Blank lines between blocks keep their height, so plain text with blank
    // lines lays out as it did under white-space: pre-wrap.
    const gapSx = { mt: block.gap > 0 ? `${block.gap}lh` : 0 };
    switch (block.type) {
      case 'paragraph':
        return (
          <Box key={positionalKey(idx)} component="p" sx={{ m: 0, ...gapSx }}>
            {renderInline(parseInlineSource(block.lines), t)}
          </Box>
        );
      case 'heading': {
        const level = Math.min(6, block.level + 2); // keep the page's own h1/h2 outline intact
        return (
          <Box
            key={positionalKey(idx)}
            component={`h${level}` as 'h3'}
            sx={{
              m: 0,
              ...gapSx,
              fontSize: `${Math.max(1, 1.5 - (block.level - 1) * 0.1)}em`,
              fontWeight: 600,
              lineHeight: 1.3,
            }}
          >
            {renderInline(parseInlineSource([block.text]), t)}
          </Box>
        );
      }
      case 'list':
        return (
          <Box
            key={positionalKey(idx)}
            component={block.ordered ? 'ol' : 'ul'}
            start={block.ordered ? block.start : undefined}
            sx={{ m: 0, pl: 3, ...gapSx }}
          >
            {block.items.map((item, j) => (
              <li key={positionalKey(j)}>{renderInline(parseInlineSource([item]), t)}</li>
            ))}
          </Box>
        );
      case 'quote':
        return (
          <Box
            key={positionalKey(idx)}
            component="blockquote"
            sx={{
              m: 0,
              pl: 1.5,
              borderLeft: 3,
              borderColor: 'divider',
              color: 'text.secondary',
              ...gapSx,
            }}
          >
            {renderBlocks(block.blocks, t)}
          </Box>
        );
      case 'rule':
        return (
          <Box key={positionalKey(idx)} component="hr" sx={{ ...gapSx, borderColor: 'divider' }} />
        );
      case 'code':
        return (
          <Box
            key={positionalKey(idx)}
            component="pre"
            tabIndex={0}
            sx={{
              m: 0,
              p: 1,
              overflow: 'auto',
              fontFamily: 'monospace',
              whiteSpace: 'pre',
              bgcolor: 'action.hover',
              borderRadius: 1,
              ...gapSx,
            }}
          >
            {block.text}
          </Box>
        );
      default:
        return null;
    }
  });
}

function parseInlineSource(lines: string[]): Inline[] {
  return parseInlineMarkdown(lines.join('\n'));
}

/**
 * Renders note / activity-description text. Plain text keeps its paragraphs and
 * line breaks (white-space: pre-wrap); a small Markdown subset adds structure.
 * Output is React elements only — never raw HTML (#2005).
 */
export default function MarkdownText({ text, variant = 'body2', sx }: MarkdownTextProps) {
  const { t } = useTranslation();
  return (
    <Typography
      variant={variant}
      component="div"
      sx={[
        { whiteSpace: 'pre-wrap', overflowWrap: 'anywhere' },
        ...(Array.isArray(sx) ? sx : [sx]),
      ]}
    >
      {renderBlocks(parseMarkdown(text), t)}
    </Typography>
  );
}
