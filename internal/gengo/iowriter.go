package gengo

import (
	"strconv"

	"github.com/romshark/icumsg"
)

// writeFuncIOWriter writes a translation function as a map entry.
func (w *Writer) writeFuncIOWriter(id, tik, icuMsg string, tokens []icumsg.Token) {
	w.m = icuMsg
	w.t = tokens
	w.i = 0

	w.printf("// %s\n", id)
	w.printf("%q:\n", tik)
	w.println("func(w io.Writer, args ...any) (written int, err error) {")
	w.println("var wr int;")
	w.writeExprIOWriter(len(w.t))
	w.println("return written, nil;")
	w.println("},")
}

func (w *Writer) writeExprIOWriter(endIndex int) {
	if s := w.literalConcat(endIndex); s != "" {
		w.println("_ = args")
		w.printf("wr, err = w.Write([]byte(%q));\n", s)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	}

	for w.i < endIndex {
		t := w.t[w.i]
		switch t.Type {
		case icumsg.TokenTypeLiteral:
			w.printf("wr, err = w.Write([]byte(%q))\n", t.String(w.m, w.t))
			w.println("if err != nil {return written, err}; written += wr;")
			w.i++ // Advance.
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArgIOWriter()
		case icumsg.TokenTypePlural, icumsg.TokenTypeSelectOrdinal:
			w.writePluralIOWriter(false)
		case icumsg.TokenTypeArgTypeOrdinal:
			w.writePluralIOWriter(true)
		case icumsg.TokenTypeSelect:
			w.writeSelectIOWriter()
		default:
			// This should never happen since writeExpr is always
			// called on the above mentioned token types.
			panic(t.Type.String())
		}
	}
}

func (w *Writer) writeSimpleArgIOWriter() {
	argNameToken := w.t[w.i+1].String(w.m, w.t)
	arg, err := parseArgName(argNameToken)
	if err != nil {
		// This should never happen because argument names are checked
		// before the Go bundle code is generated.
		panic(err)
	}
	if w.i+2 <= len(w.t) || !isTokenArgType(w.t[w.i+2].Type) {
		// No argument type.
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		w.i += 2
		return
	}

	tokStyle := w.t[w.i+3]
	if !isTokenArgStyle(tokStyle.Type) {
		// Argument type only.
		w.i += 3
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	}
	// Has argument type and style parameters.
	_ = tokStyle
	w.i += 4
	switch tokStyle.Type {
	case icumsg.TokenTypeArgStyleShort:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleMedium:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleLong:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleFull:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleInteger:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleCurrency:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStylePercent:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	case icumsg.TokenTypeArgStyleCustom:
		// TODO
		w.printf("wr, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += wr;")
		return
	default:
		// This should never happen because this switch is exhaustive.
		panic(tokStyle.Type.String())
	}
}

func (w *Writer) writePluralIOWriter(ordinal bool) {
	t := w.t[w.i]
	argNameToken := w.t[w.i+1].String(w.m, w.t)
	argIndex, err := parseArgName(argNameToken)
	if err != nil {
		panic(err)
	}

	offset := uint64(0)
	if ot := w.t[w.i+2]; ot.Type == icumsg.TokenTypePluralOffset {
		// Has offset parameter.
		s := ot.String(w.m, w.t)
		if offset, err = strconv.ParseUint(s, 10, 64); err != nil {
			// This should never happen because the offset number
			// is validated when the ICU message is parsed.
			panic(err)
		}
	}

	getterFunc := "pluralRuleCardinal"
	if ordinal {
		getterFunc = "pluralRuleOrdinal"
	}

	var iOther, iZero, iOne, iTwo, iFew, iMany int
	for i := range icumsg.Options(w.t, w.i) {
		switch w.t[i].Type {
		case icumsg.TokenTypeOptionZero:
			iZero = i
		case icumsg.TokenTypeOptionOne:
			iOne = i
		case icumsg.TokenTypeOptionTwo:
			iTwo = i
		case icumsg.TokenTypeOptionFew:
			iFew = i
		case icumsg.TokenTypeOptionMany:
			iMany = i
		case icumsg.TokenTypeOptionOther:
			iOther = i
		}
	}

	w.printf("switch %s(%s, args[%d]) {\n",
		getterFunc, w.translatorVar, argIndex.Index)
	if iZero != 0 {
		w.println("case locales.PluralRuleZero:")
		w.i = iZero
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	if iOne != 0 {
		w.println("case locales.PluralRuleOne:")
		w.i = iOne
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	if iTwo != 0 {
		w.println("case locales.PluralRuleTwo:")
		w.i = iTwo
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	if iFew != 0 {
		w.println("case locales.PluralRuleFew:")
		w.i = iFew
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	if iMany != 0 {
		w.println("case locales.PluralRuleMany:")
		w.i = iMany
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	if iOther != 0 {
		w.println("default:")
		w.i = iOther
		w.writePluralOptionIOWriter(argIndex, offset)
	}
	w.println("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}

func (w *Writer) writePluralOptionIOWriter(arg argName, offset uint64) {
	indexEnd := w.t[w.i].IndexEnd
	defer func() { w.i = indexEnd + 1 }()
	w.i++
	for w.i < indexEnd {
		t := w.t[w.i]
		switch t.Type {
		case icumsg.TokenTypeLiteral:
			for s := range iterPluralLiteralParts(t.String(w.m, w.t)) {
				if s == "#" {
					if offset != 0 {
						w.printf(
							"wr, err = fmt.Fprintf(w, %q, subtract(args[%d], %d));\n",
							"%v", arg.Index, offset,
						)
						w.println("if err != nil {return written, err}; written += wr;")
						continue
					} else {
						w.printf(
							"wr, err = fmt.Fprintf(w, %q, args[%d]);\n",
							"%v", arg.Index,
						)
						w.println("if err != nil {return written, err}; written += wr;")
						continue
					}
				}
				w.printf("wr, err = w.Write([]byte(%q));\n", s)
				w.println("if err != nil {return written, err}; written += wr;")
			}
			w.i++
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArgIOWriter()
		case icumsg.TokenTypePlural,
			icumsg.TokenTypeSelect,
			icumsg.TokenTypeSelectOrdinal:
			w.writeExprIOWriter(indexEnd)
			w.i = t.IndexEnd + 1
		default:
			w.i++
		}
	}
}

func (w *Writer) writeSelectIOWriter() {
	t := w.t[w.i]
	argNameToken := w.t[w.i+1].String(w.m, w.t)
	argIndex, err := parseArgName(argNameToken)
	if err != nil {
		panic(err)
	}

	w.printf("switch args[%d].(string) {\n", argIndex.Index)
	for i := range icumsg.Options(w.t, w.i) {
		if w.t[i].Type == icumsg.TokenTypeOptionOther {
			// Default case.
			w.println("default:")
			w.i = i + 1
			w.writeExprIOWriter(w.t[i].IndexEnd)
			continue
		}
		// Named case.
		optionValStr := w.t[i+1].String(w.m, w.t)
		w.printf("case %q:\n", optionValStr)
		w.i = i + 2
		w.writeExprIOWriter(w.t[i].IndexEnd)
	}
	w.println("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}
