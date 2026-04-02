// ICU message syntax mode for CodeMirror 6, using StreamLanguage compatibility.
import { StreamLanguage } from "@codemirror/language";

var typeRE = /^(plural|select|selectordinal|offset|number|date|time)\b/;
var selectorRE = /^(?:=\d+|zero|one|two|few|many|other)\b/;
var styleRE = /^(?:integer|currency|percent|scientific|short|medium|long|full)\b/;
var numberRE = /^\d+/;
var varRE = /^[A-Za-z_]\w*/;

// Raw tokenizer — used both for StreamLanguage and static highlighting.
export const icuTokenizer = {
  startState() {
    return {
      depth: 0,
      headerDepths: [],
      headerVar: null,
      inQuote: false,
    };
  },

  token(stream, state) {
    if (state.inQuote) {
      if (stream.skipTo("'")) {
        stream.next();
        state.inQuote = false;
      } else {
        stream.skipToEnd();
      }
      return "string";
    }

    if (stream.peek() === "'") {
      stream.next();
      if (stream.peek() === "'") {
        stream.next();
        return "string";
      }
      state.inQuote = true;
      return "string";
    }

    if (stream.eatSpace()) return null;
    var ch = stream.peek();

    if (ch === "{") {
      if (stream.match(/^\{\s*[A-Za-z_]\w*\s*(?:,|\})/, false)) {
        stream.next();
        state.depth++;
        state.headerDepths.push(state.depth);
        state.headerVar = state.depth;
        return "bracket";
      }
      stream.next();
      state.depth++;
      return "bracket";
    }

    if (ch === "}") {
      stream.next();
      var hd = state.headerDepths;
      if (hd.length && hd[hd.length - 1] === state.depth) {
        hd.pop();
        if (state.headerVar === state.depth) state.headerVar = null;
      }
      state.depth--;
      return "bracket";
    }

    var topHeader = state.headerDepths[state.headerDepths.length - 1] || 0;

    if (state.depth > 0) {
      if (state.depth === topHeader) {
        if (state.headerVar === state.depth) {
          if (stream.match(varRE)) {
            state.headerVar = null;
            return "variableName";
          }
        }
        if (stream.match(",")) return "operator";
        if (stream.match(typeRE)) return "keyword";
        if (stream.match(selectorRE)) return "atom";
        if (stream.match(styleRE)) return "keyword";
        var id = stream.match(varRE, true);
        if (id) {
          var s = stream.string, p = stream.pos, i = p;
          while (i < s.length && /\s/.test(s.charAt(i))) i++;
          if (s.charAt(i) === "{") return "atom";
          return null;
        }
        stream.next();
        return null;
      }

      if (ch === "#") {
        stream.next();
        return "atom";
      }
      if (stream.match(numberRE)) return "number";
      stream.next();
      return null;
    }

    stream.next();
    return null;
  },
};

// CM6 StreamLanguage wrapper for use in EditorView.
export const icuMode = StreamLanguage.define(icuTokenizer);
