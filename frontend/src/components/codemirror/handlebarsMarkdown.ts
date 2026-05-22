// Markdown extensions for the Template Code editor.
//
// We layer two parsers on top of the stock CommonMark grammar so the
// Template editor matches what's actually in those files:
//
//   1. Frontmatter - `---` ... `---` at the top of the document is
//      consumed as a single block. Without this the YAML inside reads
//      as a stream of "label: value" link-reference definitions, which
//      paints every line with the link-reference underline.
//   2. Handlebars - inline `{{ … }}` / `{{# … }}` / `{{/ … }}` /
//      `{{! … }}` / `{{> … }}` / `{{{ … }}}` get their own syntax
//      nodes so the highlighter can colour them as keywords / names /
//      strings instead of plain markdown text.

import type {
  BlockContext,
  BlockParser,
  InlineContext,
  InlineParser,
  Line,
  MarkdownExtension,
} from "@lezer/markdown";
import { styleTags, tags as t } from "@lezer/highlight";

// ── Frontmatter ────────────────────────────────────────────────────
// Block parser that fires only on line 1. Matches `---` exactly (no
// trailing content), then scans forward until the closing `---` or
// end-of-document. Styled as `meta` so it reads as a quiet header
// block rather than mis-tokenized link refs.

const FENCE_RE = /^---\s*$/;

const FrontmatterParser: BlockParser = {
  name: "Frontmatter",
  // `---` on its own line is also valid CommonMark for `<hr/>`, and
  // the built-in HorizontalRule parser consumes the opening fence
  // before we get a chance. Run before it so frontmatter wins.
  before: "HorizontalRule",
  parse(cx: BlockContext, line: Line): boolean {
    if (cx.lineStart !== 0) return false;
    if (!FENCE_RE.test(line.text)) return false;

    const from = cx.lineStart;
    while (cx.nextLine()) {
      if (FENCE_RE.test(line.text)) {
        const to = cx.lineStart + line.text.length;
        cx.addElement(cx.elt("Frontmatter", from, to));
        cx.nextLine();
        return true;
      }
    }
    // Unterminated frontmatter - consume what we have so the rest of
    // the doc parses cleanly as markdown rather than re-trying the
    // fence on every paragraph below.
    cx.addElement(cx.elt("Frontmatter", from, cx.lineStart));
    return true;
  },
};

// ── Handlebars ─────────────────────────────────────────────────────
// Inline parser that recognises every `{{`-led construct. We don't
// fully tokenise the expression body - that would mean re-implementing
// a Handlebars parser. Instead each construct is split into:
//
//   open      - `{{` / `{{{` / `{{#` / `{{/` / `{{!` / `{{>` plus the
//                immediately-following identifier (helper or path).
//                Identifier node type depends on the prefix: # / /
//                forms style as keyword, others as variableName, !
//                styles the whole body as comment.
//   body      - everything between the identifier and the closing
//                braces (string-like).
//   close     - the trailing `}}` or `}}}`.
//
// All three live under a `HandlebarsExpression` parent so a future
// styler can hook the whole construct in one place.

type HandlebarsPrefix = "{{" | "{{{" | "{{#" | "{{/" | "{{!" | "{{>";
type IdentKind = "keyword" | "name" | "comment";

interface HandlebarsMatch {
  prefix: HandlebarsPrefix;
  prefixLen: number;
  identEnd: number;
  bodyEnd: number;
  closeEnd: number;
  identKind: IdentKind;
}

function matchHandlebars(text: string, start: number): HandlebarsMatch | null {
  if (text.charCodeAt(start) !== 0x7b /* { */) return null;
  if (text.charCodeAt(start + 1) !== 0x7b) return null;

  let prefix: HandlebarsPrefix = "{{";
  let prefixLen = 2;
  let identKind: IdentKind = "name";

  const c2 = text[start + 2];
  if (c2 === "{") {
    prefix = "{{{";
    prefixLen = 3;
  } else if (c2 === "#") {
    prefix = "{{#";
    prefixLen = 3;
    identKind = "keyword";
  } else if (c2 === "/") {
    prefix = "{{/";
    prefixLen = 3;
    identKind = "keyword";
  } else if (c2 === "!") {
    prefix = "{{!";
    prefixLen = 3;
    identKind = "comment";
  } else if (c2 === ">") {
    prefix = "{{>";
    prefixLen = 3;
  }

  const closer = prefix === "{{{" ? "}}}" : "}}";
  const closeRelIdx = text.indexOf(closer, start + prefixLen);
  if (closeRelIdx < 0) return null;

  // Skip leading whitespace inside the braces and consume the
  // identifier (for keyword/name kinds). For comments the entire body
  // is the comment.
  let identStart = start + prefixLen;
  while (identStart < closeRelIdx && /\s/.test(text[identStart])) identStart++;
  let identEnd = identStart;
  if (identKind === "comment") {
    identEnd = closeRelIdx;
  } else {
    while (identEnd < closeRelIdx && /[\w@./-]/.test(text[identEnd])) identEnd++;
  }

  return {
    prefix,
    prefixLen,
    identEnd,
    bodyEnd: closeRelIdx,
    closeEnd: closeRelIdx + closer.length,
    identKind,
  };
}

function identNodeName(kind: IdentKind): string {
  switch (kind) {
    case "keyword":
      return "HandlebarsKeyword";
    case "comment":
      return "HandlebarsComment";
    default:
      return "HandlebarsName";
  }
}

const HandlebarsParser: InlineParser = {
  name: "Handlebars",
  parse(cx: InlineContext, next: number, pos: number): number {
    // Fast reject - the inline parser is called for every char.
    if (next !== 0x7b /* { */) return -1;

    // `pos` is document-relative. `cx.slice` takes document-relative
    // positions and returns the text between them, so we work with
    // relative offsets inside `text` and reapply `pos` to convert
    // back when constructing element ranges.
    const text = cx.slice(pos, Math.min(cx.end, pos + 256));
    const m = matchHandlebars(text, 0);
    if (!m) return -1;

    const children = [
      cx.elt(identNodeName(m.identKind), pos, pos + m.identEnd),
    ];
    if (m.bodyEnd > m.identEnd) {
      children.push(cx.elt("HandlebarsBody", pos + m.identEnd, pos + m.bodyEnd));
    }
    children.push(cx.elt("HandlebarsClose", pos + m.bodyEnd, pos + m.closeEnd));

    return cx.addElement(
      cx.elt("HandlebarsExpression", pos, pos + m.closeEnd, children),
    );
  },
};

// ── Style tags ─────────────────────────────────────────────────────
// One tag per leaf node. NO parent-scoped selector (e.g.
// "HandlebarsExpression/...") - that paints every child with the
// parent's brace style and overrides the more specific keyword /
// variableName / string tags on the children.
const handlebarsStyle = styleTags({
  Frontmatter: t.meta,
  HandlebarsKeyword: t.keyword,
  HandlebarsName: t.variableName,
  HandlebarsBody: t.string,
  HandlebarsClose: t.special(t.brace),
  HandlebarsComment: t.comment,
});

export const handlebarsMarkdownExtensions: MarkdownExtension = {
  defineNodes: [
    { name: "Frontmatter", block: true },
    "HandlebarsExpression",
    "HandlebarsKeyword",
    "HandlebarsName",
    "HandlebarsBody",
    "HandlebarsClose",
    "HandlebarsComment",
  ],
  parseBlock: [FrontmatterParser],
  parseInline: [HandlebarsParser],
  props: [handlebarsStyle],
};
