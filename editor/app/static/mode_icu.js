(function (mod) {
  if (typeof exports == "object" && typeof module == "object") mod(require("codemirror"));
  else if (typeof define == "function" && define.amd) define(["codemirror"], mod);
  else mod(CodeMirror);
})(function (CodeMirror) {
  "use strict";

  CodeMirror.defineMode("icu", function () {
    var typeRE = /^(plural|select|selectordinal|offset|number|date|time)\b/;
    var selectorRE = /^(?:=\d+|zero|one|two|few|many|other)\b/;
    var styleRE = /^(?:integer|currency|percent|scientific|short|medium|long|full)\b/;
    var numberRE = /^\d+/;
    var varRE = /^[A-Za-z_]\w*/;

    return {
      startState: function () {
        return {
          depth: 0,           // count of all “{”
          headerDepths: [],   // stack of depths where argument‐headers live
          headerVar: null,    // depth at which the next varRE is the arg-name
          inQuote: false      // are we inside a quoted literal?
        };
      },

      token: function (stream, state) {
        // 1) If inside a quoted literal, consume up to the closing quote
        if (state.inQuote) {
          if (stream.skipTo("'")) {
            stream.next();           // consume the closing '
            state.inQuote = false;
          } else {
            stream.skipToEnd();      // rest is literal
          }
          return "string";
        }

        // 2) Handle single-quote literals:
        if (stream.peek() === "'") {
          stream.next();
          if (stream.peek() === "'") {
            // it's a doubled quote => literal single-quote
            stream.next();
            return "string";
          }
          // start of a quoted literal
          state.inQuote = true;
          return "string";
        }

        // 3) Skip whitespace
        if (stream.eatSpace()) return null;
        var ch = stream.peek();

        // —— OPEN BRACE —— detect header (with comma) or simple {var}
        if (ch === "{") {
          if (stream.match(/^\{\s*[A-Za-z_]\w*\s*(?:,|\})/, false)) {
            stream.next();              // consume "{"
            state.depth++;
            state.headerDepths.push(state.depth);
            state.headerVar = state.depth;
            return "bracket";
          }
          // otherwise a plain brace (nested content)
          stream.next();
          state.depth++;
          return "bracket";
        }

        // —— CLOSE BRACE — pop header if it was one
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

        // current header context depth
        var topHeader = state.headerDepths[state.headerDepths.length - 1] || 0;

        // —— INSIDE ANY BRACES
        if (state.depth > 0) {
          // *** HEADER CONTEXT ***
          if (state.depth === topHeader) {
            // 1) the very first varRE after "{" or "{var}" is the arg name
            if (state.headerVar === state.depth) {
              if (stream.match(varRE)) {
                state.headerVar = null;
                return "variable";
              }
            }
            // 2) comma
            if (stream.match(",")) return "operator";
            // 3) type keywords
            if (stream.match(typeRE)) return "keyword";
            // 4) built-in selectors (=0, one, other…)
            if (stream.match(selectorRE)) return "atom";
            // 5) style names (integer, currency,…)
            if (stream.match(styleRE)) return "keyword";
            // 6) any other identifier followed by "{"  → an option name
            var id = stream.match(varRE, true);
            if (id) {
              var s = stream.string, p = stream.pos, i = p;
              while (i < s.length && /\s/.test(s.charAt(i))) i++;
              if (s.charAt(i) === "{") return "atom";
              // otherwise it's plain text
              return null;
            }
            // fallback consume one char
            stream.next();
            return null;
          }

          // *** MESSAGE CONTEXT (inside choice bodies) ***
          if (ch === "#") {
            stream.next();
            return "atom";
          }
          if (stream.match(numberRE)) return "number";
          stream.next();
          return null;
        }

        // —— OUTSIDE ANY BRACES
        stream.next();
        return null;
      }
    };
  });

  CodeMirror.defineMIME("text/x-icu", "icu");
});
