// TokiEditor — CodeMirror 6 web component.
// <toki-editor value="..." font="..." font-size="..." theme="light|dark" readonly></toki-editor>
//
// Uses Shadow DOM so morphdom cannot see or modify the internal CM6 DOM.
// Readonly editors stay as lightweight static HTML (never create EditorView).
// Editable editors show a static preview first, then create EditorView on click.
//
// Reactive attributes:
//   value     — editor content
//   font      — CSS font-family for the editor
//   font-size — CSS font-size for the editor
//   theme     — optional "light" or "dark" override; otherwise inherits document theme

import { EditorState, Compartment, type Extension } from "@codemirror/state";
import {
  EditorView,
  lineNumbers,
  drawSelection,
  highlightSpecialChars,
  keymap,
} from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { icuMode, icuTokenizer } from "./icu-mode";
import { lightTheme, darkTheme } from "./themes";

// --- Shared shadow-root stylesheet ---

const SHADOW_CSS = [
  ":host{display:block}",
  ".cm-static{position:relative;box-sizing:border-box;display:flex;flex-direction:column}",
  ".cm-static .cm-scroller{display:flex;align-items:flex-start;",
  "font-family:var(--te-font,var(--font-editor,monospace));",
  "font-size:var(--te-font-size,var(--font-size-editor,14px));",
  "line-height:1.4;overflow-x:auto;position:relative}",
  ".cm-static .cm-content{margin:0;flex-grow:2;flex-shrink:1;display:block;",
  "white-space:break-spaces;word-break:break-word;overflow-wrap:anywhere;",
  "box-sizing:border-box;padding:4px 0}",
  ".cm-static .cm-line{display:block;padding:0 2px 0 6px}",
  ".cm-static .cm-gutters{flex-shrink:0;display:flex;height:100%;box-sizing:border-box}",
  ".cm-static .cm-gutter{display:flex;flex-direction:column;flex-shrink:0;",
  "box-sizing:border-box;min-height:100%;overflow:hidden;padding:4px 0}",
  ".cm-static .cm-gutterElement{box-sizing:border-box;padding:0 3px 0 5px;",
  "min-width:20px;text-align:right;white-space:nowrap}",
  ".cm-static{background:#f5f5f5;color:#202020}",
  ".cm-static .cm-gutters{background:#f5f5f5;color:#b0b0b0;border-right:none}",
  '.cm-static[data-theme="dark"]{background:#151515;color:#e0e0e0}',
  '.cm-static[data-theme="dark"] .cm-gutters{background:#151515;color:#505050}',
  ".tok-keyword{color:#ac4142}",
  ".tok-atom,.tok-number{color:#aa759f}",
  ".tok-string{color:#f4bf75}",
  ".tok-variableName{color:#90a959}",
  ".tok-bracket{color:#202020}",
  ".tok-operator{color:#d28445}",
  '[data-theme="dark"] .tok-bracket{color:#e0e0e0}',
  ".cm-editor .cm-scroller{font-family:var(--te-font,var(--font-editor,monospace));",
  "font-size:var(--te-font-size,var(--font-size-editor,14px));overflow:auto}",
  ".cm-whitespace{position:relative}",
  '.cm-space::before{content:"\\00B7";position:absolute;inset:0;',
  "display:flex;align-items:center;justify-content:center;",
  "color:var(--border,oklch(0 0 0/0.2));pointer-events:none}",
  '.cm-nbsp::before{content:"\\2423";position:absolute;inset:0;',
  "display:flex;align-items:center;justify-content:center;",
  "color:var(--border,oklch(0 0 0/0.2));pointer-events:none}",
  ":host(:not([readonly])) .cm-static{cursor:text}",
].join("\n");

// --- Static highlight rendering ---

const tokenClasses: Record<string, string> = {
  string: "tok-string",
  bracket: "tok-bracket",
  variableName: "tok-variableName",
  keyword: "tok-keyword",
  atom: "tok-atom",
  number: "tok-number",
  operator: "tok-operator",
};

interface SimpleStreamLike {
  string: string;
  pos: number;
  start: number;
  eol(): boolean;
  peek(): string | undefined;
  next(): string | undefined;
  eat(match: string | RegExp | ((ch: string) => boolean)): string | undefined;
  eatWhile(match: string | RegExp | ((ch: string) => boolean)): boolean;
  eatSpace(): boolean;
  skipToEnd(): void;
  skipTo(ch: string): boolean;
  match(pattern: string | RegExp, consume?: boolean): string[] | boolean | null;
  current(): string;
}

class SimpleStream implements SimpleStreamLike {
  string: string;
  pos: number;
  start: number;

  constructor(line: string) {
    this.string = line;
    this.pos = 0;
    this.start = 0;
  }
  eol() { return this.pos >= this.string.length; }
  peek() { return this.string.charAt(this.pos) || undefined; }
  next() {
    if (this.pos < this.string.length) return this.string.charAt(this.pos++);
  }
  eat(match: string | RegExp | ((ch: string) => boolean)) {
    const ch = this.string.charAt(this.pos);
    let ok: boolean;
    if (typeof match === "string") ok = ch === match;
    else if (match instanceof RegExp) ok = match.test(ch);
    else ok = !!ch && match(ch);
    if (ok) { this.pos++; return ch; }
  }
  eatWhile(match: string | RegExp | ((ch: string) => boolean)) {
    const start = this.pos;
    while (this.eat(match)) {}
    return this.pos > start;
  }
  eatSpace() {
    const start = this.pos;
    while (/\s/.test(this.string.charAt(this.pos))) this.pos++;
    return this.pos > start;
  }
  skipToEnd() { this.pos = this.string.length; }
  skipTo(ch: string) {
    const found = this.string.indexOf(ch, this.pos);
    if (found > -1) { this.pos = found; return true; }
    return false;
  }
  match(pattern: string | RegExp, consume?: boolean): string[] | boolean | null {
    if (typeof pattern === "string") {
      if (this.string.slice(this.pos, this.pos + pattern.length) === pattern) {
        if (consume !== false) this.pos += pattern.length;
        return true;
      }
      return null;
    }
    const m = this.string.slice(this.pos).match(pattern);
    if (m && m.index === 0) {
      if (consume !== false) this.pos += m[0].length;
      return m;
    }
    return null;
  }
  current() { return this.string.slice(this.start, this.pos); }
}

function renderStatic(text: string, dark: boolean): HTMLElement {
  const editor = document.createElement("div");
  editor.className = "cm-static";
  editor.setAttribute("data-theme", dark ? "dark" : "light");

  const scroller = document.createElement("div");
  scroller.className = "cm-scroller";

  const gutters = document.createElement("div");
  gutters.className = "cm-gutters";
  gutters.setAttribute("aria-hidden", "true");
  const gutter = document.createElement("div");
  gutter.className = "cm-gutter cm-lineNumbers";
  gutters.appendChild(gutter);

  const content = document.createElement("div");
  content.className = "cm-content";
  content.setAttribute("role", "textbox");

  const lines = text ? text.split("\n") : [""];
  const state = icuTokenizer.startState();

  for (let i = 0; i < lines.length; i++) {
    const gutterEl = document.createElement("div");
    gutterEl.className = "cm-gutterElement";
    gutterEl.textContent = String(i + 1);
    gutter.appendChild(gutterEl);

    const lineEl = document.createElement("div");
    lineEl.className = "cm-line";

    const line = lines[i];
    if (line.length === 0) {
      lineEl.appendChild(document.createElement("br"));
    } else {
      const stream = new SimpleStream(line);
      while (!stream.eol()) {
        const start = stream.pos;
        const tokenType = icuTokenizer.token(stream as any, state);
        const value = line.slice(start, stream.pos);
        if (tokenType && tokenClasses[tokenType]) {
          const span = document.createElement("span");
          span.className = tokenClasses[tokenType];
          span.textContent = value;
          lineEl.appendChild(span);
        } else {
          lineEl.appendChild(document.createTextNode(value));
        }
      }
    }
    content.appendChild(lineEl);
  }

  scroller.appendChild(gutters);
  scroller.appendChild(content);
  editor.appendChild(scroller);
  return editor;
}

function getResolvedDarkTheme(host: HTMLElement): boolean {
  const theme = host.getAttribute("theme");
  if (theme === "dark") return true;
  if (theme === "light") return false;
  return document.documentElement.classList.contains("dark");
}

class TokiEditorElement extends HTMLElement {
  static observedAttributes = ["value", "font", "font-size", "theme"];

  _shadow: ShadowRoot;
  _view: EditorView | null = null;
  _themeComp: Compartment | null = null;
  _syncing = false;
  _dirty = false;
  _staticDom: HTMLElement | null = null;
  _handleDocumentThemeChange: () => void;

  constructor() {
    super();
    this._shadow = this.attachShadow({ mode: "open" });
    this._handleDocumentThemeChange = () => {
      if (!this.hasAttribute("theme")) {
        this._onThemeChange(null);
      }
    };
  }

  connectedCallback() {
    document.addEventListener("toki-theme-change", this._handleDocumentThemeChange);
    const style = document.createElement("style");
    style.textContent = SHADOW_CSS;
    this._shadow.appendChild(style);

    const text = this.getAttribute("value") || "";
    const dark = getResolvedDarkTheme(this);
    this._staticDom = renderStatic(text, dark);
    this._shadow.appendChild(this._staticDom);

    // Apply initial font/font-size as CSS custom properties on :host.
    const font = this.getAttribute("font");
    const fontSize = this.getAttribute("font-size");
    if (font) this.style.setProperty("--te-font", font);
    if (fontSize) this.style.setProperty("--te-font-size", fontSize);

    if (!this.hasAttribute("readonly")) {
      this._staticDom.addEventListener("click", (e) => {
        this._promote(e);
      }, { once: true });
    }
  }

  disconnectedCallback() {
    document.removeEventListener("toki-theme-change", this._handleDocumentThemeChange);
  }

  attributeChangedCallback(name: string, _oldVal: string | null, newVal: string | null) {
    switch (name) {
      case "value":
        this._onValueChange(newVal);
        break;
      case "theme":
        this._onThemeChange(newVal);
        break;
      case "font":
        if (newVal) this.style.setProperty("--te-font", newVal);
        else this.style.removeProperty("--te-font");
        break;
      case "font-size":
        if (newVal) this.style.setProperty("--te-font-size", newVal);
        else this.style.removeProperty("--te-font-size");
        break;
    }
  }

  private _onValueChange(newVal: string | null) {
    if (this._syncing) return;
    const val = newVal || "";

    if (this._view) {
      if (this._dirty) return;
      const cur = this._view.state.doc.toString();
      if (cur !== val) {
        this._syncing = true;
        this._view.dispatch({
          changes: { from: 0, to: cur.length, insert: val },
        });
        this._syncing = false;
      }
    } else if (this._staticDom) {
      const dark = getResolvedDarkTheme(this);
      const newDom = renderStatic(val, dark);
      if (!this.hasAttribute("readonly")) {
        newDom.addEventListener("click", () => {
          this._promote();
        }, { once: true });
      }
      this._staticDom.replaceWith(newDom);
      this._staticDom = newDom;
    }
  }

  private _onThemeChange(newVal: string | null) {
    const dark = newVal == null ? getResolvedDarkTheme(this) : newVal === "dark";
    const ext = dark ? darkTheme : lightTheme;
    if (this._view && this._themeComp) {
      this._view.dispatch({
        effects: this._themeComp.reconfigure(ext),
      });
    } else if (this._staticDom) {
      this._staticDom.setAttribute("data-theme", dark ? "dark" : "light");
    }
  }

  get value(): string {
    if (this._view) return this._view.state.doc.toString();
    return this.getAttribute("value") || "";
  }

  set value(v: string) {
    this.setAttribute("value", v || "");
  }

  _promote(clickEvent?: MouseEvent) {
    if (this._view) { this._view.focus(); return; }

    const text = this.getAttribute("value") || "";
    const dark = getResolvedDarkTheme(this);
    this._themeComp = new Compartment();

    const extensions: Extension[] = [
      icuMode,
      lineNumbers(),
      drawSelection(),
      highlightSpecialChars({ addSpecialChars: /[\u00a0]/g }),
      this._themeComp.of(dark ? darkTheme : lightTheme),
      EditorView.lineWrapping,
      history(),
      keymap.of([...defaultKeymap, ...historyKeymap]),
    ];

    let timer: ReturnType<typeof setTimeout>;
    extensions.push(
      EditorView.updateListener.of((update) => {
        if (!update.docChanged || this._syncing) return;
        this._dirty = true;
        clearTimeout(timer);
        timer = setTimeout(() => {
          if (!this._view) return;
          const val = this._view.state.doc.toString();
          this._syncing = true;
          this.setAttribute("value", val);
          this._syncing = false;
          this.dispatchEvent(
            new CustomEvent("toki-change", {
              bubbles: true,
              detail: {
                value: val,
                tikid: this.dataset.tikid,
                locale: this.dataset.locale,
              },
            })
          );
          this._dirty = false;
        }, 200);
      })
    );

    const state = EditorState.create({ doc: text, extensions });
    this._view = new EditorView({ state, parent: this._shadow });

    if (this._staticDom) {
      this._staticDom.remove();
      this._staticDom = null;
    }

    if (clickEvent) {
      const pos = this._view.posAtCoords({
        x: clickEvent.clientX,
        y: clickEvent.clientY,
      });
      if (pos != null) {
        this._view.dispatch({ selection: { anchor: pos } });
      }
    }
    this._view.focus();
  }
}

customElements.define("toki-editor", TokiEditorElement);
