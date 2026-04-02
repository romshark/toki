import { EditorView } from "@codemirror/view";
import { HighlightStyle, syntaxHighlighting } from "@codemirror/language";
import { tags } from "@lezer/highlight";

// Base16 Light
const lightEditorTheme = EditorView.theme({
  "&": { backgroundColor: "#f5f5f5", color: "#202020" },
  ".cm-scroller": {
    fontFamily: "var(--font-editor, monospace)",
    fontSize: "var(--font-size-editor, 14px)",
  },
  ".cm-content": { caretColor: "#505050" },
  "&.cm-focused .cm-cursor": { borderLeftColor: "#505050" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "#e0e0e0",
  },
  ".cm-gutters": { backgroundColor: "#f5f5f5", color: "#b0b0b0", borderRight: "none" },
  ".cm-activeLineGutter": { backgroundColor: "#dddcdc" },
  ".cm-activeLine": { backgroundColor: "#dddcdc" },
}, { dark: false });

const lightHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: "#ac4142" },
  { tag: tags.atom, color: "#aa759f" },
  { tag: tags.number, color: "#aa759f" },
  { tag: tags.string, color: "#f4bf75" },
  { tag: tags.variableName, color: "#90a959" },
  { tag: tags.propertyName, color: "#90a959" },
  { tag: tags.definition(tags.variableName), color: "#d28445" },
  { tag: tags.bracket, color: "#202020" },
  { tag: tags.tagName, color: "#ac4142" },
  { tag: tags.link, color: "#aa759f" },
  { tag: tags.operator, color: "#d28445" },
  { tag: tags.comment, color: "#8f5536" },
  { tag: tags.invalid, color: "#505050", backgroundColor: "#ac4142" },
]);

// Base16 Dark
const darkEditorTheme = EditorView.theme({
  "&": { backgroundColor: "#151515", color: "#e0e0e0" },
  ".cm-scroller": {
    fontFamily: "var(--font-editor, monospace)",
    fontSize: "var(--font-size-editor, 14px)",
  },
  ".cm-content": { caretColor: "#b0b0b0" },
  "&.cm-focused .cm-cursor": { borderLeftColor: "#b0b0b0" },
  "&.cm-focused .cm-selectionBackground, .cm-selectionBackground, ::selection": {
    backgroundColor: "#303030",
  },
  ".cm-gutters": { backgroundColor: "#151515", color: "#505050", borderRight: "none" },
  ".cm-activeLineGutter": { backgroundColor: "#202020" },
  ".cm-activeLine": { backgroundColor: "#202020" },
}, { dark: true });

const darkHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: "#ac4142" },
  { tag: tags.atom, color: "#aa759f" },
  { tag: tags.number, color: "#aa759f" },
  { tag: tags.string, color: "#f4bf75" },
  { tag: tags.variableName, color: "#90a959" },
  { tag: tags.propertyName, color: "#90a959" },
  { tag: tags.definition(tags.variableName), color: "#d28445" },
  { tag: tags.bracket, color: "#e0e0e0" },
  { tag: tags.tagName, color: "#ac4142" },
  { tag: tags.link, color: "#aa759f" },
  { tag: tags.operator, color: "#d28445" },
  { tag: tags.comment, color: "#8f5536" },
  { tag: tags.invalid, color: "#b0b0b0", backgroundColor: "#ac4142" },
]);

export const lightTheme = [lightEditorTheme, syntaxHighlighting(lightHighlightStyle)];
export const darkTheme = [darkEditorTheme, syntaxHighlighting(darkHighlightStyle)];
