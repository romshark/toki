package gengo

import (
	"strconv"

	"github.com/romshark/icumsg"
)

// writeFunc writes a translation function as a map entry.
func (w *Writer) writeFunc(id, icuMsg string, tokens []icumsg.Token) {
	w.m = icuMsg
	w.t = tokens
	w.i = 0

	w.printf("%s: func(w io.Writer, args ...any) (written int, err error) {\n", id)
	endIndex := len(w.t)
	if s := w.literalConcat(endIndex); s != "" {
		w.printf("return wrs(w, %q)\n", unescapeICULiteral(s))
	} else {
		w.println("var n int;")
		w.writeExpr(endIndex)
		w.println("return written, nil;")
	}
	w.println("},")
}

func (w *Writer) writeExpr(endIndex int) {
	if s := w.literalConcat(endIndex); s != "" {
		w.println("_ = args")
		w.printf("n, err = wrs(w, %q);\n", s)
		w.println("if err != nil {return written, err}; written += n;")
		return
	}

	for w.i < endIndex {
		t := w.t[w.i]
		switch t.Type {
		case icumsg.TokenTypeLiteral:
			w.printf("n, err = wrs(w, %q)\n", unescapeICULiteral(t.String(w.m, w.t)))
			w.println("if err != nil {return written, err}; written += n;")
			w.i++ // Advance.
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArg()
		case icumsg.TokenTypePlural, icumsg.TokenTypeSelectOrdinal:
			w.writePlural(false)
		case icumsg.TokenTypeArgTypeOrdinal:
			w.writePlural(true)
		case icumsg.TokenTypeSelect:
			w.writeSelect()
		default:
			// This should never happen since writeExpr is always
			// called on the above mentioned token types.
			panic(t.Type.String())
		}
	}
}

func (w *Writer) writeSimpleArg() {
	argNameToken := w.t[w.i+1].String(w.m, w.t)
	arg, err := parseArgName(argNameToken)
	if err != nil {
		// This should never happen because argument names are checked
		// before the Go bundle code is generated.
		panic(err)
	}
	if w.i+2 >= len(w.t) || !isTokenArgType(w.t[w.i+2].Type) {
		// No argument type.
		w.printf("{s, _ := sv(args[%d]); n, err = wrs(w, s)};\n", arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		w.i += 2
		return
	}

	switch w.t[w.i+2].Type {
	case icumsg.TokenTypeArgTypeDate:
		w.writeArgDate(arg.Index)
		return
	case icumsg.TokenTypeArgTypeTime:
		w.writeArgTime(arg.Index)
		return
	}

	tokStyle := w.t[w.i+3]
	if !isTokenArgStyle(tokStyle.Type) {
		// Argument type only.
		w.i += 3
		w.printf("n, err = wrs(w, args[%d].(string));\n", arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	}
	// Has argument type and style parameters.
	w.i += 4
	switch tokStyle.Type {
	case icumsg.TokenTypeArgStyleShort:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleMedium:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleLong:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleFull:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleInteger:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleCurrency:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStylePercent:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleCustom:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	case icumsg.TokenTypeArgStyleSkeleton:
		// TODO
		w.printf("n, err = fmt.Fprintf(w, %q, args[%d]);\n", `%v`, arg.Index)
		w.println("if err != nil {return written, err}; written += n;")
		return
	default:
		// This should never happen because this switch is exhaustive.
		panic(tokStyle.Type.String())
	}
}

func (w *Writer) writeArgDate(argIndex int) {
	tokStyle := w.t[w.i+3]
	w.i += 4
	switch tokStyle.Type {
	case icumsg.TokenTypeArgStyleFull:
		w.printf("n, err = io.WriteString(w, %s.FmtDateFull(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleLong:
		w.printf("n, err = io.WriteString(w, %s.FmtDateLong(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleMedium:
		w.printf("n, err = io.WriteString(w, %s.FmtDateMedium(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleShort:
		w.printf("n, err = io.WriteString(w, %s.FmtDateShort(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	default:
		// This should never happen because this switch is exhaustive.
		panic(tokStyle.Type.String())
	}
	w.println("if err != nil {return written, err}; written += n;")
}

func (w *Writer) writeArgTime(argIndex int) {
	tokStyle := w.t[w.i+3]
	w.i += 4
	switch tokStyle.Type {
	case icumsg.TokenTypeArgStyleFull:
		w.printf("n, err = io.WriteString(w, %s.FmtTimeFull(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleLong:
		w.printf("n, err = io.WriteString(w, %s.FmtTimeLong(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleMedium:
		w.printf("n, err = io.WriteString(w, %s.FmtTimeMedium(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	case icumsg.TokenTypeArgStyleShort:
		w.printf("n, err = io.WriteString(w, %s.FmtTimeShort(args[%d].(time.Time)));",
			w.translatorVar, argIndex)
	default:
		// This should never happen because this switch is exhaustive.
		panic(tokStyle.Type.String())
	}
	w.println("if err != nil {return written, err}; written += n;")
}

func (w *Writer) writePlural(ordinal bool) {
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
		w.writePluralOption(argIndex, offset)
	}
	if iOne != 0 {
		w.println("case locales.PluralRuleOne:")
		w.i = iOne
		w.writePluralOption(argIndex, offset)
	}
	if iTwo != 0 {
		w.println("case locales.PluralRuleTwo:")
		w.i = iTwo
		w.writePluralOption(argIndex, offset)
	}
	if iFew != 0 {
		w.println("case locales.PluralRuleFew:")
		w.i = iFew
		w.writePluralOption(argIndex, offset)
	}
	if iMany != 0 {
		w.println("case locales.PluralRuleMany:")
		w.i = iMany
		w.writePluralOption(argIndex, offset)
	}
	if iOther != 0 {
		w.println("default:")
		w.i = iOther
		w.writePluralOption(argIndex, offset)
	}
	w.println("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}

func (w *Writer) writePluralOption(arg argName, offset uint64) {
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
							"n, err = fmt.Fprintf(w, %q, subtract(args[%d], %d));\n",
							"%v", arg.Index, offset,
						)
						w.println("if err != nil {return written, err}; written += n;")
						continue
					} else {
						w.printf(
							"n, err = fmt.Fprintf(w, %q, args[%d]);\n",
							"%v", arg.Index,
						)
						w.println("if err != nil {return written, err}; written += n;")
						continue
					}
				}
				w.printf("n, err = wrs(w, %q);\n", unescapeICULiteral(s))
				w.println("if err != nil {return written, err}; written += n;")
			}
			w.i++
		case icumsg.TokenTypeSimpleArg:
			w.writeSimpleArg()
		case icumsg.TokenTypePlural,
			icumsg.TokenTypeSelect,
			icumsg.TokenTypeSelectOrdinal:
			w.writeExpr(indexEnd)
			w.i = t.IndexEnd + 1
		default:
			w.i++
		}
	}
}

func (w *Writer) writeSelect() {
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
			w.writeExpr(w.t[i].IndexEnd)
			continue
		}
		// Named case.
		optionValStr := w.t[i+1].String(w.m, w.t)
		w.printf("case %q:\n", optionValStr)
		w.i = i + 2
		w.writeExpr(w.t[i].IndexEnd)
	}
	w.println("};")
	w.i = t.IndexEnd + 1 // Skip the whole block.
}
