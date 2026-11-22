package greatings

import "tokiexample/tokibundle"

func Casual(r tokibundle.Reader, name tokibundle.String) string {
	// Informal greeting shown to returning users on the home page.
	return r.String("Hey {name}, what's up?", name)
}

func Formal(r tokibundle.Reader, name tokibundle.String) string {
	// Polite greeting used on official pages and in formal correspondence.
	return r.String("Good day, {name}. Welcome.", name)
}

func Farewell(r tokibundle.Reader, name tokibundle.String) string {
	// Friendly goodbye shown when the user logs out or leaves a session.
	return r.String("See you later, {name}!", name)
}
