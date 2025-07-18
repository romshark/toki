// Generated by github.com/romshark/toki. DO NOT EDIT.
// Copyright - All Rights Reserved (example head)

package tokibundle

import (
	"fmt"
	"io"
	"time"

	locales "github.com/go-playground/locales"
	locale "github.com/go-playground/locales/en"
	language "golang.org/x/text/language"
)

// This prevents the "imported and not used" error when some features are not used.
var (
	_ fmt.Formatter = nil
	_ time.Time

	tr_en  = locale.New()
	loc_en = language.MustParse("en")
)

type catalog_en struct{}

var writers_en = map[string]func(w io.Writer, args ...any) (int, error){
	msg62489e1e07578e6e: func(w io.Writer, args ...any) (written int, err error) {
		var n int
		n, err = wrs(w, "Nothing found in folder ")
		if err != nil {
			return written, err
		}
		written += n
		{
			s, _ := sv(args[0])
			n, err = wrs(w, s)
		}
		if err != nil {
			return written, err
		}
		written += n
		return written, nil
	},
	msg6aa44c2f549ae5e8: func(w io.Writer, args ...any) (written int, err error) {
		return wrs(w, "translated text")
	},
	msgd2497314df5ae7e6: func(w io.Writer, args ...any) (written int, err error) {
		var n int
		n, err = wrs(w, "It was finished on ")
		if err != nil {
			return written, err
		}
		written += n
		n, err = io.WriteString(w, tr_en.FmtDateFull(args[0].(time.Time)))
		if err != nil {
			return written, err
		}
		written += n
		n, err = wrs(w, " at ")
		if err != nil {
			return written, err
		}
		written += n
		n, err = io.WriteString(w, tr_en.FmtTimeFull(args[1].(time.Time)))
		if err != nil {
			return written, err
		}
		written += n
		return written, nil
	},
	msgdc0a1830b671625c: func(w io.Writer, args ...any) (written int, err error) {
		var n int
		n, err = wrs(w, "searched ")
		if err != nil {
			return written, err
		}
		written += n
		switch pluralRuleCardinal(tr_en, args[0]) {
		default:
			n, err = fmt.Fprintf(w, "%v", args[0])
			if err != nil {
				return written, err
			}
			written += n
			n, err = wrs(w, " files")
			if err != nil {
				return written, err
			}
			written += n
		}
		n, err = wrs(w, " in ")
		if err != nil {
			return written, err
		}
		written += n
		switch pluralRuleCardinal(tr_en, args[1]) {
		default:
			n, err = fmt.Fprintf(w, "%v", args[1])
			if err != nil {
				return written, err
			}
			written += n
			n, err = wrs(w, " folders")
			if err != nil {
				return written, err
			}
			written += n
		}
		return written, nil
	},
	msgf5b4499f95971294: func(w io.Writer, args ...any) (written int, err error) {
		var n int
		switch pluralRuleCardinal(tr_en, args[0]) {
		default:
			n, err = fmt.Fprintf(w, "%v", args[0])
			if err != nil {
				return written, err
			}
			written += n
			n, err = wrs(w, " projects were")
			if err != nil {
				return written, err
			}
			written += n
		}
		n, err = wrs(w, " finished on ")
		if err != nil {
			return written, err
		}
		written += n
		n, err = io.WriteString(w, tr_en.FmtDateFull(args[1].(time.Time)))
		if err != nil {
			return written, err
		}
		written += n
		n, err = wrs(w, " at ")
		if err != nil {
			return written, err
		}
		written += n
		n, err = io.WriteString(w, tr_en.FmtTimeFull(args[2].(time.Time)))
		if err != nil {
			return written, err
		}
		written += n
		n, err = wrs(w, " by ")
		if err != nil {
			return written, err
		}
		written += n
		{
			s, _ := sv(args[3])
			n, err = wrs(w, s)
		}
		if err != nil {
			return written, err
		}
		written += n
		return written, nil
	},
	msgfb968a4dc3768ccd: func(w io.Writer, args ...any) (written int, err error) {
		return wrs(w, "Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut posuere tortor ex, at interdum lacus facilisis vel. In sed metus sit amet ex pellentesque consectetur quis in leo. Proin a tortor dolor. Duis sed sollicitudin diam. Vestibulum ante ipsum primis in faucibus orci luctus et ultrices posuere cubilia curae; Phasellus accumsan gravida lorem vel commodo. Nulla congue ligula leo, eget vulputate ipsum pharetra non. Etiam at ornare lacus, vel feugiat nulla. Nam sagittis, ligula ut ultrices hendrerit, ante erat convallis lectus, et blandit lectus lacus blandit velit. Etiam egestas, erat aliquet aliquam sodales, felis diam sagittis elit, a finibus augue libero eu mi. Sed libero arcu, aliquam sed nisi et, dignissim sollicitudin tortor. Nulla auctor ante quam, vitae placerat urna cursus sed. In hac habitasse platea dictumst. Nulla sagittis mauris et nulla dignissim mollis.\n\nInteger non lacus pulvinar, ultricies magna a, consequat risus. Cras at arcu ullamcorper, hendrerit urna at, blandit quam. Curabitur mi diam, tempor a augue ut, tincidunt aliquam arcu. Sed luctus sem id fringilla tempor. In sed tempor quam. Nam turpis nunc, placerat et risus nec, rutrum malesuada est. Phasellus lobortis vehicula lacus. Donec lacus tellus, mollis ut nibh ut, bibendum congue lectus. Aenean tincidunt, nunc id rutrum posuere, arcu eros pharetra ipsum, eu blandit elit nunc ut lacus. Fusce dui arcu, commodo sit amet felis quis, tempor tincidunt dolor. Nullam urna enim, convallis eget gravida vitae, vestibulum convallis augue. Ut vel risus hendrerit, euismod est eget, pulvinar lacus. Aliquam quis hendrerit lorem. Proin semper pretium nunc at tempus. Mauris ac lacus nisl. Etiam semper mauris finibus massa ultricies euismod.\n\nPraesent auctor aliquet sem auctor cursus. Phasellus vehicula, augue vel fringilla molestie, urna quam tincidunt mi, ac maximus ligula lectus sit amet nunc. Mauris sem justo, aliquam sed volutpat eget, aliquet sed orci. Phasellus aliquam neque vitae elit pulvinar, a faucibus urna fringilla. Duis faucibus tempus magna eget maximus. Nam tristique a ante et rutrum. Vestibulum id finibus tellus, eget aliquam arcu. In vel lacinia nibh, sit amet dignissim justo.\n\nCras gravida vulputate elit, sit amet facilisis est consectetur vitae. Aliquam erat volutpat. Aliquam in tortor in erat aliquet aliquet. Praesent at justo sed tortor pellentesque imperdiet. Phasellus enim dolor, consequat vel bibendum eu, laoreet interdum dui. Morbi sed vehicula lectus. Nam malesuada lorem consectetur luctus rutrum. Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Proin eget libero in odio vestibulum euismod. Pellentesque eget viverra ex, quis sagittis massa. Quisque consectetur, odio eget posuere dictum, lorem velit pellentesque purus, eget suscipit turpis orci a augue. Morbi et ante leo. Nam pulvinar arcu vel nulla efficitur pharetra. Nunc condimentum elementum metus vitae aliquet. Vestibulum molestie molestie leo, in vulputate nulla interdum sit amet. Nam porttitor dui enim.\n\nNulla eget sodales sem, dignissim elementum nulla. Sed sed elit feugiat, fringilla leo sit amet, vestibulum purus. Mauris suscipit felis id ante vestibulum, ut scelerisque felis faucibus. Cras tempus est non scelerisque rutrum. Vivamus augue libero, auctor eget mi sit amet, varius volutpat dolor. Praesent est tortor, gravida ut justo ut, pellentesque sollicitudin metus. Nullam scelerisque velit vitae suscipit elementum. Ut mattis diam et consectetur aliquet. Morbi consequat urna nisl. Duis dolor tellus, faucibus in hendrerit vitae, egestas vel ipsum. Sed sem lacus, rhoncus in purus ut, consequat fringilla tortor. Integer velit nisl, egestas blandit dui id, vulputate mollis velit. Nullam metus lectus, pharetra lobortis sem vel, accumsan mollis nisl.")
	},
}

func (catalog_en) Locale() language.Tag { return loc_en }

func (catalog_en) Translator() locales.Translator { return tr_en }

func (catalog_en) String(tik string, args ...any) string {
	b := poolBufGet()
	defer poolBufPut(b)
	f := writers_en[tik]
	if f == nil {
		_, _ = MissingTranslation(b, loc_en, tik, args...)
	} else {
		_, _ = f(b, args...)
	}
	return b.String()
}

func (catalog_en) Write(
	writer io.Writer, tik string, args ...any,
) (written int, err error) {
	f := writers_en[tik]
	if f == nil {
		return MissingTranslation(writer, loc_en, tik, args...)
	}
	return f(writer, args...)
}
