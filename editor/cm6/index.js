// TokiEditor — CodeMirror 6 web component.
// <toki-editor value="..." readonly></toki-editor>
//
// Uses Shadow DOM so morphdom cannot see or modify the internal CM6 DOM.
// Readonly editors stay as lightweight static HTML (never create EditorView).
// Editable editors show a static preview first, then create EditorView on click.

import { EditorState, Compartment } from "@codemirror/state";
import {
  EditorView,
  lineNumbers,
  drawSelection,
  highlightSpecialChars,
  keymap,
} from "@codemirror/view";
import { defaultKeymap, history, historyKeymap } from "@codemirror/commands";
import { icuMode, icuTokenizer } from "./icu-mode.js";
import { lightTheme, darkTheme } from "./themes.js";

function isDark() {
  return document.documentElement.classList.contains("dark");
}

function themeExt() {
  return isDark() ? darkTheme : lightTheme;
}

// --- Shared observers (one for all instances) ---

var connected = new Set();

new MutationObserver(function (mutations) {
  for (var i = 0; i < mutations.length; i++) {
    if (mutations[i].attributeName === "class") {
      var ext = themeExt();
      var dark = isDark();
      for (var el of connected) el._applyTheme(ext, dark);
    }
    if (mutations[i].attributeName === "style") {
      // Font or font-size changed via CSS custom property on <html>.
      // CSS custom properties inherit into shadow DOM in theory,
      // but some browsers don't re-render; push values directly.
      var s = getComputedStyle(document.documentElement);
      var font = s.getPropertyValue("--font-editor").trim();
      var size = s.getPropertyValue("--font-size-editor").trim();
      for (var el of connected) el._applyFont(font, size);
    }
  }
}).observe(document.documentElement, {
  attributes: true,
  attributeFilter: ["class", "style"],
});

// --- Shared shadow-root stylesheet ---
// Layout values are copied from CM6's baseTheme (@codemirror/view)
// so that static previews are pixel-identical to real EditorViews.
// CM6 injects its own styles when an EditorView is created.

var SHADOW_CSS = [
  ":host{display:block}",

  // --- Static preview (matches CM6 baseTheme) ---
  ".cm-static{position:relative;box-sizing:border-box;display:flex;flex-direction:column}",
  ".cm-static .cm-scroller{display:flex;align-items:flex-start;",
  "font-family:var(--font-editor,monospace);font-size:var(--font-size-editor,14px);",
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

  // Light theme (default).
  ".cm-static{background:#f5f5f5;color:#202020}",
  ".cm-static .cm-gutters{background:#f5f5f5;color:#b0b0b0;border-right:none}",
  // Dark theme.
  '.cm-static[data-theme="dark"]{background:#151515;color:#e0e0e0}',
  '.cm-static[data-theme="dark"] .cm-gutters{background:#151515;color:#505050}',

  // Token colors (matches themes.js HighlightStyle).
  ".tok-keyword{color:#ac4142}",
  ".tok-atom,.tok-number{color:#aa759f}",
  ".tok-string{color:#f4bf75}",
  ".tok-variableName{color:#90a959}",
  ".tok-bracket{color:#202020}",
  ".tok-operator{color:#d28445}",
  '[data-theme="dark"] .tok-bracket{color:#e0e0e0}',

  // EditorView overrides (applied once CM6 is promoted).
  ".cm-editor .cm-scroller{font-family:var(--font-editor,monospace);",
  "font-size:var(--font-size-editor,14px);overflow:auto}",

  // Whitespace markers.
  ".cm-whitespace{position:relative}",
  '.cm-space::before{content:"\\00B7";position:absolute;inset:0;',
  "display:flex;align-items:center;justify-content:center;",
  "color:var(--border,oklch(0 0 0/0.2));pointer-events:none}",
  '.cm-nbsp::before{content:"\\2423";position:absolute;inset:0;',
  "display:flex;align-items:center;justify-content:center;",
  "color:var(--border,oklch(0 0 0/0.2));pointer-events:none}",

  // Editable hint.
  ":host(:not([readonly])) .cm-static{cursor:text}",
].join("\n");

// --- Static highlight rendering ---

var tokenClasses = {
  string: "tok-string",
  bracket: "tok-bracket",
  variableName: "tok-variableName",
  keyword: "tok-keyword",
  atom: "tok-atom",
  number: "tok-number",
  operator: "tok-operator",
};

/** Minimal stream interface for the ICU tokenizer. */
function SimpleStream(line) {
  this.string = line;
  this.pos = 0;
  this.start = 0;
}
SimpleStream.prototype = {
  eol: function () { return this.pos >= this.string.length; },
  peek: function () { return this.string.charAt(this.pos) || undefined; },
  next: function () {
    if (this.pos < this.string.length) return this.string.charAt(this.pos++);
  },
  eat: function (match) {
    var ch = this.string.charAt(this.pos);
    var ok;
    if (typeof match === "string") ok = ch === match;
    else if (match instanceof RegExp) ok = match.test(ch);
    else ok = ch && match(ch);
    if (ok) { this.pos++; return ch; }
  },
  eatWhile: function (match) {
    var start = this.pos;
    while (this.eat(match)) {}
    return this.pos > start;
  },
  eatSpace: function () {
    var start = this.pos;
    while (/\s/.test(this.string.charAt(this.pos))) this.pos++;
    return this.pos > start;
  },
  skipToEnd: function () { this.pos = this.string.length; },
  skipTo: function (ch) {
    var found = this.string.indexOf(ch, this.pos);
    if (found > -1) { this.pos = found; return true; }
  },
  match: function (pattern, consume) {
    if (typeof pattern === "string") {
      if (this.string.slice(this.pos, this.pos + pattern.length) === pattern) {
        if (consume !== false) this.pos += pattern.length;
        return true;
      }
      return null;
    }
    var m = this.string.slice(this.pos).match(pattern);
    if (m && m.index === 0) {
      if (consume !== false) this.pos += m[0].length;
      return m;
    }
    return null;
  },
  current: function () { return this.string.slice(this.start, this.pos); },
};

/**
 * Render static HTML that mirrors CM6's actual DOM structure:
 *   .cm-editor > .cm-scroller > (.cm-gutters > .cm-gutter.cm-lineNumbers > .cm-gutterElement) + .cm-content
 */
function renderStatic(text, dark) {
  var editor = document.createElement("div");
  editor.className = "cm-static";
  editor.setAttribute("data-theme", dark ? "dark" : "light");

  var scroller = document.createElement("div");
  scroller.className = "cm-scroller";

  // Gutters > gutter > elements (matches CM6's actual DOM).
  var gutters = document.createElement("div");
  gutters.className = "cm-gutters";
  gutters.setAttribute("aria-hidden", "true");
  var gutter = document.createElement("div");
  gutter.className = "cm-gutter cm-lineNumbers";
  gutters.appendChild(gutter);

  var content = document.createElement("div");
  content.className = "cm-content";
  content.setAttribute("role", "textbox");

  var lines = text ? text.split("\n") : [""];
  var state = icuTokenizer.startState();

  for (var i = 0; i < lines.length; i++) {
    var gutterEl = document.createElement("div");
    gutterEl.className = "cm-gutterElement";
    gutterEl.textContent = String(i + 1);
    gutter.appendChild(gutterEl);

    var lineEl = document.createElement("div");
    lineEl.className = "cm-line";

    var line = lines[i];
    if (line.length === 0) {
      lineEl.appendChild(document.createElement("br"));
    } else {
      var stream = new SimpleStream(line);
      while (!stream.eol()) {
        var start = stream.pos;
        var tokenType = icuTokenizer.token(stream, state);
        var value = line.slice(start, stream.pos);
        if (tokenType && tokenClasses[tokenType]) {
          var span = document.createElement("span");
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

// --- Web component ---

class TokiEditorElement extends HTMLElement {
  static observedAttributes = ["value"];

  constructor() {
    super();
    this._shadow = this.attachShadow({ mode: "open" });
    this._view = null;
    this._themeComp = null;
    this._syncing = false;
    this._dirty = false; // true while user has unsaved local edits
    this._staticDom = null;
  }

  connectedCallback() {
    var style = document.createElement("style");
    style.textContent = SHADOW_CSS;
    this._shadow.appendChild(style);

    var text = this.getAttribute("value") || "";

    // Render static preview (lightweight, no EditorView overhead).
    this._staticDom = renderStatic(text, isDark());
    this._shadow.appendChild(this._staticDom);

    // Editable editors promote to a real EditorView on click.
    if (!this.hasAttribute("readonly")) {
      var self = this;
      this._staticDom.addEventListener("click", function (e) {
        self._promote(e);
      }, { once: true });
    }

    connected.add(this);
  }

  disconnectedCallback() {
    connected.delete(this);
    if (this._view) {
      this._view.destroy();
      this._view = null;
    }
    this._themeComp = null;
    this._syncing = false;
    this._dirty = false;
    this._staticDom = null;
  }

  attributeChangedCallback(name, oldVal, newVal) {
    if (name !== "value" || this._syncing) return;
    var val = newVal || "";

    if (this._view) {
      // Don't overwrite the editor while the user has unsaved local edits.
      // This prevents syncEditorValues (which runs after every SSE morph)
      // from clobbering in-progress typing with a stale server value.
      if (this._dirty) return;
      var cur = this._view.state.doc.toString();
      if (cur !== val) {
        this._syncing = true;
        this._view.dispatch({
          changes: { from: 0, to: cur.length, insert: val },
        });
        this._syncing = false;
      }
    } else if (this._staticDom) {
      // Still static — re-render preview.
      var newDom = renderStatic(val, isDark());
      if (!this.hasAttribute("readonly")) {
        var self = this;
        newDom.addEventListener("click", function () {
          self._promote();
        }, { once: true });
      }
      this._staticDom.replaceWith(newDom);
      this._staticDom = newDom;
    }
  }

  get value() {
    if (this._view) return this._view.state.doc.toString();
    return this.getAttribute("value") || "";
  }

  set value(v) {
    this.setAttribute("value", v || "");
  }

  /** Replace static preview with a real CM6 EditorView. */
  _promote(clickEvent) {
    if (this._view) { this._view.focus(); return; }

    var text = this.getAttribute("value") || "";
    this._themeComp = new Compartment();

    var extensions = [
      icuMode,
      lineNumbers(),
      drawSelection(),
      highlightSpecialChars({ addSpecialChars: /[\u00a0]/g }),
      this._themeComp.of(themeExt()),
      EditorView.lineWrapping,
      history(),
      keymap.of([].concat(defaultKeymap, historyKeymap)),
    ];

    // Autosave listener.
    var self = this;
    var timer;
    extensions.push(
      EditorView.updateListener.of(function (update) {
        if (!update.docChanged || self._syncing) return;
        // User made a local edit — mark dirty to block incoming syncs.
        self._dirty = true;
        clearTimeout(timer);
        timer = setTimeout(function () {
          if (!self._view) return;
          // Read current doc (not the stale update object).
          var val = self._view.state.doc.toString();
          self._syncing = true;
          self.setAttribute("value", val);
          self._syncing = false;
          self.dispatchEvent(
            new CustomEvent("toki-change", {
              bubbles: true,
              detail: {
                value: val,
                tikid: self.dataset.tikid,
                locale: self.dataset.locale,
              },
            })
          );
          // Value sent to server — accept incoming syncs again.
          self._dirty = false;
        }, 200);
      })
    );

    var state = EditorState.create({ doc: text, extensions: extensions });
    this._view = new EditorView({ state: state, parent: this._shadow });

    if (this._staticDom) {
      this._staticDom.remove();
      this._staticDom = null;
    }

    // Place cursor at the clicked position instead of index 0.
    if (clickEvent) {
      var pos = this._view.posAtCoords({
        x: clickEvent.clientX,
        y: clickEvent.clientY,
      });
      if (pos != null) {
        this._view.dispatch({ selection: { anchor: pos } });
      }
    }
    this._view.focus();
  }

  _applyTheme(ext, dark) {
    if (this._view && this._themeComp) {
      this._view.dispatch({
        effects: this._themeComp.reconfigure(ext),
      });
    } else if (this._staticDom) {
      this._staticDom.setAttribute("data-theme", dark ? "dark" : "light");
    }
  }

  _applyFont(font, size) {
    var scroller;
    if (this._view) {
      scroller = this._view.dom.querySelector(".cm-scroller");
    } else if (this._staticDom) {
      scroller = this._staticDom.querySelector(".cm-scroller");
    }
    if (scroller) {
      scroller.style.fontFamily = font || "";
      scroller.style.fontSize = size || "";
    }
  }
}

customElements.define("toki-editor", TokiEditorElement);
