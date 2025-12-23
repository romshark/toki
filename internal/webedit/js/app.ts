import { EditorState, Compartment, Extension } from "@codemirror/state";
import { EditorView } from "@codemirror/view";
import { basicSetup } from "codemirror";
import { oneDark } from "@codemirror/theme-one-dark";

import { StreamLanguage } from "@codemirror/language";
import { icu } from "./icu-mode.js";

// Types

interface TextareaSync {
  flush: () => void;
  schedule: () => void;
}

interface ExtendedEditorView extends EditorView {
  __sync: TextareaSync;
}

interface ExtendedTextarea extends HTMLTextAreaElement {
  __cm?: EditorView;
}

type QueryValue = string | number | boolean | object | null | undefined;

// CodeMirror Setup

const icuLang = StreamLanguage.define(icu);

const prefersDark = window.matchMedia("(prefers-color-scheme: dark)");

function syncDarkClass(): void {
  document.documentElement.classList.toggle("dark", prefersDark.matches);
}

// Theme compartment lets us swap theme without recreating the editor.
const themeCompartment = new Compartment();

function currentThemeExtension(): Extension {
  // Light: use default styling (no extra theme extension)
  // Dark: use One Dark
  return prefersDark.matches ? oneDark : [];
}

// Track instances so we can re-theme all on system change.
const editors = new Set<EditorView>();

function makeTextareaSync(
  view: EditorView,
  ta: HTMLTextAreaElement
): TextareaSync {
  let t: ReturnType<typeof setTimeout> | null = null;

  function flush(): void {
    ta.value = view.state.doc.toString();
    ta.dispatchEvent(new Event("input", { bubbles: true }));
    ta.dispatchEvent(new Event("change", { bubbles: true }));
    t = null;
  }

  function schedule(): void {
    if (t != null) clearTimeout(t);
    t = setTimeout(flush, 100);
  }

  return { flush, schedule };
}

function initCodeMirror(
  el: HTMLElement | (() => HTMLElement | null) | null
): void {
  if (typeof el === "function") el = el();

  const ta = (
    el?.tagName === "TEXTAREA" ? el : el?.querySelector?.("textarea.editor")
  ) as ExtendedTextarea | null;

  if (!ta || ta.dataset.initialized) return;

  const readOnly = ta.dataset.infoReadonly === "true";

  // Create a mount node and hide the textarea (but keep it in the DOM for forms).
  const mount = document.createElement("div");
  mount.className = "cm-mount";
  ta.insertAdjacentElement("afterend", mount);
  ta.style.display = "none";

  const view = new EditorView({
    parent: mount,
    state: EditorState.create({
      doc: ta.value,
      extensions: [
        basicSetup,

        // Read-only: state-level (commands respect it) + view-level (DOM editability)
        EditorState.readOnly.of(readOnly),
        EditorView.editable.of(!readOnly),

        icuLang,

        // Theme (swappable)
        themeCompartment.of(currentThemeExtension()),

        // Save/sync behavior
        EditorView.updateListener.of((update) => {
          if (update.docChanged) (view as ExtendedEditorView).__sync.schedule();
        }),

        // Blur => flush immediately
        EditorView.domEventHandlers({
          blur: () => {
            (view as ExtendedEditorView).__sync.flush();
            return false;
          },
        }),
      ],
    }),
  }) as ExtendedEditorView;

  view.__sync = makeTextareaSync(view, ta);

  ta.dataset.initialized = "true";
  ta.__cm = view;

  editors.add(view);
}

// Query String Helpers

function queryRead<T extends QueryValue>(key: string, fallback: T): T {
  const val = new URLSearchParams(location.search).get(key);

  if (val === null) return fallback;
  if (val === "true") return true as T;
  if (val === "false") return false as T;
  if (val !== "" && !isNaN(Number(val))) return Number(val) as T;

  try {
    return JSON.parse(val) as T;
  } catch {
    return val as T;
  }
}

function queryWrite(params: Record<string, QueryValue>): void {
  const url = new URL(location.href);

  for (const [k, v] of Object.entries(params)) {
    if (v === "" || v == null) {
      url.searchParams.delete(k);
    } else {
      url.searchParams.set(
        k,
        typeof v === "object" ? JSON.stringify(v) : String(v)
      );
    }
  }

  history.replaceState(null, "", url);
}

// Initialization

window.addEventListener("DOMContentLoaded", () => {
  syncDarkClass();

  prefersDark.addEventListener("change", () => {
    syncDarkClass();
    const theme = currentThemeExtension();
    for (const view of editors) {
      view.dispatch({ effects: themeCompartment.reconfigure(theme) });
    }
  });
});

// Global Exports

declare global {
  interface Window {
    initCodeMirror: typeof initCodeMirror;
    queryRead: typeof queryRead;
    queryWrite: typeof queryWrite;
  }
}

window.initCodeMirror = initCodeMirror;
window.queryRead = queryRead;
window.queryWrite = queryWrite;
