package main

// Port of org.apache.lucene.analysis.el.GreekStemmer (Lucene 10.5.1), the stemmer behind
// GreekStemFilter. Input is expected to be the output of GreekLowerCaseFilter (lowercase, no
// tonos/dialytika, final sigma folded to σ). The rules are translated literally, including the
// operator precedence quirks of rule0/rule8 and the in-place buffer rewrites.

// greekStemLucene ports GreekStemFilter applied to one (non-keyword) token.
func greekStemLucene(term string) string {
	s := []rune(term)
	n := mscGreekStem(s, len(s))
	return string(s[:n])
}

func mscSet(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

func mscIn(set map[string]bool, s []rune, n int) bool { return set[string(s[:n])] }

func mscGreekVowel(s []rune, n int) bool {
	if n == 0 {
		return false
	}
	switch s[n-1] {
	case 'α', 'ε', 'η', 'ι', 'ο', 'υ', 'ω':
		return true
	}
	return false
}

func mscGreekVowelNoY(s []rune, n int) bool {
	if n == 0 {
		return false
	}
	switch s[n-1] {
	case 'α', 'ε', 'η', 'ι', 'ο', 'ω':
		return true
	}
	return false
}

func mscGreekEndsAny(s []rune, n int, suffixes ...string) bool {
	for _, x := range suffixes {
		if mscEndsWith(s, n, x) {
			return true
		}
	}
	return false
}

func mscGreekStem(s []rune, n int) int {
	if n < 4 {
		return n
	}
	orig := n
	n = mscGreekRule0(s, n)
	n = mscGreekRule1(s, n)
	n = mscGreekRule2(s, n)
	n = mscGreekRule3(s, n)
	n = mscGreekRule4(s, n)
	n = mscGreekRule5(s, n)
	n = mscGreekRule6(s, n)
	n = mscGreekRule7(s, n)
	n = mscGreekRule8(s, n)
	n = mscGreekRule9(s, n)
	n = mscGreekRule10(s, n)
	n = mscGreekRule11(s, n)
	n = mscGreekRule12(s, n)
	n = mscGreekRule13(s, n)
	n = mscGreekRule14(s, n)
	n = mscGreekRule15(s, n)
	n = mscGreekRule16(s, n)
	n = mscGreekRule17(s, n)
	n = mscGreekRule18(s, n)
	n = mscGreekRule19(s, n)
	n = mscGreekRule20(s, n)
	if n == orig {
		n = mscGreekRule21(s, n)
	}
	return mscGreekRule22(s, n)
}

func mscGreekRule0(s []rune, n int) int {
	e := func(x string) bool { return mscEndsWith(s, n, x) }
	if n > 9 && (e("καθεστωτοσ") || e("καθεστωτων")) {
		return n - 4
	}
	if n > 8 && (e("γεγονοτοσ") || e("γεγονοτων")) {
		return n - 4
	}
	if n > 8 && e("καθεστωτα") {
		return n - 3
	}
	if n > 7 && (e("τατογιου") || e("τατογιων")) {
		return n - 4
	}
	if n > 7 && e("γεγονοτα") {
		return n - 3
	}
	if n > 7 && e("καθεστωσ") {
		return n - 2
	}
	if n > 6 && (e("σκαγιου")) || e("σκαγιων") || e("ολογιου") || e("ολογιων") || e("κρεατοσ") ||
		e("κρεατων") || e("περατοσ") || e("περατων") || e("τερατοσ") || e("τερατων") {
		return n - 4
	}
	if n > 6 && e("τατογια") {
		return n - 3
	}
	if n > 6 && e("γεγονοσ") {
		return n - 2
	}
	if n > 5 && (e("φαγιου") || e("φαγιων") || e("σογιου") || e("σογιων")) {
		return n - 4
	}
	if n > 5 && (e("σκαγια") || e("ολογια") || e("κρεατα") || e("περατα") || e("τερατα")) {
		return n - 3
	}
	if n > 4 && (e("φαγια") || e("σογια") || e("φωτοσ") || e("φωτων")) {
		return n - 3
	}
	if n > 4 && (e("κρεασ") || e("περασ") || e("τερασ")) {
		return n - 2
	}
	if n > 3 && e("φωτα") {
		return n - 2
	}
	if n > 2 && e("φωσ") {
		return n - 1
	}
	return n
}

func mscGreekRule1(s []rune, n int) int {
	if n > 4 && (mscEndsWith(s, n, "αδεσ") || mscEndsWith(s, n, "αδων")) {
		n -= 4
		if !mscGreekEndsAny(s, n, "οκ", "μαμ", "μαν", "μπαμπ", "πατερ", "γιαγι", "νταντ", "κυρ", "θει", "πεθερ") {
			n += 2
		}
	}
	return n
}

func mscGreekRule2(s []rune, n int) int {
	if n > 4 && (mscEndsWith(s, n, "εδεσ") || mscEndsWith(s, n, "εδων")) {
		n -= 4
		if mscGreekEndsAny(s, n, "οπ", "ιπ", "εμπ", "υπ", "γηπ", "δαπ", "κρασπ", "μιλ") {
			n += 2
		}
	}
	return n
}

func mscGreekRule3(s []rune, n int) int {
	if n > 5 && (mscEndsWith(s, n, "ουδεσ") || mscEndsWith(s, n, "ουδων")) {
		n -= 5
		if mscGreekEndsAny(s, n, "αρκ", "καλιακ", "πεταλ", "λιχ", "πλεξ", "σκ", "σ", "φλ", "φρ", "βελ",
			"λουλ", "χν", "σπ", "τραγ", "φε") {
			n += 3
		}
	}
	return n
}

var mscGreekExc4 = mscSet("θ", "δ", "ελ", "γαλ", "ν", "π", "ιδ", "παρ")

func mscGreekRule4(s []rune, n int) int {
	if n > 3 && (mscEndsWith(s, n, "εωσ") || mscEndsWith(s, n, "εων")) {
		n -= 3
		if mscIn(mscGreekExc4, s, n) {
			n++
		}
	}
	return n
}

func mscGreekRule5(s []rune, n int) int {
	if n > 2 && mscEndsWith(s, n, "ια") {
		n -= 2
		if mscGreekVowel(s, n) {
			n++
		}
	} else if n > 3 && (mscEndsWith(s, n, "ιου") || mscEndsWith(s, n, "ιων")) {
		n -= 3
		if mscGreekVowel(s, n) {
			n++
		}
	}
	return n
}

var mscGreekExc6 = mscSet(
	"αλ", "αδ", "ενδ", "αμαν", "αμμοχαλ", "ηθ", "ανηθ", "αντιδ", "φυσ", "βρωμ", "γερ",
	"εξωδ", "καλπ", "καλλιν", "καταδ", "μουλ", "μπαν", "μπαγιατ", "μπολ", "μποσ", "νιτ",
	"ξικ", "συνομηλ", "πετσ", "πιτσ", "πικαντ", "πλιατσ", "ποστελν", "πρωτοδ", "σερτ",
	"συναδ", "τσαμ", "υποδ", "φιλον", "φυλοδ", "χασ")

func mscGreekRule6(s []rune, n int) int {
	removed := false
	if n > 3 && (mscEndsWith(s, n, "ικα") || mscEndsWith(s, n, "ικο")) {
		n -= 3
		removed = true
	} else if n > 4 && (mscEndsWith(s, n, "ικου") || mscEndsWith(s, n, "ικων")) {
		n -= 4
		removed = true
	}
	if removed {
		if mscGreekVowel(s, n) || mscIn(mscGreekExc6, s, n) {
			n += 2
		}
	}
	return n
}

var mscGreekExc7 = mscSet("αναπ", "αποθ", "αποκ", "αποστ", "βουβ", "ξεθ", "ουλ", "πεθ", "πικρ", "ποτ", "σιχ", "χ")

func mscGreekRule7(s []rune, n int) int {
	if n == 5 && mscEndsWith(s, n, "αγαμε") {
		return n - 1
	}
	if n > 7 && mscEndsWith(s, n, "ηθηκαμε") {
		n -= 7
	} else if n > 6 && mscEndsWith(s, n, "ουσαμε") {
		n -= 6
	} else if n > 5 && (mscEndsWith(s, n, "αγαμε") || mscEndsWith(s, n, "ησαμε") || mscEndsWith(s, n, "ηκαμε")) {
		n -= 5
	}
	if n > 3 && mscEndsWith(s, n, "αμε") {
		n -= 3
		if mscIn(mscGreekExc7, s, n) {
			n += 2
		}
	}
	return n
}

var mscGreekExc8a = mscSet("τρ", "τσ")

var mscGreekExc8b = mscSet(
	"βετερ", "βουλκ", "βραχμ", "γ", "δραδουμ", "θ", "καλπουζ", "καστελ", "κορμορ", "λαοπλ",
	"μωαμεθ", "μ", "μουσουλμ", "ν", "ουλ", "π", "πελεκ", "πλ", "πολισ", "πορτολ", "σαρακατσ",
	"σουλτ", "τσαρλατ", "ορφ", "τσιγγ", "τσοπ", "φωτοστεφ", "χ", "ψυχοπλ", "αγ", "ορφ", "γαλ",
	"γερ", "δεκ", "διπλ", "αμερικαν", "ουρ", "πιθ", "πουριτ", "σ", "ζωντ", "ικ", "καστ", "κοπ",
	"λιχ", "λουθηρ", "μαιντ", "μελ", "σιγ", "σπ", "στεγ", "τραγ", "τσαγ", "φ", "ερ", "αδαπ",
	"αθιγγ", "αμηχ", "ανικ", "ανοργ", "απηγ", "απιθ", "ατσιγγ", "βασ", "βασκ", "βαθυγαλ",
	"βιομηχ", "βραχυκ", "διατ", "διαφ", "ενοργ", "θυσ", "καπνοβιομηχ", "καταγαλ", "κλιβ",
	"κοιλαρφ", "λιβ", "μεγλοβιομηχ", "μικροβιομηχ", "νταβ", "ξηροκλιβ", "ολιγοδαμ", "ολογαλ",
	"πενταρφ", "περηφ", "περιτρ", "πλατ", "πολυδαπ", "πολυμηχ", "στεφ", "ταβ", "τετ", "υπερηφ",
	"υποκοπ", "χαμηλοδαπ", "ψηλοταβ")

func mscGreekRule8(s []rune, n int) int {
	e := func(x string) bool { return mscEndsWith(s, n, x) }
	removed := false
	if n > 8 && e("ιουντανε") {
		n -= 8
		removed = true
	} else if n > 7 && e("ιοντανε") || e("ουντανε") || e("ηθηκανε") {
		n -= 7
		removed = true
	} else if n > 6 && e("ιοτανε") || e("οντανε") || e("ουσανε") {
		n -= 6
		removed = true
	} else if n > 5 && e("αγανε") || e("ησανε") || e("οτανε") || e("ηκανε") {
		n -= 5
		removed = true
	}
	if removed && mscIn(mscGreekExc8a, s, n) {
		n += 4
		s[n-4] = 'α'
		s[n-3] = 'γ'
		s[n-2] = 'α'
		s[n-1] = 'ν'
	}
	if n > 3 && mscEndsWith(s, n, "ανε") {
		n -= 3
		if mscGreekVowelNoY(s, n) || mscIn(mscGreekExc8b, s, n) {
			n += 2
		}
	}
	return n
}

var mscGreekExc9 = mscSet(
	"αβαρ", "βεν", "εναρ", "αβρ", "αδ", "αθ", "αν", "απλ", "βαρον", "ντρ", "σκ", "κοπ",
	"μπορ", "νιφ", "παγ", "παρακαλ", "σερπ", "σκελ", "συρφ", "τοκ", "υ", "δ", "εμ",
	"θαρρ", "θ")

func mscGreekRule9(s []rune, n int) int {
	if n > 5 && mscEndsWith(s, n, "ησετε") {
		n -= 5
	}
	if n > 3 && mscEndsWith(s, n, "ετε") {
		n -= 3
		if mscIn(mscGreekExc9, s, n) || mscGreekVowelNoY(s, n) ||
			mscGreekEndsAny(s, n, "οδ", "αιρ", "φορ", "ταθ", "διαθ", "σχ", "ενδ", "ευρ", "τιθ", "υπερθ",
				"ραθ", "ενθ", "ροθ", "σθ", "πυρ", "αιν", "συνδ", "συν", "συνθ", "χωρ", "πον", "βρ", "καθ",
				"ευθ", "εκθ", "νετ", "ρον", "αρκ", "βαρ", "βολ", "ωφελ") {
			n += 2
		}
	}
	return n
}

func mscGreekRule10(s []rune, n int) int {
	if n > 5 && (mscEndsWith(s, n, "οντασ") || mscEndsWith(s, n, "ωντασ")) {
		n -= 5
		if n == 3 && mscEndsWith(s, n, "αρχ") {
			n += 3
			s[n-3] = 'ο'
		}
		if mscEndsWith(s, n, "κρε") {
			n += 3
			s[n-3] = 'ω'
		}
	}
	return n
}

func mscGreekRule11(s []rune, n int) int {
	if n > 6 && mscEndsWith(s, n, "ομαστε") {
		n -= 6
		if n == 2 && mscEndsWith(s, n, "ον") {
			n += 5
		}
	} else if n > 7 && mscEndsWith(s, n, "ιομαστε") {
		n -= 7
		if n == 2 && mscEndsWith(s, n, "ον") {
			n += 5
			s[n-5] = 'ο'
			s[n-4] = 'μ'
			s[n-3] = 'α'
			s[n-2] = 'σ'
			s[n-1] = 'τ'
		}
	}
	return n
}

var mscGreekExc12a = mscSet("π", "απ", "συμπ", "ασυμπ", "ακαταπ", "αμεταμφ")

var mscGreekExc12b = mscSet("αλ", "αρ", "εκτελ", "ζ", "μ", "ξ", "παρακαλ", "αρ", "προ", "νισ")

func mscGreekRule12(s []rune, n int) int {
	if n > 5 && mscEndsWith(s, n, "ιεστε") {
		n -= 5
		if mscIn(mscGreekExc12a, s, n) {
			n += 4
		}
	}
	if n > 4 && mscEndsWith(s, n, "εστε") {
		n -= 4
		if mscIn(mscGreekExc12b, s, n) {
			n += 3
		}
	}
	return n
}

var mscGreekExc13 = mscSet("διαθ", "θ", "παρακαταθ", "προσθ", "συνθ")

func mscGreekRule13(s []rune, n int) int {
	if n > 6 && mscEndsWith(s, n, "ηθηκεσ") {
		n -= 6
	} else if n > 5 && (mscEndsWith(s, n, "ηθηκα") || mscEndsWith(s, n, "ηθηκε")) {
		n -= 5
	}
	removed := false
	if n > 4 && mscEndsWith(s, n, "ηκεσ") {
		n -= 4
		removed = true
	} else if n > 3 && (mscEndsWith(s, n, "ηκα") || mscEndsWith(s, n, "ηκε")) {
		n -= 3
		removed = true
	}
	if removed && (mscIn(mscGreekExc13, s, n) || mscGreekEndsAny(s, n, "σκωλ", "σκουλ", "ναρθ", "σφ", "οθ", "πιθ")) {
		n += 2
	}
	return n
}

var mscGreekExc14 = mscSet(
	"φαρμακ", "χαδ", "αγκ", "αναρρ", "βρομ", "εκλιπ", "λαμπιδ", "λεχ", "μ", "πατ", "ρ", "λ",
	"μεδ", "μεσαζ", "υποτειν", "αμ", "αιθ", "ανηκ", "δεσποζ", "ενδιαφερ", "δε", "δευτερευ",
	"καθαρευ", "πλε", "τσα")

func mscGreekRule14(s []rune, n int) int {
	removed := false
	if n > 5 && mscEndsWith(s, n, "ουσεσ") {
		n -= 5
		removed = true
	} else if n > 4 && (mscEndsWith(s, n, "ουσα") || mscEndsWith(s, n, "ουσε")) {
		n -= 4
		removed = true
	}
	if removed && (mscIn(mscGreekExc14, s, n) || mscGreekVowel(s, n) ||
		mscGreekEndsAny(s, n, "ποδαρ", "βλεπ", "πανταχ", "φρυδ", "μαντιλ", "μαλλ", "κυματ", "λαχ", "ληγ",
			"φαγ", "ομ", "πρωτ")) {
		n += 3
	}
	return n
}

var mscGreekExc15a = mscSet(
	"αβαστ", "πολυφ", "αδηφ", "παμφ", "ρ", "ασπ", "αφ", "αμαλ", "αμαλλι", "ανυστ", "απερ",
	"ασπαρ", "αχαρ", "δερβεν", "δροσοπ", "ξεφ", "νεοπ", "νομοτ", "ολοπ", "ομοτ", "προστ",
	"προσωποπ", "συμπ", "συντ", "τ", "υποτ", "χαρ", "αειπ", "αιμοστ", "ανυπ", "αποτ",
	"αρτιπ", "διατ", "εν", "επιτ", "κροκαλοπ", "σιδηροπ", "λ", "ναυ", "ουλαμ", "ουρ", "π",
	"τρ", "μ")

var mscGreekExc15b = mscSet("ψοφ", "ναυλοχ")

func mscGreekRule15(s []rune, n int) int {
	removed := false
	if n > 4 && mscEndsWith(s, n, "αγεσ") {
		n -= 4
		removed = true
	} else if n > 3 && (mscEndsWith(s, n, "αγα") || mscEndsWith(s, n, "αγε")) {
		n -= 3
		removed = true
	}
	if removed {
		cond1 := mscIn(mscGreekExc15a, s, n) ||
			mscGreekEndsAny(s, n, "οφ", "πελ", "χορτ", "λλ", "σφ", "ρπ", "φρ", "πρ", "λοχ", "σμην")
		cond2 := mscIn(mscGreekExc15b, s, n) || mscEndsWith(s, n, "κολλ")
		if cond1 && !cond2 {
			n += 2
		}
	}
	return n
}

var mscGreekExc16 = mscSet("ν", "χερσον", "δωδεκαν", "ερημον", "μεγαλον", "επταν")

func mscGreekRule16(s []rune, n int) int {
	removed := false
	if n > 4 && mscEndsWith(s, n, "ησου") {
		n -= 4
		removed = true
	} else if n > 3 && (mscEndsWith(s, n, "ησε") || mscEndsWith(s, n, "ησα")) {
		n -= 3
		removed = true
	}
	if removed && mscIn(mscGreekExc16, s, n) {
		n += 2
	}
	return n
}

var mscGreekExc17 = mscSet("ασβ", "σβ", "αχρ", "χρ", "απλ", "αειμν", "δυσχρ", "ευχρ", "κοινοχρ", "παλιμψ")

func mscGreekRule17(s []rune, n int) int {
	if n > 4 && mscEndsWith(s, n, "ηστε") {
		n -= 4
		if mscIn(mscGreekExc17, s, n) {
			n += 3
		}
	}
	return n
}

var mscGreekExc18 = mscSet("ν", "ρ", "σπι", "στραβομουτσ", "κακομουτσ", "εξων")

func mscGreekRule18(s []rune, n int) int {
	removed := false
	if n > 6 && (mscEndsWith(s, n, "ησουνε") || mscEndsWith(s, n, "ηθουνε")) {
		n -= 6
		removed = true
	} else if n > 4 && mscEndsWith(s, n, "ουνε") {
		n -= 4
		removed = true
	}
	if removed && mscIn(mscGreekExc18, s, n) {
		n += 3
		s[n-3] = 'ο'
		s[n-2] = 'υ'
		s[n-1] = 'ν'
	}
	return n
}

var mscGreekExc19 = mscSet("παρασουσ", "φ", "χ", "ωριοπλ", "αζ", "αλλοσουσ", "ασουσ")

func mscGreekRule19(s []rune, n int) int {
	removed := false
	if n > 6 && (mscEndsWith(s, n, "ησουμε") || mscEndsWith(s, n, "ηθουμε")) {
		n -= 6
		removed = true
	} else if n > 4 && mscEndsWith(s, n, "ουμε") {
		n -= 4
		removed = true
	}
	if removed && mscIn(mscGreekExc19, s, n) {
		n += 3
		s[n-3] = 'ο'
		s[n-2] = 'υ'
		s[n-1] = 'μ'
	}
	return n
}

func mscGreekRule20(s []rune, n int) int {
	if n > 5 && (mscEndsWith(s, n, "ματων") || mscEndsWith(s, n, "ματοσ")) {
		n -= 3
	} else if n > 4 && mscEndsWith(s, n, "ματα") {
		n -= 2
	}
	return n
}

func mscGreekRule21(s []rune, n int) int {
	if n > 9 && mscEndsWith(s, n, "ιοντουσαν") {
		return n - 9
	}
	if n > 8 && mscGreekEndsAny(s, n, "ιομασταν", "ιοσασταν", "ιουμαστε", "οντουσαν") {
		return n - 8
	}
	if n > 7 && mscGreekEndsAny(s, n, "ιεμαστε", "ιεσαστε", "ιομουνα", "ιοσαστε", "ιοσουνα", "ιουνται",
		"ιουνταν", "ηθηκατε", "ομασταν", "οσασταν", "ουμαστε") {
		return n - 7
	}
	if n > 6 && mscGreekEndsAny(s, n, "ιομουν", "ιονταν", "ιοσουν", "ηθειτε", "ηθηκαν", "ομουνα", "οσαστε",
		"οσουνα", "ουνται", "ουνταν", "ουσατε") {
		return n - 6
	}
	if n > 5 && mscGreekEndsAny(s, n, "αγατε", "ιεμαι", "ιεται", "ιεσαι", "ιοταν", "ιουμα", "ηθεισ", "ηθουν",
		"ηκατε", "ησατε", "ησουν", "ομουν", "ονται", "ονταν", "οσουν", "ουμαι", "ουσαν") {
		return n - 5
	}
	if n > 4 && mscGreekEndsAny(s, n, "αγαν", "αμαι", "ασαι", "αται", "ειτε", "εσαι", "εται", "ηδεσ", "ηδων",
		"ηθει", "ηκαν", "ησαν", "ησει", "ησεσ", "ομαι", "οταν") {
		return n - 4
	}
	if n > 3 && mscGreekEndsAny(s, n, "αει", "εισ", "ηθω", "ησω", "ουν", "ουσ") {
		return n - 3
	}
	if n > 2 && mscGreekEndsAny(s, n, "αν", "ασ", "αω", "ει", "εσ", "ησ", "οι", "οσ", "ου", "υσ", "ων") {
		return n - 2
	}
	if n > 1 && mscGreekVowel(s, n) {
		return n - 1
	}
	return n
}

func mscGreekRule22(s []rune, n int) int {
	if mscEndsWith(s, n, "εστερ") || mscEndsWith(s, n, "εστατ") {
		return n - 5
	}
	if mscGreekEndsAny(s, n, "οτερ", "οτατ", "υτερ", "υτατ", "ωτερ", "ωτατ") {
		return n - 4
	}
	return n
}
