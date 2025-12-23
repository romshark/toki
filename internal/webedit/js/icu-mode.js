// static/icu-mode.js
export const icu = {
  startState() {
    return { inQuote: false, braceDepth: 0 };
  },
  copyState(s) {
    return { ...s };
  },
  token(stream, state) {
    // Quoted literal handling (basic)
    if (state.inQuote) {
      if (stream.match("''")) return "string"; // escaped '
      while (!stream.eol()) {
        const ch = stream.next();
        if (ch === "'") {
          state.inQuote = false;
          break;
        }
      }
      return "string";
    }

    const ch = stream.peek();

    if (ch === "'") {
      stream.next();
      state.inQuote = true;
      return "string";
    }

    if (ch === "{") {
      stream.next();
      state.braceDepth++;
      return "bracket";
    }

    if (ch === "}") {
      stream.next();
      state.braceDepth = Math.max(0, state.braceDepth - 1);
      return "bracket";
    }

    // Inside {...}
    if (state.braceDepth > 0) {
      if (stream.eatSpace()) return null;

      if (stream.match(/^(plural|select|selectordinal|number|date|time)\b/))
        return "keyword";
      if (stream.match(/^(other|zero|one|two|few|many)\b/)) return "atom";
      if (stream.match(/^=\d+/)) return "number";
      if (stream.match(/^\d+/)) return "number";
      if (stream.match(/^[A-Za-z_][\w-]*/)) return "variableName";

      const p = stream.next();
      if (p === "," || p === ":") return "punctuation";
      if (p === "#") return "operator";
      return null;
    }

    // Outside braces: consume until next special char
    while (!stream.eol()) {
      const p = stream.peek();
      if (p === "{" || p === "}" || p === "'") break;
      stream.next();
    }
    return null;
  },
};
