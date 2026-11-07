package maintenance

import "testing"

// IR64, IR640: the contact window carries no e-mail address and no phone number, however they are typed: with ASCII
// characters or with the full-width ones a Japanese input method produces.
func TestContactWindowOK(t *testing.T) {
	t.Parallel()

	for _, text := range []string{
		"Weekdays 09:00-18:00",
		"Weekdays 09:00–18:00",
		"へいじつ 9:00〜18:00",
		"ごごならだいじょうぶです",
		"１２３４５６", // six digits are a time or a count, not a number to call
	} {
		if !ContactWindowOK(text) {
			t.Errorf("%q is a contact window and was refused", text)
		}
	}

	for _, text := range []string{
		"call me at me@example.com",
		"090-1234-5678",
		"call +60 12-345 6789",
		"０９０１２３４５６７８",           // full-width digits
		"０９０－１２３４－５６７８",         // full-width digits and hyphens
		"でんわ ０９０　１２３４　５６７８",     // full-width digits and ideographic spaces
		"（０３）１２３４　５６７８",         // full-width parentheses
		"＋６０　１２－３４５　６７８９",       // full-width plus
		"me＠example.com",        // full-width at sign
		"٠١٢٣٤٥٦٧٨٩",            // Arabic-Indic digits
		"0 9 0 1 2 3 4 5 6 7 8", // spaces between every digit
	} {
		if ContactWindowOK(text) {
			t.Errorf("%q has a phone number or an e-mail address and was accepted", text)
		}
	}
}
