package main

// Code generated from Apache Lucene 10.5.1's org.tartarus.snowball.ext Porter, Nepali,
// Tamil and Yiddish stemmers (Snowball 2.2.0) by j2go.py. DO NOT EDIT.

type porterSnowballStemmer struct {
	snowball
	B_Y_found bool
	I_p2      int
	I_p1      int
}

func newPorterSnowball() *porterSnowballStemmer { return &porterSnowballStemmer{} }

var sporter_a_0 = []among{
	{"s", -1, 3, ""},
	{"ies", 0, 2, ""},
	{"sses", 0, 1, ""},
	{"ss", 0, -1, ""},
}
var sporter_a_1 = []among{
	{"", -1, 3, ""},
	{"bb", 0, 2, ""},
	{"dd", 0, 2, ""},
	{"ff", 0, 2, ""},
	{"gg", 0, 2, ""},
	{"bl", 0, 1, ""},
	{"mm", 0, 2, ""},
	{"nn", 0, 2, ""},
	{"pp", 0, 2, ""},
	{"rr", 0, 2, ""},
	{"at", 0, 1, ""},
	{"tt", 0, 2, ""},
	{"iz", 0, 1, ""},
}
var sporter_a_2 = []among{
	{"ed", -1, 2, ""},
	{"eed", 0, 1, ""},
	{"ing", -1, 2, ""},
}
var sporter_a_3 = []among{
	{"anci", -1, 3, ""},
	{"enci", -1, 2, ""},
	{"abli", -1, 4, ""},
	{"eli", -1, 6, ""},
	{"alli", -1, 9, ""},
	{"ousli", -1, 11, ""},
	{"entli", -1, 5, ""},
	{"aliti", -1, 9, ""},
	{"biliti", -1, 13, ""},
	{"iviti", -1, 12, ""},
	{"tional", -1, 1, ""},
	{"ational", 10, 8, ""},
	{"alism", -1, 9, ""},
	{"ation", -1, 8, ""},
	{"ization", 13, 7, ""},
	{"izer", -1, 7, ""},
	{"ator", -1, 8, ""},
	{"iveness", -1, 12, ""},
	{"fulness", -1, 10, ""},
	{"ousness", -1, 11, ""},
}
var sporter_a_4 = []among{
	{"icate", -1, 2, ""},
	{"ative", -1, 3, ""},
	{"alize", -1, 1, ""},
	{"iciti", -1, 2, ""},
	{"ical", -1, 2, ""},
	{"ful", -1, 3, ""},
	{"ness", -1, 3, ""},
}
var sporter_a_5 = []among{
	{"ic", -1, 1, ""},
	{"ance", -1, 1, ""},
	{"ence", -1, 1, ""},
	{"able", -1, 1, ""},
	{"ible", -1, 1, ""},
	{"ate", -1, 1, ""},
	{"ive", -1, 1, ""},
	{"ize", -1, 1, ""},
	{"iti", -1, 1, ""},
	{"al", -1, 1, ""},
	{"ism", -1, 1, ""},
	{"ion", -1, 2, ""},
	{"er", -1, 1, ""},
	{"ous", -1, 1, ""},
	{"ant", -1, 1, ""},
	{"ent", -1, 1, ""},
	{"ment", 15, 1, ""},
	{"ement", 16, 1, ""},
	{"ou", -1, 1, ""},
}
var sporter_g_v = []byte{17, 65, 16, 1}
var sporter_g_v_WXY = []byte{1, 17, 65, 208, 1}

func (z *porterSnowballStemmer) r_shortv() bool {
	if !(z.outGroupingB(sporter_g_v_WXY, 89, 121)) {
		return false
	}
	if !(z.inGroupingB(sporter_g_v, 97, 121)) {
		return false
	}
	if !(z.outGroupingB(sporter_g_v, 97, 121)) {
		return false
	}
	return true
}
func (z *porterSnowballStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *porterSnowballStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *porterSnowballStemmer) r_Step_1a() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(sporter_a_0)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("ss")
		break
	case 2:
		z.sliceFrom("i")
		break
	case 3:
		z.sliceDel()
		break
	}
	return true
}
func (z *porterSnowballStemmer) r_Step_1b() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(sporter_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R1() {
			return false
		}
		z.sliceFrom("ee")
		break
	case 2:
		v_1 := z.limit - z.cursor
	golab0:
		for {
			{
				if !(z.inGroupingB(sporter_g_v, 97, 121)) {
					goto lab1_end
				}
				break golab0
			}
		lab1_end:
			;
			if z.cursor <= z.limitBackward {
				return false
			}
			z.cursor--
		}
		z.cursor = z.limit - v_1
		z.sliceDel()
		v_3 := z.limit - z.cursor
		among_var = z.findAmongB(sporter_a_1)
		z.cursor = z.limit - v_3
		switch among_var {
		case 1:
			{
				c := z.cursor
				z.insert(z.cursor, z.cursor, "e")
				z.cursor = c
			}
			break
		case 2:
			z.ket = z.cursor
			if z.cursor <= z.limitBackward {
				return false
			}
			z.cursor--
			z.bra = z.cursor
			z.sliceDel()
			break
		case 3:
			if z.cursor != z.I_p1 {
				return false
			}
			v_4 := z.limit - z.cursor
			if !z.r_shortv() {
				return false
			}
			z.cursor = z.limit - v_4
			{
				c := z.cursor
				z.insert(z.cursor, z.cursor, "e")
				z.cursor = c
			}
			break
		}
		break
	}
	return true
}
func (z *porterSnowballStemmer) r_Step_1c() bool {
	z.ket = z.cursor
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("y")) {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		if !(z.eqSB("Y")) {
			return false
		}
	}
lab0_end:
	;
	z.bra = z.cursor
golab2:
	for {
		{
			if !(z.inGroupingB(sporter_g_v, 97, 121)) {
				goto lab3_end
			}
			break golab2
		}
	lab3_end:
		;
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
	}
	z.sliceFrom("i")
	return true
}
func (z *porterSnowballStemmer) r_Step_2() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(sporter_a_3)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("tion")
		break
	case 2:
		z.sliceFrom("ence")
		break
	case 3:
		z.sliceFrom("ance")
		break
	case 4:
		z.sliceFrom("able")
		break
	case 5:
		z.sliceFrom("ent")
		break
	case 6:
		z.sliceFrom("e")
		break
	case 7:
		z.sliceFrom("ize")
		break
	case 8:
		z.sliceFrom("ate")
		break
	case 9:
		z.sliceFrom("al")
		break
	case 10:
		z.sliceFrom("ful")
		break
	case 11:
		z.sliceFrom("ous")
		break
	case 12:
		z.sliceFrom("ive")
		break
	case 13:
		z.sliceFrom("ble")
		break
	}
	return true
}
func (z *porterSnowballStemmer) r_Step_3() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(sporter_a_4)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("al")
		break
	case 2:
		z.sliceFrom("ic")
		break
	case 3:
		z.sliceDel()
		break
	}
	return true
}
func (z *porterSnowballStemmer) r_Step_4() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(sporter_a_5)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R2() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("s")) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			if !(z.eqSB("t")) {
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	}
	return true
}
func (z *porterSnowballStemmer) r_Step_5a() bool {
	z.ket = z.cursor
	if !(z.eqSB("e")) {
		return false
	}
	z.bra = z.cursor
	{
		v_1 := z.limit - z.cursor
		{
			if !z.r_R2() {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		if !z.r_R1() {
			return false
		}
		{
			v_2 := z.limit - z.cursor
			{
				if !z.r_shortv() {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_2
		}
	}
lab0_end:
	;
	z.sliceDel()
	return true
}
func (z *porterSnowballStemmer) r_Step_5b() bool {
	z.ket = z.cursor
	if !(z.eqSB("l")) {
		return false
	}
	z.bra = z.cursor
	if !z.r_R2() {
		return false
	}
	if !(z.eqSB("l")) {
		return false
	}
	z.sliceDel()
	return true
}
func (z *porterSnowballStemmer) stem() {
	z.B_Y_found = false
	v_1 := z.cursor
	{
		z.bra = z.cursor
		if !(z.eqS("y")) {
			goto lab0_end
		}
		z.ket = z.cursor
		z.sliceFrom("Y")
		z.B_Y_found = true
	}
lab0_end:
	;
	z.cursor = v_1
	v_2 := z.cursor
	{
		for {
			v_3 := z.cursor
			{
			golab3:
				for {
					v_4 := z.cursor
					{
						if !(z.inGrouping(sporter_g_v, 97, 121)) {
							goto lab4_end
						}
						z.bra = z.cursor
						if !(z.eqS("y")) {
							goto lab4_end
						}
						z.ket = z.cursor
						z.cursor = v_4
						break golab3
					}
				lab4_end:
					;
					z.cursor = v_4
					if z.cursor >= z.limit {
						goto lab2_end
					}
					z.cursor++
				}
				z.sliceFrom("Y")
				z.B_Y_found = true
				continue
			}
		lab2_end:
			;
			z.cursor = v_3
			break
		}
	}
	z.cursor = v_2
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_5 := z.cursor
	{
	golab6:
		for {
			{
				if !(z.inGrouping(sporter_g_v, 97, 121)) {
					goto lab7_end
				}
				break golab6
			}
		lab7_end:
			;
			if z.cursor >= z.limit {
				goto lab5_end
			}
			z.cursor++
		}
	golab8:
		for {
			{
				if !(z.outGrouping(sporter_g_v, 97, 121)) {
					goto lab9_end
				}
				break golab8
			}
		lab9_end:
			;
			if z.cursor >= z.limit {
				goto lab5_end
			}
			z.cursor++
		}
		z.I_p1 = z.cursor
	golab10:
		for {
			{
				if !(z.inGrouping(sporter_g_v, 97, 121)) {
					goto lab11_end
				}
				break golab10
			}
		lab11_end:
			;
			if z.cursor >= z.limit {
				goto lab5_end
			}
			z.cursor++
		}
	golab12:
		for {
			{
				if !(z.outGrouping(sporter_g_v, 97, 121)) {
					goto lab13_end
				}
				break golab12
			}
		lab13_end:
			;
			if z.cursor >= z.limit {
				goto lab5_end
			}
			z.cursor++
		}
		z.I_p2 = z.cursor
	}
lab5_end:
	;
	z.cursor = v_5
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_10 := z.limit - z.cursor
	z.r_Step_1a()
	z.cursor = z.limit - v_10
	v_11 := z.limit - z.cursor
	z.r_Step_1b()
	z.cursor = z.limit - v_11
	v_12 := z.limit - z.cursor
	z.r_Step_1c()
	z.cursor = z.limit - v_12
	v_13 := z.limit - z.cursor
	z.r_Step_2()
	z.cursor = z.limit - v_13
	v_14 := z.limit - z.cursor
	z.r_Step_3()
	z.cursor = z.limit - v_14
	v_15 := z.limit - z.cursor
	z.r_Step_4()
	z.cursor = z.limit - v_15
	v_16 := z.limit - z.cursor
	z.r_Step_5a()
	z.cursor = z.limit - v_16
	v_17 := z.limit - z.cursor
	z.r_Step_5b()
	z.cursor = z.limit - v_17
	z.cursor = z.limitBackward
	v_18 := z.cursor
	{
		if !(z.B_Y_found) {
			goto lab14_end
		}
		for {
			v_19 := z.cursor
			{
			golab16:
				for {
					v_20 := z.cursor
					{
						z.bra = z.cursor
						if !(z.eqS("Y")) {
							goto lab17_end
						}
						z.ket = z.cursor
						z.cursor = v_20
						break golab16
					}
				lab17_end:
					;
					z.cursor = v_20
					if z.cursor >= z.limit {
						goto lab15_end
					}
					z.cursor++
				}
				z.sliceFrom("y")
				continue
			}
		lab15_end:
			;
			z.cursor = v_19
			break
		}
	}
lab14_end:
	;
	z.cursor = v_18
	return
}

type nepaliStemmer struct {
	snowball
}

func newNepali() *nepaliStemmer { return &nepaliStemmer{} }

var nep_a_0 = []among{
	{"\u0932\u093E\u0907", -1, 1, ""},
	{"\u0932\u093E\u0908", -1, 1, ""},
	{"\u0938\u0901\u0917", -1, 1, ""},
	{"\u0938\u0902\u0917", -1, 1, ""},
	{"\u092E\u093E\u0930\u094D\u092B\u0924", -1, 1, ""},
	{"\u0930\u0924", -1, 1, ""},
	{"\u0915\u093E", -1, 2, ""},
	{"\u092E\u093E", -1, 1, ""},
	{"\u0926\u094D\u0935\u093E\u0930\u093E", -1, 1, ""},
	{"\u0915\u093F", -1, 2, ""},
	{"\u092A\u091B\u093F", -1, 1, ""},
	{"\u0915\u0940", -1, 2, ""},
	{"\u0932\u0947", -1, 1, ""},
	{"\u0915\u0948", -1, 2, ""},
	{"\u0938\u0901\u0917\u0948", -1, 1, ""},
	{"\u092E\u0948", -1, 1, ""},
	{"\u0915\u094B", -1, 2, ""},
}
var nep_a_1 = []among{
	{"\u0901", -1, -1, ""},
	{"\u0902", -1, -1, ""},
	{"\u0948", -1, -1, ""},
}
var nep_a_2 = []among{
	{"\u0901", -1, 1, ""},
	{"\u0902", -1, 1, ""},
	{"\u0948", -1, 2, ""},
}
var nep_a_3 = []among{
	{"\u0925\u093F\u090F", -1, 1, ""},
	{"\u091B", -1, 1, ""},
	{"\u0907\u091B", 1, 1, ""},
	{"\u090F\u091B", 1, 1, ""},
	{"\u093F\u091B", 1, 1, ""},
	{"\u0947\u091B", 1, 1, ""},
	{"\u0928\u0947\u091B", 5, 1, ""},
	{"\u0939\u0941\u0928\u0947\u091B", 6, 1, ""},
	{"\u0907\u0928\u094D\u091B", 1, 1, ""},
	{"\u093F\u0928\u094D\u091B", 1, 1, ""},
	{"\u0939\u0941\u0928\u094D\u091B", 1, 1, ""},
	{"\u090F\u0915\u093E", -1, 1, ""},
	{"\u0907\u090F\u0915\u093E", 11, 1, ""},
	{"\u093F\u090F\u0915\u093E", 11, 1, ""},
	{"\u0947\u0915\u093E", -1, 1, ""},
	{"\u0928\u0947\u0915\u093E", 14, 1, ""},
	{"\u0926\u093E", -1, 1, ""},
	{"\u0907\u0926\u093E", 16, 1, ""},
	{"\u093F\u0926\u093E", 16, 1, ""},
	{"\u0926\u0947\u0916\u093F", -1, 1, ""},
	{"\u092E\u093E\u0925\u093F", -1, 1, ""},
	{"\u090F\u0915\u0940", -1, 1, ""},
	{"\u0907\u090F\u0915\u0940", 21, 1, ""},
	{"\u093F\u090F\u0915\u0940", 21, 1, ""},
	{"\u0947\u0915\u0940", -1, 1, ""},
	{"\u0926\u0947\u0916\u0940", -1, 1, ""},
	{"\u0925\u0940", -1, 1, ""},
	{"\u0926\u0940", -1, 1, ""},
	{"\u091B\u0941", -1, 1, ""},
	{"\u090F\u091B\u0941", 28, 1, ""},
	{"\u0947\u091B\u0941", 28, 1, ""},
	{"\u0928\u0947\u091B\u0941", 30, 1, ""},
	{"\u0928\u0941", -1, 1, ""},
	{"\u0939\u0930\u0941", -1, 1, ""},
	{"\u0939\u0930\u0942", -1, 1, ""},
	{"\u091B\u0947", -1, 1, ""},
	{"\u0925\u0947", -1, 1, ""},
	{"\u0928\u0947", -1, 1, ""},
	{"\u090F\u0915\u0948", -1, 1, ""},
	{"\u0947\u0915\u0948", -1, 1, ""},
	{"\u0928\u0947\u0915\u0948", 39, 1, ""},
	{"\u0926\u0948", -1, 1, ""},
	{"\u0907\u0926\u0948", 41, 1, ""},
	{"\u093F\u0926\u0948", 41, 1, ""},
	{"\u090F\u0915\u094B", -1, 1, ""},
	{"\u0907\u090F\u0915\u094B", 44, 1, ""},
	{"\u093F\u090F\u0915\u094B", 44, 1, ""},
	{"\u0947\u0915\u094B", -1, 1, ""},
	{"\u0928\u0947\u0915\u094B", 47, 1, ""},
	{"\u0926\u094B", -1, 1, ""},
	{"\u0907\u0926\u094B", 49, 1, ""},
	{"\u093F\u0926\u094B", 49, 1, ""},
	{"\u092F\u094B", -1, 1, ""},
	{"\u0907\u092F\u094B", 52, 1, ""},
	{"\u092D\u092F\u094B", 52, 1, ""},
	{"\u093F\u092F\u094B", 52, 1, ""},
	{"\u0925\u093F\u092F\u094B", 55, 1, ""},
	{"\u0926\u093F\u092F\u094B", 55, 1, ""},
	{"\u0925\u094D\u092F\u094B", 52, 1, ""},
	{"\u091B\u094C", -1, 1, ""},
	{"\u0907\u091B\u094C", 59, 1, ""},
	{"\u090F\u091B\u094C", 59, 1, ""},
	{"\u093F\u091B\u094C", 59, 1, ""},
	{"\u0947\u091B\u094C", 59, 1, ""},
	{"\u0928\u0947\u091B\u094C", 63, 1, ""},
	{"\u092F\u094C", -1, 1, ""},
	{"\u0925\u093F\u092F\u094C", 65, 1, ""},
	{"\u091B\u094D\u092F\u094C", 65, 1, ""},
	{"\u0925\u094D\u092F\u094C", 65, 1, ""},
	{"\u091B\u0928\u094D", -1, 1, ""},
	{"\u0907\u091B\u0928\u094D", 69, 1, ""},
	{"\u090F\u091B\u0928\u094D", 69, 1, ""},
	{"\u093F\u091B\u0928\u094D", 69, 1, ""},
	{"\u0947\u091B\u0928\u094D", 69, 1, ""},
	{"\u0928\u0947\u091B\u0928\u094D", 73, 1, ""},
	{"\u0932\u093E\u0928\u094D", -1, 1, ""},
	{"\u091B\u093F\u0928\u094D", -1, 1, ""},
	{"\u0925\u093F\u0928\u094D", -1, 1, ""},
	{"\u092A\u0930\u094D", -1, 1, ""},
	{"\u0907\u0938\u094D", -1, 1, ""},
	{"\u0925\u093F\u0907\u0938\u094D", 79, 1, ""},
	{"\u091B\u0938\u094D", -1, 1, ""},
	{"\u0907\u091B\u0938\u094D", 81, 1, ""},
	{"\u090F\u091B\u0938\u094D", 81, 1, ""},
	{"\u093F\u091B\u0938\u094D", 81, 1, ""},
	{"\u0947\u091B\u0938\u094D", 81, 1, ""},
	{"\u0928\u0947\u091B\u0938\u094D", 85, 1, ""},
	{"\u093F\u0938\u094D", -1, 1, ""},
	{"\u0925\u093F\u0938\u094D", 87, 1, ""},
	{"\u091B\u0947\u0938\u094D", -1, 1, ""},
	{"\u0939\u094B\u0938\u094D", -1, 1, ""},
}

func (z *nepaliStemmer) r_remove_category_1() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(nep_a_0)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		{
			v_1 := z.limit - z.cursor
			{
				{
					v_2 := z.limit - z.cursor
					{
						if !(z.eqSB("\u090F")) {
							goto lab3_end
						}
						goto lab2_end
					}
				lab3_end:
					;
					z.cursor = z.limit - v_2
					if !(z.eqSB("\u0947")) {
						goto lab1_end
					}
				}
			lab2_end:
				;
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			z.sliceDel()
		}
	lab0_end:
		;
		break
	}
	return true
}
func (z *nepaliStemmer) r_check_category_2() bool {
	z.ket = z.cursor
	if z.findAmongB(nep_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	return true
}
func (z *nepaliStemmer) r_remove_category_2() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(nep_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("\u092F\u094C")) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			{
				if !(z.eqSB("\u091B\u094C")) {
					goto lab2_end
				}
				goto lab0_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_1
			{
				if !(z.eqSB("\u0928\u094C")) {
					goto lab3_end
				}
				goto lab0_end
			}
		lab3_end:
			;
			z.cursor = z.limit - v_1
			if !(z.eqSB("\u0925\u0947")) {
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	case 2:
		if !(z.eqSB("\u0924\u094D\u0930")) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *nepaliStemmer) r_remove_category_3() bool {
	z.ket = z.cursor
	if z.findAmongB(nep_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *nepaliStemmer) stem() {
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_remove_category_1()
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	{
		for {
			v_3 := z.limit - z.cursor
			{
				v_4 := z.limit - z.cursor
				{
					v_5 := z.limit - z.cursor
					if !z.r_check_category_2() {
						goto lab2_end
					}
					z.cursor = z.limit - v_5
					if !z.r_remove_category_2() {
						goto lab2_end
					}
				}
			lab2_end:
				;
				z.cursor = z.limit - v_4
				if !z.r_remove_category_3() {
					goto lab1_end
				}
				continue
			}
		lab1_end:
			;
			z.cursor = z.limit - v_3
			break
		}
	}
	z.cursor = z.limit - v_2
	z.cursor = z.limitBackward
	return
}

type tamilStemmer struct {
	snowball
	B_found_vetrumai_urupu bool
	B_found_a_match        bool
}

func newTamil() *tamilStemmer { return &tamilStemmer{} }

var tam_a_0 = []among{
	{"\u0BB5\u0BC1", -1, 3, ""},
	{"\u0BB5\u0BC2", -1, 4, ""},
	{"\u0BB5\u0BCA", -1, 2, ""},
	{"\u0BB5\u0BCB", -1, 1, ""},
}
var tam_a_1 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B99", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9E", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BA8", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BAE", -1, -1, ""},
	{"\u0BAF", -1, -1, ""},
	{"\u0BB5", -1, -1, ""},
}
var tam_a_2 = []among{
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_3 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_4 = []among{
	{"", -1, 2, ""},
	{"\u0BC8", 0, 1, ""},
	{"\u0BCD", 0, 1, ""},
}
var tam_a_5 = []among{
	{"\u0BA8\u0BCD\u0BA4", -1, 1, ""},
	{"\u0BAF", -1, 1, ""},
	{"\u0BB5", -1, 1, ""},
	{"\u0BA9\u0BC1", -1, 8, ""},
	{"\u0BC1\u0B95\u0BCD", -1, 7, ""},
	{"\u0BC1\u0B95\u0BCD\u0B95\u0BCD", -1, 7, ""},
	{"\u0B9F\u0BCD\u0B95\u0BCD", -1, 3, ""},
	{"\u0BB1\u0BCD\u0B95\u0BCD", -1, 4, ""},
	{"\u0B99\u0BCD", -1, 9, ""},
	{"\u0B9F\u0BCD\u0B9F\u0BCD", -1, 5, ""},
	{"\u0BA4\u0BCD\u0BA4\u0BCD", -1, 6, ""},
	{"\u0BA8\u0BCD\u0BA4\u0BCD", -1, 1, ""},
	{"\u0BA8\u0BCD", -1, 1, ""},
	{"\u0B9F\u0BCD\u0BAA\u0BCD", -1, 3, ""},
	{"\u0BAF\u0BCD", -1, 2, ""},
	{"\u0BA9\u0BCD\u0BB1\u0BCD", -1, 4, ""},
	{"\u0BB5\u0BCD", -1, 1, ""},
}
var tam_a_6 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9F", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BB1", -1, -1, ""},
}
var tam_a_7 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9F", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BB1", -1, -1, ""},
}
var tam_a_8 = []among{
	{"\u0B9E", -1, -1, ""},
	{"\u0BA3", -1, -1, ""},
	{"\u0BA8", -1, -1, ""},
	{"\u0BA9", -1, -1, ""},
	{"\u0BAE", -1, -1, ""},
	{"\u0BAF", -1, -1, ""},
	{"\u0BB0", -1, -1, ""},
	{"\u0BB2", -1, -1, ""},
	{"\u0BB3", -1, -1, ""},
	{"\u0BB4", -1, -1, ""},
	{"\u0BB5", -1, -1, ""},
}
var tam_a_9 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
	{"\u0BCD", -1, -1, ""},
}
var tam_a_10 = []among{
	{"\u0B85", -1, -1, ""},
	{"\u0B87", -1, -1, ""},
	{"\u0B89", -1, -1, ""},
}
var tam_a_11 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B99", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9E", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BA8", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BAE", -1, -1, ""},
	{"\u0BAF", -1, -1, ""},
	{"\u0BB5", -1, -1, ""},
}
var tam_a_12 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9F", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BB1", -1, -1, ""},
}
var tam_a_13 = []among{
	{"\u0B95\u0BB3\u0BCD", -1, 4, ""},
	{"\u0BC1\u0B99\u0BCD\u0B95\u0BB3\u0BCD", 0, 1, ""},
	{"\u0B9F\u0BCD\u0B95\u0BB3\u0BCD", 0, 3, ""},
	{"\u0BB1\u0BCD\u0B95\u0BB3\u0BCD", 0, 2, ""},
}
var tam_a_14 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BCB", -1, -1, ""},
}
var tam_a_15 = []among{
	{"\u0BAA\u0BBF", -1, -1, ""},
	{"\u0BB5\u0BBF", -1, -1, ""},
}
var tam_a_16 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_17 = []among{
	{"\u0BAA\u0B9F\u0BCD\u0B9F", -1, 3, ""},
	{"\u0BAA\u0B9F\u0BCD\u0B9F\u0BA3", -1, 3, ""},
	{"\u0BA4\u0BBE\u0BA9", -1, 3, ""},
	{"\u0BAA\u0B9F\u0BBF\u0BA4\u0BBE\u0BA9", 2, 3, ""},
	{"\u0BC6\u0BA9", -1, 1, ""},
	{"\u0BBE\u0B95\u0BBF\u0BAF", -1, 1, ""},
	{"\u0B95\u0BC1\u0BB0\u0BBF\u0BAF", -1, 3, ""},
	{"\u0BC1\u0B9F\u0BC8\u0BAF", -1, 1, ""},
	{"\u0BB2\u0BCD\u0BB2", -1, 2, ""},
	{"\u0BC1\u0BB3\u0BCD\u0BB3", -1, 1, ""},
	{"\u0BBE\u0B95\u0BBF", -1, 1, ""},
	{"\u0BAA\u0B9F\u0BBF", -1, 3, ""},
	{"\u0BBF\u0BA9\u0BCD\u0BB1\u0BBF", -1, 1, ""},
	{"\u0BAA\u0BB1\u0BCD\u0BB1\u0BBF", -1, 3, ""},
	{"\u0BAA\u0B9F\u0BC1", -1, 3, ""},
	{"\u0BB5\u0BBF\u0B9F\u0BC1", -1, 3, ""},
	{"\u0BAA\u0B9F\u0BCD\u0B9F\u0BC1", -1, 3, ""},
	{"\u0BB5\u0BBF\u0B9F\u0BCD\u0B9F\u0BC1", -1, 3, ""},
	{"\u0BAA\u0B9F\u0BCD\u0B9F\u0BA4\u0BC1", -1, 3, ""},
	{"\u0BC6\u0BA9\u0BCD\u0BB1\u0BC1", -1, 1, ""},
	{"\u0BC1\u0B9F\u0BC8", -1, 1, ""},
	{"\u0BBF\u0BB2\u0BCD\u0BB2\u0BC8", -1, 1, ""},
	{"\u0BC1\u0B9F\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BBF\u0B9F\u0BAE\u0BCD", -1, 1, ""},
	{"\u0BC6\u0BB2\u0BCD\u0BB2\u0BBE\u0BAE\u0BCD", -1, 3, ""},
	{"\u0BC6\u0BA9\u0BC1\u0BAE\u0BCD", -1, 1, ""},
}
var tam_a_18 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_19 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_20 = []among{
	{"\u0BB5\u0BBF\u0B9F", -1, 2, ""},
	{"\u0BC0", -1, 7, ""},
	{"\u0BCA\u0B9F\u0BC1", -1, 2, ""},
	{"\u0BCB\u0B9F\u0BC1", -1, 2, ""},
	{"\u0BA4\u0BC1", -1, 6, ""},
	{"\u0BBF\u0BB0\u0BC1\u0BA8\u0BCD\u0BA4\u0BC1", 4, 2, ""},
	{"\u0BBF\u0BA9\u0BCD\u0BB1\u0BC1", -1, 2, ""},
	{"\u0BC1\u0B9F\u0BC8", -1, 2, ""},
	{"\u0BA9\u0BC8", -1, 1, ""},
	{"\u0B95\u0BA3\u0BCD", -1, 1, ""},
	{"\u0BBF\u0BA9\u0BCD", -1, 3, ""},
	{"\u0BAE\u0BC1\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BBF\u0B9F\u0BAE\u0BCD", -1, 4, ""},
	{"\u0BBF\u0BB1\u0BCD", -1, 2, ""},
	{"\u0BAE\u0BC7\u0BB1\u0BCD", -1, 1, ""},
	{"\u0BB2\u0BCD", -1, 5, ""},
	{"\u0BBE\u0BAE\u0BB2\u0BCD", 15, 2, ""},
	{"\u0BBE\u0BB2\u0BCD", 15, 2, ""},
	{"\u0BBF\u0BB2\u0BCD", 15, 2, ""},
	{"\u0BAE\u0BC7\u0BB2\u0BCD", 15, 1, ""},
	{"\u0BC1\u0BB3\u0BCD", -1, 2, ""},
	{"\u0B95\u0BC0\u0BB4\u0BCD", -1, 1, ""},
}
var tam_a_21 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9F", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BB1", -1, -1, ""},
}
var tam_a_22 = []among{
	{"\u0B95", -1, -1, ""},
	{"\u0B9A", -1, -1, ""},
	{"\u0B9F", -1, -1, ""},
	{"\u0BA4", -1, -1, ""},
	{"\u0BAA", -1, -1, ""},
	{"\u0BB1", -1, -1, ""},
}
var tam_a_23 = []among{
	{"\u0B85", -1, -1, ""},
	{"\u0B86", -1, -1, ""},
	{"\u0B87", -1, -1, ""},
	{"\u0B88", -1, -1, ""},
	{"\u0B89", -1, -1, ""},
	{"\u0B8A", -1, -1, ""},
	{"\u0B8E", -1, -1, ""},
	{"\u0B8F", -1, -1, ""},
	{"\u0B90", -1, -1, ""},
	{"\u0B92", -1, -1, ""},
	{"\u0B93", -1, -1, ""},
	{"\u0B94", -1, -1, ""},
}
var tam_a_24 = []among{
	{"\u0BBE", -1, -1, ""},
	{"\u0BBF", -1, -1, ""},
	{"\u0BC0", -1, -1, ""},
	{"\u0BC1", -1, -1, ""},
	{"\u0BC2", -1, -1, ""},
	{"\u0BC6", -1, -1, ""},
	{"\u0BC7", -1, -1, ""},
	{"\u0BC8", -1, -1, ""},
}
var tam_a_25 = []among{
	{"\u0B95", -1, 1, ""},
	{"\u0BA4", -1, 1, ""},
	{"\u0BA9", -1, 1, ""},
	{"\u0BAA", -1, 1, ""},
	{"\u0BAF", -1, 1, ""},
	{"\u0BBE", -1, 5, ""},
	{"\u0B95\u0BC1", -1, 6, ""},
	{"\u0BAA\u0B9F\u0BC1", -1, 1, ""},
	{"\u0BA4\u0BC1", -1, 3, ""},
	{"\u0BBF\u0BB1\u0BCD\u0BB1\u0BC1", -1, 1, ""},
	{"\u0BA9\u0BC8", -1, 1, ""},
	{"\u0BB5\u0BC8", -1, 1, ""},
	{"\u0BA9\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BAA\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BB5\u0BA9\u0BCD", -1, 2, ""},
	{"\u0BBE\u0BA9\u0BCD", -1, 4, ""},
	{"\u0BA9\u0BBE\u0BA9\u0BCD", 15, 1, ""},
	{"\u0BAE\u0BBF\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BA9\u0BC6\u0BA9\u0BCD", -1, 1, ""},
	{"\u0BC7\u0BA9\u0BCD", -1, 5, ""},
	{"\u0BA9\u0BAE\u0BCD", -1, 1, ""},
	{"\u0BAA\u0BAE\u0BCD", -1, 1, ""},
	{"\u0BBE\u0BAE\u0BCD", -1, 5, ""},
	{"\u0B95\u0BC1\u0BAE\u0BCD", -1, 1, ""},
	{"\u0B9F\u0BC1\u0BAE\u0BCD", -1, 5, ""},
	{"\u0BA4\u0BC1\u0BAE\u0BCD", -1, 1, ""},
	{"\u0BB1\u0BC1\u0BAE\u0BCD", -1, 1, ""},
	{"\u0BC6\u0BAE\u0BCD", -1, 5, ""},
	{"\u0BC7\u0BAE\u0BCD", -1, 5, ""},
	{"\u0BCB\u0BAE\u0BCD", -1, 5, ""},
	{"\u0BBE\u0BAF\u0BCD", -1, 5, ""},
	{"\u0BA9\u0BB0\u0BCD", -1, 1, ""},
	{"\u0BAA\u0BB0\u0BCD", -1, 1, ""},
	{"\u0BC0\u0BAF\u0BB0\u0BCD", -1, 5, ""},
	{"\u0BB5\u0BB0\u0BCD", -1, 1, ""},
	{"\u0BBE\u0BB0\u0BCD", -1, 5, ""},
	{"\u0BA9\u0BBE\u0BB0\u0BCD", 35, 1, ""},
	{"\u0BAE\u0BBE\u0BB0\u0BCD", 35, 1, ""},
	{"\u0B95\u0BCA\u0BA3\u0BCD\u0B9F\u0BBF\u0BB0\u0BCD", -1, 1, ""},
	{"\u0BA9\u0BBF\u0BB0\u0BCD", -1, 5, ""},
	{"\u0BC0\u0BB0\u0BCD", -1, 5, ""},
	{"\u0BA9\u0BB3\u0BCD", -1, 1, ""},
	{"\u0BAA\u0BB3\u0BCD", -1, 1, ""},
	{"\u0BB5\u0BB3\u0BCD", -1, 1, ""},
	{"\u0BBE\u0BB3\u0BCD", -1, 5, ""},
	{"\u0BA9\u0BBE\u0BB3\u0BCD", 44, 1, ""},
}
var tam_a_26 = []among{
	{"\u0B95\u0BBF\u0BB1", -1, -1, ""},
	{"\u0B95\u0BBF\u0BA9\u0BCD\u0BB1", -1, -1, ""},
	{"\u0BBE\u0BA8\u0BBF\u0BA9\u0BCD\u0BB1", -1, -1, ""},
	{"\u0B95\u0BBF\u0BB1\u0BCD", -1, -1, ""},
	{"\u0B95\u0BBF\u0BA9\u0BCD\u0BB1\u0BCD", -1, -1, ""},
	{"\u0BBE\u0BA8\u0BBF\u0BA9\u0BCD\u0BB1\u0BCD", -1, -1, ""},
}

func (z *tamilStemmer) r_has_min_length() bool {
	return z.length > 4
}
func (z *tamilStemmer) r_fix_va_start() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(tam_a_0)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("\u0B93")
		break
	case 2:
		z.sliceFrom("\u0B92")
		break
	case 3:
		z.sliceFrom("\u0B89")
		break
	case 4:
		z.sliceFrom("\u0B8A")
		break
	}
	return true
}
func (z *tamilStemmer) r_fix_endings() bool {
	v_1 := z.cursor
	{
		for {
			v_2 := z.cursor
			{
				if !z.r_fix_ending() {
					goto lab1_end
				}
				continue
			}
		lab1_end:
			;
			z.cursor = v_2
			break
		}
	}
	z.cursor = v_1
	return true
}
func (z *tamilStemmer) r_remove_question_prefixes() bool {
	z.bra = z.cursor
	if !(z.eqS("\u0B8E")) {
		return false
	}
	if z.findAmong(tam_a_1) == 0 {
		return false
	}
	if !(z.eqS("\u0BCD")) {
		return false
	}
	z.ket = z.cursor
	z.sliceDel()
	v_1 := z.cursor
	z.r_fix_va_start()
	z.cursor = v_1
	return true
}
func (z *tamilStemmer) r_fix_ending() bool {
	var among_var int
	if z.length <= 3 {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(tam_a_5)
			if among_var == 0 {
				goto lab1_end
			}
			z.bra = z.cursor
			switch among_var {
			case 1:
				z.sliceDel()
				break
			case 2:
				v_2 := z.limit - z.cursor
				if z.findAmongB(tam_a_2) == 0 {
					goto lab1_end
				}
				z.cursor = z.limit - v_2
				z.sliceDel()
				break
			case 3:
				z.sliceFrom("\u0BB3\u0BCD")
				break
			case 4:
				z.sliceFrom("\u0BB2\u0BCD")
				break
			case 5:
				z.sliceFrom("\u0B9F\u0BC1")
				break
			case 6:
				if !(z.B_found_vetrumai_urupu) {
					goto lab1_end
				}
				{
					v_3 := z.limit - z.cursor
					{
						if !(z.eqSB("\u0BC8")) {
							goto lab2_end
						}
						goto lab1_end
					}
				lab2_end:
					;
					z.cursor = z.limit - v_3
				}
				z.sliceFrom("\u0BAE\u0BCD")
				break
			case 7:
				z.sliceFrom("\u0BCD")
				break
			case 8:
				{
					v_4 := z.limit - z.cursor
					{
						if z.findAmongB(tam_a_3) == 0 {
							goto lab3_end
						}
						goto lab1_end
					}
				lab3_end:
					;
					z.cursor = z.limit - v_4
				}
				z.sliceDel()
				break
			case 9:
				among_var = z.findAmongB(tam_a_4)
				switch among_var {
				case 1:
					z.sliceDel()
					break
				case 2:
					z.sliceFrom("\u0BAE\u0BCD")
					break
				}
				break
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		if !(z.eqSB("\u0BCD")) {
			return false
		}
		{
			v_5 := z.limit - z.cursor
			{
				if z.findAmongB(tam_a_6) == 0 {
					goto lab5_end
				}
				v_6 := z.limit - z.cursor
				{
					if !(z.eqSB("\u0BCD")) {
						z.cursor = z.limit - v_6
						goto lab6_end
					}
					if z.findAmongB(tam_a_7) == 0 {
						z.cursor = z.limit - v_6
						goto lab6_end
					}
				}
			lab6_end:
				;
				z.bra = z.cursor
				z.sliceDel()
				goto lab4_end
			}
		lab5_end:
			;
			z.cursor = z.limit - v_5
			{
				if z.findAmongB(tam_a_8) == 0 {
					goto lab7_end
				}
				z.bra = z.cursor
				if !(z.eqSB("\u0BCD")) {
					goto lab7_end
				}
				z.sliceDel()
				goto lab4_end
			}
		lab7_end:
			;
			z.cursor = z.limit - v_5
			v_7 := z.limit - z.cursor
			if z.findAmongB(tam_a_9) == 0 {
				return false
			}
			z.cursor = z.limit - v_7
			z.bra = z.cursor
			z.sliceDel()
		}
	lab4_end:
	}
lab0_end:
	;
	z.cursor = z.limitBackward
	return true
}
func (z *tamilStemmer) r_remove_pronoun_prefixes() bool {
	z.bra = z.cursor
	if z.findAmong(tam_a_10) == 0 {
		return false
	}
	if z.findAmong(tam_a_11) == 0 {
		return false
	}
	if !(z.eqS("\u0BCD")) {
		return false
	}
	z.ket = z.cursor
	z.sliceDel()
	v_1 := z.cursor
	z.r_fix_va_start()
	z.cursor = v_1
	return true
}
func (z *tamilStemmer) r_remove_plural_suffix() bool {
	var among_var int
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.ket = z.cursor
	among_var = z.findAmongB(tam_a_13)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_1 := z.limit - z.cursor
			{
				if z.findAmongB(tam_a_12) == 0 {
					goto lab1_end
				}
				z.sliceFrom("\u0BC1\u0B99\u0BCD")
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			z.sliceFrom("\u0BCD")
		}
	lab0_end:
		;
		break
	case 2:
		z.sliceFrom("\u0BB2\u0BCD")
		break
	case 3:
		z.sliceFrom("\u0BB3\u0BCD")
		break
	case 4:
		z.sliceDel()
		break
	}
	z.cursor = z.limitBackward
	return true
}
func (z *tamilStemmer) r_remove_question_suffixes() bool {
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(tam_a_14) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceFrom("\u0BCD")
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	z.cursor = z.limitBackward
	z.r_fix_endings()
	return true
}
func (z *tamilStemmer) r_remove_command_suffixes() bool {
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.ket = z.cursor
	if z.findAmongB(tam_a_15) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.cursor = z.limitBackward
	return true
}
func (z *tamilStemmer) r_remove_um() bool {
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.ket = z.cursor
	if !(z.eqSB("\u0BC1\u0BAE\u0BCD")) {
		return false
	}
	z.bra = z.cursor
	z.sliceFrom("\u0BCD")
	z.cursor = z.limitBackward
	v_1 := z.cursor
	z.r_fix_ending()
	z.cursor = v_1
	return true
}
func (z *tamilStemmer) r_remove_common_word_endings() bool {
	var among_var int
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.ket = z.cursor
	among_var = z.findAmongB(tam_a_17)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("\u0BCD")
		break
	case 2:
		{
			v_1 := z.limit - z.cursor
			{
				if z.findAmongB(tam_a_16) == 0 {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_1
		}
		z.sliceFrom("\u0BCD")
		break
	case 3:
		z.sliceDel()
		break
	}
	z.cursor = z.limitBackward
	z.r_fix_endings()
	return true
}
func (z *tamilStemmer) r_remove_vetrumai_urupukal() bool {
	var among_var int
	z.B_found_vetrumai_urupu = false
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	{
		v_1 := z.limit - z.cursor
		{
			v_2 := z.limit - z.cursor
			z.ket = z.cursor
			among_var = z.findAmongB(tam_a_20)
			if among_var == 0 {
				goto lab1_end
			}
			z.bra = z.cursor
			switch among_var {
			case 1:
				z.sliceDel()
				break
			case 2:
				z.sliceFrom("\u0BCD")
				break
			case 3:
				{
					v_3 := z.limit - z.cursor
					{
						if !(z.eqSB("\u0BAE")) {
							goto lab2_end
						}
						goto lab1_end
					}
				lab2_end:
					;
					z.cursor = z.limit - v_3
				}
				z.sliceFrom("\u0BCD")
				break
			case 4:
				if z.length < 7 {
					goto lab1_end
				}
				z.sliceFrom("\u0BCD")
				break
			case 5:
				{
					v_4 := z.limit - z.cursor
					{
						if z.findAmongB(tam_a_18) == 0 {
							goto lab3_end
						}
						goto lab1_end
					}
				lab3_end:
					;
					z.cursor = z.limit - v_4
				}
				z.sliceFrom("\u0BCD")
				break
			case 6:
				{
					v_5 := z.limit - z.cursor
					{
						if z.findAmongB(tam_a_19) == 0 {
							goto lab4_end
						}
						goto lab1_end
					}
				lab4_end:
					;
					z.cursor = z.limit - v_5
				}
				z.sliceDel()
				break
			case 7:
				z.sliceFrom("\u0BBF")
				break
			}
			z.cursor = z.limit - v_2
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		v_6 := z.limit - z.cursor
		z.ket = z.cursor
		if !(z.eqSB("\u0BC8")) {
			return false
		}
		{
			v_7 := z.limit - z.cursor
			{
				{
					v_8 := z.limit - z.cursor
					{
						if z.findAmongB(tam_a_21) == 0 {
							goto lab7_end
						}
						goto lab6_end
					}
				lab7_end:
					;
					z.cursor = z.limit - v_8
				}
				goto lab5_end
			}
		lab6_end:
			;
			z.cursor = z.limit - v_7
			v_9 := z.limit - z.cursor
			if z.findAmongB(tam_a_22) == 0 {
				return false
			}
			if !(z.eqSB("\u0BCD")) {
				return false
			}
			z.cursor = z.limit - v_9
		}
	lab5_end:
		;
		z.bra = z.cursor
		z.sliceFrom("\u0BCD")
		z.cursor = z.limit - v_6
	}
lab0_end:
	;
	z.B_found_vetrumai_urupu = true
	v_10 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("\u0BBF\u0BA9\u0BCD")) {
			goto lab8_end
		}
		z.bra = z.cursor
		z.sliceFrom("\u0BCD")
	}
lab8_end:
	;
	z.cursor = z.limit - v_10
	z.cursor = z.limitBackward
	z.r_fix_endings()
	return true
}
func (z *tamilStemmer) r_remove_tense_suffixes() bool {
	z.B_found_a_match = true
	for {
		v_1 := z.cursor
		{
			if !(z.B_found_a_match) {
				goto lab0_end
			}
			v_2 := z.cursor
			z.r_remove_tense_suffix()
			z.cursor = v_2
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *tamilStemmer) r_remove_tense_suffix() bool {
	var among_var int
	z.B_found_a_match = false
	if !z.r_has_min_length() {
		return false
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	{
		v_2 := z.limit - z.cursor
		z.ket = z.cursor
		among_var = z.findAmongB(tam_a_25)
		if among_var == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			z.sliceDel()
			break
		case 2:
			{
				v_3 := z.limit - z.cursor
				{
					if z.findAmongB(tam_a_23) == 0 {
						goto lab1_end
					}
					goto lab0_end
				}
			lab1_end:
				;
				z.cursor = z.limit - v_3
			}
			z.sliceDel()
			break
		case 3:
			{
				v_4 := z.limit - z.cursor
				{
					if z.findAmongB(tam_a_24) == 0 {
						goto lab2_end
					}
					goto lab0_end
				}
			lab2_end:
				;
				z.cursor = z.limit - v_4
			}
			z.sliceDel()
			break
		case 4:
			{
				v_5 := z.limit - z.cursor
				{
					if !(z.eqSB("\u0B9A")) {
						goto lab3_end
					}
					goto lab0_end
				}
			lab3_end:
				;
				z.cursor = z.limit - v_5
			}
			z.sliceFrom("\u0BCD")
			break
		case 5:
			z.sliceFrom("\u0BCD")
			break
		case 6:
			v_6 := z.limit - z.cursor
			if !(z.eqSB("\u0BCD")) {
				goto lab0_end
			}
			z.cursor = z.limit - v_6
			z.sliceDel()
			break
		}
		z.B_found_a_match = true
		z.cursor = z.limit - v_2
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	v_7 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(tam_a_26) == 0 {
			goto lab4_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_found_a_match = true
	}
lab4_end:
	;
	z.cursor = z.limit - v_7
	z.cursor = z.limitBackward
	z.r_fix_endings()
	return true
}
func (z *tamilStemmer) stem() {
	z.B_found_vetrumai_urupu = false
	v_1 := z.cursor
	z.r_fix_ending()
	z.cursor = v_1
	if !z.r_has_min_length() {
		return
	}
	v_2 := z.cursor
	z.r_remove_question_prefixes()
	z.cursor = v_2
	v_3 := z.cursor
	z.r_remove_pronoun_prefixes()
	z.cursor = v_3
	v_4 := z.cursor
	z.r_remove_question_suffixes()
	z.cursor = v_4
	v_5 := z.cursor
	z.r_remove_um()
	z.cursor = v_5
	v_6 := z.cursor
	z.r_remove_common_word_endings()
	z.cursor = v_6
	v_7 := z.cursor
	z.r_remove_vetrumai_urupukal()
	z.cursor = v_7
	v_8 := z.cursor
	z.r_remove_plural_suffix()
	z.cursor = v_8
	v_9 := z.cursor
	z.r_remove_command_suffixes()
	z.cursor = v_9
	v_10 := z.cursor
	z.r_remove_tense_suffixes()
	z.cursor = v_10
	return
}

type yiddishStemmer struct {
	snowball
	I_x  int
	I_p1 int
}

func newYiddish() *yiddishStemmer { return &yiddishStemmer{} }

var yid_a_0 = []among{
	{"\u05D5\u05D5", -1, 1, ""},
	{"\u05D5\u05D9", -1, 2, ""},
	{"\u05D9\u05D9", -1, 3, ""},
	{"\u05DA", -1, 4, ""},
	{"\u05DD", -1, 5, ""},
	{"\u05DF", -1, 6, ""},
	{"\u05E3", -1, 7, ""},
	{"\u05E5", -1, 8, ""},
}
var yid_a_1 = []among{
	{"\u05D0\u05D3\u05D5\u05E8\u05DB", -1, 1, ""},
	{"\u05D0\u05D4\u05D9\u05E0", -1, 1, ""},
	{"\u05D0\u05D4\u05E2\u05E8", -1, 1, ""},
	{"\u05D0\u05D4\u05F2\u05DE", -1, 1, ""},
	{"\u05D0\u05D5\u05DE", -1, 1, ""},
	{"\u05D0\u05D5\u05E0\u05D8\u05E2\u05E8", -1, 1, ""},
	{"\u05D0\u05D9\u05D1\u05E2\u05E8", -1, 1, ""},
	{"\u05D0\u05E0", -1, 1, ""},
	{"\u05D0\u05E0\u05D8", 7, 1, ""},
	{"\u05D0\u05E0\u05D8\u05E7\u05E2\u05D2\u05E0", 8, 1, ""},
	{"\u05D0\u05E0\u05D9\u05D3\u05E2\u05E8", 7, 1, ""},
	{"\u05D0\u05E4", -1, 1, ""},
	{"\u05D0\u05E4\u05D9\u05E8", 11, 1, ""},
	{"\u05D0\u05E7\u05E2\u05D2\u05E0", -1, 1, ""},
	{"\u05D0\u05E8\u05D0\u05E4", -1, 1, ""},
	{"\u05D0\u05E8\u05D5\u05DE", -1, 1, ""},
	{"\u05D0\u05E8\u05D5\u05E0\u05D8\u05E2\u05E8", -1, 1, ""},
	{"\u05D0\u05E8\u05D9\u05D1\u05E2\u05E8", -1, 1, ""},
	{"\u05D0\u05E8\u05F1\u05E1", -1, 1, ""},
	{"\u05D0\u05E8\u05F1\u05E4", -1, 1, ""},
	{"\u05D0\u05E8\u05F2\u05E0", -1, 1, ""},
	{"\u05D0\u05F0\u05E2\u05E7", -1, 1, ""},
	{"\u05D0\u05F1\u05E1", -1, 1, ""},
	{"\u05D0\u05F1\u05E4", -1, 1, ""},
	{"\u05D0\u05F2\u05E0", -1, 1, ""},
	{"\u05D1\u05D0", -1, 1, ""},
	{"\u05D1\u05F2", -1, 1, ""},
	{"\u05D3\u05D5\u05E8\u05DB", -1, 1, ""},
	{"\u05D3\u05E2\u05E8", -1, 1, ""},
	{"\u05DE\u05D9\u05D8", -1, 1, ""},
	{"\u05E0\u05D0\u05DB", -1, 1, ""},
	{"\u05E4\u05D0\u05E8", -1, 1, ""},
	{"\u05E4\u05D0\u05E8\u05D1\u05F2", 31, 1, ""},
	{"\u05E4\u05D0\u05E8\u05F1\u05E1", 31, 1, ""},
	{"\u05E4\u05D5\u05E0\u05D0\u05E0\u05D3\u05E2\u05E8", -1, 1, ""},
	{"\u05E6\u05D5", -1, 1, ""},
	{"\u05E6\u05D5\u05D6\u05D0\u05DE\u05E2\u05E0", 35, 1, ""},
	{"\u05E6\u05D5\u05E0\u05F1\u05E4", 35, 1, ""},
	{"\u05E6\u05D5\u05E8\u05D9\u05E7", 35, 1, ""},
	{"\u05E6\u05E2", -1, 1, ""},
}
var yid_a_2 = []among{
	{"\u05D3\u05D6\u05E9", -1, -1, ""},
	{"\u05E9\u05D8\u05E8", -1, -1, ""},
	{"\u05E9\u05D8\u05E9", -1, -1, ""},
	{"\u05E9\u05E4\u05E8", -1, -1, ""},
}
var yid_a_3 = []among{
	{"\u05E7\u05DC\u05D9\u05D1", -1, 9, ""},
	{"\u05E8\u05D9\u05D1", -1, 10, ""},
	{"\u05D8\u05E8\u05D9\u05D1", 1, 7, ""},
	{"\u05E9\u05E8\u05D9\u05D1", 1, 15, ""},
	{"\u05D4\u05F1\u05D1", -1, 23, ""},
	{"\u05E9\u05F0\u05D9\u05D2", -1, 12, ""},
	{"\u05D2\u05D0\u05E0\u05D2", -1, 1, ""},
	{"\u05D6\u05D5\u05E0\u05D2", -1, 18, ""},
	{"\u05E9\u05DC\u05D5\u05E0\u05D2", -1, 21, ""},
	{"\u05E6\u05F0\u05D5\u05E0\u05D2", -1, 20, ""},
	{"\u05D1\u05F1\u05D2", -1, 22, ""},
	{"\u05D1\u05D5\u05E0\u05D3", -1, 16, ""},
	{"\u05F0\u05D9\u05D6", -1, 6, ""},
	{"\u05D1\u05D9\u05D8", -1, 4, ""},
	{"\u05DC\u05D9\u05D8", -1, 8, ""},
	{"\u05DE\u05D9\u05D8", -1, 3, ""},
	{"\u05E9\u05E0\u05D9\u05D8", -1, 14, ""},
	{"\u05E0\u05D5\u05DE", -1, 2, ""},
	{"\u05E9\u05D8\u05D0\u05E0", -1, 25, ""},
	{"\u05D1\u05D9\u05E1", -1, 5, ""},
	{"\u05E9\u05DE\u05D9\u05E1", -1, 13, ""},
	{"\u05E8\u05D9\u05E1", -1, 11, ""},
	{"\u05D8\u05E8\u05D5\u05E0\u05E7", -1, 19, ""},
	{"\u05E4\u05D0\u05E8\u05DC\u05F1\u05E8", -1, 24, ""},
	{"\u05E9\u05F0\u05F1\u05E8", -1, 26, ""},
	{"\u05F0\u05D5\u05D8\u05E9", -1, 17, ""},
}
var yid_a_4 = []among{
	{"\u05D5\u05E0\u05D2", -1, 1, ""},
	{"\u05E1\u05D8\u05D5", -1, 1, ""},
	{"\u05D8", -1, 1, ""},
	{"\u05D1\u05E8\u05D0\u05DB\u05D8", 2, 31, ""},
	{"\u05E1\u05D8", 2, 1, ""},
	{"\u05D9\u05E1\u05D8", 4, 33, ""},
	{"\u05E2\u05D8", 2, 1, ""},
	{"\u05E9\u05D0\u05E4\u05D8", 2, 1, ""},
	{"\u05D4\u05F2\u05D8", 2, 1, ""},
	{"\u05E7\u05F2\u05D8", 2, 1, ""},
	{"\u05D9\u05E7\u05F2\u05D8", 9, 1, ""},
	{"\u05DC\u05E2\u05DB", -1, 1, ""},
	{"\u05E2\u05DC\u05E2\u05DB", 11, 1, ""},
	{"\u05D9\u05D6\u05DE", -1, 1, ""},
	{"\u05D9\u05DE", -1, 1, ""},
	{"\u05E2\u05DE", -1, 1, ""},
	{"\u05E2\u05E0\u05E2\u05DE", 15, 3, ""},
	{"\u05D8\u05E2\u05E0\u05E2\u05DE", 16, 4, ""},
	{"\u05E0", -1, 1, ""},
	{"\u05E7\u05DC\u05D9\u05D1\u05E0", 18, 14, ""},
	{"\u05E8\u05D9\u05D1\u05E0", 18, 15, ""},
	{"\u05D8\u05E8\u05D9\u05D1\u05E0", 20, 12, ""},
	{"\u05E9\u05E8\u05D9\u05D1\u05E0", 20, 7, ""},
	{"\u05D4\u05F1\u05D1\u05E0", 18, 27, ""},
	{"\u05E9\u05F0\u05D9\u05D2\u05E0", 18, 17, ""},
	{"\u05D6\u05D5\u05E0\u05D2\u05E0", 18, 22, ""},
	{"\u05E9\u05DC\u05D5\u05E0\u05D2\u05E0", 18, 25, ""},
	{"\u05E6\u05F0\u05D5\u05E0\u05D2\u05E0", 18, 24, ""},
	{"\u05D1\u05F1\u05D2\u05E0", 18, 26, ""},
	{"\u05D1\u05D5\u05E0\u05D3\u05E0", 18, 20, ""},
	{"\u05F0\u05D9\u05D6\u05E0", 18, 11, ""},
	{"\u05D8\u05E0", 18, 4, ""},
	{"GE\u05D1\u05D9\u05D8\u05E0", 31, 9, ""},
	{"GE\u05DC\u05D9\u05D8\u05E0", 31, 13, ""},
	{"GE\u05DE\u05D9\u05D8\u05E0", 31, 8, ""},
	{"\u05E9\u05E0\u05D9\u05D8\u05E0", 31, 19, ""},
	{"\u05E1\u05D8\u05E0", 31, 1, ""},
	{"\u05D9\u05E1\u05D8\u05E0", 36, 1, ""},
	{"\u05E2\u05D8\u05E0", 31, 1, ""},
	{"GE\u05D1\u05D9\u05E1\u05E0", 18, 10, ""},
	{"\u05E9\u05DE\u05D9\u05E1\u05E0", 18, 18, ""},
	{"GE\u05E8\u05D9\u05E1\u05E0", 18, 16, ""},
	{"\u05E2\u05E0", 18, 1, ""},
	{"\u05D2\u05D0\u05E0\u05D2\u05E2\u05E0", 42, 5, ""},
	{"\u05E2\u05DC\u05E2\u05E0", 42, 1, ""},
	{"\u05E0\u05D5\u05DE\u05E2\u05E0", 42, 6, ""},
	{"\u05D9\u05D6\u05DE\u05E2\u05E0", 42, 1, ""},
	{"\u05E9\u05D8\u05D0\u05E0\u05E2\u05E0", 42, 29, ""},
	{"\u05D8\u05E8\u05D5\u05E0\u05E7\u05E0", 18, 23, ""},
	{"\u05E4\u05D0\u05E8\u05DC\u05F1\u05E8\u05E0", 18, 28, ""},
	{"\u05E9\u05F0\u05F1\u05E8\u05E0", 18, 30, ""},
	{"\u05F0\u05D5\u05D8\u05E9\u05E0", 18, 21, ""},
	{"\u05D2\u05F2\u05E0", 18, 5, ""},
	{"\u05E1", -1, 1, ""},
	{"\u05D8\u05E1", 53, 4, ""},
	{"\u05E2\u05D8\u05E1", 54, 1, ""},
	{"\u05E0\u05E1", 53, 1, ""},
	{"\u05D8\u05E0\u05E1", 56, 4, ""},
	{"\u05E2\u05E0\u05E1", 56, 3, ""},
	{"\u05E2\u05E1", 53, 1, ""},
	{"\u05D9\u05E2\u05E1", 59, 2, ""},
	{"\u05E2\u05DC\u05E2\u05E1", 59, 1, ""},
	{"\u05E2\u05E8\u05E1", 53, 1, ""},
	{"\u05E2\u05E0\u05E2\u05E8\u05E1", 62, 1, ""},
	{"\u05E2", -1, 1, ""},
	{"\u05D8\u05E2", 64, 4, ""},
	{"\u05E1\u05D8\u05E2", 65, 1, ""},
	{"\u05E2\u05D8\u05E2", 65, 1, ""},
	{"\u05D9\u05E2", 64, -1, ""},
	{"\u05E2\u05DC\u05E2", 64, 1, ""},
	{"\u05E2\u05E0\u05E2", 64, 3, ""},
	{"\u05D8\u05E2\u05E0\u05E2", 70, 4, ""},
	{"\u05E2\u05E8", -1, 1, ""},
	{"\u05D8\u05E2\u05E8", 72, 4, ""},
	{"\u05E1\u05D8\u05E2\u05E8", 73, 1, ""},
	{"\u05E2\u05D8\u05E2\u05E8", 73, 1, ""},
	{"\u05E2\u05E0\u05E2\u05E8", 72, 3, ""},
	{"\u05D8\u05E2\u05E0\u05E2\u05E8", 76, 4, ""},
	{"\u05D5\u05EA", -1, 32, ""},
}
var yid_a_5 = []among{
	{"\u05D5\u05E0\u05D2", -1, 1, ""},
	{"\u05E9\u05D0\u05E4\u05D8", -1, 1, ""},
	{"\u05D4\u05F2\u05D8", -1, 1, ""},
	{"\u05E7\u05F2\u05D8", -1, 1, ""},
	{"\u05D9\u05E7\u05F2\u05D8", 3, 1, ""},
	{"\u05DC", -1, 2, ""},
}
var yid_a_6 = []among{
	{"\u05D9\u05D2", -1, 1, ""},
	{"\u05D9\u05E7", -1, 1, ""},
	{"\u05D3\u05D9\u05E7", 1, 1, ""},
	{"\u05E0\u05D3\u05D9\u05E7", 2, 1, ""},
	{"\u05E2\u05E0\u05D3\u05D9\u05E7", 3, 1, ""},
	{"\u05D1\u05DC\u05D9\u05E7", 1, -1, ""},
	{"\u05D2\u05DC\u05D9\u05E7", 1, -1, ""},
	{"\u05E0\u05D9\u05E7", 1, 1, ""},
	{"\u05D9\u05E9", -1, 1, ""},
}
var yid_g_niked = []byte{255, 155, 6}
var yid_g_vowel = []byte{33, 2, 4, 0, 6}
var yid_g_consonant = []byte{239, 254, 253, 131}

func (z *yiddishStemmer) r_prelude() bool {
	var among_var int
	v_1 := z.cursor
	{
		for {
			v_2 := z.cursor
			{
			golab2:
				for {
					v_3 := z.cursor
					{
						z.bra = z.cursor
						among_var = z.findAmong(yid_a_0)
						if among_var == 0 {
							goto lab3_end
						}
						z.ket = z.cursor
						switch among_var {
						case 1:
							{
								v_4 := z.cursor
								{
									if !(z.eqS("\u05BC")) {
										goto lab4_end
									}
									goto lab3_end
								}
							lab4_end:
								;
								z.cursor = v_4
							}
							z.sliceFrom("\u05F0")
							break
						case 2:
							{
								v_5 := z.cursor
								{
									if !(z.eqS("\u05B4")) {
										goto lab5_end
									}
									goto lab3_end
								}
							lab5_end:
								;
								z.cursor = v_5
							}
							z.sliceFrom("\u05F1")
							break
						case 3:
							{
								v_6 := z.cursor
								{
									if !(z.eqS("\u05B4")) {
										goto lab6_end
									}
									goto lab3_end
								}
							lab6_end:
								;
								z.cursor = v_6
							}
							z.sliceFrom("\u05F2")
							break
						case 4:
							z.sliceFrom("\u05DB")
							break
						case 5:
							z.sliceFrom("\u05DE")
							break
						case 6:
							z.sliceFrom("\u05E0")
							break
						case 7:
							z.sliceFrom("\u05E4")
							break
						case 8:
							z.sliceFrom("\u05E6")
							break
						}
						z.cursor = v_3
						break golab2
					}
				lab3_end:
					;
					z.cursor = v_3
					if z.cursor >= z.limit {
						goto lab1_end
					}
					z.cursor++
				}
				continue
			}
		lab1_end:
			;
			z.cursor = v_2
			break
		}
	}
	z.cursor = v_1
	v_7 := z.cursor
	{
		for {
			v_8 := z.cursor
			{
			golab9:
				for {
					v_9 := z.cursor
					{
						z.bra = z.cursor
						if !(z.inGrouping(yid_g_niked, 1456, 1474)) {
							goto lab10_end
						}
						z.ket = z.cursor
						z.sliceDel()
						z.cursor = v_9
						break golab9
					}
				lab10_end:
					;
					z.cursor = v_9
					if z.cursor >= z.limit {
						goto lab8_end
					}
					z.cursor++
				}
				continue
			}
		lab8_end:
			;
			z.cursor = v_8
			break
		}
	}
	z.cursor = v_7
	return true
}
func (z *yiddishStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	v_1 := z.cursor
	{
		z.bra = z.cursor
		if !(z.eqS("\u05D2\u05E2")) {
			z.cursor = v_1
			goto lab0_end
		}
		z.ket = z.cursor
		{
			v_2 := z.cursor
			{
				{
					v_3 := z.cursor
					{
						if !(z.eqS("\u05DC\u05D8")) {
							goto lab3_end
						}
						goto lab2_end
					}
				lab3_end:
					;
					z.cursor = v_3
					if !(z.eqS("\u05D1\u05E0")) {
						goto lab1_end
					}
				}
			lab2_end:
				;
				z.cursor = v_1
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = v_2
		}
		z.sliceFrom("GE")
	}
lab0_end:
	;
	v_4 := z.cursor
	{
		if z.findAmong(yid_a_1) == 0 {
			z.cursor = v_4
			goto lab4_end
		}
		{
			v_5 := z.cursor
			{
				v_6 := z.cursor
				{
					v_7 := z.cursor
					{
						if !(z.eqS("\u05E6\u05D5\u05D2\u05E0")) {
							goto lab8_end
						}
						goto lab7_end
					}
				lab8_end:
					;
					z.cursor = v_7
					{
						if !(z.eqS("\u05E6\u05D5\u05E7\u05D8")) {
							goto lab9_end
						}
						goto lab7_end
					}
				lab9_end:
					;
					z.cursor = v_7
					if !(z.eqS("\u05E6\u05D5\u05E7\u05E0")) {
						goto lab6_end
					}
				}
			lab7_end:
				;
				if z.cursor < z.limit {
					goto lab6_end
				}
				z.cursor = v_6
				goto lab5_end
			}
		lab6_end:
			;
			z.cursor = v_5
			{
				v_8 := z.cursor
				if !(z.eqS("\u05D2\u05E2\u05D1\u05E0")) {
					goto lab10_end
				}
				z.cursor = v_8
				goto lab5_end
			}
		lab10_end:
			;
			z.cursor = v_5
			{
				z.bra = z.cursor
				if !(z.eqS("\u05D2\u05E2")) {
					goto lab11_end
				}
				z.ket = z.cursor
				z.sliceFrom("GE")
				goto lab5_end
			}
		lab11_end:
			;
			z.cursor = v_5
			z.bra = z.cursor
			if !(z.eqS("\u05E6\u05D5")) {
				z.cursor = v_4
				goto lab4_end
			}
			z.ket = z.cursor
			z.sliceFrom("TSU")
		}
	lab5_end:
	}
lab4_end:
	;
	v_9 := z.cursor
	{
		c := z.cursor + 3
		if c > z.limit {
			return false
		}
		z.cursor = c
	}
	z.I_x = z.cursor
	z.cursor = v_9
	v_10 := z.cursor
	{
		if z.findAmong(yid_a_2) == 0 {
			z.cursor = v_10
			goto lab12_end
		}
	}
lab12_end:
	;
	{
		v_11 := z.cursor
		{
			if !(z.inGrouping(yid_g_consonant, 1489, 1520)) {
				goto lab13_end
			}
			if !(z.inGrouping(yid_g_consonant, 1489, 1520)) {
				goto lab13_end
			}
			if !(z.inGrouping(yid_g_consonant, 1489, 1520)) {
				goto lab13_end
			}
			z.I_p1 = z.cursor
			return false
		}
	lab13_end:
		;
		z.cursor = v_11
	}
golab14:
	for {
		v_12 := z.cursor
		{
			if !(z.inGrouping(yid_g_vowel, 1488, 1522)) {
				goto lab15_end
			}
			z.cursor = v_12
			break golab14
		}
	lab15_end:
		;
		z.cursor = v_12
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	for {
		{
			if !(z.inGrouping(yid_g_vowel, 1488, 1522)) {
				goto lab16_end
			}
			continue
		}
	lab16_end:
		;
		break
	}
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= z.I_x {
			goto lab17_end
		}
		z.I_p1 = z.I_x
	}
lab17_end:
	;
	return true
}
func (z *yiddishStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *yiddishStemmer) r_R1plus3() bool {
	return z.I_p1 <= (z.cursor + 3)
}
func (z *yiddishStemmer) r_standard_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(yid_a_4)
		if among_var == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			if !z.r_R1() {
				goto lab0_end
			}
			z.sliceDel()
			break
		case 2:
			if !z.r_R1() {
				goto lab0_end
			}
			z.sliceFrom("\u05D9\u05E2")
			break
		case 3:
			if !z.r_R1() {
				goto lab0_end
			}
			z.sliceDel()
			z.ket = z.cursor
			among_var = z.findAmongB(yid_a_3)
			if among_var == 0 {
				goto lab0_end
			}
			z.bra = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("\u05D2\u05F2")
				break
			case 2:
				z.sliceFrom("\u05E0\u05E2\u05DE")
				break
			case 3:
				z.sliceFrom("\u05DE\u05F2\u05D3")
				break
			case 4:
				z.sliceFrom("\u05D1\u05F2\u05D8")
				break
			case 5:
				z.sliceFrom("\u05D1\u05F2\u05E1")
				break
			case 6:
				z.sliceFrom("\u05F0\u05F2\u05D6")
				break
			case 7:
				z.sliceFrom("\u05D8\u05E8\u05F2\u05D1")
				break
			case 8:
				z.sliceFrom("\u05DC\u05F2\u05D8")
				break
			case 9:
				z.sliceFrom("\u05E7\u05DC\u05F2\u05D1")
				break
			case 10:
				z.sliceFrom("\u05E8\u05F2\u05D1")
				break
			case 11:
				z.sliceFrom("\u05E8\u05F2\u05E1")
				break
			case 12:
				z.sliceFrom("\u05E9\u05F0\u05F2\u05D2")
				break
			case 13:
				z.sliceFrom("\u05E9\u05DE\u05F2\u05E1")
				break
			case 14:
				z.sliceFrom("\u05E9\u05E0\u05F2\u05D3")
				break
			case 15:
				z.sliceFrom("\u05E9\u05E8\u05F2\u05D1")
				break
			case 16:
				z.sliceFrom("\u05D1\u05D9\u05E0\u05D3")
				break
			case 17:
				z.sliceFrom("\u05F0\u05D9\u05D8\u05E9")
				break
			case 18:
				z.sliceFrom("\u05D6\u05D9\u05E0\u05D2")
				break
			case 19:
				z.sliceFrom("\u05D8\u05E8\u05D9\u05E0\u05E7")
				break
			case 20:
				z.sliceFrom("\u05E6\u05F0\u05D9\u05E0\u05D2")
				break
			case 21:
				z.sliceFrom("\u05E9\u05DC\u05D9\u05E0\u05D2")
				break
			case 22:
				z.sliceFrom("\u05D1\u05F2\u05D2")
				break
			case 23:
				z.sliceFrom("\u05D4\u05F2\u05D1")
				break
			case 24:
				z.sliceFrom("\u05E4\u05D0\u05E8\u05DC\u05D9\u05E8")
				break
			case 25:
				z.sliceFrom("\u05E9\u05D8\u05F2")
				break
			case 26:
				z.sliceFrom("\u05E9\u05F0\u05E2\u05E8")
				break
			}
			break
		case 4:
			{
				v_2 := z.limit - z.cursor
				{
					if !z.r_R1() {
						goto lab2_end
					}
					z.sliceDel()
					goto lab1_end
				}
			lab2_end:
				;
				z.cursor = z.limit - v_2
				z.sliceFrom("\u05D8")
			}
		lab1_end:
			;
			z.ket = z.cursor
			if !(z.eqSB("\u05D1\u05E8\u05D0\u05DB")) {
				goto lab0_end
			}
			v_3 := z.limit - z.cursor
			{
				if !(z.eqSB("\u05D2\u05E2")) {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
			}
		lab3_end:
			;
			z.bra = z.cursor
			z.sliceFrom("\u05D1\u05E8\u05E2\u05E0\u05D2")
			break
		case 5:
			z.sliceFrom("\u05D2\u05F2")
			break
		case 6:
			z.sliceFrom("\u05E0\u05E2\u05DE")
			break
		case 7:
			z.sliceFrom("\u05E9\u05E8\u05F2\u05D1")
			break
		case 8:
			z.sliceFrom("\u05DE\u05F2\u05D3")
			break
		case 9:
			z.sliceFrom("\u05D1\u05F2\u05D8")
			break
		case 10:
			z.sliceFrom("\u05D1\u05F2\u05E1")
			break
		case 11:
			z.sliceFrom("\u05F0\u05F2\u05D6")
			break
		case 12:
			z.sliceFrom("\u05D8\u05E8\u05F2\u05D1")
			break
		case 13:
			z.sliceFrom("\u05DC\u05F2\u05D8")
			break
		case 14:
			z.sliceFrom("\u05E7\u05DC\u05F2\u05D1")
			break
		case 15:
			z.sliceFrom("\u05E8\u05F2\u05D1")
			break
		case 16:
			z.sliceFrom("\u05E8\u05F2\u05E1")
			break
		case 17:
			z.sliceFrom("\u05E9\u05F0\u05F2\u05D2")
			break
		case 18:
			z.sliceFrom("\u05E9\u05DE\u05F2\u05E1")
			break
		case 19:
			z.sliceFrom("\u05E9\u05E0\u05F2\u05D3")
			break
		case 20:
			z.sliceFrom("\u05D1\u05D9\u05E0\u05D3")
			break
		case 21:
			z.sliceFrom("\u05F0\u05D9\u05D8\u05E9")
			break
		case 22:
			z.sliceFrom("\u05D6\u05D9\u05E0\u05D2")
			break
		case 23:
			z.sliceFrom("\u05D8\u05E8\u05D9\u05E0\u05E7")
			break
		case 24:
			z.sliceFrom("\u05E6\u05F0\u05D9\u05E0\u05D2")
			break
		case 25:
			z.sliceFrom("\u05E9\u05DC\u05D9\u05E0\u05D2")
			break
		case 26:
			z.sliceFrom("\u05D1\u05F2\u05D2")
			break
		case 27:
			z.sliceFrom("\u05D4\u05F2\u05D1")
			break
		case 28:
			z.sliceFrom("\u05E4\u05D0\u05E8\u05DC\u05D9\u05E8")
			break
		case 29:
			z.sliceFrom("\u05E9\u05D8\u05F2")
			break
		case 30:
			z.sliceFrom("\u05E9\u05F0\u05E2\u05E8")
			break
		case 31:
			z.sliceFrom("\u05D1\u05E8\u05E2\u05E0\u05D2")
			break
		case 32:
			if !z.r_R1() {
				goto lab0_end
			}
			z.sliceFrom("\u05D4")
			break
		case 33:
			{
				v_4 := z.limit - z.cursor
				{
					{
						v_5 := z.limit - z.cursor
						{
							if !(z.eqSB("\u05D2")) {
								goto lab7_end
							}
							goto lab6_end
						}
					lab7_end:
						;
						z.cursor = z.limit - v_5
						if !(z.eqSB("\u05E9")) {
							goto lab5_end
						}
					}
				lab6_end:
					;
					v_6 := z.limit - z.cursor
					{
						if !z.r_R1plus3() {
							z.cursor = z.limit - v_6
							goto lab8_end
						}
						z.sliceFrom("\u05D9\u05E1")
					}
				lab8_end:
					;
					goto lab4_end
				}
			lab5_end:
				;
				z.cursor = z.limit - v_4
				if !z.r_R1() {
					goto lab0_end
				}
				z.sliceDel()
			}
		lab4_end:
			;
			break
		}
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	v_7 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(yid_a_5)
		if among_var == 0 {
			goto lab9_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			if !z.r_R1() {
				goto lab9_end
			}
			z.sliceDel()
			break
		case 2:
			if !z.r_R1() {
				goto lab9_end
			}
			if !(z.inGroupingB(yid_g_consonant, 1489, 1520)) {
				goto lab9_end
			}
			z.sliceDel()
			break
		}
	}
lab9_end:
	;
	z.cursor = z.limit - v_7
	v_8 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(yid_a_6)
		if among_var == 0 {
			goto lab10_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			if !z.r_R1() {
				goto lab10_end
			}
			z.sliceDel()
			break
		}
	}
lab10_end:
	;
	z.cursor = z.limit - v_8
	v_9 := z.limit - z.cursor
	{
		for {
			v_10 := z.limit - z.cursor
			{
			golab13:
				for {
					v_11 := z.limit - z.cursor
					{
						z.ket = z.cursor
						{
							v_12 := z.limit - z.cursor
							{
								if !(z.eqSB("GE")) {
									goto lab16_end
								}
								goto lab15_end
							}
						lab16_end:
							;
							z.cursor = z.limit - v_12
							if !(z.eqSB("TSU")) {
								goto lab14_end
							}
						}
					lab15_end:
						;
						z.bra = z.cursor
						z.sliceDel()
						z.cursor = z.limit - v_11
						break golab13
					}
				lab14_end:
					;
					z.cursor = z.limit - v_11
					if z.cursor <= z.limitBackward {
						goto lab12_end
					}
					z.cursor--
				}
				continue
			}
		lab12_end:
			;
			z.cursor = z.limit - v_10
			break
		}
	}
	z.cursor = z.limit - v_9
	return true
}
func (z *yiddishStemmer) stem() {
	z.r_prelude()
	v_2 := z.cursor
	z.r_mark_regions()
	z.cursor = v_2
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.r_standard_suffix()
	z.cursor = z.limitBackward
	return
}
