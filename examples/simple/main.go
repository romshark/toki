package main

import (
	"flag"
	"fmt"
	"time"

	"tokiexample/tokibundle"

	"golang.org/x/text/language"
)

func main() {
	fLocale := flag.String("l", "en", "i18n locale")
	flag.Parse()
	locale := language.MustParse(*fLocale)

	fmt.Println("Supported locales:", tokibundle.Locales())

	// Get a localized reader for the locale provided.
	// Toki will automatically select the most appropriate available translation catalog.
	reader, conf := tokibundle.Match(locale)
	fmt.Println("Selected", reader.Locale(), "with confidence:", conf)

	fmt.Println(reader.String("translated text"))
	fmt.Println(reader.String("Nothing found in folder {text}", "images"))

	fmt.Println(reader.String("It was finished on {date-full} at {time-full}",
		time.Now(), time.Now()))

	fmt.Println(reader.String("{# projects were} finished on {date-full} at {time-full} by {name}",
		4, time.Now(), time.Now(), tokibundle.String{Value: "Rafael", Gender: tokibundle.GenderMale}))

	fmt.Println(reader.String("searched {# files} in {# folders}", 56, 21))

	//  Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut posuere tortor ex,
	// at interdum lacus facilisis vel. In sed metus sit amet ex pellentesque consectetur
	// quis in leo.
	// Proin a tortor dolor. Duis sed sollicitudin diam. Vestibulum ante ipsum primis in
	// faucibus orci luctus et ultrices posuere cubilia curae; Phasellus accumsan gravida
	// lorem vel commodo. Nulla congue ligula leo, eget vulputate ipsum pharetra non.
	// Etiam at ornare lacus, vel feugiat nulla. Nam sagittis,
	// ligula ut ultrices hendrerit, ante erat convallis lectus,
	// et blandit lectus lacus blandit velit. Etiam egestas,
	// erat aliquet aliquam sodales, felis diam sagittis elit,
	// a finibus augue libero eu mi. Sed libero arcu,
	// aliquam sed nisi et, dignissim sollicitudin tortor.
	//
	// Nulla auctor ante quam, vitae placerat urna cursus sed.
	// In hac habitasse platea dictumst. Nulla sagittis mauris et nulla dignissim mollis.
	_ = reader.String(`Lorem ipsum dolor sit amet, consectetur adipiscing elit. Ut posuere tortor ex, at interdum lacus facilisis vel. In sed metus sit amet ex pellentesque consectetur quis in leo. Proin a tortor dolor. Duis sed sollicitudin diam. Vestibulum ante ipsum primis in faucibus orci luctus et ultrices posuere cubilia curae; Phasellus accumsan gravida lorem vel commodo. Nulla congue ligula leo, eget vulputate ipsum pharetra non. Etiam at ornare lacus, vel feugiat nulla. Nam sagittis, ligula ut ultrices hendrerit, ante erat convallis lectus, et blandit lectus lacus blandit velit. Etiam egestas, erat aliquet aliquam sodales, felis diam sagittis elit, a finibus augue libero eu mi. Sed libero arcu, aliquam sed nisi et, dignissim sollicitudin tortor. Nulla auctor ante quam, vitae placerat urna cursus sed. In hac habitasse platea dictumst. Nulla sagittis mauris et nulla dignissim mollis.

Integer non lacus pulvinar, ultricies magna a, consequat risus. Cras at arcu ullamcorper, hendrerit urna at, blandit quam. Curabitur mi diam, tempor a augue ut, tincidunt aliquam arcu. Sed luctus sem id fringilla tempor. In sed tempor quam. Nam turpis nunc, placerat et risus nec, rutrum malesuada est. Phasellus lobortis vehicula lacus. Donec lacus tellus, mollis ut nibh ut, bibendum congue lectus. Aenean tincidunt, nunc id rutrum posuere, arcu eros pharetra ipsum, eu blandit elit nunc ut lacus. Fusce dui arcu, commodo sit amet felis quis, tempor tincidunt dolor. Nullam urna enim, convallis eget gravida vitae, vestibulum convallis augue. Ut vel risus hendrerit, euismod est eget, pulvinar lacus. Aliquam quis hendrerit lorem. Proin semper pretium nunc at tempus. Mauris ac lacus nisl. Etiam semper mauris finibus massa ultricies euismod.

Praesent auctor aliquet sem auctor cursus. Phasellus vehicula, augue vel fringilla molestie, urna quam tincidunt mi, ac maximus ligula lectus sit amet nunc. Mauris sem justo, aliquam sed volutpat eget, aliquet sed orci. Phasellus aliquam neque vitae elit pulvinar, a faucibus urna fringilla. Duis faucibus tempus magna eget maximus. Nam tristique a ante et rutrum. Vestibulum id finibus tellus, eget aliquam arcu. In vel lacinia nibh, sit amet dignissim justo.

Cras gravida vulputate elit, sit amet facilisis est consectetur vitae. Aliquam erat volutpat. Aliquam in tortor in erat aliquet aliquet. Praesent at justo sed tortor pellentesque imperdiet. Phasellus enim dolor, consequat vel bibendum eu, laoreet interdum dui. Morbi sed vehicula lectus. Nam malesuada lorem consectetur luctus rutrum. Pellentesque habitant morbi tristique senectus et netus et malesuada fames ac turpis egestas. Proin eget libero in odio vestibulum euismod. Pellentesque eget viverra ex, quis sagittis massa. Quisque consectetur, odio eget posuere dictum, lorem velit pellentesque purus, eget suscipit turpis orci a augue. Morbi et ante leo. Nam pulvinar arcu vel nulla efficitur pharetra. Nunc condimentum elementum metus vitae aliquet. Vestibulum molestie molestie leo, in vulputate nulla interdum sit amet. Nam porttitor dui enim.

Nulla eget sodales sem, dignissim elementum nulla. Sed sed elit feugiat, fringilla leo sit amet, vestibulum purus. Mauris suscipit felis id ante vestibulum, ut scelerisque felis faucibus. Cras tempus est non scelerisque rutrum. Vivamus augue libero, auctor eget mi sit amet, varius volutpat dolor. Praesent est tortor, gravida ut justo ut, pellentesque sollicitudin metus. Nullam scelerisque velit vitae suscipit elementum. Ut mattis diam et consectetur aliquet. Morbi consequat urna nisl. Duis dolor tellus, faucibus in hendrerit vitae, egestas vel ipsum. Sed sem lacus, rhoncus in purus ut, consequat fringilla tortor. Integer velit nisl, egestas blandit dui id, vulputate mollis velit. Nullam metus lectus, pharetra lobortis sem vel, accumsan mollis nisl.`)
}
