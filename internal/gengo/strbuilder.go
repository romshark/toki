package gengo

import (
	"strconv"

	"github.com/romshark/icumsg"
)

// writeFuncStrBuilder writes a translation function as a map entry.
func (w *Writer) writeFuncStrBuilder(id, tik, icuMsg string, tokens []icumsg.Token) {
	w.m = icuMsg
	w.t = tokens
	w.i = 0

	w.printf("// %s\n", id)
	w.printf("%q:\n", tik)
	w.printf("func(args ...any) string {\n")
	w.writeExprStrBuilder(len(w.t))
	w.println("},")
}

func (w *Writer) writeExprStrBuilder(endIndex int) {
	if s := w.literalConcat(endIndex); s != "" {
		w.println("_ = args")
		w.printf("return %q\n", s)
		return
	}

	w.println("var b strings.Builder;")
	for w.i < endIndex {
		t := w.t[w.i]
		switch t.Type {
		case icumsg.TokenTypeLiteral:
			w.printf("_, _ = b.WriteString(%q);\n", t.String(w.m, w.t))
			w.i++ // Advance.
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArgStrBuilder()
		case icumsg.TokenTypePlural, icumsg.TokenTypeSelectOrdinal:
			w.writePluralStrBuilder(false)
		case icumsg.TokenTypeArgTypeOrdinal:
			w.writePluralStrBuilder(true)
		case icumsg.TokenTypeSelect:
			w.writeSelectStrBuilder()
		default:
			// This should never happen since writeExpr is always
			// called on the above mentioned token types.
			panic(t.Type.String())
		}
	}
	w.println("return b.String();")
}

func (w *Writer) writeSimpleArgStrBuilder() {
	argNameToken := w.t[w.i+1].String(w.m, w.t)
	arg, err := parseArgName(argNameToken)
	if err != nil {
		// This should never happen because argument names are checked
		// before the Go bundle code is generated.
		panic(err)
	}
	if w.i+2 <= len(w.t) || !isTokenArgType(w.t[w.i+2].Type) {
		// No argument type.
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		w.i += 2
		return
	}

	tokStyle := w.t[w.i+3]
	if !isTokenArgStyle(tokStyle.Type) {
		// Argument type only.
		w.i += 3
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	}
	// Has argument type and style parameters.
	_ = tokStyle
	w.i += 4
	switch tokStyle.Type {
	case icumsg.TokenTypeArgStyleShort:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleMedium:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleLong:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleFull:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleInteger:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleCurrency:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStylePercent:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	case icumsg.TokenTypeArgStyleCustom:
		// TODO
		w.printf("_, _ = fmt.Fprintf(&b, %q, args[%d]);\n", `%v`, arg.Index)
		return
	default:
		// This should never happen because this switch is exhaustive.
		panic(tokStyle.Type.String())
	}
}

func (w *Writer) writePluralStrBuilder(ordinal bool) {
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
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	if iOne != 0 {
		w.println("case locales.PluralRuleOne:")
		w.i = iOne
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	if iTwo != 0 {
		w.println("case locales.PluralRuleTwo:")
		w.i = iTwo
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	if iFew != 0 {
		w.println("case locales.PluralRuleFew:")
		w.i = iFew
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	if iMany != 0 {
		w.println("case locales.PluralRuleMany:")
		w.i = iMany
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	if iOther != 0 {
		w.println("default:")
		w.i = iOther
		w.writePluralOptionStrBuilder(argIndex, offset)
	}
	w.print("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}

func (w *Writer) writePluralOptionStrBuilder(arg argName, offset uint64) {
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
							"_, _ = fmt.Fprintf(&b, %q, subtract(args[%d], %d));\n",
							"%v", arg.Index, offset,
						)
						continue
					} else {
						w.printf(
							"_, _ = fmt.Fprintf(&b, %q, args[%d]);\n",
							"%v", arg.Index,
						)
						continue
					}
				}
				w.printf("_, _ = b.WriteString(%q);\n", s)
			}
			w.i++
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArgStrBuilder()
		case icumsg.TokenTypePlural,
			icumsg.TokenTypeSelect,
			icumsg.TokenTypeSelectOrdinal:
			w.writeExprStrBuilder(indexEnd)
			w.i = t.IndexEnd + 1
		default:
			w.i++
		}
	}
}

func (w *Writer) writeSelectStrBuilder() {
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
			w.writeExprStrBuilder(w.t[i].IndexEnd)
			continue
		}
		// Named case.
		optionValStr := w.t[i+1].String(w.m, w.t)
		w.printf("case %q:\n", optionValStr)
		w.i = i + 2
		w.writeExprStrBuilder(w.t[i].IndexEnd)
	}
	w.print("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}
