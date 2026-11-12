package main

// Code generated from Apache Lucene's generated snowball stemmers. DO NOT EDIT.

type arabicStemmer struct {
	snowball
	I_p1, I_p2   int
	S_ch, S_x    []rune
	LS_ch, LS_x  int
	B_is_defined bool
	B_is_verb    bool
	B_is_noun    bool
}

func newArabic() *arabicStemmer { return &arabicStemmer{} }

var ara_a_0 = []among{
	{"\u0640", -1, 1, ""},
	{"\u064B", -1, 1, ""},
	{"\u064C", -1, 1, ""},
	{"\u064D", -1, 1, ""},
	{"\u064E", -1, 1, ""},
	{"\u064F", -1, 1, ""},
	{"\u0650", -1, 1, ""},
	{"\u0651", -1, 1, ""},
	{"\u0652", -1, 1, ""},
	{"\u0660", -1, 2, ""},
	{"\u0661", -1, 3, ""},
	{"\u0662", -1, 4, ""},
	{"\u0663", -1, 5, ""},
	{"\u0664", -1, 6, ""},
	{"\u0665", -1, 7, ""},
	{"\u0666", -1, 8, ""},
	{"\u0667", -1, 9, ""},
	{"\u0668", -1, 10, ""},
	{"\u0669", -1, 11, ""},
	{"\uFE80", -1, 12, ""},
	{"\uFE81", -1, 16, ""},
	{"\uFE82", -1, 16, ""},
	{"\uFE83", -1, 13, ""},
	{"\uFE84", -1, 13, ""},
	{"\uFE85", -1, 17, ""},
	{"\uFE86", -1, 17, ""},
	{"\uFE87", -1, 14, ""},
	{"\uFE88", -1, 14, ""},
	{"\uFE89", -1, 15, ""},
	{"\uFE8A", -1, 15, ""},
	{"\uFE8B", -1, 15, ""},
	{"\uFE8C", -1, 15, ""},
	{"\uFE8D", -1, 18, ""},
	{"\uFE8E", -1, 18, ""},
	{"\uFE8F", -1, 19, ""},
	{"\uFE90", -1, 19, ""},
	{"\uFE91", -1, 19, ""},
	{"\uFE92", -1, 19, ""},
	{"\uFE93", -1, 20, ""},
	{"\uFE94", -1, 20, ""},
	{"\uFE95", -1, 21, ""},
	{"\uFE96", -1, 21, ""},
	{"\uFE97", -1, 21, ""},
	{"\uFE98", -1, 21, ""},
	{"\uFE99", -1, 22, ""},
	{"\uFE9A", -1, 22, ""},
	{"\uFE9B", -1, 22, ""},
	{"\uFE9C", -1, 22, ""},
	{"\uFE9D", -1, 23, ""},
	{"\uFE9E", -1, 23, ""},
	{"\uFE9F", -1, 23, ""},
	{"\uFEA0", -1, 23, ""},
	{"\uFEA1", -1, 24, ""},
	{"\uFEA2", -1, 24, ""},
	{"\uFEA3", -1, 24, ""},
	{"\uFEA4", -1, 24, ""},
	{"\uFEA5", -1, 25, ""},
	{"\uFEA6", -1, 25, ""},
	{"\uFEA7", -1, 25, ""},
	{"\uFEA8", -1, 25, ""},
	{"\uFEA9", -1, 26, ""},
	{"\uFEAA", -1, 26, ""},
	{"\uFEAB", -1, 27, ""},
	{"\uFEAC", -1, 27, ""},
	{"\uFEAD", -1, 28, ""},
	{"\uFEAE", -1, 28, ""},
	{"\uFEAF", -1, 29, ""},
	{"\uFEB0", -1, 29, ""},
	{"\uFEB1", -1, 30, ""},
	{"\uFEB2", -1, 30, ""},
	{"\uFEB3", -1, 30, ""},
	{"\uFEB4", -1, 30, ""},
	{"\uFEB5", -1, 31, ""},
	{"\uFEB6", -1, 31, ""},
	{"\uFEB7", -1, 31, ""},
	{"\uFEB8", -1, 31, ""},
	{"\uFEB9", -1, 32, ""},
	{"\uFEBA", -1, 32, ""},
	{"\uFEBB", -1, 32, ""},
	{"\uFEBC", -1, 32, ""},
	{"\uFEBD", -1, 33, ""},
	{"\uFEBE", -1, 33, ""},
	{"\uFEBF", -1, 33, ""},
	{"\uFEC0", -1, 33, ""},
	{"\uFEC1", -1, 34, ""},
	{"\uFEC2", -1, 34, ""},
	{"\uFEC3", -1, 34, ""},
	{"\uFEC4", -1, 34, ""},
	{"\uFEC5", -1, 35, ""},
	{"\uFEC6", -1, 35, ""},
	{"\uFEC7", -1, 35, ""},
	{"\uFEC8", -1, 35, ""},
	{"\uFEC9", -1, 36, ""},
	{"\uFECA", -1, 36, ""},
	{"\uFECB", -1, 36, ""},
	{"\uFECC", -1, 36, ""},
	{"\uFECD", -1, 37, ""},
	{"\uFECE", -1, 37, ""},
	{"\uFECF", -1, 37, ""},
	{"\uFED0", -1, 37, ""},
	{"\uFED1", -1, 38, ""},
	{"\uFED2", -1, 38, ""},
	{"\uFED3", -1, 38, ""},
	{"\uFED4", -1, 38, ""},
	{"\uFED5", -1, 39, ""},
	{"\uFED6", -1, 39, ""},
	{"\uFED7", -1, 39, ""},
	{"\uFED8", -1, 39, ""},
	{"\uFED9", -1, 40, ""},
	{"\uFEDA", -1, 40, ""},
	{"\uFEDB", -1, 40, ""},
	{"\uFEDC", -1, 40, ""},
	{"\uFEDD", -1, 41, ""},
	{"\uFEDE", -1, 41, ""},
	{"\uFEDF", -1, 41, ""},
	{"\uFEE0", -1, 41, ""},
	{"\uFEE1", -1, 42, ""},
	{"\uFEE2", -1, 42, ""},
	{"\uFEE3", -1, 42, ""},
	{"\uFEE4", -1, 42, ""},
	{"\uFEE5", -1, 43, ""},
	{"\uFEE6", -1, 43, ""},
	{"\uFEE7", -1, 43, ""},
	{"\uFEE8", -1, 43, ""},
	{"\uFEE9", -1, 44, ""},
	{"\uFEEA", -1, 44, ""},
	{"\uFEEB", -1, 44, ""},
	{"\uFEEC", -1, 44, ""},
	{"\uFEED", -1, 45, ""},
	{"\uFEEE", -1, 45, ""},
	{"\uFEEF", -1, 46, ""},
	{"\uFEF0", -1, 46, ""},
	{"\uFEF1", -1, 47, ""},
	{"\uFEF2", -1, 47, ""},
	{"\uFEF3", -1, 47, ""},
	{"\uFEF4", -1, 47, ""},
	{"\uFEF5", -1, 51, ""},
	{"\uFEF6", -1, 51, ""},
	{"\uFEF7", -1, 49, ""},
	{"\uFEF8", -1, 49, ""},
	{"\uFEF9", -1, 50, ""},
	{"\uFEFA", -1, 50, ""},
	{"\uFEFB", -1, 48, ""},
	{"\uFEFC", -1, 48, ""},
}
var ara_a_1 = []among{
	{"\u0622", -1, 1, ""},
	{"\u0623", -1, 1, ""},
	{"\u0624", -1, 1, ""},
	{"\u0625", -1, 1, ""},
	{"\u0626", -1, 1, ""},
}
var ara_a_2 = []among{
	{"\u0622", -1, 1, ""},
	{"\u0623", -1, 1, ""},
	{"\u0624", -1, 2, ""},
	{"\u0625", -1, 1, ""},
	{"\u0626", -1, 3, ""},
}
var ara_a_3 = []among{
	{"\u0627\u0644", -1, 2, ""},
	{"\u0628\u0627\u0644", -1, 1, ""},
	{"\u0643\u0627\u0644", -1, 1, ""},
	{"\u0644\u0644", -1, 2, ""},
}
var ara_a_4 = []among{
	{"\u0623\u0622", -1, 2, ""},
	{"\u0623\u0623", -1, 1, ""},
	{"\u0623\u0624", -1, 1, ""},
	{"\u0623\u0625", -1, 4, ""},
	{"\u0623\u0627", -1, 3, ""},
}
var ara_a_5 = []among{
	{"\u0641", -1, 1, ""},
	{"\u0648", -1, 1, ""},
}
var ara_a_6 = []among{
	{"\u0627\u0644", -1, 2, ""},
	{"\u0628\u0627\u0644", -1, 1, ""},
	{"\u0643\u0627\u0644", -1, 1, ""},
	{"\u0644\u0644", -1, 2, ""},
}
var ara_a_7 = []among{
	{"\u0628", -1, 1, ""},
	{"\u0628\u0627", 0, -1, ""},
	{"\u0628\u0628", 0, 2, ""},
	{"\u0643\u0643", -1, 3, ""},
}
var ara_a_8 = []among{
	{"\u0633\u0623", -1, 4, ""},
	{"\u0633\u062A", -1, 2, ""},
	{"\u0633\u0646", -1, 3, ""},
	{"\u0633\u064A", -1, 1, ""},
}
var ara_a_9 = []among{
	{"\u062A\u0633\u062A", -1, 1, ""},
	{"\u0646\u0633\u062A", -1, 1, ""},
	{"\u064A\u0633\u062A", -1, 1, ""},
}
var ara_a_10 = []among{
	{"\u0643\u0645\u0627", -1, 3, ""},
	{"\u0647\u0645\u0627", -1, 3, ""},
	{"\u0646\u0627", -1, 2, ""},
	{"\u0647\u0627", -1, 2, ""},
	{"\u0643", -1, 1, ""},
	{"\u0643\u0645", -1, 2, ""},
	{"\u0647\u0645", -1, 2, ""},
	{"\u0647\u0646", -1, 2, ""},
	{"\u0647", -1, 1, ""},
	{"\u064A", -1, 1, ""},
}
var ara_a_11 = []among{
	{"\u0627", -1, 1, ""},
	{"\u0648", -1, 1, ""},
	{"\u064A", -1, 1, ""},
}
var ara_a_12 = []among{
	{"\u0643\u0645\u0627", -1, 3, ""},
	{"\u0647\u0645\u0627", -1, 3, ""},
	{"\u0646\u0627", -1, 2, ""},
	{"\u0647\u0627", -1, 2, ""},
	{"\u0643", -1, 1, ""},
	{"\u0643\u0645", -1, 2, ""},
	{"\u0647\u0645", -1, 2, ""},
	{"\u0643\u0646", -1, 2, ""},
	{"\u0647\u0646", -1, 2, ""},
	{"\u0647", -1, 1, ""},
	{"\u0643\u0645\u0648", -1, 3, ""},
	{"\u0646\u064A", -1, 2, ""},
}
var ara_a_13 = []among{
	{"\u0627", -1, 1, ""},
	{"\u062A\u0627", 0, 2, ""},
	{"\u062A\u0645\u0627", 0, 4, ""},
	{"\u0646\u0627", 0, 2, ""},
	{"\u062A", -1, 1, ""},
	{"\u0646", -1, 1, ""},
	{"\u0627\u0646", 5, 3, ""},
	{"\u062A\u0646", 5, 2, ""},
	{"\u0648\u0646", 5, 3, ""},
	{"\u064A\u0646", 5, 3, ""},
	{"\u064A", -1, 1, ""},
}
var ara_a_14 = []among{
	{"\u0648\u0627", -1, 1, ""},
	{"\u062A\u0645", -1, 1, ""},
}
var ara_a_15 = []among{
	{"\u0648", -1, 1, ""},
	{"\u062A\u0645\u0648", 0, 2, ""},
}

func (z *arabicStemmer) r_Normalize_pre() bool {
	var among_var int
	v_1 := z.cursor
	{
		for {
			v_2 := z.cursor
			{
				{
					v_3 := z.cursor
					{
						z.bra = z.cursor
						among_var = z.findAmong(ara_a_0)
						if among_var == 0 {
							goto lab3_end
						}
						z.ket = z.cursor
						switch among_var {
						case 1:
							z.sliceDel()
							break
						case 2:
							z.sliceFrom("0")
							break
						case 3:
							z.sliceFrom("1")
							break
						case 4:
							z.sliceFrom("2")
							break
						case 5:
							z.sliceFrom("3")
							break
						case 6:
							z.sliceFrom("4")
							break
						case 7:
							z.sliceFrom("5")
							break
						case 8:
							z.sliceFrom("6")
							break
						case 9:
							z.sliceFrom("7")
							break
						case 10:
							z.sliceFrom("8")
							break
						case 11:
							z.sliceFrom("9")
							break
						case 12:
							z.sliceFrom("\u0621")
							break
						case 13:
							z.sliceFrom("\u0623")
							break
						case 14:
							z.sliceFrom("\u0625")
							break
						case 15:
							z.sliceFrom("\u0626")
							break
						case 16:
							z.sliceFrom("\u0622")
							break
						case 17:
							z.sliceFrom("\u0624")
							break
						case 18:
							z.sliceFrom("\u0627")
							break
						case 19:
							z.sliceFrom("\u0628")
							break
						case 20:
							z.sliceFrom("\u0629")
							break
						case 21:
							z.sliceFrom("\u062A")
							break
						case 22:
							z.sliceFrom("\u062B")
							break
						case 23:
							z.sliceFrom("\u062C")
							break
						case 24:
							z.sliceFrom("\u062D")
							break
						case 25:
							z.sliceFrom("\u062E")
							break
						case 26:
							z.sliceFrom("\u062F")
							break
						case 27:
							z.sliceFrom("\u0630")
							break
						case 28:
							z.sliceFrom("\u0631")
							break
						case 29:
							z.sliceFrom("\u0632")
							break
						case 30:
							z.sliceFrom("\u0633")
							break
						case 31:
							z.sliceFrom("\u0634")
							break
						case 32:
							z.sliceFrom("\u0635")
							break
						case 33:
							z.sliceFrom("\u0636")
							break
						case 34:
							z.sliceFrom("\u0637")
							break
						case 35:
							z.sliceFrom("\u0638")
							break
						case 36:
							z.sliceFrom("\u0639")
							break
						case 37:
							z.sliceFrom("\u063A")
							break
						case 38:
							z.sliceFrom("\u0641")
							break
						case 39:
							z.sliceFrom("\u0642")
							break
						case 40:
							z.sliceFrom("\u0643")
							break
						case 41:
							z.sliceFrom("\u0644")
							break
						case 42:
							z.sliceFrom("\u0645")
							break
						case 43:
							z.sliceFrom("\u0646")
							break
						case 44:
							z.sliceFrom("\u0647")
							break
						case 45:
							z.sliceFrom("\u0648")
							break
						case 46:
							z.sliceFrom("\u0649")
							break
						case 47:
							z.sliceFrom("\u064A")
							break
						case 48:
							z.sliceFrom("\u0644\u0627")
							break
						case 49:
							z.sliceFrom("\u0644\u0623")
							break
						case 50:
							z.sliceFrom("\u0644\u0625")
							break
						case 51:
							z.sliceFrom("\u0644\u0622")
							break
						}
						goto lab2_end
					}
				lab3_end:
					;
					z.cursor = v_3
					if z.cursor >= z.limit {
						goto lab1_end
					}
					z.cursor++
				}
			lab2_end:
				;
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
func (z *arabicStemmer) r_Normalize_post() bool {
	var among_var int
	v_1 := z.cursor
	{
		z.limitBackward = z.cursor
		z.cursor = z.limit
		z.ket = z.cursor
		if z.findAmongB(ara_a_1) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceFrom("\u0621")
		z.cursor = z.limitBackward
	}
lab0_end:
	;
	z.cursor = v_1
	v_2 := z.cursor
	{
		for {
			v_3 := z.cursor
			{
				{
					v_4 := z.cursor
					{
						z.bra = z.cursor
						among_var = z.findAmong(ara_a_2)
						if among_var == 0 {
							goto lab4_end
						}
						z.ket = z.cursor
						switch among_var {
						case 1:
							z.sliceFrom("\u0627")
							break
						case 2:
							z.sliceFrom("\u0648")
							break
						case 3:
							z.sliceFrom("\u064A")
							break
						}
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_4
					if z.cursor >= z.limit {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				continue
			}
		lab2_end:
			;
			z.cursor = v_3
			break
		}
	}
	z.cursor = v_2
	return true
}
func (z *arabicStemmer) r_Checks1() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ara_a_3)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		if z.length <= 4 {
			return false
		}
		z.B_is_noun = true
		z.B_is_verb = false
		z.B_is_defined = true
		break
	case 2:
		if z.length <= 3 {
			return false
		}
		z.B_is_noun = true
		z.B_is_verb = false
		z.B_is_defined = true
		break
	}
	return true
}
func (z *arabicStemmer) r_Prefix_Step1() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ara_a_4)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0623")
		break
	case 2:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0622")
		break
	case 3:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0627")
		break
	case 4:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0625")
		break
	}
	return true
}
func (z *arabicStemmer) r_Prefix_Step2() bool {
	z.bra = z.cursor
	if z.findAmong(ara_a_5) == 0 {
		return false
	}
	z.ket = z.cursor
	if z.length <= 3 {
		return false
	}
	{
		v_1 := z.cursor
		{
			if !(z.eqS("\u0627")) {
				goto lab0_end
			}
			return false
		}
	lab0_end:
		;
		z.cursor = v_1
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Prefix_Step3a_Noun() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ara_a_6)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		if z.length <= 5 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length <= 4 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *arabicStemmer) r_Prefix_Step3b_Noun() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ara_a_7)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		if z.length <= 3 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0628")
		break
	case 3:
		if z.length <= 3 {
			return false
		}
		z.sliceFrom("\u0643")
		break
	}
	return true
}
func (z *arabicStemmer) r_Prefix_Step3_Verb() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ara_a_8)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		if z.length <= 4 {
			return false
		}
		z.sliceFrom("\u064A")
		break
	case 2:
		if z.length <= 4 {
			return false
		}
		z.sliceFrom("\u062A")
		break
	case 3:
		if z.length <= 4 {
			return false
		}
		z.sliceFrom("\u0646")
		break
	case 4:
		if z.length <= 4 {
			return false
		}
		z.sliceFrom("\u0623")
		break
	}
	return true
}
func (z *arabicStemmer) r_Prefix_Step4_Verb() bool {
	z.bra = z.cursor
	if z.findAmong(ara_a_9) == 0 {
		return false
	}
	z.ket = z.cursor
	if z.length <= 4 {
		return false
	}
	z.B_is_verb = true
	z.B_is_noun = false
	z.sliceFrom("\u0627\u0633\u062A")
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step1a() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ara_a_10)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if z.length < 4 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length < 5 {
			return false
		}
		z.sliceDel()
		break
	case 3:
		if z.length < 6 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step1b() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u0646")) {
		return false
	}
	z.bra = z.cursor
	if z.length <= 5 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step2a() bool {
	z.ket = z.cursor
	if z.findAmongB(ara_a_11) == 0 {
		return false
	}
	z.bra = z.cursor
	if z.length <= 4 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step2b() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u0627\u062A")) {
		return false
	}
	z.bra = z.cursor
	if z.length < 5 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step2c1() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u062A")) {
		return false
	}
	z.bra = z.cursor
	if z.length < 4 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step2c2() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u0629")) {
		return false
	}
	z.bra = z.cursor
	if z.length < 4 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Noun_Step3() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u064A")) {
		return false
	}
	z.bra = z.cursor
	if z.length < 3 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Verb_Step1() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ara_a_12)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if z.length < 4 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length < 5 {
			return false
		}
		z.sliceDel()
		break
	case 3:
		if z.length < 6 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *arabicStemmer) r_Suffix_Verb_Step2a() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ara_a_13)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if z.length < 4 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length < 5 {
			return false
		}
		z.sliceDel()
		break
	case 3:
		if z.length <= 5 {
			return false
		}
		z.sliceDel()
		break
	case 4:
		if z.length < 6 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *arabicStemmer) r_Suffix_Verb_Step2b() bool {
	z.ket = z.cursor
	if z.findAmongB(ara_a_14) == 0 {
		return false
	}
	z.bra = z.cursor
	if z.length < 5 {
		return false
	}
	z.sliceDel()
	return true
}
func (z *arabicStemmer) r_Suffix_Verb_Step2c() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ara_a_15)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if z.length < 4 {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if z.length < 6 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *arabicStemmer) r_Suffix_All_alef_maqsura() bool {
	z.ket = z.cursor
	if !(z.eqSB("\u0649")) {
		return false
	}
	z.bra = z.cursor
	z.sliceFrom("\u064A")
	return true
}
func (z *arabicStemmer) stem() {
	z.B_is_noun = true
	z.B_is_verb = true
	z.B_is_defined = false
	v_1 := z.cursor
	z.r_Checks1()
	z.cursor = v_1
	z.r_Normalize_pre()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	{
		{
			v_3 := z.limit - z.cursor
			{
				if !z.B_is_verb {
					goto lab2_end
				}
				{
					v_4 := z.limit - z.cursor
					{
						{
							v_5 := 1
							for {
								v_6 := z.limit - z.cursor
								{
									if !z.r_Suffix_Verb_Step1() {
										goto lab5_end
									}
									v_5--
									continue
								}
							lab5_end:
								;
								z.cursor = z.limit - v_6
								break
							}
							if v_5 > 0 {
								goto lab4_end
							}
						}
						{
							v_7 := z.limit - z.cursor
							{
								if !z.r_Suffix_Verb_Step2a() {
									goto lab7_end
								}
								goto lab6_end
							}
						lab7_end:
							;
							z.cursor = z.limit - v_7
							{
								if !z.r_Suffix_Verb_Step2c() {
									goto lab8_end
								}
								goto lab6_end
							}
						lab8_end:
							;
							z.cursor = z.limit - v_7
							if z.cursor <= z.limitBackward {
								goto lab4_end
							}
							z.cursor--
						}
					lab6_end:
						;
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = z.limit - v_4
					{
						if !z.r_Suffix_Verb_Step2b() {
							goto lab9_end
						}
						goto lab3_end
					}
				lab9_end:
					;
					z.cursor = z.limit - v_4
					if !z.r_Suffix_Verb_Step2a() {
						goto lab2_end
					}
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			{
				if !z.B_is_noun {
					goto lab10_end
				}
				v_8 := z.limit - z.cursor
				{
					{
						v_9 := z.limit - z.cursor
						{
							if !z.r_Suffix_Noun_Step2c2() {
								goto lab13_end
							}
							goto lab12_end
						}
					lab13_end:
						;
						z.cursor = z.limit - v_9
						{
							if z.B_is_defined {
								goto lab14_end
							}
							if !z.r_Suffix_Noun_Step1a() {
								goto lab14_end
							}
							{
								v_10 := z.limit - z.cursor
								{
									if !z.r_Suffix_Noun_Step2a() {
										goto lab16_end
									}
									goto lab15_end
								}
							lab16_end:
								;
								z.cursor = z.limit - v_10
								{
									if !z.r_Suffix_Noun_Step2b() {
										goto lab17_end
									}
									goto lab15_end
								}
							lab17_end:
								;
								z.cursor = z.limit - v_10
								{
									if !z.r_Suffix_Noun_Step2c1() {
										goto lab18_end
									}
									goto lab15_end
								}
							lab18_end:
								;
								z.cursor = z.limit - v_10
								if z.cursor <= z.limitBackward {
									goto lab14_end
								}
								z.cursor--
							}
						lab15_end:
							;
							goto lab12_end
						}
					lab14_end:
						;
						z.cursor = z.limit - v_9
						{
							if !z.r_Suffix_Noun_Step1b() {
								goto lab19_end
							}
							{
								v_11 := z.limit - z.cursor
								{
									if !z.r_Suffix_Noun_Step2a() {
										goto lab21_end
									}
									goto lab20_end
								}
							lab21_end:
								;
								z.cursor = z.limit - v_11
								{
									if !z.r_Suffix_Noun_Step2b() {
										goto lab22_end
									}
									goto lab20_end
								}
							lab22_end:
								;
								z.cursor = z.limit - v_11
								if !z.r_Suffix_Noun_Step2c1() {
									goto lab19_end
								}
							}
						lab20_end:
							;
							goto lab12_end
						}
					lab19_end:
						;
						z.cursor = z.limit - v_9
						{
							if z.B_is_defined {
								goto lab23_end
							}
							if !z.r_Suffix_Noun_Step2a() {
								goto lab23_end
							}
							goto lab12_end
						}
					lab23_end:
						;
						z.cursor = z.limit - v_9
						if !z.r_Suffix_Noun_Step2b() {
							z.cursor = z.limit - v_8
							goto lab11_end
						}
					}
				lab12_end:
				}
			lab11_end:
				;
				if !z.r_Suffix_Noun_Step3() {
					goto lab10_end
				}
				goto lab1_end
			}
		lab10_end:
			;
			z.cursor = z.limit - v_3
			if !z.r_Suffix_All_alef_maqsura() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	z.cursor = z.limitBackward
	v_12 := z.cursor
	{
		v_13 := z.cursor
		{
			if !z.r_Prefix_Step1() {
				z.cursor = v_13
				goto lab25_end
			}
		}
	lab25_end:
		;
		v_14 := z.cursor
		{
			if !z.r_Prefix_Step2() {
				z.cursor = v_14
				goto lab26_end
			}
		}
	lab26_end:
		;
		{
			v_15 := z.cursor
			{
				if !z.r_Prefix_Step3a_Noun() {
					goto lab28_end
				}
				goto lab27_end
			}
		lab28_end:
			;
			z.cursor = v_15
			{
				if !z.B_is_noun {
					goto lab29_end
				}
				if !z.r_Prefix_Step3b_Noun() {
					goto lab29_end
				}
				goto lab27_end
			}
		lab29_end:
			;
			z.cursor = v_15
			if !z.B_is_verb {
				goto lab24_end
			}
			v_16 := z.cursor
			{
				if !z.r_Prefix_Step3_Verb() {
					z.cursor = v_16
					goto lab30_end
				}
			}
		lab30_end:
			;
			if !z.r_Prefix_Step4_Verb() {
				goto lab24_end
			}
		}
	lab27_end:
	}
lab24_end:
	;
	z.cursor = v_12
	z.r_Normalize_post()
	return
}

type armenianStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newArmenian() *armenianStemmer { return &armenianStemmer{} }

var arm_a_0 = []among{
	{"\u0580\u0578\u0580\u0564", -1, 1, ""},
	{"\u0565\u0580\u0578\u0580\u0564", 0, 1, ""},
	{"\u0561\u056C\u056B", -1, 1, ""},
	{"\u0561\u056F\u056B", -1, 1, ""},
	{"\u0578\u0580\u0561\u056F", -1, 1, ""},
	{"\u0565\u0572", -1, 1, ""},
	{"\u0561\u056F\u0561\u0576", -1, 1, ""},
	{"\u0561\u0580\u0561\u0576", -1, 1, ""},
	{"\u0565\u0576", -1, 1, ""},
	{"\u0565\u056F\u0565\u0576", 8, 1, ""},
	{"\u0565\u0580\u0565\u0576", 8, 1, ""},
	{"\u0578\u0580\u0567\u0576", -1, 1, ""},
	{"\u056B\u0576", -1, 1, ""},
	{"\u0563\u056B\u0576", 12, 1, ""},
	{"\u0578\u057E\u056B\u0576", 12, 1, ""},
	{"\u056C\u0561\u0575\u0576", -1, 1, ""},
	{"\u057E\u0578\u0582\u0576", -1, 1, ""},
	{"\u057A\u0565\u057D", -1, 1, ""},
	{"\u056B\u057E", -1, 1, ""},
	{"\u0561\u057F", -1, 1, ""},
	{"\u0561\u057E\u0565\u057F", -1, 1, ""},
	{"\u056F\u0578\u057F", -1, 1, ""},
	{"\u0562\u0561\u0580", -1, 1, ""},
}
var arm_a_1 = []among{
	{"\u0561", -1, 1, ""},
	{"\u0561\u0581\u0561", 0, 1, ""},
	{"\u0565\u0581\u0561", 0, 1, ""},
	{"\u057E\u0565", -1, 1, ""},
	{"\u0561\u0581\u0580\u056B", -1, 1, ""},
	{"\u0561\u0581\u056B", -1, 1, ""},
	{"\u0565\u0581\u056B", -1, 1, ""},
	{"\u057E\u0565\u0581\u056B", 6, 1, ""},
	{"\u0561\u056C", -1, 1, ""},
	{"\u0568\u0561\u056C", 8, 1, ""},
	{"\u0561\u0576\u0561\u056C", 8, 1, ""},
	{"\u0565\u0576\u0561\u056C", 8, 1, ""},
	{"\u0561\u0581\u0576\u0561\u056C", 8, 1, ""},
	{"\u0565\u056C", -1, 1, ""},
	{"\u0568\u0565\u056C", 13, 1, ""},
	{"\u0576\u0565\u056C", 13, 1, ""},
	{"\u0581\u0576\u0565\u056C", 15, 1, ""},
	{"\u0565\u0581\u0576\u0565\u056C", 16, 1, ""},
	{"\u0579\u0565\u056C", 13, 1, ""},
	{"\u057E\u0565\u056C", 13, 1, ""},
	{"\u0561\u0581\u057E\u0565\u056C", 19, 1, ""},
	{"\u0565\u0581\u057E\u0565\u056C", 19, 1, ""},
	{"\u057F\u0565\u056C", 13, 1, ""},
	{"\u0561\u057F\u0565\u056C", 22, 1, ""},
	{"\u0578\u057F\u0565\u056C", 22, 1, ""},
	{"\u056F\u0578\u057F\u0565\u056C", 24, 1, ""},
	{"\u057E\u0561\u056E", -1, 1, ""},
	{"\u0578\u0582\u0574", -1, 1, ""},
	{"\u057E\u0578\u0582\u0574", 27, 1, ""},
	{"\u0561\u0576", -1, 1, ""},
	{"\u0581\u0561\u0576", 29, 1, ""},
	{"\u0561\u0581\u0561\u0576", 30, 1, ""},
	{"\u0561\u0581\u0580\u056B\u0576", -1, 1, ""},
	{"\u0561\u0581\u056B\u0576", -1, 1, ""},
	{"\u0565\u0581\u056B\u0576", -1, 1, ""},
	{"\u057E\u0565\u0581\u056B\u0576", 34, 1, ""},
	{"\u0561\u056C\u056B\u057D", -1, 1, ""},
	{"\u0565\u056C\u056B\u057D", -1, 1, ""},
	{"\u0561\u057E", -1, 1, ""},
	{"\u0561\u0581\u0561\u057E", 38, 1, ""},
	{"\u0565\u0581\u0561\u057E", 38, 1, ""},
	{"\u0561\u056C\u0578\u057E", -1, 1, ""},
	{"\u0565\u056C\u0578\u057E", -1, 1, ""},
	{"\u0561\u0580", -1, 1, ""},
	{"\u0561\u0581\u0561\u0580", 43, 1, ""},
	{"\u0565\u0581\u0561\u0580", 43, 1, ""},
	{"\u0561\u0581\u0580\u056B\u0580", -1, 1, ""},
	{"\u0561\u0581\u056B\u0580", -1, 1, ""},
	{"\u0565\u0581\u056B\u0580", -1, 1, ""},
	{"\u057E\u0565\u0581\u056B\u0580", 48, 1, ""},
	{"\u0561\u0581", -1, 1, ""},
	{"\u0565\u0581", -1, 1, ""},
	{"\u0561\u0581\u0580\u0565\u0581", 51, 1, ""},
	{"\u0561\u056C\u0578\u0582\u0581", -1, 1, ""},
	{"\u0565\u056C\u0578\u0582\u0581", -1, 1, ""},
	{"\u0561\u056C\u0578\u0582", -1, 1, ""},
	{"\u0565\u056C\u0578\u0582", -1, 1, ""},
	{"\u0561\u0584", -1, 1, ""},
	{"\u0581\u0561\u0584", 57, 1, ""},
	{"\u0561\u0581\u0561\u0584", 58, 1, ""},
	{"\u0561\u0581\u0580\u056B\u0584", -1, 1, ""},
	{"\u0561\u0581\u056B\u0584", -1, 1, ""},
	{"\u0565\u0581\u056B\u0584", -1, 1, ""},
	{"\u057E\u0565\u0581\u056B\u0584", 62, 1, ""},
	{"\u0561\u0576\u0584", -1, 1, ""},
	{"\u0581\u0561\u0576\u0584", 64, 1, ""},
	{"\u0561\u0581\u0561\u0576\u0584", 65, 1, ""},
	{"\u0561\u0581\u0580\u056B\u0576\u0584", -1, 1, ""},
	{"\u0561\u0581\u056B\u0576\u0584", -1, 1, ""},
	{"\u0565\u0581\u056B\u0576\u0584", -1, 1, ""},
	{"\u057E\u0565\u0581\u056B\u0576\u0584", 69, 1, ""},
}
var arm_a_2 = []among{
	{"\u0578\u0580\u0564", -1, 1, ""},
	{"\u0578\u0582\u0575\u0569", -1, 1, ""},
	{"\u0578\u0582\u0570\u056B", -1, 1, ""},
	{"\u0581\u056B", -1, 1, ""},
	{"\u056B\u056C", -1, 1, ""},
	{"\u0561\u056F", -1, 1, ""},
	{"\u0575\u0561\u056F", 5, 1, ""},
	{"\u0561\u0576\u0561\u056F", 5, 1, ""},
	{"\u056B\u056F", -1, 1, ""},
	{"\u0578\u0582\u056F", -1, 1, ""},
	{"\u0561\u0576", -1, 1, ""},
	{"\u057A\u0561\u0576", 10, 1, ""},
	{"\u057D\u057F\u0561\u0576", 10, 1, ""},
	{"\u0561\u0580\u0561\u0576", 10, 1, ""},
	{"\u0565\u0572\u0567\u0576", -1, 1, ""},
	{"\u0575\u0578\u0582\u0576", -1, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0578\u0582\u0576", 15, 1, ""},
	{"\u0561\u056E\u0578", -1, 1, ""},
	{"\u056B\u0579", -1, 1, ""},
	{"\u0578\u0582\u057D", -1, 1, ""},
	{"\u0578\u0582\u057D\u057F", -1, 1, ""},
	{"\u0563\u0561\u0580", -1, 1, ""},
	{"\u057E\u0578\u0580", -1, 1, ""},
	{"\u0561\u057E\u0578\u0580", 22, 1, ""},
	{"\u0578\u0581", -1, 1, ""},
	{"\u0561\u0576\u0585\u0581", -1, 1, ""},
	{"\u0578\u0582", -1, 1, ""},
	{"\u0584", -1, 1, ""},
	{"\u0579\u0565\u0584", 27, 1, ""},
	{"\u056B\u0584", 27, 1, ""},
	{"\u0561\u056C\u056B\u0584", 29, 1, ""},
	{"\u0561\u0576\u056B\u0584", 29, 1, ""},
	{"\u057E\u0561\u056E\u0584", 27, 1, ""},
	{"\u0578\u0582\u0575\u0584", 27, 1, ""},
	{"\u0565\u0576\u0584", 27, 1, ""},
	{"\u0578\u0576\u0584", 27, 1, ""},
	{"\u0578\u0582\u0576\u0584", 27, 1, ""},
	{"\u0574\u0578\u0582\u0576\u0584", 36, 1, ""},
	{"\u056B\u0579\u0584", 27, 1, ""},
	{"\u0561\u0580\u0584", 27, 1, ""},
}
var arm_a_3 = []among{
	{"\u057D\u0561", -1, 1, ""},
	{"\u057E\u0561", -1, 1, ""},
	{"\u0561\u0574\u0562", -1, 1, ""},
	{"\u0564", -1, 1, ""},
	{"\u0561\u0576\u0564", 3, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0561\u0576\u0564", 4, 1, ""},
	{"\u057E\u0561\u0576\u0564", 4, 1, ""},
	{"\u0578\u057B\u0564", 3, 1, ""},
	{"\u0565\u0580\u0564", 3, 1, ""},
	{"\u0576\u0565\u0580\u0564", 8, 1, ""},
	{"\u0578\u0582\u0564", 3, 1, ""},
	{"\u0568", -1, 1, ""},
	{"\u0561\u0576\u0568", 11, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0561\u0576\u0568", 12, 1, ""},
	{"\u057E\u0561\u0576\u0568", 12, 1, ""},
	{"\u0578\u057B\u0568", 11, 1, ""},
	{"\u0565\u0580\u0568", 11, 1, ""},
	{"\u0576\u0565\u0580\u0568", 16, 1, ""},
	{"\u056B", -1, 1, ""},
	{"\u057E\u056B", 18, 1, ""},
	{"\u0565\u0580\u056B", 18, 1, ""},
	{"\u0576\u0565\u0580\u056B", 20, 1, ""},
	{"\u0561\u0576\u0578\u0582\u0574", -1, 1, ""},
	{"\u0565\u0580\u0578\u0582\u0574", -1, 1, ""},
	{"\u0576\u0565\u0580\u0578\u0582\u0574", 23, 1, ""},
	{"\u0576", -1, 1, ""},
	{"\u0561\u0576", 25, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0561\u0576", 26, 1, ""},
	{"\u057E\u0561\u0576", 26, 1, ""},
	{"\u056B\u0576", 25, 1, ""},
	{"\u0565\u0580\u056B\u0576", 29, 1, ""},
	{"\u0576\u0565\u0580\u056B\u0576", 30, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0561\u0576\u0576", 25, 1, ""},
	{"\u0565\u0580\u0576", 25, 1, ""},
	{"\u0576\u0565\u0580\u0576", 33, 1, ""},
	{"\u0578\u0582\u0576", 25, 1, ""},
	{"\u0578\u057B", -1, 1, ""},
	{"\u0578\u0582\u0569\u0575\u0561\u0576\u057D", -1, 1, ""},
	{"\u057E\u0561\u0576\u057D", -1, 1, ""},
	{"\u0578\u057B\u057D", -1, 1, ""},
	{"\u0578\u057E", -1, 1, ""},
	{"\u0561\u0576\u0578\u057E", 40, 1, ""},
	{"\u057E\u0578\u057E", 40, 1, ""},
	{"\u0565\u0580\u0578\u057E", 40, 1, ""},
	{"\u0576\u0565\u0580\u0578\u057E", 43, 1, ""},
	{"\u0565\u0580", -1, 1, ""},
	{"\u0576\u0565\u0580", 45, 1, ""},
	{"\u0581", -1, 1, ""},
	{"\u056B\u0581", 47, 1, ""},
	{"\u057E\u0561\u0576\u056B\u0581", 48, 1, ""},
	{"\u0578\u057B\u056B\u0581", 48, 1, ""},
	{"\u057E\u056B\u0581", 48, 1, ""},
	{"\u0565\u0580\u056B\u0581", 48, 1, ""},
	{"\u0576\u0565\u0580\u056B\u0581", 52, 1, ""},
	{"\u0581\u056B\u0581", 48, 1, ""},
	{"\u0578\u0581", 47, 1, ""},
	{"\u0578\u0582\u0581", 47, 1, ""},
}
var arm_g_v = []byte{209, 4, 128, 0, 18}

func (z *armenianStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		if !z.goOutGrouping(arm_g_v, 1377, 1413) {
			goto lab0_end
		}
		z.cursor++
		z.I_pV = z.cursor
		if !z.goInGrouping(arm_g_v, 1377, 1413) {
			goto lab0_end
		}
		z.cursor++
		if !z.goOutGrouping(arm_g_v, 1377, 1413) {
			goto lab0_end
		}
		z.cursor++
		if !z.goInGrouping(arm_g_v, 1377, 1413) {
			goto lab0_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	return true
}
func (z *armenianStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *armenianStemmer) r_adjective() bool {
	z.ket = z.cursor
	if z.findAmongB(arm_a_0) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *armenianStemmer) r_verb() bool {
	z.ket = z.cursor
	if z.findAmongB(arm_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *armenianStemmer) r_noun() bool {
	z.ket = z.cursor
	if z.findAmongB(arm_a_2) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *armenianStemmer) r_ending() bool {
	z.ket = z.cursor
	if z.findAmongB(arm_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R2() {
		return false
	}
	z.sliceDel()
	return true
}
func (z *armenianStemmer) stem() {
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	if z.cursor < z.I_pV {
		return
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	v_2 := z.limit - z.cursor
	z.r_ending()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_verb()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_adjective()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_noun()
	z.cursor = z.limit - v_5
	z.limitBackward = v_1
	z.cursor = z.limitBackward
	return
}

type basqueStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newBasque() *basqueStemmer { return &basqueStemmer{} }

var bas_a_0 = []among{
	{"idea", -1, 1, ""},
	{"bidea", 0, 1, ""},
	{"kidea", 0, 1, ""},
	{"pidea", 0, 1, ""},
	{"kundea", -1, 1, ""},
	{"galea", -1, 1, ""},
	{"tailea", -1, 1, ""},
	{"tzailea", -1, 1, ""},
	{"gunea", -1, 1, ""},
	{"kunea", -1, 1, ""},
	{"tzaga", -1, 1, ""},
	{"gaia", -1, 1, ""},
	{"aldia", -1, 1, ""},
	{"taldia", 12, 1, ""},
	{"karia", -1, 1, ""},
	{"garria", -1, 2, ""},
	{"karria", -1, 1, ""},
	{"ka", -1, 1, ""},
	{"tzaka", 17, 1, ""},
	{"la", -1, 1, ""},
	{"mena", -1, 1, ""},
	{"pena", -1, 1, ""},
	{"kina", -1, 1, ""},
	{"ezina", -1, 1, ""},
	{"tezina", 23, 1, ""},
	{"kuna", -1, 1, ""},
	{"tuna", -1, 1, ""},
	{"kizuna", -1, 1, ""},
	{"era", -1, 1, ""},
	{"bera", 28, 1, ""},
	{"arabera", 29, -1, ""},
	{"kera", 28, 1, ""},
	{"pera", 28, 1, ""},
	{"orra", -1, 1, ""},
	{"korra", 33, 1, ""},
	{"dura", -1, 1, ""},
	{"gura", -1, 1, ""},
	{"kura", -1, 1, ""},
	{"tura", -1, 1, ""},
	{"eta", -1, 1, ""},
	{"keta", 39, 1, ""},
	{"gailua", -1, 1, ""},
	{"eza", -1, 1, ""},
	{"erreza", 42, 1, ""},
	{"tza", -1, 2, ""},
	{"gaitza", 44, 1, ""},
	{"kaitza", 44, 1, ""},
	{"kuntza", 44, 1, ""},
	{"ide", -1, 1, ""},
	{"bide", 48, 1, ""},
	{"kide", 48, 1, ""},
	{"pide", 48, 1, ""},
	{"kunde", -1, 1, ""},
	{"tzake", -1, 1, ""},
	{"tzeke", -1, 1, ""},
	{"le", -1, 1, ""},
	{"gale", 55, 1, ""},
	{"taile", 55, 1, ""},
	{"tzaile", 55, 1, ""},
	{"gune", -1, 1, ""},
	{"kune", -1, 1, ""},
	{"tze", -1, 1, ""},
	{"atze", 61, 1, ""},
	{"gai", -1, 1, ""},
	{"aldi", -1, 1, ""},
	{"taldi", 64, 1, ""},
	{"ki", -1, 1, ""},
	{"ari", -1, 1, ""},
	{"kari", 67, 1, ""},
	{"lari", 67, 1, ""},
	{"tari", 67, 1, ""},
	{"etari", 70, 1, ""},
	{"garri", -1, 2, ""},
	{"karri", -1, 1, ""},
	{"arazi", -1, 1, ""},
	{"tarazi", 74, 1, ""},
	{"an", -1, 1, ""},
	{"ean", 76, 1, ""},
	{"rean", 77, 1, ""},
	{"kan", 76, 1, ""},
	{"etan", 76, 1, ""},
	{"atseden", -1, -1, ""},
	{"men", -1, 1, ""},
	{"pen", -1, 1, ""},
	{"kin", -1, 1, ""},
	{"rekin", 84, 1, ""},
	{"ezin", -1, 1, ""},
	{"tezin", 86, 1, ""},
	{"tun", -1, 1, ""},
	{"kizun", -1, 1, ""},
	{"go", -1, 1, ""},
	{"ago", 90, 1, ""},
	{"tio", -1, 1, ""},
	{"dako", -1, 1, ""},
	{"or", -1, 1, ""},
	{"kor", 94, 1, ""},
	{"tzat", -1, 1, ""},
	{"du", -1, 1, ""},
	{"gailu", -1, 1, ""},
	{"tu", -1, 1, ""},
	{"atu", 99, 1, ""},
	{"aldatu", 100, 1, ""},
	{"tatu", 100, 1, ""},
	{"baditu", 99, -1, ""},
	{"ez", -1, 1, ""},
	{"errez", 104, 1, ""},
	{"tzez", 104, 1, ""},
	{"gaitz", -1, 1, ""},
	{"kaitz", -1, 1, ""},
}
var bas_a_1 = []among{
	{"ada", -1, 1, ""},
	{"kada", 0, 1, ""},
	{"anda", -1, 1, ""},
	{"denda", -1, 1, ""},
	{"gabea", -1, 1, ""},
	{"kabea", -1, 1, ""},
	{"aldea", -1, 1, ""},
	{"kaldea", 6, 1, ""},
	{"taldea", 6, 1, ""},
	{"ordea", -1, 1, ""},
	{"zalea", -1, 1, ""},
	{"tzalea", 10, 1, ""},
	{"gilea", -1, 1, ""},
	{"emea", -1, 1, ""},
	{"kumea", -1, 1, ""},
	{"nea", -1, 1, ""},
	{"enea", 15, 1, ""},
	{"zionea", 15, 1, ""},
	{"unea", 15, 1, ""},
	{"gunea", 18, 1, ""},
	{"pea", -1, 1, ""},
	{"aurrea", -1, 1, ""},
	{"tea", -1, 1, ""},
	{"kotea", 22, 1, ""},
	{"artea", 22, 1, ""},
	{"ostea", 22, 1, ""},
	{"etxea", -1, 1, ""},
	{"ga", -1, 1, ""},
	{"anga", 27, 1, ""},
	{"gaia", -1, 1, ""},
	{"aldia", -1, 1, ""},
	{"taldia", 30, 1, ""},
	{"handia", -1, 1, ""},
	{"mendia", -1, 1, ""},
	{"geia", -1, 1, ""},
	{"egia", -1, 1, ""},
	{"degia", 35, 1, ""},
	{"tegia", 35, 1, ""},
	{"nahia", -1, 1, ""},
	{"ohia", -1, 1, ""},
	{"kia", -1, 1, ""},
	{"tokia", 40, 1, ""},
	{"oia", -1, 1, ""},
	{"koia", 42, 1, ""},
	{"aria", -1, 1, ""},
	{"karia", 44, 1, ""},
	{"laria", 44, 1, ""},
	{"taria", 44, 1, ""},
	{"eria", -1, 1, ""},
	{"keria", 48, 1, ""},
	{"teria", 48, 1, ""},
	{"garria", -1, 2, ""},
	{"larria", -1, 1, ""},
	{"kirria", -1, 1, ""},
	{"duria", -1, 1, ""},
	{"asia", -1, 1, ""},
	{"tia", -1, 1, ""},
	{"ezia", -1, 1, ""},
	{"bizia", -1, 1, ""},
	{"ontzia", -1, 1, ""},
	{"ka", -1, 1, ""},
	{"joka", 60, 3, ""},
	{"aurka", 60, -1, ""},
	{"ska", 60, 1, ""},
	{"xka", 60, 1, ""},
	{"zka", 60, 1, ""},
	{"gibela", -1, 1, ""},
	{"gela", -1, 1, ""},
	{"kaila", -1, 1, ""},
	{"skila", -1, 1, ""},
	{"tila", -1, 1, ""},
	{"ola", -1, 1, ""},
	{"na", -1, 1, ""},
	{"kana", 72, 1, ""},
	{"ena", 72, 1, ""},
	{"garrena", 74, 1, ""},
	{"gerrena", 74, 1, ""},
	{"urrena", 74, 1, ""},
	{"zaina", 72, 1, ""},
	{"tzaina", 78, 1, ""},
	{"kina", 72, 1, ""},
	{"mina", 72, 1, ""},
	{"garna", 72, 1, ""},
	{"una", 72, 1, ""},
	{"duna", 83, 1, ""},
	{"asuna", 83, 1, ""},
	{"tasuna", 85, 1, ""},
	{"ondoa", -1, 1, ""},
	{"kondoa", 87, 1, ""},
	{"ngoa", -1, 1, ""},
	{"zioa", -1, 1, ""},
	{"koa", -1, 1, ""},
	{"takoa", 91, 1, ""},
	{"zkoa", 91, 1, ""},
	{"noa", -1, 1, ""},
	{"zinoa", 94, 1, ""},
	{"aroa", -1, 1, ""},
	{"taroa", 96, 1, ""},
	{"zaroa", 96, 1, ""},
	{"eroa", -1, 1, ""},
	{"oroa", -1, 1, ""},
	{"osoa", -1, 1, ""},
	{"toa", -1, 1, ""},
	{"ttoa", 102, 1, ""},
	{"ztoa", 102, 1, ""},
	{"txoa", -1, 1, ""},
	{"tzoa", -1, 1, ""},
	{"\u00F1oa", -1, 1, ""},
	{"ra", -1, 1, ""},
	{"ara", 108, 1, ""},
	{"dara", 109, 1, ""},
	{"liara", 109, 1, ""},
	{"tiara", 109, 1, ""},
	{"tara", 109, 1, ""},
	{"etara", 113, 1, ""},
	{"tzara", 109, 1, ""},
	{"bera", 108, 1, ""},
	{"kera", 108, 1, ""},
	{"pera", 108, 1, ""},
	{"ora", 108, 2, ""},
	{"tzarra", 108, 1, ""},
	{"korra", 108, 1, ""},
	{"tra", 108, 1, ""},
	{"sa", -1, 1, ""},
	{"osa", 123, 1, ""},
	{"ta", -1, 1, ""},
	{"eta", 125, 1, ""},
	{"keta", 126, 1, ""},
	{"sta", 125, 1, ""},
	{"dua", -1, 1, ""},
	{"mendua", 129, 1, ""},
	{"ordua", 129, 1, ""},
	{"lekua", -1, 1, ""},
	{"burua", -1, 1, ""},
	{"durua", -1, 1, ""},
	{"tsua", -1, 1, ""},
	{"tua", -1, 1, ""},
	{"mentua", 136, 1, ""},
	{"estua", 136, 1, ""},
	{"txua", -1, 1, ""},
	{"zua", -1, 1, ""},
	{"tzua", 140, 1, ""},
	{"za", -1, 1, ""},
	{"eza", 142, 1, ""},
	{"eroza", 142, 1, ""},
	{"tza", 142, 2, ""},
	{"koitza", 145, 1, ""},
	{"antza", 145, 1, ""},
	{"gintza", 145, 1, ""},
	{"kintza", 145, 1, ""},
	{"kuntza", 145, 1, ""},
	{"gabe", -1, 1, ""},
	{"kabe", -1, 1, ""},
	{"kide", -1, 1, ""},
	{"alde", -1, 1, ""},
	{"kalde", 154, 1, ""},
	{"talde", 154, 1, ""},
	{"orde", -1, 1, ""},
	{"ge", -1, 1, ""},
	{"zale", -1, 1, ""},
	{"tzale", 159, 1, ""},
	{"gile", -1, 1, ""},
	{"eme", -1, 1, ""},
	{"kume", -1, 1, ""},
	{"ne", -1, 1, ""},
	{"zione", 164, 1, ""},
	{"une", 164, 1, ""},
	{"gune", 166, 1, ""},
	{"pe", -1, 1, ""},
	{"aurre", -1, 1, ""},
	{"te", -1, 1, ""},
	{"kote", 170, 1, ""},
	{"arte", 170, 1, ""},
	{"oste", 170, 1, ""},
	{"etxe", -1, 1, ""},
	{"gai", -1, 1, ""},
	{"di", -1, 1, ""},
	{"aldi", 176, 1, ""},
	{"taldi", 177, 1, ""},
	{"geldi", 176, -1, ""},
	{"handi", 176, 1, ""},
	{"mendi", 176, 1, ""},
	{"gei", -1, 1, ""},
	{"egi", -1, 1, ""},
	{"degi", 183, 1, ""},
	{"tegi", 183, 1, ""},
	{"nahi", -1, 1, ""},
	{"ohi", -1, 1, ""},
	{"ki", -1, 1, ""},
	{"toki", 188, 1, ""},
	{"oi", -1, 1, ""},
	{"goi", 190, 1, ""},
	{"koi", 190, 1, ""},
	{"ari", -1, 1, ""},
	{"kari", 193, 1, ""},
	{"lari", 193, 1, ""},
	{"tari", 193, 1, ""},
	{"garri", -1, 2, ""},
	{"larri", -1, 1, ""},
	{"kirri", -1, 1, ""},
	{"duri", -1, 1, ""},
	{"asi", -1, 1, ""},
	{"ti", -1, 1, ""},
	{"ontzi", -1, 1, ""},
	{"\u00F1i", -1, 1, ""},
	{"ak", -1, 1, ""},
	{"ek", -1, 1, ""},
	{"tarik", -1, 1, ""},
	{"gibel", -1, 1, ""},
	{"ail", -1, 1, ""},
	{"kail", 209, 1, ""},
	{"kan", -1, 1, ""},
	{"tan", -1, 1, ""},
	{"etan", 212, 1, ""},
	{"en", -1, 4, ""},
	{"ren", 214, 2, ""},
	{"garren", 215, 1, ""},
	{"gerren", 215, 1, ""},
	{"urren", 215, 1, ""},
	{"ten", 214, 4, ""},
	{"tzen", 214, 4, ""},
	{"zain", -1, 1, ""},
	{"tzain", 221, 1, ""},
	{"kin", -1, 1, ""},
	{"min", -1, 1, ""},
	{"dun", -1, 1, ""},
	{"asun", -1, 1, ""},
	{"tasun", 226, 1, ""},
	{"aizun", -1, 1, ""},
	{"ondo", -1, 1, ""},
	{"kondo", 229, 1, ""},
	{"go", -1, 1, ""},
	{"ngo", 231, 1, ""},
	{"zio", -1, 1, ""},
	{"ko", -1, 1, ""},
	{"trako", 234, 5, ""},
	{"tako", 234, 1, ""},
	{"etako", 236, 1, ""},
	{"eko", 234, 1, ""},
	{"tariko", 234, 1, ""},
	{"sko", 234, 1, ""},
	{"tuko", 234, 1, ""},
	{"minutuko", 241, 6, ""},
	{"zko", 234, 1, ""},
	{"no", -1, 1, ""},
	{"zino", 244, 1, ""},
	{"ro", -1, 1, ""},
	{"aro", 246, 1, ""},
	{"igaro", 247, -1, ""},
	{"taro", 247, 1, ""},
	{"zaro", 247, 1, ""},
	{"ero", 246, 1, ""},
	{"giro", 246, 1, ""},
	{"oro", 246, 1, ""},
	{"oso", -1, 1, ""},
	{"to", -1, 1, ""},
	{"tto", 255, 1, ""},
	{"zto", 255, 1, ""},
	{"txo", -1, 1, ""},
	{"tzo", -1, 1, ""},
	{"gintzo", 259, 1, ""},
	{"\u00F1o", -1, 1, ""},
	{"zp", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"dar", 263, 1, ""},
	{"behar", 263, 1, ""},
	{"zehar", 263, -1, ""},
	{"liar", 263, 1, ""},
	{"tiar", 263, 1, ""},
	{"tar", 263, 1, ""},
	{"tzar", 263, 1, ""},
	{"or", -1, 2, ""},
	{"kor", 271, 1, ""},
	{"os", -1, 1, ""},
	{"ket", -1, 1, ""},
	{"du", -1, 1, ""},
	{"mendu", 275, 1, ""},
	{"ordu", 275, 1, ""},
	{"leku", -1, 1, ""},
	{"buru", -1, 2, ""},
	{"duru", -1, 1, ""},
	{"tsu", -1, 1, ""},
	{"tu", -1, 1, ""},
	{"tatu", 282, 4, ""},
	{"mentu", 282, 1, ""},
	{"estu", 282, 1, ""},
	{"txu", -1, 1, ""},
	{"zu", -1, 1, ""},
	{"tzu", 287, 1, ""},
	{"gintzu", 288, 1, ""},
	{"z", -1, 1, ""},
	{"ez", 290, 1, ""},
	{"eroz", 290, 1, ""},
	{"tz", 290, 1, ""},
	{"koitz", 293, 1, ""},
}
var bas_a_2 = []among{
	{"zlea", -1, 2, ""},
	{"keria", -1, 1, ""},
	{"la", -1, 1, ""},
	{"era", -1, 1, ""},
	{"dade", -1, 1, ""},
	{"tade", -1, 1, ""},
	{"date", -1, 1, ""},
	{"tate", -1, 1, ""},
	{"gi", -1, 1, ""},
	{"ki", -1, 1, ""},
	{"ik", -1, 1, ""},
	{"lanik", 10, 1, ""},
	{"rik", 10, 1, ""},
	{"larik", 12, 1, ""},
	{"ztik", 10, 1, ""},
	{"go", -1, 1, ""},
	{"ro", -1, 1, ""},
	{"ero", 16, 1, ""},
	{"to", -1, 1, ""},
}
var bas_g_v = []byte{17, 65, 16}

func (z *basqueStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(bas_g_v, 97, 117)) {
					goto lab2_end
				}
				{
					v_3 := z.cursor
					{
						if !(z.outGrouping(bas_g_v, 97, 117)) {
							goto lab4_end
						}
						if !z.goOutGrouping(bas_g_v, 97, 117) {
							goto lab4_end
						}
						z.cursor++
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_3
					if !(z.inGrouping(bas_g_v, 97, 117)) {
						goto lab2_end
					}
					if !z.goInGrouping(bas_g_v, 97, 117) {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			if !(z.outGrouping(bas_g_v, 97, 117)) {
				goto lab0_end
			}
			{
				v_4 := z.cursor
				{
					if !(z.outGrouping(bas_g_v, 97, 117)) {
						goto lab6_end
					}
					if !z.goOutGrouping(bas_g_v, 97, 117) {
						goto lab6_end
					}
					z.cursor++
					goto lab5_end
				}
			lab6_end:
				;
				z.cursor = v_4
				if !(z.inGrouping(bas_g_v, 97, 117)) {
					goto lab0_end
				}
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		lab5_end:
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_5 := z.cursor
	{
		if !z.goOutGrouping(bas_g_v, 97, 117) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(bas_g_v, 97, 117) {
			goto lab7_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(bas_g_v, 97, 117) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(bas_g_v, 97, 117) {
			goto lab7_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab7_end:
	;
	z.cursor = v_5
	return true
}
func (z *basqueStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *basqueStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *basqueStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *basqueStemmer) r_aditzak() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(bas_a_0)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *basqueStemmer) r_izenak() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(bas_a_1)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 3:
		z.sliceFrom("jok")
		break
	case 4:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	case 5:
		z.sliceFrom("tra")
		break
	case 6:
		z.sliceFrom("minutu")
		break
	}
	return true
}
func (z *basqueStemmer) r_adjetiboak() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(bas_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("z")
		break
	}
	return true
}
func (z *basqueStemmer) stem() {
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	for {
		v_1 := z.limit - z.cursor
		{
			if !z.r_aditzak() {
				goto lab0_end
			}
			continue
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
		break
	}
	for {
		v_2 := z.limit - z.cursor
		{
			if !z.r_izenak() {
				goto lab1_end
			}
			continue
		}
	lab1_end:
		;
		z.cursor = z.limit - v_2
		break
	}
	v_3 := z.limit - z.cursor
	z.r_adjetiboak()
	z.cursor = z.limit - v_3
	z.cursor = z.limitBackward
	return
}

type catalanStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
}

func newCatalan() *catalanStemmer { return &catalanStemmer{} }

var cat_a_0 = []among{
	{"", -1, 7, ""},
	{"\u00B7", 0, 6, ""},
	{"\u00E0", 0, 1, ""},
	{"\u00E1", 0, 1, ""},
	{"\u00E8", 0, 2, ""},
	{"\u00E9", 0, 2, ""},
	{"\u00EC", 0, 3, ""},
	{"\u00ED", 0, 3, ""},
	{"\u00EF", 0, 3, ""},
	{"\u00F2", 0, 4, ""},
	{"\u00F3", 0, 4, ""},
	{"\u00FA", 0, 5, ""},
	{"\u00FC", 0, 5, ""},
}
var cat_a_1 = []among{
	{"la", -1, 1, ""},
	{"-la", 0, 1, ""},
	{"sela", 0, 1, ""},
	{"le", -1, 1, ""},
	{"me", -1, 1, ""},
	{"-me", 4, 1, ""},
	{"se", -1, 1, ""},
	{"-te", -1, 1, ""},
	{"hi", -1, 1, ""},
	{"'hi", 8, 1, ""},
	{"li", -1, 1, ""},
	{"-li", 10, 1, ""},
	{"'l", -1, 1, ""},
	{"'m", -1, 1, ""},
	{"-m", -1, 1, ""},
	{"'n", -1, 1, ""},
	{"-n", -1, 1, ""},
	{"ho", -1, 1, ""},
	{"'ho", 17, 1, ""},
	{"lo", -1, 1, ""},
	{"selo", 19, 1, ""},
	{"'s", -1, 1, ""},
	{"las", -1, 1, ""},
	{"selas", 22, 1, ""},
	{"les", -1, 1, ""},
	{"-les", 24, 1, ""},
	{"'ls", -1, 1, ""},
	{"-ls", -1, 1, ""},
	{"'ns", -1, 1, ""},
	{"-ns", -1, 1, ""},
	{"ens", -1, 1, ""},
	{"los", -1, 1, ""},
	{"selos", 31, 1, ""},
	{"nos", -1, 1, ""},
	{"-nos", 33, 1, ""},
	{"vos", -1, 1, ""},
	{"us", -1, 1, ""},
	{"-us", 36, 1, ""},
	{"'t", -1, 1, ""},
}
var cat_a_2 = []among{
	{"ica", -1, 4, ""},
	{"l\u00F3gica", 0, 3, ""},
	{"enca", -1, 1, ""},
	{"ada", -1, 2, ""},
	{"ancia", -1, 1, ""},
	{"encia", -1, 1, ""},
	{"\u00E8ncia", -1, 1, ""},
	{"\u00EDcia", -1, 1, ""},
	{"logia", -1, 3, ""},
	{"inia", -1, 1, ""},
	{"\u00EDinia", 9, 1, ""},
	{"eria", -1, 1, ""},
	{"\u00E0ria", -1, 1, ""},
	{"at\u00F2ria", -1, 1, ""},
	{"alla", -1, 1, ""},
	{"ella", -1, 1, ""},
	{"\u00EDvola", -1, 1, ""},
	{"ima", -1, 1, ""},
	{"\u00EDssima", 17, 1, ""},
	{"qu\u00EDssima", 18, 5, ""},
	{"ana", -1, 1, ""},
	{"ina", -1, 1, ""},
	{"era", -1, 1, ""},
	{"sfera", 22, 1, ""},
	{"ora", -1, 1, ""},
	{"dora", 24, 1, ""},
	{"adora", 25, 1, ""},
	{"adura", -1, 1, ""},
	{"esa", -1, 1, ""},
	{"osa", -1, 1, ""},
	{"assa", -1, 1, ""},
	{"essa", -1, 1, ""},
	{"issa", -1, 1, ""},
	{"eta", -1, 1, ""},
	{"ita", -1, 1, ""},
	{"ota", -1, 1, ""},
	{"ista", -1, 1, ""},
	{"ialista", 36, 1, ""},
	{"ionista", 36, 1, ""},
	{"iva", -1, 1, ""},
	{"ativa", 39, 1, ""},
	{"n\u00E7a", -1, 1, ""},
	{"log\u00EDa", -1, 3, ""},
	{"ic", -1, 4, ""},
	{"\u00EDstic", 43, 1, ""},
	{"enc", -1, 1, ""},
	{"esc", -1, 1, ""},
	{"ud", -1, 1, ""},
	{"atge", -1, 1, ""},
	{"ble", -1, 1, ""},
	{"able", 49, 1, ""},
	{"ible", 49, 1, ""},
	{"isme", -1, 1, ""},
	{"ialisme", 52, 1, ""},
	{"ionisme", 52, 1, ""},
	{"ivisme", 52, 1, ""},
	{"aire", -1, 1, ""},
	{"icte", -1, 1, ""},
	{"iste", -1, 1, ""},
	{"ici", -1, 1, ""},
	{"\u00EDci", -1, 1, ""},
	{"logi", -1, 3, ""},
	{"ari", -1, 1, ""},
	{"tori", -1, 1, ""},
	{"al", -1, 1, ""},
	{"il", -1, 1, ""},
	{"all", -1, 1, ""},
	{"ell", -1, 1, ""},
	{"\u00EDvol", -1, 1, ""},
	{"isam", -1, 1, ""},
	{"issem", -1, 1, ""},
	{"\u00ECssem", -1, 1, ""},
	{"\u00EDssem", -1, 1, ""},
	{"\u00EDssim", -1, 1, ""},
	{"qu\u00EDssim", 73, 5, ""},
	{"amen", -1, 1, ""},
	{"\u00ECssin", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"ificar", 77, 1, ""},
	{"egar", 77, 1, ""},
	{"ejar", 77, 1, ""},
	{"itar", 77, 1, ""},
	{"itzar", 77, 1, ""},
	{"fer", -1, 1, ""},
	{"or", -1, 1, ""},
	{"dor", 84, 1, ""},
	{"dur", -1, 1, ""},
	{"doras", -1, 1, ""},
	{"ics", -1, 4, ""},
	{"l\u00F3gics", 88, 3, ""},
	{"uds", -1, 1, ""},
	{"nces", -1, 1, ""},
	{"ades", -1, 2, ""},
	{"ancies", -1, 1, ""},
	{"encies", -1, 1, ""},
	{"\u00E8ncies", -1, 1, ""},
	{"\u00EDcies", -1, 1, ""},
	{"logies", -1, 3, ""},
	{"inies", -1, 1, ""},
	{"\u00EDnies", -1, 1, ""},
	{"eries", -1, 1, ""},
	{"\u00E0ries", -1, 1, ""},
	{"at\u00F2ries", -1, 1, ""},
	{"bles", -1, 1, ""},
	{"ables", 103, 1, ""},
	{"ibles", 103, 1, ""},
	{"imes", -1, 1, ""},
	{"\u00EDssimes", 106, 1, ""},
	{"qu\u00EDssimes", 107, 5, ""},
	{"formes", -1, 1, ""},
	{"ismes", -1, 1, ""},
	{"ialismes", 110, 1, ""},
	{"ines", -1, 1, ""},
	{"eres", -1, 1, ""},
	{"ores", -1, 1, ""},
	{"dores", 114, 1, ""},
	{"idores", 115, 1, ""},
	{"dures", -1, 1, ""},
	{"eses", -1, 1, ""},
	{"oses", -1, 1, ""},
	{"asses", -1, 1, ""},
	{"ictes", -1, 1, ""},
	{"ites", -1, 1, ""},
	{"otes", -1, 1, ""},
	{"istes", -1, 1, ""},
	{"ialistes", 124, 1, ""},
	{"ionistes", 124, 1, ""},
	{"iques", -1, 4, ""},
	{"l\u00F3giques", 127, 3, ""},
	{"ives", -1, 1, ""},
	{"atives", 129, 1, ""},
	{"log\u00EDes", -1, 3, ""},
	{"alleng\u00FCes", -1, 1, ""},
	{"icis", -1, 1, ""},
	{"\u00EDcis", -1, 1, ""},
	{"logis", -1, 3, ""},
	{"aris", -1, 1, ""},
	{"toris", -1, 1, ""},
	{"ls", -1, 1, ""},
	{"als", 138, 1, ""},
	{"ells", 138, 1, ""},
	{"ims", -1, 1, ""},
	{"\u00EDssims", 141, 1, ""},
	{"qu\u00EDssims", 142, 5, ""},
	{"ions", -1, 1, ""},
	{"cions", 144, 1, ""},
	{"acions", 145, 2, ""},
	{"esos", -1, 1, ""},
	{"osos", -1, 1, ""},
	{"assos", -1, 1, ""},
	{"issos", -1, 1, ""},
	{"ers", -1, 1, ""},
	{"ors", -1, 1, ""},
	{"dors", 152, 1, ""},
	{"adors", 153, 1, ""},
	{"idors", 153, 1, ""},
	{"ats", -1, 1, ""},
	{"itats", 156, 1, ""},
	{"bilitats", 157, 1, ""},
	{"ivitats", 157, 1, ""},
	{"ativitats", 159, 1, ""},
	{"\u00EFtats", 156, 1, ""},
	{"ets", -1, 1, ""},
	{"ants", -1, 1, ""},
	{"ents", -1, 1, ""},
	{"ments", 164, 1, ""},
	{"aments", 165, 1, ""},
	{"ots", -1, 1, ""},
	{"uts", -1, 1, ""},
	{"ius", -1, 1, ""},
	{"trius", 169, 1, ""},
	{"atius", 169, 1, ""},
	{"\u00E8s", -1, 1, ""},
	{"\u00E9s", -1, 1, ""},
	{"\u00EDs", -1, 1, ""},
	{"d\u00EDs", 174, 1, ""},
	{"\u00F3s", -1, 1, ""},
	{"itat", -1, 1, ""},
	{"bilitat", 177, 1, ""},
	{"ivitat", 177, 1, ""},
	{"ativitat", 179, 1, ""},
	{"\u00EFtat", -1, 1, ""},
	{"et", -1, 1, ""},
	{"ant", -1, 1, ""},
	{"ent", -1, 1, ""},
	{"ient", 184, 1, ""},
	{"ment", 184, 1, ""},
	{"ament", 186, 1, ""},
	{"isament", 187, 1, ""},
	{"ot", -1, 1, ""},
	{"isseu", -1, 1, ""},
	{"\u00ECsseu", -1, 1, ""},
	{"\u00EDsseu", -1, 1, ""},
	{"triu", -1, 1, ""},
	{"\u00EDssiu", -1, 1, ""},
	{"atiu", -1, 1, ""},
	{"\u00F3", -1, 1, ""},
	{"i\u00F3", 196, 1, ""},
	{"ci\u00F3", 197, 1, ""},
	{"aci\u00F3", 198, 1, ""},
}
var cat_a_3 = []among{
	{"aba", -1, 1, ""},
	{"esca", -1, 1, ""},
	{"isca", -1, 1, ""},
	{"\u00EFsca", -1, 1, ""},
	{"ada", -1, 1, ""},
	{"ida", -1, 1, ""},
	{"uda", -1, 1, ""},
	{"\u00EFda", -1, 1, ""},
	{"ia", -1, 1, ""},
	{"aria", 8, 1, ""},
	{"iria", 8, 1, ""},
	{"ara", -1, 1, ""},
	{"iera", -1, 1, ""},
	{"ira", -1, 1, ""},
	{"adora", -1, 1, ""},
	{"\u00EFra", -1, 1, ""},
	{"ava", -1, 1, ""},
	{"ixa", -1, 1, ""},
	{"itza", -1, 1, ""},
	{"\u00EDa", -1, 1, ""},
	{"ar\u00EDa", 19, 1, ""},
	{"er\u00EDa", 19, 1, ""},
	{"ir\u00EDa", 19, 1, ""},
	{"\u00EFa", -1, 1, ""},
	{"isc", -1, 1, ""},
	{"\u00EFsc", -1, 1, ""},
	{"ad", -1, 1, ""},
	{"ed", -1, 1, ""},
	{"id", -1, 1, ""},
	{"ie", -1, 1, ""},
	{"re", -1, 1, ""},
	{"dre", 30, 1, ""},
	{"ase", -1, 1, ""},
	{"iese", -1, 1, ""},
	{"aste", -1, 1, ""},
	{"iste", -1, 1, ""},
	{"ii", -1, 1, ""},
	{"ini", -1, 1, ""},
	{"esqui", -1, 1, ""},
	{"eixi", -1, 1, ""},
	{"itzi", -1, 1, ""},
	{"am", -1, 1, ""},
	{"em", -1, 1, ""},
	{"arem", 42, 1, ""},
	{"irem", 42, 1, ""},
	{"\u00E0rem", 42, 1, ""},
	{"\u00EDrem", 42, 1, ""},
	{"\u00E0ssem", 42, 1, ""},
	{"\u00E9ssem", 42, 1, ""},
	{"iguem", 42, 1, ""},
	{"\u00EFguem", 42, 1, ""},
	{"avem", 42, 1, ""},
	{"\u00E0vem", 42, 1, ""},
	{"\u00E1vem", 42, 1, ""},
	{"ir\u00ECem", 42, 1, ""},
	{"\u00EDem", 42, 1, ""},
	{"ar\u00EDem", 55, 1, ""},
	{"ir\u00EDem", 55, 1, ""},
	{"assim", -1, 1, ""},
	{"essim", -1, 1, ""},
	{"issim", -1, 1, ""},
	{"\u00E0ssim", -1, 1, ""},
	{"\u00E8ssim", -1, 1, ""},
	{"\u00E9ssim", -1, 1, ""},
	{"\u00EDssim", -1, 1, ""},
	{"\u00EFm", -1, 1, ""},
	{"an", -1, 1, ""},
	{"aban", 66, 1, ""},
	{"arian", 66, 1, ""},
	{"aran", 66, 1, ""},
	{"ieran", 66, 1, ""},
	{"iran", 66, 1, ""},
	{"\u00EDan", 66, 1, ""},
	{"ar\u00EDan", 72, 1, ""},
	{"er\u00EDan", 72, 1, ""},
	{"ir\u00EDan", 72, 1, ""},
	{"en", -1, 1, ""},
	{"ien", 76, 1, ""},
	{"arien", 77, 1, ""},
	{"irien", 77, 1, ""},
	{"aren", 76, 1, ""},
	{"eren", 76, 1, ""},
	{"iren", 76, 1, ""},
	{"\u00E0ren", 76, 1, ""},
	{"\u00EFren", 76, 1, ""},
	{"asen", 76, 1, ""},
	{"iesen", 76, 1, ""},
	{"assen", 76, 1, ""},
	{"essen", 76, 1, ""},
	{"issen", 76, 1, ""},
	{"\u00E9ssen", 76, 1, ""},
	{"\u00EFssen", 76, 1, ""},
	{"esquen", 76, 1, ""},
	{"isquen", 76, 1, ""},
	{"\u00EFsquen", 76, 1, ""},
	{"aven", 76, 1, ""},
	{"ixen", 76, 1, ""},
	{"eixen", 96, 1, ""},
	{"\u00EFxen", 76, 1, ""},
	{"\u00EFen", 76, 1, ""},
	{"in", -1, 1, ""},
	{"inin", 100, 1, ""},
	{"sin", 100, 1, ""},
	{"isin", 102, 1, ""},
	{"assin", 102, 1, ""},
	{"essin", 102, 1, ""},
	{"issin", 102, 1, ""},
	{"\u00EFssin", 102, 1, ""},
	{"esquin", 100, 1, ""},
	{"eixin", 100, 1, ""},
	{"aron", -1, 1, ""},
	{"ieron", -1, 1, ""},
	{"ar\u00E1n", -1, 1, ""},
	{"er\u00E1n", -1, 1, ""},
	{"ir\u00E1n", -1, 1, ""},
	{"i\u00EFn", -1, 1, ""},
	{"ado", -1, 1, ""},
	{"ido", -1, 1, ""},
	{"ando", -1, 2, ""},
	{"iendo", -1, 1, ""},
	{"io", -1, 1, ""},
	{"ixo", -1, 1, ""},
	{"eixo", 121, 1, ""},
	{"\u00EFxo", -1, 1, ""},
	{"itzo", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"tzar", 125, 1, ""},
	{"er", -1, 1, ""},
	{"eixer", 127, 1, ""},
	{"ir", -1, 1, ""},
	{"ador", -1, 1, ""},
	{"as", -1, 1, ""},
	{"abas", 131, 1, ""},
	{"adas", 131, 1, ""},
	{"idas", 131, 1, ""},
	{"aras", 131, 1, ""},
	{"ieras", 131, 1, ""},
	{"\u00EDas", 131, 1, ""},
	{"ar\u00EDas", 137, 1, ""},
	{"er\u00EDas", 137, 1, ""},
	{"ir\u00EDas", 137, 1, ""},
	{"ids", -1, 1, ""},
	{"es", -1, 1, ""},
	{"ades", 142, 1, ""},
	{"ides", 142, 1, ""},
	{"udes", 142, 1, ""},
	{"\u00EFdes", 142, 1, ""},
	{"atges", 142, 1, ""},
	{"ies", 142, 1, ""},
	{"aries", 148, 1, ""},
	{"iries", 148, 1, ""},
	{"ares", 142, 1, ""},
	{"ires", 142, 1, ""},
	{"adores", 142, 1, ""},
	{"\u00EFres", 142, 1, ""},
	{"ases", 142, 1, ""},
	{"ieses", 142, 1, ""},
	{"asses", 142, 1, ""},
	{"esses", 142, 1, ""},
	{"isses", 142, 1, ""},
	{"\u00EFsses", 142, 1, ""},
	{"ques", 142, 1, ""},
	{"esques", 161, 1, ""},
	{"\u00EFsques", 161, 1, ""},
	{"aves", 142, 1, ""},
	{"ixes", 142, 1, ""},
	{"eixes", 165, 1, ""},
	{"\u00EFxes", 142, 1, ""},
	{"\u00EFes", 142, 1, ""},
	{"abais", -1, 1, ""},
	{"arais", -1, 1, ""},
	{"ierais", -1, 1, ""},
	{"\u00EDais", -1, 1, ""},
	{"ar\u00EDais", 172, 1, ""},
	{"er\u00EDais", 172, 1, ""},
	{"ir\u00EDais", 172, 1, ""},
	{"aseis", -1, 1, ""},
	{"ieseis", -1, 1, ""},
	{"asteis", -1, 1, ""},
	{"isteis", -1, 1, ""},
	{"inis", -1, 1, ""},
	{"sis", -1, 1, ""},
	{"isis", 181, 1, ""},
	{"assis", 181, 1, ""},
	{"essis", 181, 1, ""},
	{"issis", 181, 1, ""},
	{"\u00EFssis", 181, 1, ""},
	{"esquis", -1, 1, ""},
	{"eixis", -1, 1, ""},
	{"itzis", -1, 1, ""},
	{"\u00E1is", -1, 1, ""},
	{"ar\u00E9is", -1, 1, ""},
	{"er\u00E9is", -1, 1, ""},
	{"ir\u00E9is", -1, 1, ""},
	{"ams", -1, 1, ""},
	{"ados", -1, 1, ""},
	{"idos", -1, 1, ""},
	{"amos", -1, 1, ""},
	{"\u00E1bamos", 197, 1, ""},
	{"\u00E1ramos", 197, 1, ""},
	{"i\u00E9ramos", 197, 1, ""},
	{"\u00EDamos", 197, 1, ""},
	{"ar\u00EDamos", 201, 1, ""},
	{"er\u00EDamos", 201, 1, ""},
	{"ir\u00EDamos", 201, 1, ""},
	{"aremos", -1, 1, ""},
	{"eremos", -1, 1, ""},
	{"iremos", -1, 1, ""},
	{"\u00E1semos", -1, 1, ""},
	{"i\u00E9semos", -1, 1, ""},
	{"imos", -1, 1, ""},
	{"adors", -1, 1, ""},
	{"ass", -1, 1, ""},
	{"erass", 212, 1, ""},
	{"ess", -1, 1, ""},
	{"ats", -1, 1, ""},
	{"its", -1, 1, ""},
	{"ents", -1, 1, ""},
	{"\u00E0s", -1, 1, ""},
	{"ar\u00E0s", 218, 1, ""},
	{"ir\u00E0s", 218, 1, ""},
	{"ar\u00E1s", -1, 1, ""},
	{"er\u00E1s", -1, 1, ""},
	{"ir\u00E1s", -1, 1, ""},
	{"\u00E9s", -1, 1, ""},
	{"ar\u00E9s", 224, 1, ""},
	{"\u00EDs", -1, 1, ""},
	{"i\u00EFs", -1, 1, ""},
	{"at", -1, 1, ""},
	{"it", -1, 1, ""},
	{"ant", -1, 1, ""},
	{"ent", -1, 1, ""},
	{"int", -1, 1, ""},
	{"ut", -1, 1, ""},
	{"\u00EFt", -1, 1, ""},
	{"au", -1, 1, ""},
	{"erau", 235, 1, ""},
	{"ieu", -1, 1, ""},
	{"ineu", -1, 1, ""},
	{"areu", -1, 1, ""},
	{"ireu", -1, 1, ""},
	{"\u00E0reu", -1, 1, ""},
	{"\u00EDreu", -1, 1, ""},
	{"asseu", -1, 1, ""},
	{"esseu", -1, 1, ""},
	{"eresseu", 244, 1, ""},
	{"\u00E0sseu", -1, 1, ""},
	{"\u00E9sseu", -1, 1, ""},
	{"igueu", -1, 1, ""},
	{"\u00EFgueu", -1, 1, ""},
	{"\u00E0veu", -1, 1, ""},
	{"\u00E1veu", -1, 1, ""},
	{"itzeu", -1, 1, ""},
	{"\u00ECeu", -1, 1, ""},
	{"ir\u00ECeu", 253, 1, ""},
	{"\u00EDeu", -1, 1, ""},
	{"ar\u00EDeu", 255, 1, ""},
	{"ir\u00EDeu", 255, 1, ""},
	{"assiu", -1, 1, ""},
	{"issiu", -1, 1, ""},
	{"\u00E0ssiu", -1, 1, ""},
	{"\u00E8ssiu", -1, 1, ""},
	{"\u00E9ssiu", -1, 1, ""},
	{"\u00EDssiu", -1, 1, ""},
	{"\u00EFu", -1, 1, ""},
	{"ix", -1, 1, ""},
	{"eix", 265, 1, ""},
	{"\u00EFx", -1, 1, ""},
	{"itz", -1, 1, ""},
	{"i\u00E0", -1, 1, ""},
	{"ar\u00E0", -1, 1, ""},
	{"ir\u00E0", -1, 1, ""},
	{"itz\u00E0", -1, 1, ""},
	{"ar\u00E1", -1, 1, ""},
	{"er\u00E1", -1, 1, ""},
	{"ir\u00E1", -1, 1, ""},
	{"ir\u00E8", -1, 1, ""},
	{"ar\u00E9", -1, 1, ""},
	{"er\u00E9", -1, 1, ""},
	{"ir\u00E9", -1, 1, ""},
	{"\u00ED", -1, 1, ""},
	{"i\u00EF", -1, 1, ""},
	{"i\u00F3", -1, 1, ""},
}
var cat_a_4 = []among{
	{"a", -1, 1, ""},
	{"e", -1, 1, ""},
	{"i", -1, 1, ""},
	{"\u00EFn", -1, 1, ""},
	{"o", -1, 1, ""},
	{"ir", -1, 1, ""},
	{"s", -1, 1, ""},
	{"is", 6, 1, ""},
	{"os", 6, 1, ""},
	{"\u00EFs", 6, 1, ""},
	{"it", -1, 1, ""},
	{"eu", -1, 1, ""},
	{"iu", -1, 1, ""},
	{"iqu", -1, 2, ""},
	{"itz", -1, 1, ""},
	{"\u00E0", -1, 1, ""},
	{"\u00E1", -1, 1, ""},
	{"\u00E9", -1, 1, ""},
	{"\u00EC", -1, 1, ""},
	{"\u00ED", -1, 1, ""},
	{"\u00EF", -1, 1, ""},
	{"\u00F3", -1, 1, ""},
}
var cat_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 129, 81, 6, 10}

func (z *catalanStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		if !z.goOutGrouping(cat_g_v, 97, 252) {
			goto lab0_end
		}
		z.cursor++
		if !z.goInGrouping(cat_g_v, 97, 252) {
			goto lab0_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(cat_g_v, 97, 252) {
			goto lab0_end
		}
		z.cursor++
		if !z.goInGrouping(cat_g_v, 97, 252) {
			goto lab0_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	return true
}
func (z *catalanStemmer) r_cleaning() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(cat_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("a")
				break
			case 2:
				z.sliceFrom("e")
				break
			case 3:
				z.sliceFrom("i")
				break
			case 4:
				z.sliceFrom("o")
				break
			case 5:
				z.sliceFrom("u")
				break
			case 6:
				z.sliceFrom(".")
				break
			case 7:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *catalanStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *catalanStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *catalanStemmer) r_attached_pronoun() bool {
	z.ket = z.cursor
	if z.findAmongB(cat_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	z.sliceDel()
	return true
}
func (z *catalanStemmer) r_standard_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(cat_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 3:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("log")
		break
	case 4:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("ic")
		break
	case 5:
		if !z.r_R1() {
			return false
		}
		z.sliceFrom("c")
		break
	}
	return true
}
func (z *catalanStemmer) r_verb_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(cat_a_3)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *catalanStemmer) r_residual_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(cat_a_4)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R1() {
			return false
		}
		z.sliceFrom("ic")
		break
	}
	return true
}
func (z *catalanStemmer) stem() {
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_attached_pronoun()
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	{
		{
			v_3 := z.limit - z.cursor
			{
				if !z.r_standard_suffix() {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			if !z.r_verb_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	v_4 := z.limit - z.cursor
	z.r_residual_suffix()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	v_5 := z.cursor
	z.r_cleaning()
	z.cursor = v_5
	return
}

type danishStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
}

func newDanish() *danishStemmer { return &danishStemmer{} }

var dan_a_0 = []among{
	{"hed", -1, 1, ""},
	{"ethed", 0, 1, ""},
	{"ered", -1, 1, ""},
	{"e", -1, 1, ""},
	{"erede", 3, 1, ""},
	{"ende", 3, 1, ""},
	{"erende", 5, 1, ""},
	{"ene", 3, 1, ""},
	{"erne", 3, 1, ""},
	{"ere", 3, 1, ""},
	{"en", -1, 1, ""},
	{"heden", 10, 1, ""},
	{"eren", 10, 1, ""},
	{"er", -1, 1, ""},
	{"heder", 13, 1, ""},
	{"erer", 13, 1, ""},
	{"s", -1, 2, ""},
	{"heds", 16, 1, ""},
	{"es", 16, 1, ""},
	{"endes", 18, 1, ""},
	{"erendes", 19, 1, ""},
	{"enes", 18, 1, ""},
	{"ernes", 18, 1, ""},
	{"eres", 18, 1, ""},
	{"ens", 16, 1, ""},
	{"hedens", 24, 1, ""},
	{"erens", 24, 1, ""},
	{"ers", 16, 1, ""},
	{"ets", 16, 1, ""},
	{"erets", 28, 1, ""},
	{"et", -1, 1, ""},
	{"eret", 30, 1, ""},
}
var dan_a_1 = []among{
	{"gd", -1, -1, ""},
	{"dt", -1, -1, ""},
	{"gt", -1, -1, ""},
	{"kt", -1, -1, ""},
}
var dan_a_2 = []among{
	{"ig", -1, 1, ""},
	{"lig", 0, 1, ""},
	{"elig", 1, 1, ""},
	{"els", -1, 1, ""},
	{"l\u00F8st", -1, 2, ""},
}
var dan_g_c = []byte{119, 223, 119, 1}
var dan_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 0, 128}
var dan_g_s_ending = []byte{239, 254, 42, 3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16}

func (z *danishStemmer) r_mark_regions() bool {
	var I_x int
	z.I_p1 = z.limit
	v_1 := z.cursor
	{
		c := z.cursor + 3
		if c > z.limit {
			return false
		}
		z.cursor = c
	}
	I_x = z.cursor
	z.cursor = v_1
	if !z.goOutGrouping(dan_g_v, 97, 248) {
		return false
	}
	z.cursor++
	if !z.goInGrouping(dan_g_v, 97, 248) {
		return false
	}
	z.cursor++
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= I_x {
			goto lab0_end
		}
		z.I_p1 = I_x
	}
lab0_end:
	;
	return true
}
func (z *danishStemmer) r_main_suffix() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(dan_a_0)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		if !(z.inGroupingB(dan_g_s_ending, 97, 229)) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *danishStemmer) r_consonant_pair() bool {
	v_1 := z.limit - z.cursor
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(dan_a_1) == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	z.cursor = z.limit - v_1
	if z.cursor <= z.limitBackward {
		return false
	}
	z.cursor--
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *danishStemmer) r_other_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("st")) {
			goto lab0_end
		}
		z.bra = z.cursor
		if !(z.eqSB("ig")) {
			goto lab0_end
		}
		z.sliceDel()
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(dan_a_2)
	if among_var == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	switch among_var {
	case 1:
		z.sliceDel()
		v_3 := z.limit - z.cursor
		z.r_consonant_pair()
		z.cursor = z.limit - v_3
		break
	case 2:
		z.sliceFrom("l\u00F8s")
		break
	}
	return true
}
func (z *danishStemmer) r_undouble() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if !(z.inGroupingB(dan_g_c, 98, 122)) {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.sliceCheck()
	if len(z.S_ch) < z.ket-z.bra {
		z.S_ch = append([]rune(nil), z.current[z.bra:z.ket]...)
	} else {
		z.S_ch = append(z.S_ch[:0], z.current[z.bra:z.ket]...)
	}
	z.LS_ch = z.ket - z.bra
	z.limitBackward = v_1
	if !(z.eqSB(string(z.S_ch[:z.LS_ch]))) {
		return false
	}
	z.sliceDel()
	return true
}
func (z *danishStemmer) stem() {
	v_1 := z.cursor
	z.r_mark_regions()
	z.cursor = v_1
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_main_suffix()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_consonant_pair()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_other_suffix()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_undouble()
	z.cursor = z.limit - v_5
	z.cursor = z.limitBackward
	return
}

type estonianStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
}

func newEstonian() *estonianStemmer { return &estonianStemmer{} }

var est_a_0 = []among{
	{"gi", -1, 1, ""},
	{"ki", -1, 2, ""},
}
var est_a_1 = []among{
	{"da", -1, 3, ""},
	{"mata", -1, 1, ""},
	{"b", -1, 3, ""},
	{"ksid", -1, 1, ""},
	{"nuksid", 3, 1, ""},
	{"me", -1, 3, ""},
	{"sime", 5, 1, ""},
	{"ksime", 6, 1, ""},
	{"nuksime", 7, 1, ""},
	{"akse", -1, 2, ""},
	{"dakse", 9, 1, ""},
	{"takse", 9, 1, ""},
	{"site", -1, 1, ""},
	{"ksite", 12, 1, ""},
	{"nuksite", 13, 1, ""},
	{"n", -1, 3, ""},
	{"sin", 15, 1, ""},
	{"ksin", 16, 1, ""},
	{"nuksin", 17, 1, ""},
	{"daks", -1, 1, ""},
	{"taks", -1, 1, ""},
}
var est_a_2 = []among{
	{"aa", -1, -1, ""},
	{"ee", -1, -1, ""},
	{"ii", -1, -1, ""},
	{"oo", -1, -1, ""},
	{"uu", -1, -1, ""},
	{"\u00E4\u00E4", -1, -1, ""},
	{"\u00F5\u00F5", -1, -1, ""},
	{"\u00F6\u00F6", -1, -1, ""},
	{"\u00FC\u00FC", -1, -1, ""},
}
var est_a_3 = []among{
	{"lane", -1, 1, ""},
	{"line", -1, 3, ""},
	{"mine", -1, 2, ""},
	{"lasse", -1, 1, ""},
	{"lisse", -1, 3, ""},
	{"misse", -1, 2, ""},
	{"lasi", -1, 1, ""},
	{"lisi", -1, 3, ""},
	{"misi", -1, 2, ""},
	{"last", -1, 1, ""},
	{"list", -1, 3, ""},
	{"mist", -1, 2, ""},
}
var est_a_4 = []among{
	{"ga", -1, 1, ""},
	{"ta", -1, 1, ""},
	{"le", -1, 1, ""},
	{"sse", -1, 1, ""},
	{"l", -1, 1, ""},
	{"s", -1, 1, ""},
	{"ks", 5, 1, ""},
	{"t", -1, 2, ""},
	{"lt", 7, 1, ""},
	{"st", 7, 1, ""},
}
var est_a_5 = []among{
	{"", -1, 2, ""},
	{"las", 0, 1, ""},
	{"lis", 0, 1, ""},
	{"mis", 0, 1, ""},
	{"t", 0, -1, ""},
}
var est_a_6 = []among{
	{"d", -1, 4, ""},
	{"sid", 0, 2, ""},
	{"de", -1, 4, ""},
	{"ikkude", 2, 1, ""},
	{"ike", -1, 1, ""},
	{"ikke", -1, 1, ""},
	{"te", -1, 3, ""},
}
var est_a_7 = []among{
	{"va", -1, -1, ""},
	{"du", -1, -1, ""},
	{"nu", -1, -1, ""},
	{"tu", -1, -1, ""},
}
var est_a_8 = []among{
	{"kk", -1, 1, ""},
	{"pp", -1, 2, ""},
	{"tt", -1, 3, ""},
}
var est_a_9 = []among{
	{"ma", -1, 2, ""},
	{"mai", -1, 1, ""},
	{"m", -1, 1, ""},
}
var est_a_10 = []among{
	{"joob", -1, 1, ""},
	{"jood", -1, 1, ""},
	{"joodakse", 1, 1, ""},
	{"jooma", -1, 1, ""},
	{"joomata", 3, 1, ""},
	{"joome", -1, 1, ""},
	{"joon", -1, 1, ""},
	{"joote", -1, 1, ""},
	{"joovad", -1, 1, ""},
	{"juua", -1, 1, ""},
	{"juuakse", 9, 1, ""},
	{"j\u00E4i", -1, 12, ""},
	{"j\u00E4id", 11, 12, ""},
	{"j\u00E4ime", 11, 12, ""},
	{"j\u00E4in", 11, 12, ""},
	{"j\u00E4ite", 11, 12, ""},
	{"j\u00E4\u00E4b", -1, 12, ""},
	{"j\u00E4\u00E4d", -1, 12, ""},
	{"j\u00E4\u00E4da", 17, 12, ""},
	{"j\u00E4\u00E4dakse", 18, 12, ""},
	{"j\u00E4\u00E4di", 17, 12, ""},
	{"j\u00E4\u00E4ks", -1, 12, ""},
	{"j\u00E4\u00E4ksid", 21, 12, ""},
	{"j\u00E4\u00E4ksime", 21, 12, ""},
	{"j\u00E4\u00E4ksin", 21, 12, ""},
	{"j\u00E4\u00E4ksite", 21, 12, ""},
	{"j\u00E4\u00E4ma", -1, 12, ""},
	{"j\u00E4\u00E4mata", 26, 12, ""},
	{"j\u00E4\u00E4me", -1, 12, ""},
	{"j\u00E4\u00E4n", -1, 12, ""},
	{"j\u00E4\u00E4te", -1, 12, ""},
	{"j\u00E4\u00E4vad", -1, 12, ""},
	{"j\u00F5i", -1, 1, ""},
	{"j\u00F5id", 32, 1, ""},
	{"j\u00F5ime", 32, 1, ""},
	{"j\u00F5in", 32, 1, ""},
	{"j\u00F5ite", 32, 1, ""},
	{"keeb", -1, 4, ""},
	{"keed", -1, 4, ""},
	{"keedakse", 38, 4, ""},
	{"keeks", -1, 4, ""},
	{"keeksid", 40, 4, ""},
	{"keeksime", 40, 4, ""},
	{"keeksin", 40, 4, ""},
	{"keeksite", 40, 4, ""},
	{"keema", -1, 4, ""},
	{"keemata", 45, 4, ""},
	{"keeme", -1, 4, ""},
	{"keen", -1, 4, ""},
	{"kees", -1, 4, ""},
	{"keeta", -1, 4, ""},
	{"keete", -1, 4, ""},
	{"keevad", -1, 4, ""},
	{"k\u00E4ia", -1, 8, ""},
	{"k\u00E4iakse", 53, 8, ""},
	{"k\u00E4ib", -1, 8, ""},
	{"k\u00E4id", -1, 8, ""},
	{"k\u00E4idi", 56, 8, ""},
	{"k\u00E4iks", -1, 8, ""},
	{"k\u00E4iksid", 58, 8, ""},
	{"k\u00E4iksime", 58, 8, ""},
	{"k\u00E4iksin", 58, 8, ""},
	{"k\u00E4iksite", 58, 8, ""},
	{"k\u00E4ima", -1, 8, ""},
	{"k\u00E4imata", 63, 8, ""},
	{"k\u00E4ime", -1, 8, ""},
	{"k\u00E4in", -1, 8, ""},
	{"k\u00E4is", -1, 8, ""},
	{"k\u00E4ite", -1, 8, ""},
	{"k\u00E4ivad", -1, 8, ""},
	{"laob", -1, 16, ""},
	{"laod", -1, 16, ""},
	{"laoks", -1, 16, ""},
	{"laoksid", 72, 16, ""},
	{"laoksime", 72, 16, ""},
	{"laoksin", 72, 16, ""},
	{"laoksite", 72, 16, ""},
	{"laome", -1, 16, ""},
	{"laon", -1, 16, ""},
	{"laote", -1, 16, ""},
	{"laovad", -1, 16, ""},
	{"loeb", -1, 14, ""},
	{"loed", -1, 14, ""},
	{"loeks", -1, 14, ""},
	{"loeksid", 83, 14, ""},
	{"loeksime", 83, 14, ""},
	{"loeksin", 83, 14, ""},
	{"loeksite", 83, 14, ""},
	{"loeme", -1, 14, ""},
	{"loen", -1, 14, ""},
	{"loete", -1, 14, ""},
	{"loevad", -1, 14, ""},
	{"loob", -1, 7, ""},
	{"lood", -1, 7, ""},
	{"loodi", 93, 7, ""},
	{"looks", -1, 7, ""},
	{"looksid", 95, 7, ""},
	{"looksime", 95, 7, ""},
	{"looksin", 95, 7, ""},
	{"looksite", 95, 7, ""},
	{"looma", -1, 7, ""},
	{"loomata", 100, 7, ""},
	{"loome", -1, 7, ""},
	{"loon", -1, 7, ""},
	{"loote", -1, 7, ""},
	{"loovad", -1, 7, ""},
	{"luua", -1, 7, ""},
	{"luuakse", 106, 7, ""},
	{"l\u00F5i", -1, 6, ""},
	{"l\u00F5id", 108, 6, ""},
	{"l\u00F5ime", 108, 6, ""},
	{"l\u00F5in", 108, 6, ""},
	{"l\u00F5ite", 108, 6, ""},
	{"l\u00F6\u00F6b", -1, 5, ""},
	{"l\u00F6\u00F6d", -1, 5, ""},
	{"l\u00F6\u00F6dakse", 114, 5, ""},
	{"l\u00F6\u00F6di", 114, 5, ""},
	{"l\u00F6\u00F6ks", -1, 5, ""},
	{"l\u00F6\u00F6ksid", 117, 5, ""},
	{"l\u00F6\u00F6ksime", 117, 5, ""},
	{"l\u00F6\u00F6ksin", 117, 5, ""},
	{"l\u00F6\u00F6ksite", 117, 5, ""},
	{"l\u00F6\u00F6ma", -1, 5, ""},
	{"l\u00F6\u00F6mata", 122, 5, ""},
	{"l\u00F6\u00F6me", -1, 5, ""},
	{"l\u00F6\u00F6n", -1, 5, ""},
	{"l\u00F6\u00F6te", -1, 5, ""},
	{"l\u00F6\u00F6vad", -1, 5, ""},
	{"l\u00FC\u00FCa", -1, 5, ""},
	{"l\u00FC\u00FCakse", 128, 5, ""},
	{"m\u00FC\u00FCa", -1, 13, ""},
	{"m\u00FC\u00FCakse", 130, 13, ""},
	{"m\u00FC\u00FCb", -1, 13, ""},
	{"m\u00FC\u00FCd", -1, 13, ""},
	{"m\u00FC\u00FCdi", 133, 13, ""},
	{"m\u00FC\u00FCks", -1, 13, ""},
	{"m\u00FC\u00FCksid", 135, 13, ""},
	{"m\u00FC\u00FCksime", 135, 13, ""},
	{"m\u00FC\u00FCksin", 135, 13, ""},
	{"m\u00FC\u00FCksite", 135, 13, ""},
	{"m\u00FC\u00FCma", -1, 13, ""},
	{"m\u00FC\u00FCmata", 140, 13, ""},
	{"m\u00FC\u00FCme", -1, 13, ""},
	{"m\u00FC\u00FCn", -1, 13, ""},
	{"m\u00FC\u00FCs", -1, 13, ""},
	{"m\u00FC\u00FCte", -1, 13, ""},
	{"m\u00FC\u00FCvad", -1, 13, ""},
	{"n\u00E4eb", -1, 18, ""},
	{"n\u00E4ed", -1, 18, ""},
	{"n\u00E4eks", -1, 18, ""},
	{"n\u00E4eksid", 149, 18, ""},
	{"n\u00E4eksime", 149, 18, ""},
	{"n\u00E4eksin", 149, 18, ""},
	{"n\u00E4eksite", 149, 18, ""},
	{"n\u00E4eme", -1, 18, ""},
	{"n\u00E4en", -1, 18, ""},
	{"n\u00E4ete", -1, 18, ""},
	{"n\u00E4evad", -1, 18, ""},
	{"n\u00E4gema", -1, 18, ""},
	{"n\u00E4gemata", 158, 18, ""},
	{"n\u00E4ha", -1, 18, ""},
	{"n\u00E4hakse", 160, 18, ""},
	{"n\u00E4hti", -1, 18, ""},
	{"p\u00F5eb", -1, 15, ""},
	{"p\u00F5ed", -1, 15, ""},
	{"p\u00F5eks", -1, 15, ""},
	{"p\u00F5eksid", 165, 15, ""},
	{"p\u00F5eksime", 165, 15, ""},
	{"p\u00F5eksin", 165, 15, ""},
	{"p\u00F5eksite", 165, 15, ""},
	{"p\u00F5eme", -1, 15, ""},
	{"p\u00F5en", -1, 15, ""},
	{"p\u00F5ete", -1, 15, ""},
	{"p\u00F5evad", -1, 15, ""},
	{"saab", -1, 2, ""},
	{"saad", -1, 2, ""},
	{"saada", 175, 2, ""},
	{"saadakse", 176, 2, ""},
	{"saadi", 175, 2, ""},
	{"saaks", -1, 2, ""},
	{"saaksid", 179, 2, ""},
	{"saaksime", 179, 2, ""},
	{"saaksin", 179, 2, ""},
	{"saaksite", 179, 2, ""},
	{"saama", -1, 2, ""},
	{"saamata", 184, 2, ""},
	{"saame", -1, 2, ""},
	{"saan", -1, 2, ""},
	{"saate", -1, 2, ""},
	{"saavad", -1, 2, ""},
	{"sai", -1, 2, ""},
	{"said", 190, 2, ""},
	{"saime", 190, 2, ""},
	{"sain", 190, 2, ""},
	{"saite", 190, 2, ""},
	{"s\u00F5i", -1, 9, ""},
	{"s\u00F5id", 195, 9, ""},
	{"s\u00F5ime", 195, 9, ""},
	{"s\u00F5in", 195, 9, ""},
	{"s\u00F5ite", 195, 9, ""},
	{"s\u00F6\u00F6b", -1, 9, ""},
	{"s\u00F6\u00F6d", -1, 9, ""},
	{"s\u00F6\u00F6dakse", 201, 9, ""},
	{"s\u00F6\u00F6di", 201, 9, ""},
	{"s\u00F6\u00F6ks", -1, 9, ""},
	{"s\u00F6\u00F6ksid", 204, 9, ""},
	{"s\u00F6\u00F6ksime", 204, 9, ""},
	{"s\u00F6\u00F6ksin", 204, 9, ""},
	{"s\u00F6\u00F6ksite", 204, 9, ""},
	{"s\u00F6\u00F6ma", -1, 9, ""},
	{"s\u00F6\u00F6mata", 209, 9, ""},
	{"s\u00F6\u00F6me", -1, 9, ""},
	{"s\u00F6\u00F6n", -1, 9, ""},
	{"s\u00F6\u00F6te", -1, 9, ""},
	{"s\u00F6\u00F6vad", -1, 9, ""},
	{"s\u00FC\u00FCa", -1, 9, ""},
	{"s\u00FC\u00FCakse", 215, 9, ""},
	{"teeb", -1, 17, ""},
	{"teed", -1, 17, ""},
	{"teeks", -1, 17, ""},
	{"teeksid", 219, 17, ""},
	{"teeksime", 219, 17, ""},
	{"teeksin", 219, 17, ""},
	{"teeksite", 219, 17, ""},
	{"teeme", -1, 17, ""},
	{"teen", -1, 17, ""},
	{"teete", -1, 17, ""},
	{"teevad", -1, 17, ""},
	{"tegema", -1, 17, ""},
	{"tegemata", 228, 17, ""},
	{"teha", -1, 17, ""},
	{"tehakse", 230, 17, ""},
	{"tehti", -1, 17, ""},
	{"toob", -1, 10, ""},
	{"tood", -1, 10, ""},
	{"toodi", 234, 10, ""},
	{"tooks", -1, 10, ""},
	{"tooksid", 236, 10, ""},
	{"tooksime", 236, 10, ""},
	{"tooksin", 236, 10, ""},
	{"tooksite", 236, 10, ""},
	{"tooma", -1, 10, ""},
	{"toomata", 241, 10, ""},
	{"toome", -1, 10, ""},
	{"toon", -1, 10, ""},
	{"toote", -1, 10, ""},
	{"toovad", -1, 10, ""},
	{"tuua", -1, 10, ""},
	{"tuuakse", 247, 10, ""},
	{"t\u00F5i", -1, 10, ""},
	{"t\u00F5id", 249, 10, ""},
	{"t\u00F5ime", 249, 10, ""},
	{"t\u00F5in", 249, 10, ""},
	{"t\u00F5ite", 249, 10, ""},
	{"viia", -1, 3, ""},
	{"viiakse", 254, 3, ""},
	{"viib", -1, 3, ""},
	{"viid", -1, 3, ""},
	{"viidi", 257, 3, ""},
	{"viiks", -1, 3, ""},
	{"viiksid", 259, 3, ""},
	{"viiksime", 259, 3, ""},
	{"viiksin", 259, 3, ""},
	{"viiksite", 259, 3, ""},
	{"viima", -1, 3, ""},
	{"viimata", 264, 3, ""},
	{"viime", -1, 3, ""},
	{"viin", -1, 3, ""},
	{"viisime", -1, 3, ""},
	{"viisin", -1, 3, ""},
	{"viisite", -1, 3, ""},
	{"viite", -1, 3, ""},
	{"viivad", -1, 3, ""},
	{"v\u00F5ib", -1, 11, ""},
	{"v\u00F5id", -1, 11, ""},
	{"v\u00F5ida", 274, 11, ""},
	{"v\u00F5idakse", 275, 11, ""},
	{"v\u00F5idi", 274, 11, ""},
	{"v\u00F5iks", -1, 11, ""},
	{"v\u00F5iksid", 278, 11, ""},
	{"v\u00F5iksime", 278, 11, ""},
	{"v\u00F5iksin", 278, 11, ""},
	{"v\u00F5iksite", 278, 11, ""},
	{"v\u00F5ima", -1, 11, ""},
	{"v\u00F5imata", 283, 11, ""},
	{"v\u00F5ime", -1, 11, ""},
	{"v\u00F5in", -1, 11, ""},
	{"v\u00F5is", -1, 11, ""},
	{"v\u00F5ite", -1, 11, ""},
	{"v\u00F5ivad", -1, 11, ""},
}
var est_g_V1 = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 48, 8}
var est_g_RV = []byte{17, 65, 16}
var est_g_KI = []byte{117, 66, 6, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 16}
var est_g_GI = []byte{21, 123, 243, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 48, 8}

func (z *estonianStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	if !z.goOutGrouping(est_g_V1, 97, 252) {
		return false
	}
	z.cursor++
	if !z.goInGrouping(est_g_V1, 97, 252) {
		return false
	}
	z.cursor++
	z.I_p1 = z.cursor
	return true
}
func (z *estonianStemmer) r_emphasis() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_0)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	v_2 := z.limit - z.cursor
	{
		c := z.cursor - 4
		if c < z.limitBackward {
			return false
		}
		z.cursor = c
	}
	z.cursor = z.limit - v_2
	switch among_var {
	case 1:
		v_3 := z.limit - z.cursor
		if !(z.inGroupingB(est_g_GI, 97, 252)) {
			return false
		}
		z.cursor = z.limit - v_3
		{
			v_4 := z.limit - z.cursor
			{
				if !z.r_LONGV() {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_4
		}
		z.sliceDel()
		break
	case 2:
		if !(z.inGroupingB(est_g_KI, 98, 382)) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *estonianStemmer) r_verb() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_1)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("a")
		break
	case 3:
		if !(z.inGroupingB(est_g_V1, 97, 252)) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *estonianStemmer) r_LONGV() bool {
	return z.findAmongB(est_a_2) != 0
}
func (z *estonianStemmer) r_i_plural() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if !(z.eqSB("i")) {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	if !(z.inGroupingB(est_g_RV, 97, 117)) {
		return false
	}
	z.sliceDel()
	return true
}
func (z *estonianStemmer) r_special_noun_endings() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_3)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		z.sliceFrom("lase")
		break
	case 2:
		z.sliceFrom("mise")
		break
	case 3:
		z.sliceFrom("lise")
		break
	}
	return true
}
func (z *estonianStemmer) r_case_ending() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_4)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		{
			v_2 := z.limit - z.cursor
			{
				if !(z.inGroupingB(est_g_RV, 97, 117)) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_2
			if !z.r_LONGV() {
				return false
			}
		}
	lab0_end:
		;
		break
	case 2:
		v_3 := z.limit - z.cursor
		{
			c := z.cursor - 4
			if c < z.limitBackward {
				return false
			}
			z.cursor = c
		}
		z.cursor = z.limit - v_3
		break
	}
	z.sliceDel()
	return true
}
func (z *estonianStemmer) r_plural_three_first_cases() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_6)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		z.sliceFrom("iku")
		break
	case 2:
		{
			v_2 := z.limit - z.cursor
			{
				if !z.r_LONGV() {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_2
		}
		z.sliceDel()
		break
	case 3:
		{
			v_3 := z.limit - z.cursor
			{
				v_4 := z.limit - z.cursor
				{
					c := z.cursor - 4
					if c < z.limitBackward {
						goto lab2_end
					}
					z.cursor = c
				}
				z.cursor = z.limit - v_4
				among_var = z.findAmongB(est_a_5)
				switch among_var {
				case 1:
					z.sliceFrom("e")
					break
				case 2:
					z.sliceDel()
					break
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			z.sliceFrom("t")
		}
	lab1_end:
		;
		break
	case 4:
		{
			v_5 := z.limit - z.cursor
			{
				if !(z.inGroupingB(est_g_RV, 97, 117)) {
					goto lab4_end
				}
				goto lab3_end
			}
		lab4_end:
			;
			z.cursor = z.limit - v_5
			if !z.r_LONGV() {
				return false
			}
		}
	lab3_end:
		;
		z.sliceDel()
		break
	}
	return true
}
func (z *estonianStemmer) r_nu() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(est_a_7) == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	z.sliceDel()
	return true
}
func (z *estonianStemmer) r_undouble_kpt() bool {
	var among_var int
	if !(z.inGroupingB(est_g_V1, 97, 252)) {
		return false
	}
	if z.I_p1 > z.cursor {
		return false
	}
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_8)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("k")
		break
	case 2:
		z.sliceFrom("p")
		break
	case 3:
		z.sliceFrom("t")
		break
	}
	return true
}
func (z *estonianStemmer) r_degrees() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(est_a_9)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		if !(z.inGroupingB(est_g_RV, 97, 117)) {
			return false
		}
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	}
	return true
}
func (z *estonianStemmer) r_substantive() bool {
	v_1 := z.limit - z.cursor
	z.r_special_noun_endings()
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	z.r_case_ending()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_plural_three_first_cases()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_degrees()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_i_plural()
	z.cursor = z.limit - v_5
	v_6 := z.limit - z.cursor
	z.r_nu()
	z.cursor = z.limit - v_6
	return true
}
func (z *estonianStemmer) r_verb_exceptions() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(est_a_10)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	if z.cursor < z.limit {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("joo")
		break
	case 2:
		z.sliceFrom("saa")
		break
	case 3:
		z.sliceFrom("viima")
		break
	case 4:
		z.sliceFrom("keesi")
		break
	case 5:
		z.sliceFrom("l\u00F6\u00F6")
		break
	case 6:
		z.sliceFrom("l\u00F5i")
		break
	case 7:
		z.sliceFrom("loo")
		break
	case 8:
		z.sliceFrom("k\u00E4isi")
		break
	case 9:
		z.sliceFrom("s\u00F6\u00F6")
		break
	case 10:
		z.sliceFrom("too")
		break
	case 11:
		z.sliceFrom("v\u00F5isi")
		break
	case 12:
		z.sliceFrom("j\u00E4\u00E4ma")
		break
	case 13:
		z.sliceFrom("m\u00FC\u00FCsi")
		break
	case 14:
		z.sliceFrom("luge")
		break
	case 15:
		z.sliceFrom("p\u00F5de")
		break
	case 16:
		z.sliceFrom("ladu")
		break
	case 17:
		z.sliceFrom("tegi")
		break
	case 18:
		z.sliceFrom("n\u00E4gi")
		break
	}
	return true
}
func (z *estonianStemmer) stem() {
	{
		v_1 := z.cursor
		{
			if !z.r_verb_exceptions() {
				goto lab0_end
			}
			return
		}
	lab0_end:
		;
		z.cursor = v_1
	}
	v_2 := z.cursor
	z.r_mark_regions()
	z.cursor = v_2
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_3 := z.limit - z.cursor
	z.r_emphasis()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	{
		{
			v_5 := z.limit - z.cursor
			{
				if !z.r_verb() {
					goto lab3_end
				}
				goto lab2_end
			}
		lab3_end:
			;
			z.cursor = z.limit - v_5
			z.r_substantive()
		}
	lab2_end:
	}
	z.cursor = z.limit - v_4
	v_6 := z.limit - z.cursor
	z.r_undouble_kpt()
	z.cursor = z.limit - v_6
	z.cursor = z.limitBackward
	return
}

type finnishStemmer struct {
	snowball
	I_p1, I_p2       int
	S_ch, S_x        []rune
	LS_ch, LS_x      int
	B_ending_removed bool
}

func newFinnish() *finnishStemmer {
	z := &finnishStemmer{}
	z.methods = map[string]func() bool{
		"r_LONG": z.r_LONG,
		"r_VI":   z.r_VI,
	}
	return z
}

var fin_a_0 = []among{
	{"pa", -1, 1, ""},
	{"sti", -1, 2, ""},
	{"kaan", -1, 1, ""},
	{"han", -1, 1, ""},
	{"kin", -1, 1, ""},
	{"h\u00E4n", -1, 1, ""},
	{"k\u00E4\u00E4n", -1, 1, ""},
	{"ko", -1, 1, ""},
	{"p\u00E4", -1, 1, ""},
	{"k\u00F6", -1, 1, ""},
}
var fin_a_1 = []among{
	{"lla", -1, -1, ""},
	{"na", -1, -1, ""},
	{"ssa", -1, -1, ""},
	{"ta", -1, -1, ""},
	{"lta", 3, -1, ""},
	{"sta", 3, -1, ""},
}
var fin_a_2 = []among{
	{"ll\u00E4", -1, -1, ""},
	{"n\u00E4", -1, -1, ""},
	{"ss\u00E4", -1, -1, ""},
	{"t\u00E4", -1, -1, ""},
	{"lt\u00E4", 3, -1, ""},
	{"st\u00E4", 3, -1, ""},
}
var fin_a_3 = []among{
	{"lle", -1, -1, ""},
	{"ine", -1, -1, ""},
}
var fin_a_4 = []among{
	{"nsa", -1, 3, ""},
	{"mme", -1, 3, ""},
	{"nne", -1, 3, ""},
	{"ni", -1, 2, ""},
	{"si", -1, 1, ""},
	{"an", -1, 4, ""},
	{"en", -1, 6, ""},
	{"\u00E4n", -1, 5, ""},
	{"ns\u00E4", -1, 3, ""},
}
var fin_a_5 = []among{
	{"aa", -1, -1, ""},
	{"ee", -1, -1, ""},
	{"ii", -1, -1, ""},
	{"oo", -1, -1, ""},
	{"uu", -1, -1, ""},
	{"\u00E4\u00E4", -1, -1, ""},
	{"\u00F6\u00F6", -1, -1, ""},
}
var fin_a_6 = []among{
	{"a", -1, 8, ""},
	{"lla", 0, -1, ""},
	{"na", 0, -1, ""},
	{"ssa", 0, -1, ""},
	{"ta", 0, -1, ""},
	{"lta", 4, -1, ""},
	{"sta", 4, -1, ""},
	{"tta", 4, 2, ""},
	{"lle", -1, -1, ""},
	{"ine", -1, -1, ""},
	{"ksi", -1, -1, ""},
	{"n", -1, 7, ""},
	{"han", 11, 1, ""},
	{"den", 11, -1, "r_VI"},
	{"seen", 11, -1, "r_LONG"},
	{"hen", 11, 2, ""},
	{"tten", 11, -1, "r_VI"},
	{"hin", 11, 3, ""},
	{"siin", 11, -1, "r_VI"},
	{"hon", 11, 4, ""},
	{"h\u00E4n", 11, 5, ""},
	{"h\u00F6n", 11, 6, ""},
	{"\u00E4", -1, 8, ""},
	{"ll\u00E4", 22, -1, ""},
	{"n\u00E4", 22, -1, ""},
	{"ss\u00E4", 22, -1, ""},
	{"t\u00E4", 22, -1, ""},
	{"lt\u00E4", 26, -1, ""},
	{"st\u00E4", 26, -1, ""},
	{"tt\u00E4", 26, 2, ""},
}
var fin_a_7 = []among{
	{"eja", -1, -1, ""},
	{"mma", -1, 1, ""},
	{"imma", 1, -1, ""},
	{"mpa", -1, 1, ""},
	{"impa", 3, -1, ""},
	{"mmi", -1, 1, ""},
	{"immi", 5, -1, ""},
	{"mpi", -1, 1, ""},
	{"impi", 7, -1, ""},
	{"ej\u00E4", -1, -1, ""},
	{"mm\u00E4", -1, 1, ""},
	{"imm\u00E4", 10, -1, ""},
	{"mp\u00E4", -1, 1, ""},
	{"imp\u00E4", 12, -1, ""},
}
var fin_a_8 = []among{
	{"i", -1, -1, ""},
	{"j", -1, -1, ""},
}
var fin_a_9 = []among{
	{"mma", -1, 1, ""},
	{"imma", 0, -1, ""},
}
var fin_g_AEI = []byte{17, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8}
var fin_g_C = []byte{119, 223, 119, 1}
var fin_g_V1 = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32}
var fin_g_V2 = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32}
var fin_g_particle_end = []byte{17, 97, 24, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32}

func (z *finnishStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	if !z.goOutGrouping(fin_g_V1, 97, 246) {
		return false
	}
	z.cursor++
	if !z.goInGrouping(fin_g_V1, 97, 246) {
		return false
	}
	z.cursor++
	z.I_p1 = z.cursor
	if !z.goOutGrouping(fin_g_V1, 97, 246) {
		return false
	}
	z.cursor++
	if !z.goInGrouping(fin_g_V1, 97, 246) {
		return false
	}
	z.cursor++
	z.I_p2 = z.cursor
	return true
}
func (z *finnishStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *finnishStemmer) r_particle_etc() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(fin_a_0)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		if !(z.inGroupingB(fin_g_particle_end, 97, 246)) {
			return false
		}
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		break
	}
	z.sliceDel()
	return true
}
func (z *finnishStemmer) r_possessive() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(fin_a_4)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		{
			v_2 := z.limit - z.cursor
			{
				if !(z.eqSB("k")) {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_2
		}
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		z.ket = z.cursor
		if !(z.eqSB("kse")) {
			return false
		}
		z.bra = z.cursor
		z.sliceFrom("ksi")
		break
	case 3:
		z.sliceDel()
		break
	case 4:
		if z.findAmongB(fin_a_1) == 0 {
			return false
		}
		z.sliceDel()
		break
	case 5:
		if z.findAmongB(fin_a_2) == 0 {
			return false
		}
		z.sliceDel()
		break
	case 6:
		if z.findAmongB(fin_a_3) == 0 {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *finnishStemmer) r_LONG() bool {
	return z.findAmongB(fin_a_5) != 0
}
func (z *finnishStemmer) r_VI() bool {
	if !(z.eqSB("i")) {
		return false
	}
	return z.inGroupingB(fin_g_V2, 97, 246)
}
func (z *finnishStemmer) r_case_ending() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(fin_a_6)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		if !(z.eqSB("a")) {
			return false
		}
		break
	case 2:
		if !(z.eqSB("e")) {
			return false
		}
		break
	case 3:
		if !(z.eqSB("i")) {
			return false
		}
		break
	case 4:
		if !(z.eqSB("o")) {
			return false
		}
		break
	case 5:
		if !(z.eqSB("\u00E4")) {
			return false
		}
		break
	case 6:
		if !(z.eqSB("\u00F6")) {
			return false
		}
		break
	case 7:
		v_2 := z.limit - z.cursor
		{
			v_3 := z.limit - z.cursor
			{
				v_4 := z.limit - z.cursor
				{
					if !z.r_LONG() {
						goto lab2_end
					}
					goto lab1_end
				}
			lab2_end:
				;
				z.cursor = z.limit - v_4
				if !(z.eqSB("ie")) {
					z.cursor = z.limit - v_2
					goto lab0_end
				}
			}
		lab1_end:
			;
			z.cursor = z.limit - v_3
			if z.cursor <= z.limitBackward {
				z.cursor = z.limit - v_2
				goto lab0_end
			}
			z.cursor--
			z.bra = z.cursor
		}
	lab0_end:
		;
		break
	case 8:
		if !(z.inGroupingB(fin_g_V1, 97, 246)) {
			return false
		}
		if !(z.inGroupingB(fin_g_C, 98, 122)) {
			return false
		}
		break
	}
	z.sliceDel()
	z.B_ending_removed = true
	return true
}
func (z *finnishStemmer) r_other_endings() bool {
	var among_var int
	if z.cursor < z.I_p2 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p2
	z.ket = z.cursor
	among_var = z.findAmongB(fin_a_7)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		{
			v_2 := z.limit - z.cursor
			{
				if !(z.eqSB("po")) {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_2
		}
		break
	}
	z.sliceDel()
	return true
}
func (z *finnishStemmer) r_i_plural() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(fin_a_8) == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	z.sliceDel()
	return true
}
func (z *finnishStemmer) r_t_plural() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if !(z.eqSB("t")) {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	v_2 := z.limit - z.cursor
	if !(z.inGroupingB(fin_g_V1, 97, 246)) {
		z.limitBackward = v_1
		return false
	}
	z.cursor = z.limit - v_2
	z.sliceDel()
	z.limitBackward = v_1
	if z.cursor < z.I_p2 {
		return false
	}
	v_3 := z.limitBackward
	z.limitBackward = z.I_p2
	z.ket = z.cursor
	among_var = z.findAmongB(fin_a_9)
	if among_var == 0 {
		z.limitBackward = v_3
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_3
	switch among_var {
	case 1:
		{
			v_4 := z.limit - z.cursor
			{
				if !(z.eqSB("po")) {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_4
		}
		break
	}
	z.sliceDel()
	return true
}
func (z *finnishStemmer) r_tidy() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_p1
	v_2 := z.limit - z.cursor
	{
		v_3 := z.limit - z.cursor
		if !z.r_LONG() {
			goto lab0_end
		}
		z.cursor = z.limit - v_3
		z.ket = z.cursor
		if z.cursor <= z.limitBackward {
			goto lab0_end
		}
		z.cursor--
		z.bra = z.cursor
		z.sliceDel()
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	v_4 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.inGroupingB(fin_g_AEI, 97, 228)) {
			goto lab1_end
		}
		z.bra = z.cursor
		if !(z.inGroupingB(fin_g_C, 98, 122)) {
			goto lab1_end
		}
		z.sliceDel()
	}
lab1_end:
	;
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("j")) {
			goto lab2_end
		}
		z.bra = z.cursor
		{
			v_6 := z.limit - z.cursor
			{
				if !(z.eqSB("o")) {
					goto lab4_end
				}
				goto lab3_end
			}
		lab4_end:
			;
			z.cursor = z.limit - v_6
			if !(z.eqSB("u")) {
				goto lab2_end
			}
		}
	lab3_end:
		;
		z.sliceDel()
	}
lab2_end:
	;
	z.cursor = z.limit - v_5
	v_7 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("o")) {
			goto lab5_end
		}
		z.bra = z.cursor
		if !(z.eqSB("j")) {
			goto lab5_end
		}
		z.sliceDel()
	}
lab5_end:
	;
	z.cursor = z.limit - v_7
	z.limitBackward = v_1
	if !z.goInGroupingB(fin_g_V1, 97, 246) {
		return false
	}
	z.ket = z.cursor
	if !(z.inGroupingB(fin_g_C, 98, 122)) {
		return false
	}
	z.bra = z.cursor
	z.sliceCheck()
	if len(z.S_x) < z.ket-z.bra {
		z.S_x = append([]rune(nil), z.current[z.bra:z.ket]...)
	} else {
		z.S_x = append(z.S_x[:0], z.current[z.bra:z.ket]...)
	}
	z.LS_x = z.ket - z.bra
	if !(z.eqSB(string(z.S_x[:z.LS_x]))) {
		return false
	}
	z.sliceDel()
	return true
}
func (z *finnishStemmer) stem() {
	v_1 := z.cursor
	z.r_mark_regions()
	z.cursor = v_1
	z.B_ending_removed = false
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_particle_etc()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_possessive()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_case_ending()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_other_endings()
	z.cursor = z.limit - v_5
	{
		{
			if !z.B_ending_removed {
				goto lab1_end
			}
			v_6 := z.limit - z.cursor
			z.r_i_plural()
			z.cursor = z.limit - v_6
			goto lab0_end
		}
	lab1_end:
		;
		v_7 := z.limit - z.cursor
		z.r_t_plural()
		z.cursor = z.limit - v_7
	}
lab0_end:
	;
	v_8 := z.limit - z.cursor
	z.r_tidy()
	z.cursor = z.limit - v_8
	z.cursor = z.limitBackward
	return
}

type hindiStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
}

func newHindi() *hindiStemmer {
	z := &hindiStemmer{}
	z.methods = map[string]func() bool{
		"r_CONSONANT": z.r_CONSONANT,
	}
	return z
}

var hin_a_0 = []among{
	{"\u0906\u0901", -1, -1, ""},
	{"\u093E\u0901", -1, -1, ""},
	{"\u0907\u092F\u093E\u0901", 1, -1, ""},
	{"\u0906\u0907\u092F\u093E\u0901", 2, -1, ""},
	{"\u093E\u0907\u092F\u093E\u0901", 2, -1, ""},
	{"\u093F\u092F\u093E\u0901", 1, -1, ""},
	{"\u0906\u0902", -1, -1, ""},
	{"\u0909\u0906\u0902", 6, -1, ""},
	{"\u0941\u0906\u0902", 6, -1, ""},
	{"\u0908\u0902", -1, -1, ""},
	{"\u0906\u0908\u0902", 9, -1, ""},
	{"\u093E\u0908\u0902", 9, -1, ""},
	{"\u090F\u0902", -1, -1, ""},
	{"\u0906\u090F\u0902", 12, -1, ""},
	{"\u0909\u090F\u0902", 12, -1, ""},
	{"\u093E\u090F\u0902", 12, -1, ""},
	{"\u0924\u093E\u090F\u0902", 15, -1, "r_CONSONANT"},
	{"\u0905\u0924\u093E\u090F\u0902", 16, -1, ""},
	{"\u0928\u093E\u090F\u0902", 15, -1, "r_CONSONANT"},
	{"\u0905\u0928\u093E\u090F\u0902", 18, -1, ""},
	{"\u0941\u090F\u0902", 12, -1, ""},
	{"\u0913\u0902", -1, -1, ""},
	{"\u0906\u0913\u0902", 21, -1, ""},
	{"\u0909\u0913\u0902", 21, -1, ""},
	{"\u093E\u0913\u0902", 21, -1, ""},
	{"\u0924\u093E\u0913\u0902", 24, -1, "r_CONSONANT"},
	{"\u0905\u0924\u093E\u0913\u0902", 25, -1, ""},
	{"\u0928\u093E\u0913\u0902", 24, -1, "r_CONSONANT"},
	{"\u0905\u0928\u093E\u0913\u0902", 27, -1, ""},
	{"\u0941\u0913\u0902", 21, -1, ""},
	{"\u093E\u0902", -1, -1, ""},
	{"\u0907\u092F\u093E\u0902", 30, -1, ""},
	{"\u0906\u0907\u092F\u093E\u0902", 31, -1, ""},
	{"\u093E\u0907\u092F\u093E\u0902", 31, -1, ""},
	{"\u093F\u092F\u093E\u0902", 30, -1, ""},
	{"\u0940\u0902", -1, -1, ""},
	{"\u0924\u0940\u0902", 35, -1, "r_CONSONANT"},
	{"\u0905\u0924\u0940\u0902", 36, -1, ""},
	{"\u0906\u0924\u0940\u0902", 36, -1, ""},
	{"\u093E\u0924\u0940\u0902", 36, -1, ""},
	{"\u0947\u0902", -1, -1, ""},
	{"\u094B\u0902", -1, -1, ""},
	{"\u0907\u092F\u094B\u0902", 41, -1, ""},
	{"\u0906\u0907\u092F\u094B\u0902", 42, -1, ""},
	{"\u093E\u0907\u092F\u094B\u0902", 42, -1, ""},
	{"\u093F\u092F\u094B\u0902", 41, -1, ""},
	{"\u0905", -1, -1, ""},
	{"\u0906", -1, -1, ""},
	{"\u0907", -1, -1, ""},
	{"\u0908", -1, -1, ""},
	{"\u0906\u0908", 49, -1, ""},
	{"\u093E\u0908", 49, -1, ""},
	{"\u0909", -1, -1, ""},
	{"\u090A", -1, -1, ""},
	{"\u090F", -1, -1, ""},
	{"\u0906\u090F", 54, -1, ""},
	{"\u0907\u090F", 54, -1, ""},
	{"\u0906\u0907\u090F", 56, -1, ""},
	{"\u093E\u0907\u090F", 56, -1, ""},
	{"\u093E\u090F", 54, -1, ""},
	{"\u093F\u090F", 54, -1, ""},
	{"\u0913", -1, -1, ""},
	{"\u0906\u0913", 61, -1, ""},
	{"\u093E\u0913", 61, -1, ""},
	{"\u0915\u0930", -1, -1, "r_CONSONANT"},
	{"\u0905\u0915\u0930", 64, -1, ""},
	{"\u0906\u0915\u0930", 64, -1, ""},
	{"\u093E\u0915\u0930", 64, -1, ""},
	{"\u093E", -1, -1, ""},
	{"\u090A\u0902\u0917\u093E", 68, -1, ""},
	{"\u0906\u090A\u0902\u0917\u093E", 69, -1, ""},
	{"\u093E\u090A\u0902\u0917\u093E", 69, -1, ""},
	{"\u0942\u0902\u0917\u093E", 68, -1, ""},
	{"\u090F\u0917\u093E", 68, -1, ""},
	{"\u0906\u090F\u0917\u093E", 73, -1, ""},
	{"\u093E\u090F\u0917\u093E", 73, -1, ""},
	{"\u0947\u0917\u093E", 68, -1, ""},
	{"\u0924\u093E", 68, -1, "r_CONSONANT"},
	{"\u0905\u0924\u093E", 77, -1, ""},
	{"\u0906\u0924\u093E", 77, -1, ""},
	{"\u093E\u0924\u093E", 77, -1, ""},
	{"\u0928\u093E", 68, -1, "r_CONSONANT"},
	{"\u0905\u0928\u093E", 81, -1, ""},
	{"\u0906\u0928\u093E", 81, -1, ""},
	{"\u093E\u0928\u093E", 81, -1, ""},
	{"\u0906\u092F\u093E", 68, -1, ""},
	{"\u093E\u092F\u093E", 68, -1, ""},
	{"\u093F", -1, -1, ""},
	{"\u0940", -1, -1, ""},
	{"\u090A\u0902\u0917\u0940", 88, -1, ""},
	{"\u0906\u090A\u0902\u0917\u0940", 89, -1, ""},
	{"\u093E\u090A\u0902\u0917\u0940", 89, -1, ""},
	{"\u090F\u0902\u0917\u0940", 88, -1, ""},
	{"\u0906\u090F\u0902\u0917\u0940", 92, -1, ""},
	{"\u093E\u090F\u0902\u0917\u0940", 92, -1, ""},
	{"\u0942\u0902\u0917\u0940", 88, -1, ""},
	{"\u0947\u0902\u0917\u0940", 88, -1, ""},
	{"\u090F\u0917\u0940", 88, -1, ""},
	{"\u0906\u090F\u0917\u0940", 97, -1, ""},
	{"\u093E\u090F\u0917\u0940", 97, -1, ""},
	{"\u0913\u0917\u0940", 88, -1, ""},
	{"\u0906\u0913\u0917\u0940", 100, -1, ""},
	{"\u093E\u0913\u0917\u0940", 100, -1, ""},
	{"\u0947\u0917\u0940", 88, -1, ""},
	{"\u094B\u0917\u0940", 88, -1, ""},
	{"\u0924\u0940", 88, -1, "r_CONSONANT"},
	{"\u0905\u0924\u0940", 105, -1, ""},
	{"\u0906\u0924\u0940", 105, -1, ""},
	{"\u093E\u0924\u0940", 105, -1, ""},
	{"\u0928\u0940", 88, -1, "r_CONSONANT"},
	{"\u0905\u0928\u0940", 109, -1, ""},
	{"\u0941", -1, -1, ""},
	{"\u0942", -1, -1, ""},
	{"\u0947", -1, -1, ""},
	{"\u090F\u0902\u0917\u0947", 113, -1, ""},
	{"\u0906\u090F\u0902\u0917\u0947", 114, -1, ""},
	{"\u093E\u090F\u0902\u0917\u0947", 114, -1, ""},
	{"\u0947\u0902\u0917\u0947", 113, -1, ""},
	{"\u0913\u0917\u0947", 113, -1, ""},
	{"\u0906\u0913\u0917\u0947", 118, -1, ""},
	{"\u093E\u0913\u0917\u0947", 118, -1, ""},
	{"\u094B\u0917\u0947", 113, -1, ""},
	{"\u0924\u0947", 113, -1, "r_CONSONANT"},
	{"\u0905\u0924\u0947", 122, -1, ""},
	{"\u0906\u0924\u0947", 122, -1, ""},
	{"\u093E\u0924\u0947", 122, -1, ""},
	{"\u0928\u0947", 113, -1, "r_CONSONANT"},
	{"\u0905\u0928\u0947", 126, -1, ""},
	{"\u0906\u0928\u0947", 126, -1, ""},
	{"\u093E\u0928\u0947", 126, -1, ""},
	{"\u094B", -1, -1, ""},
	{"\u094D", -1, -1, ""},
}
var hin_g_consonant = []byte{255, 255, 255, 255, 159, 0, 0, 0, 248, 7}

func (z *hindiStemmer) r_CONSONANT() bool {
	return z.inGroupingB(hin_g_consonant, 2325, 2399)
}
func (z *hindiStemmer) stem() {
	if z.cursor >= z.limit {
		return
	}
	z.cursor++
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.ket = z.cursor
	if z.findAmongB(hin_a_0) == 0 {
		return
	}
	z.bra = z.cursor
	z.sliceDel()
	z.cursor = z.limitBackward
	return
}

type indonesianStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_prefix    int
	I_measure   int
}

func newIndonesian() *indonesianStemmer { return &indonesianStemmer{} }

var ind_a_0 = []among{
	{"kah", -1, 1, ""},
	{"lah", -1, 1, ""},
	{"pun", -1, 1, ""},
}
var ind_a_1 = []among{
	{"nya", -1, 1, ""},
	{"ku", -1, 1, ""},
	{"mu", -1, 1, ""},
}
var ind_a_2 = []among{
	{"i", -1, 2, ""},
	{"an", -1, 1, ""},
}
var ind_a_3 = []among{
	{"di", -1, 1, ""},
	{"ke", -1, 3, ""},
	{"me", -1, 1, ""},
	{"mem", 2, 5, ""},
	{"men", 2, 2, ""},
	{"meng", 4, 1, ""},
	{"pem", -1, 6, ""},
	{"pen", -1, 4, ""},
	{"peng", 7, 3, ""},
	{"ter", -1, 1, ""},
}
var ind_a_4 = []among{
	{"be", -1, 2, ""},
	{"pe", -1, 1, ""},
}
var ind_g_vowel = []byte{17, 65, 16}

func (z *indonesianStemmer) r_remove_particle() bool {
	z.ket = z.cursor
	if z.findAmongB(ind_a_0) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.I_measure -= 1
	return true
}
func (z *indonesianStemmer) r_remove_possessive_pronoun() bool {
	z.ket = z.cursor
	if z.findAmongB(ind_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.I_measure -= 1
	return true
}
func (z *indonesianStemmer) r_remove_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ind_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_1 := z.limit - z.cursor
			{
				if z.I_prefix == 3 {
					goto lab1_end
				}
				if z.I_prefix == 2 {
					goto lab1_end
				}
				if !(z.eqSB("k")) {
					goto lab1_end
				}
				z.bra = z.cursor
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			if z.I_prefix == 1 {
				return false
			}
		}
	lab0_end:
		;
		break
	case 2:
		if z.I_prefix > 2 {
			return false
		}
		{
			v_2 := z.limit - z.cursor
			{
				if !(z.eqSB("s")) {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_2
		}
		break
	}
	z.sliceDel()
	z.I_measure -= 1
	return true
}
func (z *indonesianStemmer) r_remove_first_order_prefix() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ind_a_3)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		z.sliceDel()
		z.I_prefix = 1
		z.I_measure -= 1
		break
	case 2:
		{
			v_1 := z.cursor
			{
				if !(z.eqS("y")) {
					goto lab1_end
				}
				v_2 := z.cursor
				if !(z.inGrouping(ind_g_vowel, 97, 117)) {
					goto lab1_end
				}
				z.cursor = v_2
				z.ket = z.cursor
				z.sliceFrom("s")
				z.I_prefix = 1
				z.I_measure -= 1
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = v_1
			z.sliceDel()
			z.I_prefix = 1
			z.I_measure -= 1
		}
	lab0_end:
		;
		break
	case 3:
		z.sliceDel()
		z.I_prefix = 3
		z.I_measure -= 1
		break
	case 4:
		{
			v_3 := z.cursor
			{
				if !(z.eqS("y")) {
					goto lab3_end
				}
				v_4 := z.cursor
				if !(z.inGrouping(ind_g_vowel, 97, 117)) {
					goto lab3_end
				}
				z.cursor = v_4
				z.ket = z.cursor
				z.sliceFrom("s")
				z.I_prefix = 3
				z.I_measure -= 1
				goto lab2_end
			}
		lab3_end:
			;
			z.cursor = v_3
			z.sliceDel()
			z.I_prefix = 3
			z.I_measure -= 1
		}
	lab2_end:
		;
		break
	case 5:
		z.I_prefix = 1
		z.I_measure -= 1
		{
			v_5 := z.cursor
			{
				v_6 := z.cursor
				if !(z.inGrouping(ind_g_vowel, 97, 117)) {
					goto lab5_end
				}
				z.cursor = v_6
				z.sliceFrom("p")
				goto lab4_end
			}
		lab5_end:
			;
			z.cursor = v_5
			z.sliceDel()
		}
	lab4_end:
		;
		break
	case 6:
		z.I_prefix = 3
		z.I_measure -= 1
		{
			v_7 := z.cursor
			{
				v_8 := z.cursor
				if !(z.inGrouping(ind_g_vowel, 97, 117)) {
					goto lab7_end
				}
				z.cursor = v_8
				z.sliceFrom("p")
				goto lab6_end
			}
		lab7_end:
			;
			z.cursor = v_7
			z.sliceDel()
		}
	lab6_end:
		;
		break
	}
	return true
}
func (z *indonesianStemmer) r_remove_second_order_prefix() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(ind_a_4)
	if among_var == 0 {
		return false
	}
	switch among_var {
	case 1:
		{
			v_1 := z.cursor
			{
				if !(z.eqS("r")) {
					goto lab1_end
				}
				z.ket = z.cursor
				z.I_prefix = 2
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = v_1
			{
				if !(z.eqS("l")) {
					goto lab2_end
				}
				z.ket = z.cursor
				if !(z.eqS("ajar")) {
					goto lab2_end
				}
				goto lab0_end
			}
		lab2_end:
			;
			z.cursor = v_1
			z.ket = z.cursor
			z.I_prefix = 2
		}
	lab0_end:
		;
		break
	case 2:
		{
			v_2 := z.cursor
			{
				if !(z.eqS("r")) {
					goto lab4_end
				}
				z.ket = z.cursor
				goto lab3_end
			}
		lab4_end:
			;
			z.cursor = v_2
			{
				if !(z.eqS("l")) {
					goto lab5_end
				}
				z.ket = z.cursor
				if !(z.eqS("ajar")) {
					goto lab5_end
				}
				goto lab3_end
			}
		lab5_end:
			;
			z.cursor = v_2
			z.ket = z.cursor
			if !(z.outGrouping(ind_g_vowel, 97, 117)) {
				return false
			}
			if !(z.eqS("er")) {
				return false
			}
		}
	lab3_end:
		;
		z.I_prefix = 4
		break
	}
	z.I_measure -= 1
	z.sliceDel()
	return true
}
func (z *indonesianStemmer) stem() {
	z.I_measure = 0
	v_1 := z.cursor
	{
		for {
			v_2 := z.cursor
			{
				if !z.goOutGrouping(ind_g_vowel, 97, 117) {
					goto lab1_end
				}
				z.cursor++
				z.I_measure += 1
				continue
			}
		lab1_end:
			;
			z.cursor = v_2
			break
		}
	}
	z.cursor = v_1
	if z.I_measure <= 2 {
		return
	}
	z.I_prefix = 0
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_3 := z.limit - z.cursor
	z.r_remove_particle()
	z.cursor = z.limit - v_3
	if z.I_measure <= 2 {
		return
	}
	v_4 := z.limit - z.cursor
	z.r_remove_possessive_pronoun()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	if z.I_measure <= 2 {
		return
	}
	{
		v_5 := z.cursor
		{
			v_6 := z.cursor
			if !z.r_remove_first_order_prefix() {
				goto lab3_end
			}
			v_7 := z.cursor
			{
				v_8 := z.cursor
				if z.I_measure <= 2 {
					goto lab4_end
				}
				z.limitBackward = z.cursor
				z.cursor = z.limit
				if !z.r_remove_suffix() {
					goto lab4_end
				}
				z.cursor = z.limitBackward
				z.cursor = v_8
				if z.I_measure <= 2 {
					goto lab4_end
				}
				if !z.r_remove_second_order_prefix() {
					goto lab4_end
				}
			}
		lab4_end:
			;
			z.cursor = v_7
			z.cursor = v_6
			goto lab2_end
		}
	lab3_end:
		;
		z.cursor = v_5
		v_9 := z.cursor
		z.r_remove_second_order_prefix()
		z.cursor = v_9
		v_10 := z.cursor
		{
			if z.I_measure <= 2 {
				goto lab5_end
			}
			z.limitBackward = z.cursor
			z.cursor = z.limit
			if !z.r_remove_suffix() {
				goto lab5_end
			}
			z.cursor = z.limitBackward
		}
	lab5_end:
		;
		z.cursor = v_10
	}
lab2_end:
	;
	return
}

type irishStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newIrish() *irishStemmer { return &irishStemmer{} }

var iri_a_0 = []among{
	{"b'", -1, 1, ""},
	{"bh", -1, 4, ""},
	{"bhf", 1, 2, ""},
	{"bp", -1, 8, ""},
	{"ch", -1, 5, ""},
	{"d'", -1, 1, ""},
	{"d'fh", 5, 2, ""},
	{"dh", -1, 6, ""},
	{"dt", -1, 9, ""},
	{"fh", -1, 2, ""},
	{"gc", -1, 5, ""},
	{"gh", -1, 7, ""},
	{"h-", -1, 1, ""},
	{"m'", -1, 1, ""},
	{"mb", -1, 4, ""},
	{"mh", -1, 10, ""},
	{"n-", -1, 1, ""},
	{"nd", -1, 6, ""},
	{"ng", -1, 7, ""},
	{"ph", -1, 8, ""},
	{"sh", -1, 3, ""},
	{"t-", -1, 1, ""},
	{"th", -1, 9, ""},
	{"ts", -1, 3, ""},
}
var iri_a_1 = []among{
	{"\u00EDochta", -1, 1, ""},
	{"a\u00EDochta", 0, 1, ""},
	{"ire", -1, 2, ""},
	{"aire", 2, 2, ""},
	{"abh", -1, 1, ""},
	{"eabh", 4, 1, ""},
	{"ibh", -1, 1, ""},
	{"aibh", 6, 1, ""},
	{"amh", -1, 1, ""},
	{"eamh", 8, 1, ""},
	{"imh", -1, 1, ""},
	{"aimh", 10, 1, ""},
	{"\u00EDocht", -1, 1, ""},
	{"a\u00EDocht", 12, 1, ""},
	{"ir\u00ED", -1, 2, ""},
	{"air\u00ED", 14, 2, ""},
}
var iri_a_2 = []among{
	{"\u00F3ideacha", -1, 6, ""},
	{"patacha", -1, 5, ""},
	{"achta", -1, 1, ""},
	{"arcachta", 2, 2, ""},
	{"eachta", 2, 1, ""},
	{"grafa\u00EDochta", -1, 4, ""},
	{"paite", -1, 5, ""},
	{"ach", -1, 1, ""},
	{"each", 7, 1, ""},
	{"\u00F3ideach", 8, 6, ""},
	{"gineach", 8, 3, ""},
	{"patach", 7, 5, ""},
	{"grafa\u00EDoch", -1, 4, ""},
	{"pataigh", -1, 5, ""},
	{"\u00F3idigh", -1, 6, ""},
	{"acht\u00FAil", -1, 1, ""},
	{"eacht\u00FAil", 15, 1, ""},
	{"gineas", -1, 3, ""},
	{"ginis", -1, 3, ""},
	{"acht", -1, 1, ""},
	{"arcacht", 19, 2, ""},
	{"eacht", 19, 1, ""},
	{"grafa\u00EDocht", -1, 4, ""},
	{"arcachta\u00ED", -1, 2, ""},
	{"grafa\u00EDochta\u00ED", -1, 4, ""},
}
var iri_a_3 = []among{
	{"imid", -1, 1, ""},
	{"aimid", 0, 1, ""},
	{"\u00EDmid", -1, 1, ""},
	{"a\u00EDmid", 2, 1, ""},
	{"adh", -1, 2, ""},
	{"eadh", 4, 2, ""},
	{"faidh", -1, 1, ""},
	{"fidh", -1, 1, ""},
	{"\u00E1il", -1, 2, ""},
	{"ain", -1, 2, ""},
	{"tear", -1, 2, ""},
	{"tar", -1, 2, ""},
}
var iri_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 17, 4, 2}

func (z *irishStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		if !z.goOutGrouping(iri_g_v, 97, 250) {
			goto lab0_end
		}
		z.cursor++
		z.I_pV = z.cursor
		if !z.goInGrouping(iri_g_v, 97, 250) {
			goto lab0_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(iri_g_v, 97, 250) {
			goto lab0_end
		}
		z.cursor++
		if !z.goInGrouping(iri_g_v, 97, 250) {
			goto lab0_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	return true
}
func (z *irishStemmer) r_initial_morph() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(iri_a_0)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("f")
		break
	case 3:
		z.sliceFrom("s")
		break
	case 4:
		z.sliceFrom("b")
		break
	case 5:
		z.sliceFrom("c")
		break
	case 6:
		z.sliceFrom("d")
		break
	case 7:
		z.sliceFrom("g")
		break
	case 8:
		z.sliceFrom("p")
		break
	case 9:
		z.sliceFrom("t")
		break
	case 10:
		z.sliceFrom("m")
		break
	}
	return true
}
func (z *irishStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *irishStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *irishStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *irishStemmer) r_noun_sfx() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(iri_a_1)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *irishStemmer) r_deriv() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(iri_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("arc")
		break
	case 3:
		z.sliceFrom("gin")
		break
	case 4:
		z.sliceFrom("graf")
		break
	case 5:
		z.sliceFrom("paite")
		break
	case 6:
		z.sliceFrom("\u00F3id")
		break
	}
	return true
}
func (z *irishStemmer) r_verb_sfx() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(iri_a_3)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *irishStemmer) stem() {
	v_1 := z.cursor
	z.r_initial_morph()
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_noun_sfx()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_deriv()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_verb_sfx()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	return
}

type italianStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newItalian() *italianStemmer { return &italianStemmer{} }

var ita_a_0 = []among{
	{"", -1, 7, ""},
	{"qu", 0, 6, ""},
	{"\u00E1", 0, 1, ""},
	{"\u00E9", 0, 2, ""},
	{"\u00ED", 0, 3, ""},
	{"\u00F3", 0, 4, ""},
	{"\u00FA", 0, 5, ""},
}
var ita_a_1 = []among{
	{"", -1, 3, ""},
	{"I", 0, 1, ""},
	{"U", 0, 2, ""},
}
var ita_a_2 = []among{
	{"la", -1, -1, ""},
	{"cela", 0, -1, ""},
	{"gliela", 0, -1, ""},
	{"mela", 0, -1, ""},
	{"tela", 0, -1, ""},
	{"vela", 0, -1, ""},
	{"le", -1, -1, ""},
	{"cele", 6, -1, ""},
	{"gliele", 6, -1, ""},
	{"mele", 6, -1, ""},
	{"tele", 6, -1, ""},
	{"vele", 6, -1, ""},
	{"ne", -1, -1, ""},
	{"cene", 12, -1, ""},
	{"gliene", 12, -1, ""},
	{"mene", 12, -1, ""},
	{"sene", 12, -1, ""},
	{"tene", 12, -1, ""},
	{"vene", 12, -1, ""},
	{"ci", -1, -1, ""},
	{"li", -1, -1, ""},
	{"celi", 20, -1, ""},
	{"glieli", 20, -1, ""},
	{"meli", 20, -1, ""},
	{"teli", 20, -1, ""},
	{"veli", 20, -1, ""},
	{"gli", 20, -1, ""},
	{"mi", -1, -1, ""},
	{"si", -1, -1, ""},
	{"ti", -1, -1, ""},
	{"vi", -1, -1, ""},
	{"lo", -1, -1, ""},
	{"celo", 31, -1, ""},
	{"glielo", 31, -1, ""},
	{"melo", 31, -1, ""},
	{"telo", 31, -1, ""},
	{"velo", 31, -1, ""},
}
var ita_a_3 = []among{
	{"ando", -1, 1, ""},
	{"endo", -1, 1, ""},
	{"ar", -1, 2, ""},
	{"er", -1, 2, ""},
	{"ir", -1, 2, ""},
}
var ita_a_4 = []among{
	{"ic", -1, -1, ""},
	{"abil", -1, -1, ""},
	{"os", -1, -1, ""},
	{"iv", -1, 1, ""},
}
var ita_a_5 = []among{
	{"ic", -1, 1, ""},
	{"abil", -1, 1, ""},
	{"iv", -1, 1, ""},
}
var ita_a_6 = []among{
	{"ica", -1, 1, ""},
	{"logia", -1, 3, ""},
	{"osa", -1, 1, ""},
	{"ista", -1, 1, ""},
	{"iva", -1, 9, ""},
	{"anza", -1, 1, ""},
	{"enza", -1, 5, ""},
	{"ice", -1, 1, ""},
	{"atrice", 7, 1, ""},
	{"iche", -1, 1, ""},
	{"logie", -1, 3, ""},
	{"abile", -1, 1, ""},
	{"ibile", -1, 1, ""},
	{"usione", -1, 4, ""},
	{"azione", -1, 2, ""},
	{"uzione", -1, 4, ""},
	{"atore", -1, 2, ""},
	{"ose", -1, 1, ""},
	{"ante", -1, 1, ""},
	{"mente", -1, 1, ""},
	{"amente", 19, 7, ""},
	{"iste", -1, 1, ""},
	{"ive", -1, 9, ""},
	{"anze", -1, 1, ""},
	{"enze", -1, 5, ""},
	{"ici", -1, 1, ""},
	{"atrici", 25, 1, ""},
	{"ichi", -1, 1, ""},
	{"abili", -1, 1, ""},
	{"ibili", -1, 1, ""},
	{"ismi", -1, 1, ""},
	{"usioni", -1, 4, ""},
	{"azioni", -1, 2, ""},
	{"uzioni", -1, 4, ""},
	{"atori", -1, 2, ""},
	{"osi", -1, 1, ""},
	{"anti", -1, 1, ""},
	{"amenti", -1, 6, ""},
	{"imenti", -1, 6, ""},
	{"isti", -1, 1, ""},
	{"ivi", -1, 9, ""},
	{"ico", -1, 1, ""},
	{"ismo", -1, 1, ""},
	{"oso", -1, 1, ""},
	{"amento", -1, 6, ""},
	{"imento", -1, 6, ""},
	{"ivo", -1, 9, ""},
	{"it\u00E0", -1, 8, ""},
	{"ist\u00E0", -1, 1, ""},
	{"ist\u00E8", -1, 1, ""},
	{"ist\u00EC", -1, 1, ""},
}
var ita_a_7 = []among{
	{"isca", -1, 1, ""},
	{"enda", -1, 1, ""},
	{"ata", -1, 1, ""},
	{"ita", -1, 1, ""},
	{"uta", -1, 1, ""},
	{"ava", -1, 1, ""},
	{"eva", -1, 1, ""},
	{"iva", -1, 1, ""},
	{"erebbe", -1, 1, ""},
	{"irebbe", -1, 1, ""},
	{"isce", -1, 1, ""},
	{"ende", -1, 1, ""},
	{"are", -1, 1, ""},
	{"ere", -1, 1, ""},
	{"ire", -1, 1, ""},
	{"asse", -1, 1, ""},
	{"ate", -1, 1, ""},
	{"avate", 16, 1, ""},
	{"evate", 16, 1, ""},
	{"ivate", 16, 1, ""},
	{"ete", -1, 1, ""},
	{"erete", 20, 1, ""},
	{"irete", 20, 1, ""},
	{"ite", -1, 1, ""},
	{"ereste", -1, 1, ""},
	{"ireste", -1, 1, ""},
	{"ute", -1, 1, ""},
	{"erai", -1, 1, ""},
	{"irai", -1, 1, ""},
	{"isci", -1, 1, ""},
	{"endi", -1, 1, ""},
	{"erei", -1, 1, ""},
	{"irei", -1, 1, ""},
	{"assi", -1, 1, ""},
	{"ati", -1, 1, ""},
	{"iti", -1, 1, ""},
	{"eresti", -1, 1, ""},
	{"iresti", -1, 1, ""},
	{"uti", -1, 1, ""},
	{"avi", -1, 1, ""},
	{"evi", -1, 1, ""},
	{"ivi", -1, 1, ""},
	{"isco", -1, 1, ""},
	{"ando", -1, 1, ""},
	{"endo", -1, 1, ""},
	{"Yamo", -1, 1, ""},
	{"iamo", -1, 1, ""},
	{"avamo", -1, 1, ""},
	{"evamo", -1, 1, ""},
	{"ivamo", -1, 1, ""},
	{"eremo", -1, 1, ""},
	{"iremo", -1, 1, ""},
	{"assimo", -1, 1, ""},
	{"ammo", -1, 1, ""},
	{"emmo", -1, 1, ""},
	{"eremmo", 54, 1, ""},
	{"iremmo", 54, 1, ""},
	{"immo", -1, 1, ""},
	{"ano", -1, 1, ""},
	{"iscano", 58, 1, ""},
	{"avano", 58, 1, ""},
	{"evano", 58, 1, ""},
	{"ivano", 58, 1, ""},
	{"eranno", -1, 1, ""},
	{"iranno", -1, 1, ""},
	{"ono", -1, 1, ""},
	{"iscono", 65, 1, ""},
	{"arono", 65, 1, ""},
	{"erono", 65, 1, ""},
	{"irono", 65, 1, ""},
	{"erebbero", -1, 1, ""},
	{"irebbero", -1, 1, ""},
	{"assero", -1, 1, ""},
	{"essero", -1, 1, ""},
	{"issero", -1, 1, ""},
	{"ato", -1, 1, ""},
	{"ito", -1, 1, ""},
	{"uto", -1, 1, ""},
	{"avo", -1, 1, ""},
	{"evo", -1, 1, ""},
	{"ivo", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"ir", -1, 1, ""},
	{"er\u00E0", -1, 1, ""},
	{"ir\u00E0", -1, 1, ""},
	{"er\u00F2", -1, 1, ""},
	{"ir\u00F2", -1, 1, ""},
}
var ita_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 128, 8, 2, 1}
var ita_g_AEIO = []byte{17, 65, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 128, 8, 2}
var ita_g_CG = []byte{17}

func (z *italianStemmer) r_prelude() bool {
	var among_var int
	v_1 := z.cursor
	for {
		v_2 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(ita_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("\u00E0")
				break
			case 2:
				z.sliceFrom("\u00E8")
				break
			case 3:
				z.sliceFrom("\u00EC")
				break
			case 4:
				z.sliceFrom("\u00F2")
				break
			case 5:
				z.sliceFrom("\u00F9")
				break
			case 6:
				z.sliceFrom("qU")
				break
			case 7:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_2
		break
	}
	z.cursor = v_1
	for {
		v_3 := z.cursor
		{
		golab2:
			for {
				v_4 := z.cursor
				{
					if !(z.inGrouping(ita_g_v, 97, 249)) {
						goto lab3_end
					}
					z.bra = z.cursor
					{
						v_5 := z.cursor
						{
							if !(z.eqS("u")) {
								goto lab5_end
							}
							z.ket = z.cursor
							if !(z.inGrouping(ita_g_v, 97, 249)) {
								goto lab5_end
							}
							z.sliceFrom("U")
							goto lab4_end
						}
					lab5_end:
						;
						z.cursor = v_5
						if !(z.eqS("i")) {
							goto lab3_end
						}
						z.ket = z.cursor
						if !(z.inGrouping(ita_g_v, 97, 249)) {
							goto lab3_end
						}
						z.sliceFrom("I")
					}
				lab4_end:
					;
					z.cursor = v_4
					break golab2
				}
			lab3_end:
				;
				z.cursor = v_4
				if z.cursor >= z.limit {
					goto lab1_end
				}
				z.cursor++
			}
			continue
		}
	lab1_end:
		;
		z.cursor = v_3
		break
	}
	return true
}
func (z *italianStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(ita_g_v, 97, 249)) {
					goto lab2_end
				}
				{
					v_3 := z.cursor
					{
						if !(z.outGrouping(ita_g_v, 97, 249)) {
							goto lab4_end
						}
						if !z.goOutGrouping(ita_g_v, 97, 249) {
							goto lab4_end
						}
						z.cursor++
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_3
					if !(z.inGrouping(ita_g_v, 97, 249)) {
						goto lab2_end
					}
					if !z.goInGrouping(ita_g_v, 97, 249) {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			{
				if !(z.eqS("divan")) {
					goto lab5_end
				}
				goto lab1_end
			}
		lab5_end:
			;
			z.cursor = v_2
			if !(z.outGrouping(ita_g_v, 97, 249)) {
				goto lab0_end
			}
			{
				v_4 := z.cursor
				{
					if !(z.outGrouping(ita_g_v, 97, 249)) {
						goto lab7_end
					}
					if !z.goOutGrouping(ita_g_v, 97, 249) {
						goto lab7_end
					}
					z.cursor++
					goto lab6_end
				}
			lab7_end:
				;
				z.cursor = v_4
				if !(z.inGrouping(ita_g_v, 97, 249)) {
					goto lab0_end
				}
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		lab6_end:
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_5 := z.cursor
	{
		if !z.goOutGrouping(ita_g_v, 97, 249) {
			goto lab8_end
		}
		z.cursor++
		if !z.goInGrouping(ita_g_v, 97, 249) {
			goto lab8_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(ita_g_v, 97, 249) {
			goto lab8_end
		}
		z.cursor++
		if !z.goInGrouping(ita_g_v, 97, 249) {
			goto lab8_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab8_end:
	;
	z.cursor = v_5
	return true
}
func (z *italianStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(ita_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("i")
				break
			case 2:
				z.sliceFrom("u")
				break
			case 3:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *italianStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *italianStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *italianStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *italianStemmer) r_attached_pronoun() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(ita_a_2) == 0 {
		return false
	}
	z.bra = z.cursor
	among_var = z.findAmongB(ita_a_3)
	if among_var == 0 {
		return false
	}
	if !z.r_RV() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("e")
		break
	}
	return true
}
func (z *italianStemmer) r_standard_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ita_a_6)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("ic")) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.sliceDel()
		}
	lab0_end:
		;
		break
	case 3:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("log")
		break
	case 4:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("u")
		break
	case 5:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("ente")
		break
	case 6:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 7:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(ita_a_4)
			if among_var == 0 {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.sliceDel()
			switch among_var {
			case 1:
				z.ket = z.cursor
				if !(z.eqSB("at")) {
					z.cursor = z.limit - v_2
					goto lab1_end
				}
				z.bra = z.cursor
				if !z.r_R2() {
					z.cursor = z.limit - v_2
					goto lab1_end
				}
				z.sliceDel()
				break
			}
		}
	lab1_end:
		;
		break
	case 8:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_3 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if z.findAmongB(ita_a_5) == 0 {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.sliceDel()
		}
	lab2_end:
		;
		break
	case 9:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_4 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("at")) {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.sliceDel()
			z.ket = z.cursor
			if !(z.eqSB("ic")) {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.sliceDel()
		}
	lab3_end:
		;
		break
	}
	return true
}
func (z *italianStemmer) r_verb_suffix() bool {
	if z.cursor < z.I_pV {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	if z.findAmongB(ita_a_7) == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.limitBackward = v_1
	return true
}
func (z *italianStemmer) r_vowel_suffix() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.inGroupingB(ita_g_AEIO, 97, 242)) {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.bra = z.cursor
		if !z.r_RV() {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.sliceDel()
		z.ket = z.cursor
		if !(z.eqSB("i")) {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.bra = z.cursor
		if !z.r_RV() {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.sliceDel()
	}
lab0_end:
	;
	v_2 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("h")) {
			z.cursor = z.limit - v_2
			goto lab1_end
		}
		z.bra = z.cursor
		if !(z.inGroupingB(ita_g_CG, 99, 103)) {
			z.cursor = z.limit - v_2
			goto lab1_end
		}
		if !z.r_RV() {
			z.cursor = z.limit - v_2
			goto lab1_end
		}
		z.sliceDel()
	}
lab1_end:
	;
	return true
}
func (z *italianStemmer) stem() {
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_attached_pronoun()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	{
		{
			v_4 := z.limit - z.cursor
			{
				if !z.r_standard_suffix() {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_4
			if !z.r_verb_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_3
	v_5 := z.limit - z.cursor
	z.r_vowel_suffix()
	z.cursor = z.limit - v_5
	z.cursor = z.limitBackward
	v_6 := z.cursor
	z.r_postlude()
	z.cursor = v_6
	return
}

type portugueseStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newPortuguese() *portugueseStemmer { return &portugueseStemmer{} }

var por_a_0 = []among{
	{"", -1, 3, ""},
	{"\u00E3", 0, 1, ""},
	{"\u00F5", 0, 2, ""},
}
var por_a_1 = []among{
	{"", -1, 3, ""},
	{"a~", 0, 1, ""},
	{"o~", 0, 2, ""},
}
var por_a_2 = []among{
	{"ic", -1, -1, ""},
	{"ad", -1, -1, ""},
	{"os", -1, -1, ""},
	{"iv", -1, 1, ""},
}
var por_a_3 = []among{
	{"ante", -1, 1, ""},
	{"avel", -1, 1, ""},
	{"\u00EDvel", -1, 1, ""},
}
var por_a_4 = []among{
	{"ic", -1, 1, ""},
	{"abil", -1, 1, ""},
	{"iv", -1, 1, ""},
}
var por_a_5 = []among{
	{"ica", -1, 1, ""},
	{"\u00E2ncia", -1, 1, ""},
	{"\u00EAncia", -1, 4, ""},
	{"logia", -1, 2, ""},
	{"ira", -1, 9, ""},
	{"adora", -1, 1, ""},
	{"osa", -1, 1, ""},
	{"ista", -1, 1, ""},
	{"iva", -1, 8, ""},
	{"eza", -1, 1, ""},
	{"idade", -1, 7, ""},
	{"ante", -1, 1, ""},
	{"mente", -1, 6, ""},
	{"amente", 12, 5, ""},
	{"\u00E1vel", -1, 1, ""},
	{"\u00EDvel", -1, 1, ""},
	{"ico", -1, 1, ""},
	{"ismo", -1, 1, ""},
	{"oso", -1, 1, ""},
	{"amento", -1, 1, ""},
	{"imento", -1, 1, ""},
	{"ivo", -1, 8, ""},
	{"a\u00E7a~o", -1, 1, ""},
	{"u\u00E7a~o", -1, 3, ""},
	{"ador", -1, 1, ""},
	{"icas", -1, 1, ""},
	{"\u00EAncias", -1, 4, ""},
	{"logias", -1, 2, ""},
	{"iras", -1, 9, ""},
	{"adoras", -1, 1, ""},
	{"osas", -1, 1, ""},
	{"istas", -1, 1, ""},
	{"ivas", -1, 8, ""},
	{"ezas", -1, 1, ""},
	{"idades", -1, 7, ""},
	{"adores", -1, 1, ""},
	{"antes", -1, 1, ""},
	{"a\u00E7o~es", -1, 1, ""},
	{"u\u00E7o~es", -1, 3, ""},
	{"icos", -1, 1, ""},
	{"ismos", -1, 1, ""},
	{"osos", -1, 1, ""},
	{"amentos", -1, 1, ""},
	{"imentos", -1, 1, ""},
	{"ivos", -1, 8, ""},
}
var por_a_6 = []among{
	{"ada", -1, 1, ""},
	{"ida", -1, 1, ""},
	{"ia", -1, 1, ""},
	{"aria", 2, 1, ""},
	{"eria", 2, 1, ""},
	{"iria", 2, 1, ""},
	{"ara", -1, 1, ""},
	{"era", -1, 1, ""},
	{"ira", -1, 1, ""},
	{"ava", -1, 1, ""},
	{"asse", -1, 1, ""},
	{"esse", -1, 1, ""},
	{"isse", -1, 1, ""},
	{"aste", -1, 1, ""},
	{"este", -1, 1, ""},
	{"iste", -1, 1, ""},
	{"ei", -1, 1, ""},
	{"arei", 16, 1, ""},
	{"erei", 16, 1, ""},
	{"irei", 16, 1, ""},
	{"am", -1, 1, ""},
	{"iam", 20, 1, ""},
	{"ariam", 21, 1, ""},
	{"eriam", 21, 1, ""},
	{"iriam", 21, 1, ""},
	{"aram", 20, 1, ""},
	{"eram", 20, 1, ""},
	{"iram", 20, 1, ""},
	{"avam", 20, 1, ""},
	{"em", -1, 1, ""},
	{"arem", 29, 1, ""},
	{"erem", 29, 1, ""},
	{"irem", 29, 1, ""},
	{"assem", 29, 1, ""},
	{"essem", 29, 1, ""},
	{"issem", 29, 1, ""},
	{"ado", -1, 1, ""},
	{"ido", -1, 1, ""},
	{"ando", -1, 1, ""},
	{"endo", -1, 1, ""},
	{"indo", -1, 1, ""},
	{"ara~o", -1, 1, ""},
	{"era~o", -1, 1, ""},
	{"ira~o", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"er", -1, 1, ""},
	{"ir", -1, 1, ""},
	{"as", -1, 1, ""},
	{"adas", 47, 1, ""},
	{"idas", 47, 1, ""},
	{"ias", 47, 1, ""},
	{"arias", 50, 1, ""},
	{"erias", 50, 1, ""},
	{"irias", 50, 1, ""},
	{"aras", 47, 1, ""},
	{"eras", 47, 1, ""},
	{"iras", 47, 1, ""},
	{"avas", 47, 1, ""},
	{"es", -1, 1, ""},
	{"ardes", 58, 1, ""},
	{"erdes", 58, 1, ""},
	{"irdes", 58, 1, ""},
	{"ares", 58, 1, ""},
	{"eres", 58, 1, ""},
	{"ires", 58, 1, ""},
	{"asses", 58, 1, ""},
	{"esses", 58, 1, ""},
	{"isses", 58, 1, ""},
	{"astes", 58, 1, ""},
	{"estes", 58, 1, ""},
	{"istes", 58, 1, ""},
	{"is", -1, 1, ""},
	{"ais", 71, 1, ""},
	{"eis", 71, 1, ""},
	{"areis", 73, 1, ""},
	{"ereis", 73, 1, ""},
	{"ireis", 73, 1, ""},
	{"\u00E1reis", 73, 1, ""},
	{"\u00E9reis", 73, 1, ""},
	{"\u00EDreis", 73, 1, ""},
	{"\u00E1sseis", 73, 1, ""},
	{"\u00E9sseis", 73, 1, ""},
	{"\u00EDsseis", 73, 1, ""},
	{"\u00E1veis", 73, 1, ""},
	{"\u00EDeis", 73, 1, ""},
	{"ar\u00EDeis", 84, 1, ""},
	{"er\u00EDeis", 84, 1, ""},
	{"ir\u00EDeis", 84, 1, ""},
	{"ados", -1, 1, ""},
	{"idos", -1, 1, ""},
	{"amos", -1, 1, ""},
	{"\u00E1ramos", 90, 1, ""},
	{"\u00E9ramos", 90, 1, ""},
	{"\u00EDramos", 90, 1, ""},
	{"\u00E1vamos", 90, 1, ""},
	{"\u00EDamos", 90, 1, ""},
	{"ar\u00EDamos", 95, 1, ""},
	{"er\u00EDamos", 95, 1, ""},
	{"ir\u00EDamos", 95, 1, ""},
	{"emos", -1, 1, ""},
	{"aremos", 99, 1, ""},
	{"eremos", 99, 1, ""},
	{"iremos", 99, 1, ""},
	{"\u00E1ssemos", 99, 1, ""},
	{"\u00EAssemos", 99, 1, ""},
	{"\u00EDssemos", 99, 1, ""},
	{"imos", -1, 1, ""},
	{"armos", -1, 1, ""},
	{"ermos", -1, 1, ""},
	{"irmos", -1, 1, ""},
	{"\u00E1mos", -1, 1, ""},
	{"ar\u00E1s", -1, 1, ""},
	{"er\u00E1s", -1, 1, ""},
	{"ir\u00E1s", -1, 1, ""},
	{"eu", -1, 1, ""},
	{"iu", -1, 1, ""},
	{"ou", -1, 1, ""},
	{"ar\u00E1", -1, 1, ""},
	{"er\u00E1", -1, 1, ""},
	{"ir\u00E1", -1, 1, ""},
}
var por_a_7 = []among{
	{"a", -1, 1, ""},
	{"i", -1, 1, ""},
	{"o", -1, 1, ""},
	{"os", -1, 1, ""},
	{"\u00E1", -1, 1, ""},
	{"\u00ED", -1, 1, ""},
	{"\u00F3", -1, 1, ""},
}
var por_a_8 = []among{
	{"e", -1, 1, ""},
	{"\u00E7", -1, 2, ""},
	{"\u00E9", -1, 1, ""},
	{"\u00EA", -1, 1, ""},
}
var por_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 19, 12, 2}

func (z *portugueseStemmer) r_prelude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(por_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("a~")
				break
			case 2:
				z.sliceFrom("o~")
				break
			case 3:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *portugueseStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(por_g_v, 97, 250)) {
					goto lab2_end
				}
				{
					v_3 := z.cursor
					{
						if !(z.outGrouping(por_g_v, 97, 250)) {
							goto lab4_end
						}
						if !z.goOutGrouping(por_g_v, 97, 250) {
							goto lab4_end
						}
						z.cursor++
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_3
					if !(z.inGrouping(por_g_v, 97, 250)) {
						goto lab2_end
					}
					if !z.goInGrouping(por_g_v, 97, 250) {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			if !(z.outGrouping(por_g_v, 97, 250)) {
				goto lab0_end
			}
			{
				v_4 := z.cursor
				{
					if !(z.outGrouping(por_g_v, 97, 250)) {
						goto lab6_end
					}
					if !z.goOutGrouping(por_g_v, 97, 250) {
						goto lab6_end
					}
					z.cursor++
					goto lab5_end
				}
			lab6_end:
				;
				z.cursor = v_4
				if !(z.inGrouping(por_g_v, 97, 250)) {
					goto lab0_end
				}
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		lab5_end:
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_5 := z.cursor
	{
		if !z.goOutGrouping(por_g_v, 97, 250) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(por_g_v, 97, 250) {
			goto lab7_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(por_g_v, 97, 250) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(por_g_v, 97, 250) {
			goto lab7_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab7_end:
	;
	z.cursor = v_5
	return true
}
func (z *portugueseStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(por_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("\u00E3")
				break
			case 2:
				z.sliceFrom("\u00F5")
				break
			case 3:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *portugueseStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *portugueseStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *portugueseStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *portugueseStemmer) r_standard_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(por_a_5)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("log")
		break
	case 3:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("u")
		break
	case 4:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("ente")
		break
	case 5:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(por_a_2)
			if among_var == 0 {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.sliceDel()
			switch among_var {
			case 1:
				z.ket = z.cursor
				if !(z.eqSB("at")) {
					z.cursor = z.limit - v_1
					goto lab0_end
				}
				z.bra = z.cursor
				if !z.r_R2() {
					z.cursor = z.limit - v_1
					goto lab0_end
				}
				z.sliceDel()
				break
			}
		}
	lab0_end:
		;
		break
	case 6:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if z.findAmongB(por_a_3) == 0 {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.sliceDel()
		}
	lab1_end:
		;
		break
	case 7:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_3 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if z.findAmongB(por_a_4) == 0 {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.sliceDel()
		}
	lab2_end:
		;
		break
	case 8:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_4 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("at")) {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.sliceDel()
		}
	lab3_end:
		;
		break
	case 9:
		if !z.r_RV() {
			return false
		}
		if !(z.eqSB("e")) {
			return false
		}
		z.sliceFrom("ir")
		break
	}
	return true
}
func (z *portugueseStemmer) r_verb_suffix() bool {
	if z.cursor < z.I_pV {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	if z.findAmongB(por_a_6) == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.limitBackward = v_1
	return true
}
func (z *portugueseStemmer) r_residual_suffix() bool {
	z.ket = z.cursor
	if z.findAmongB(por_a_7) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_RV() {
		return false
	}
	z.sliceDel()
	return true
}
func (z *portugueseStemmer) r_residual_form() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(por_a_8)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		z.ket = z.cursor
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("u")) {
					goto lab1_end
				}
				z.bra = z.cursor
				v_2 := z.limit - z.cursor
				if !(z.eqSB("g")) {
					goto lab1_end
				}
				z.cursor = z.limit - v_2
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			if !(z.eqSB("i")) {
				return false
			}
			z.bra = z.cursor
			v_3 := z.limit - z.cursor
			if !(z.eqSB("c")) {
				return false
			}
			z.cursor = z.limit - v_3
		}
	lab0_end:
		;
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("c")
		break
	}
	return true
}
func (z *portugueseStemmer) stem() {
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	{
		{
			v_3 := z.limit - z.cursor
			{
				v_4 := z.limit - z.cursor
				{
					v_5 := z.limit - z.cursor
					{
						if !z.r_standard_suffix() {
							goto lab4_end
						}
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = z.limit - v_5
					if !z.r_verb_suffix() {
						goto lab2_end
					}
				}
			lab3_end:
				;
				z.cursor = z.limit - v_4
				v_6 := z.limit - z.cursor
				{
					z.ket = z.cursor
					if !(z.eqSB("i")) {
						goto lab5_end
					}
					z.bra = z.cursor
					v_7 := z.limit - z.cursor
					if !(z.eqSB("c")) {
						goto lab5_end
					}
					z.cursor = z.limit - v_7
					if !z.r_RV() {
						goto lab5_end
					}
					z.sliceDel()
				}
			lab5_end:
				;
				z.cursor = z.limit - v_6
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			if !z.r_residual_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	v_8 := z.limit - z.cursor
	z.r_residual_form()
	z.cursor = z.limit - v_8
	z.cursor = z.limitBackward
	v_9 := z.cursor
	z.r_postlude()
	z.cursor = v_9
	return
}

type romanianStemmer struct {
	snowball
	I_p1, I_p2                int
	S_ch, S_x                 []rune
	LS_ch, LS_x               int
	B_standard_suffix_removed bool
	I_pV                      int
}

func newRomanian() *romanianStemmer { return &romanianStemmer{} }

var rom_a_0 = []among{
	{"\u015F", -1, 1, ""},
	{"\u0163", -1, 2, ""},
}
var rom_a_1 = []among{
	{"", -1, 3, ""},
	{"I", 0, 1, ""},
	{"U", 0, 2, ""},
}
var rom_a_2 = []among{
	{"ea", -1, 3, ""},
	{"a\u021Bia", -1, 7, ""},
	{"aua", -1, 2, ""},
	{"iua", -1, 4, ""},
	{"a\u021Bie", -1, 7, ""},
	{"ele", -1, 3, ""},
	{"ile", -1, 5, ""},
	{"iile", 6, 4, ""},
	{"iei", -1, 4, ""},
	{"atei", -1, 6, ""},
	{"ii", -1, 4, ""},
	{"ului", -1, 1, ""},
	{"ul", -1, 1, ""},
	{"elor", -1, 3, ""},
	{"ilor", -1, 4, ""},
	{"iilor", 14, 4, ""},
}
var rom_a_3 = []among{
	{"icala", -1, 4, ""},
	{"iciva", -1, 4, ""},
	{"ativa", -1, 5, ""},
	{"itiva", -1, 6, ""},
	{"icale", -1, 4, ""},
	{"a\u021Biune", -1, 5, ""},
	{"i\u021Biune", -1, 6, ""},
	{"atoare", -1, 5, ""},
	{"itoare", -1, 6, ""},
	{"\u0103toare", -1, 5, ""},
	{"icitate", -1, 4, ""},
	{"abilitate", -1, 1, ""},
	{"ibilitate", -1, 2, ""},
	{"ivitate", -1, 3, ""},
	{"icive", -1, 4, ""},
	{"ative", -1, 5, ""},
	{"itive", -1, 6, ""},
	{"icali", -1, 4, ""},
	{"atori", -1, 5, ""},
	{"icatori", 18, 4, ""},
	{"itori", -1, 6, ""},
	{"\u0103tori", -1, 5, ""},
	{"icitati", -1, 4, ""},
	{"abilitati", -1, 1, ""},
	{"ivitati", -1, 3, ""},
	{"icivi", -1, 4, ""},
	{"ativi", -1, 5, ""},
	{"itivi", -1, 6, ""},
	{"icit\u0103i", -1, 4, ""},
	{"abilit\u0103i", -1, 1, ""},
	{"ivit\u0103i", -1, 3, ""},
	{"icit\u0103\u021Bi", -1, 4, ""},
	{"abilit\u0103\u021Bi", -1, 1, ""},
	{"ivit\u0103\u021Bi", -1, 3, ""},
	{"ical", -1, 4, ""},
	{"ator", -1, 5, ""},
	{"icator", 35, 4, ""},
	{"itor", -1, 6, ""},
	{"\u0103tor", -1, 5, ""},
	{"iciv", -1, 4, ""},
	{"ativ", -1, 5, ""},
	{"itiv", -1, 6, ""},
	{"ical\u0103", -1, 4, ""},
	{"iciv\u0103", -1, 4, ""},
	{"ativ\u0103", -1, 5, ""},
	{"itiv\u0103", -1, 6, ""},
}
var rom_a_4 = []among{
	{"ica", -1, 1, ""},
	{"abila", -1, 1, ""},
	{"ibila", -1, 1, ""},
	{"oasa", -1, 1, ""},
	{"ata", -1, 1, ""},
	{"ita", -1, 1, ""},
	{"anta", -1, 1, ""},
	{"ista", -1, 3, ""},
	{"uta", -1, 1, ""},
	{"iva", -1, 1, ""},
	{"ic", -1, 1, ""},
	{"ice", -1, 1, ""},
	{"abile", -1, 1, ""},
	{"ibile", -1, 1, ""},
	{"isme", -1, 3, ""},
	{"iune", -1, 2, ""},
	{"oase", -1, 1, ""},
	{"ate", -1, 1, ""},
	{"itate", 17, 1, ""},
	{"ite", -1, 1, ""},
	{"ante", -1, 1, ""},
	{"iste", -1, 3, ""},
	{"ute", -1, 1, ""},
	{"ive", -1, 1, ""},
	{"ici", -1, 1, ""},
	{"abili", -1, 1, ""},
	{"ibili", -1, 1, ""},
	{"iuni", -1, 2, ""},
	{"atori", -1, 1, ""},
	{"osi", -1, 1, ""},
	{"ati", -1, 1, ""},
	{"itati", 30, 1, ""},
	{"iti", -1, 1, ""},
	{"anti", -1, 1, ""},
	{"isti", -1, 3, ""},
	{"uti", -1, 1, ""},
	{"i\u0219ti", -1, 3, ""},
	{"ivi", -1, 1, ""},
	{"it\u0103i", -1, 1, ""},
	{"o\u0219i", -1, 1, ""},
	{"it\u0103\u021Bi", -1, 1, ""},
	{"abil", -1, 1, ""},
	{"ibil", -1, 1, ""},
	{"ism", -1, 3, ""},
	{"ator", -1, 1, ""},
	{"os", -1, 1, ""},
	{"at", -1, 1, ""},
	{"it", -1, 1, ""},
	{"ant", -1, 1, ""},
	{"ist", -1, 3, ""},
	{"ut", -1, 1, ""},
	{"iv", -1, 1, ""},
	{"ic\u0103", -1, 1, ""},
	{"abil\u0103", -1, 1, ""},
	{"ibil\u0103", -1, 1, ""},
	{"oas\u0103", -1, 1, ""},
	{"at\u0103", -1, 1, ""},
	{"it\u0103", -1, 1, ""},
	{"ant\u0103", -1, 1, ""},
	{"ist\u0103", -1, 3, ""},
	{"ut\u0103", -1, 1, ""},
	{"iv\u0103", -1, 1, ""},
}
var rom_a_5 = []among{
	{"ea", -1, 1, ""},
	{"ia", -1, 1, ""},
	{"esc", -1, 1, ""},
	{"\u0103sc", -1, 1, ""},
	{"ind", -1, 1, ""},
	{"\u00E2nd", -1, 1, ""},
	{"are", -1, 1, ""},
	{"ere", -1, 1, ""},
	{"ire", -1, 1, ""},
	{"\u00E2re", -1, 1, ""},
	{"se", -1, 2, ""},
	{"ase", 10, 1, ""},
	{"sese", 10, 2, ""},
	{"ise", 10, 1, ""},
	{"use", 10, 1, ""},
	{"\u00E2se", 10, 1, ""},
	{"e\u0219te", -1, 1, ""},
	{"\u0103\u0219te", -1, 1, ""},
	{"eze", -1, 1, ""},
	{"ai", -1, 1, ""},
	{"eai", 19, 1, ""},
	{"iai", 19, 1, ""},
	{"sei", -1, 2, ""},
	{"e\u0219ti", -1, 1, ""},
	{"\u0103\u0219ti", -1, 1, ""},
	{"ui", -1, 1, ""},
	{"ezi", -1, 1, ""},
	{"\u00E2i", -1, 1, ""},
	{"a\u0219i", -1, 1, ""},
	{"se\u0219i", -1, 2, ""},
	{"ase\u0219i", 29, 1, ""},
	{"sese\u0219i", 29, 2, ""},
	{"ise\u0219i", 29, 1, ""},
	{"use\u0219i", 29, 1, ""},
	{"\u00E2se\u0219i", 29, 1, ""},
	{"i\u0219i", -1, 1, ""},
	{"u\u0219i", -1, 1, ""},
	{"\u00E2\u0219i", -1, 1, ""},
	{"a\u021Bi", -1, 2, ""},
	{"ea\u021Bi", 38, 1, ""},
	{"ia\u021Bi", 38, 1, ""},
	{"e\u021Bi", -1, 2, ""},
	{"i\u021Bi", -1, 2, ""},
	{"\u00E2\u021Bi", -1, 2, ""},
	{"ar\u0103\u021Bi", -1, 1, ""},
	{"ser\u0103\u021Bi", -1, 2, ""},
	{"aser\u0103\u021Bi", 45, 1, ""},
	{"seser\u0103\u021Bi", 45, 2, ""},
	{"iser\u0103\u021Bi", 45, 1, ""},
	{"user\u0103\u021Bi", 45, 1, ""},
	{"\u00E2ser\u0103\u021Bi", 45, 1, ""},
	{"ir\u0103\u021Bi", -1, 1, ""},
	{"ur\u0103\u021Bi", -1, 1, ""},
	{"\u00E2r\u0103\u021Bi", -1, 1, ""},
	{"am", -1, 1, ""},
	{"eam", 54, 1, ""},
	{"iam", 54, 1, ""},
	{"em", -1, 2, ""},
	{"asem", 57, 1, ""},
	{"sesem", 57, 2, ""},
	{"isem", 57, 1, ""},
	{"usem", 57, 1, ""},
	{"\u00E2sem", 57, 1, ""},
	{"im", -1, 2, ""},
	{"\u00E2m", -1, 2, ""},
	{"\u0103m", -1, 2, ""},
	{"ar\u0103m", 65, 1, ""},
	{"ser\u0103m", 65, 2, ""},
	{"aser\u0103m", 67, 1, ""},
	{"seser\u0103m", 67, 2, ""},
	{"iser\u0103m", 67, 1, ""},
	{"user\u0103m", 67, 1, ""},
	{"\u00E2ser\u0103m", 67, 1, ""},
	{"ir\u0103m", 65, 1, ""},
	{"ur\u0103m", 65, 1, ""},
	{"\u00E2r\u0103m", 65, 1, ""},
	{"au", -1, 1, ""},
	{"eau", 76, 1, ""},
	{"iau", 76, 1, ""},
	{"indu", -1, 1, ""},
	{"\u00E2ndu", -1, 1, ""},
	{"ez", -1, 1, ""},
	{"easc\u0103", -1, 1, ""},
	{"ar\u0103", -1, 1, ""},
	{"ser\u0103", -1, 2, ""},
	{"aser\u0103", 84, 1, ""},
	{"seser\u0103", 84, 2, ""},
	{"iser\u0103", 84, 1, ""},
	{"user\u0103", 84, 1, ""},
	{"\u00E2ser\u0103", 84, 1, ""},
	{"ir\u0103", -1, 1, ""},
	{"ur\u0103", -1, 1, ""},
	{"\u00E2r\u0103", -1, 1, ""},
	{"eaz\u0103", -1, 1, ""},
}
var rom_a_6 = []among{
	{"a", -1, 1, ""},
	{"e", -1, 1, ""},
	{"ie", 1, 1, ""},
	{"i", -1, 1, ""},
	{"\u0103", -1, 1, ""},
}
var rom_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 2, 32, 0, 0, 4}

func (z *romanianStemmer) r_norm() bool {
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
						among_var = z.findAmong(rom_a_0)
						if among_var == 0 {
							goto lab3_end
						}
						z.ket = z.cursor
						switch among_var {
						case 1:
							z.sliceFrom("\u0219")
							break
						case 2:
							z.sliceFrom("\u021B")
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
	return true
}
func (z *romanianStemmer) r_prelude() bool {
	for {
		v_1 := z.cursor
		{
		golab1:
			for {
				v_2 := z.cursor
				{
					if !(z.inGrouping(rom_g_v, 97, 259)) {
						goto lab2_end
					}
					z.bra = z.cursor
					{
						v_3 := z.cursor
						{
							if !(z.eqS("u")) {
								goto lab4_end
							}
							z.ket = z.cursor
							if !(z.inGrouping(rom_g_v, 97, 259)) {
								goto lab4_end
							}
							z.sliceFrom("U")
							goto lab3_end
						}
					lab4_end:
						;
						z.cursor = v_3
						if !(z.eqS("i")) {
							goto lab2_end
						}
						z.ket = z.cursor
						if !(z.inGrouping(rom_g_v, 97, 259)) {
							goto lab2_end
						}
						z.sliceFrom("I")
					}
				lab3_end:
					;
					z.cursor = v_2
					break golab1
				}
			lab2_end:
				;
				z.cursor = v_2
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *romanianStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(rom_g_v, 97, 259)) {
					goto lab2_end
				}
				{
					v_3 := z.cursor
					{
						if !(z.outGrouping(rom_g_v, 97, 259)) {
							goto lab4_end
						}
						if !z.goOutGrouping(rom_g_v, 97, 259) {
							goto lab4_end
						}
						z.cursor++
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_3
					if !(z.inGrouping(rom_g_v, 97, 259)) {
						goto lab2_end
					}
					if !z.goInGrouping(rom_g_v, 97, 259) {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			if !(z.outGrouping(rom_g_v, 97, 259)) {
				goto lab0_end
			}
			{
				v_4 := z.cursor
				{
					if !(z.outGrouping(rom_g_v, 97, 259)) {
						goto lab6_end
					}
					if !z.goOutGrouping(rom_g_v, 97, 259) {
						goto lab6_end
					}
					z.cursor++
					goto lab5_end
				}
			lab6_end:
				;
				z.cursor = v_4
				if !(z.inGrouping(rom_g_v, 97, 259)) {
					goto lab0_end
				}
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		lab5_end:
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_5 := z.cursor
	{
		if !z.goOutGrouping(rom_g_v, 97, 259) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(rom_g_v, 97, 259) {
			goto lab7_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(rom_g_v, 97, 259) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(rom_g_v, 97, 259) {
			goto lab7_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab7_end:
	;
	z.cursor = v_5
	return true
}
func (z *romanianStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(rom_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("i")
				break
			case 2:
				z.sliceFrom("u")
				break
			case 3:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *romanianStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *romanianStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *romanianStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *romanianStemmer) r_step_0() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(rom_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("a")
		break
	case 3:
		z.sliceFrom("e")
		break
	case 4:
		z.sliceFrom("i")
		break
	case 5:
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("ab")) {
					goto lab0_end
				}
				return false
			}
		lab0_end:
			;
			z.cursor = z.limit - v_1
		}
		z.sliceFrom("i")
		break
	case 6:
		z.sliceFrom("at")
		break
	case 7:
		z.sliceFrom("a\u021Bi")
		break
	}
	return true
}
func (z *romanianStemmer) r_combo_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	z.ket = z.cursor
	among_var = z.findAmongB(rom_a_3)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("abil")
		break
	case 2:
		z.sliceFrom("ibil")
		break
	case 3:
		z.sliceFrom("iv")
		break
	case 4:
		z.sliceFrom("ic")
		break
	case 5:
		z.sliceFrom("at")
		break
	case 6:
		z.sliceFrom("it")
		break
	}
	z.B_standard_suffix_removed = true
	z.cursor = z.limit - v_1
	return true
}
func (z *romanianStemmer) r_standard_suffix() bool {
	var among_var int
	z.B_standard_suffix_removed = false
	for {
		v_1 := z.limit - z.cursor
		{
			if !z.r_combo_suffix() {
				goto lab0_end
			}
			continue
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
		break
	}
	z.ket = z.cursor
	among_var = z.findAmongB(rom_a_4)
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
		if !(z.eqSB("\u021B")) {
			return false
		}
		z.bra = z.cursor
		z.sliceFrom("t")
		break
	case 3:
		z.sliceFrom("ist")
		break
	}
	z.B_standard_suffix_removed = true
	return true
}
func (z *romanianStemmer) r_verb_suffix() bool {
	var among_var int
	if z.cursor < z.I_pV {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	among_var = z.findAmongB(rom_a_5)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_2 := z.limit - z.cursor
			{
				if !(z.outGroupingB(rom_g_v, 97, 259)) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_2
			if !(z.eqSB("u")) {
				z.limitBackward = v_1
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	}
	z.limitBackward = v_1
	return true
}
func (z *romanianStemmer) r_vowel_suffix() bool {
	z.ket = z.cursor
	if z.findAmongB(rom_a_6) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_RV() {
		return false
	}
	z.sliceDel()
	return true
}
func (z *romanianStemmer) stem() {
	z.r_norm()
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_step_0()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_standard_suffix()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	{
		{
			v_5 := z.limit - z.cursor
			{
				if !z.B_standard_suffix_removed {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_5
			if !z.r_verb_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_4
	v_6 := z.limit - z.cursor
	z.r_vowel_suffix()
	z.cursor = z.limit - v_6
	z.cursor = z.limitBackward
	v_7 := z.cursor
	z.r_postlude()
	z.cursor = v_7
	return
}

type russianStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newRussian() *russianStemmer { return &russianStemmer{} }

var rus_a_0 = []among{
	{"\u0432", -1, 1, ""},
	{"\u0438\u0432", 0, 2, ""},
	{"\u044B\u0432", 0, 2, ""},
	{"\u0432\u0448\u0438", -1, 1, ""},
	{"\u0438\u0432\u0448\u0438", 3, 2, ""},
	{"\u044B\u0432\u0448\u0438", 3, 2, ""},
	{"\u0432\u0448\u0438\u0441\u044C", -1, 1, ""},
	{"\u0438\u0432\u0448\u0438\u0441\u044C", 6, 2, ""},
	{"\u044B\u0432\u0448\u0438\u0441\u044C", 6, 2, ""},
}
var rus_a_1 = []among{
	{"\u0435\u0435", -1, 1, ""},
	{"\u0438\u0435", -1, 1, ""},
	{"\u043E\u0435", -1, 1, ""},
	{"\u044B\u0435", -1, 1, ""},
	{"\u0438\u043C\u0438", -1, 1, ""},
	{"\u044B\u043C\u0438", -1, 1, ""},
	{"\u0435\u0439", -1, 1, ""},
	{"\u0438\u0439", -1, 1, ""},
	{"\u043E\u0439", -1, 1, ""},
	{"\u044B\u0439", -1, 1, ""},
	{"\u0435\u043C", -1, 1, ""},
	{"\u0438\u043C", -1, 1, ""},
	{"\u043E\u043C", -1, 1, ""},
	{"\u044B\u043C", -1, 1, ""},
	{"\u0435\u0433\u043E", -1, 1, ""},
	{"\u043E\u0433\u043E", -1, 1, ""},
	{"\u0435\u043C\u0443", -1, 1, ""},
	{"\u043E\u043C\u0443", -1, 1, ""},
	{"\u0438\u0445", -1, 1, ""},
	{"\u044B\u0445", -1, 1, ""},
	{"\u0435\u044E", -1, 1, ""},
	{"\u043E\u044E", -1, 1, ""},
	{"\u0443\u044E", -1, 1, ""},
	{"\u044E\u044E", -1, 1, ""},
	{"\u0430\u044F", -1, 1, ""},
	{"\u044F\u044F", -1, 1, ""},
}
var rus_a_2 = []among{
	{"\u0435\u043C", -1, 1, ""},
	{"\u043D\u043D", -1, 1, ""},
	{"\u0432\u0448", -1, 1, ""},
	{"\u0438\u0432\u0448", 2, 2, ""},
	{"\u044B\u0432\u0448", 2, 2, ""},
	{"\u0449", -1, 1, ""},
	{"\u044E\u0449", 5, 1, ""},
	{"\u0443\u044E\u0449", 6, 2, ""},
}
var rus_a_3 = []among{
	{"\u0441\u044C", -1, 1, ""},
	{"\u0441\u044F", -1, 1, ""},
}
var rus_a_4 = []among{
	{"\u043B\u0430", -1, 1, ""},
	{"\u0438\u043B\u0430", 0, 2, ""},
	{"\u044B\u043B\u0430", 0, 2, ""},
	{"\u043D\u0430", -1, 1, ""},
	{"\u0435\u043D\u0430", 3, 2, ""},
	{"\u0435\u0442\u0435", -1, 1, ""},
	{"\u0438\u0442\u0435", -1, 2, ""},
	{"\u0439\u0442\u0435", -1, 1, ""},
	{"\u0435\u0439\u0442\u0435", 7, 2, ""},
	{"\u0443\u0439\u0442\u0435", 7, 2, ""},
	{"\u043B\u0438", -1, 1, ""},
	{"\u0438\u043B\u0438", 10, 2, ""},
	{"\u044B\u043B\u0438", 10, 2, ""},
	{"\u0439", -1, 1, ""},
	{"\u0435\u0439", 13, 2, ""},
	{"\u0443\u0439", 13, 2, ""},
	{"\u043B", -1, 1, ""},
	{"\u0438\u043B", 16, 2, ""},
	{"\u044B\u043B", 16, 2, ""},
	{"\u0435\u043C", -1, 1, ""},
	{"\u0438\u043C", -1, 2, ""},
	{"\u044B\u043C", -1, 2, ""},
	{"\u043D", -1, 1, ""},
	{"\u0435\u043D", 22, 2, ""},
	{"\u043B\u043E", -1, 1, ""},
	{"\u0438\u043B\u043E", 24, 2, ""},
	{"\u044B\u043B\u043E", 24, 2, ""},
	{"\u043D\u043E", -1, 1, ""},
	{"\u0435\u043D\u043E", 27, 2, ""},
	{"\u043D\u043D\u043E", 27, 1, ""},
	{"\u0435\u0442", -1, 1, ""},
	{"\u0443\u0435\u0442", 30, 2, ""},
	{"\u0438\u0442", -1, 2, ""},
	{"\u044B\u0442", -1, 2, ""},
	{"\u044E\u0442", -1, 1, ""},
	{"\u0443\u044E\u0442", 34, 2, ""},
	{"\u044F\u0442", -1, 2, ""},
	{"\u043D\u044B", -1, 1, ""},
	{"\u0435\u043D\u044B", 37, 2, ""},
	{"\u0442\u044C", -1, 1, ""},
	{"\u0438\u0442\u044C", 39, 2, ""},
	{"\u044B\u0442\u044C", 39, 2, ""},
	{"\u0435\u0448\u044C", -1, 1, ""},
	{"\u0438\u0448\u044C", -1, 2, ""},
	{"\u044E", -1, 2, ""},
	{"\u0443\u044E", 44, 2, ""},
}
var rus_a_5 = []among{
	{"\u0430", -1, 1, ""},
	{"\u0435\u0432", -1, 1, ""},
	{"\u043E\u0432", -1, 1, ""},
	{"\u0435", -1, 1, ""},
	{"\u0438\u0435", 3, 1, ""},
	{"\u044C\u0435", 3, 1, ""},
	{"\u0438", -1, 1, ""},
	{"\u0435\u0438", 6, 1, ""},
	{"\u0438\u0438", 6, 1, ""},
	{"\u0430\u043C\u0438", 6, 1, ""},
	{"\u044F\u043C\u0438", 6, 1, ""},
	{"\u0438\u044F\u043C\u0438", 10, 1, ""},
	{"\u0439", -1, 1, ""},
	{"\u0435\u0439", 12, 1, ""},
	{"\u0438\u0435\u0439", 13, 1, ""},
	{"\u0438\u0439", 12, 1, ""},
	{"\u043E\u0439", 12, 1, ""},
	{"\u0430\u043C", -1, 1, ""},
	{"\u0435\u043C", -1, 1, ""},
	{"\u0438\u0435\u043C", 18, 1, ""},
	{"\u043E\u043C", -1, 1, ""},
	{"\u044F\u043C", -1, 1, ""},
	{"\u0438\u044F\u043C", 21, 1, ""},
	{"\u043E", -1, 1, ""},
	{"\u0443", -1, 1, ""},
	{"\u0430\u0445", -1, 1, ""},
	{"\u044F\u0445", -1, 1, ""},
	{"\u0438\u044F\u0445", 26, 1, ""},
	{"\u044B", -1, 1, ""},
	{"\u044C", -1, 1, ""},
	{"\u044E", -1, 1, ""},
	{"\u0438\u044E", 30, 1, ""},
	{"\u044C\u044E", 30, 1, ""},
	{"\u044F", -1, 1, ""},
	{"\u0438\u044F", 33, 1, ""},
	{"\u044C\u044F", 33, 1, ""},
}
var rus_a_6 = []among{
	{"\u043E\u0441\u0442", -1, 1, ""},
	{"\u043E\u0441\u0442\u044C", -1, 1, ""},
}
var rus_a_7 = []among{
	{"\u0435\u0439\u0448\u0435", -1, 1, ""},
	{"\u043D", -1, 2, ""},
	{"\u0435\u0439\u0448", -1, 1, ""},
	{"\u044C", -1, 3, ""},
}
var rus_g_v = []byte{33, 65, 8, 232}

func (z *russianStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		if !z.goOutGrouping(rus_g_v, 1072, 1103) {
			goto lab0_end
		}
		z.cursor++
		z.I_pV = z.cursor
		if !z.goInGrouping(rus_g_v, 1072, 1103) {
			goto lab0_end
		}
		z.cursor++
		if !z.goOutGrouping(rus_g_v, 1072, 1103) {
			goto lab0_end
		}
		z.cursor++
		if !z.goInGrouping(rus_g_v, 1072, 1103) {
			goto lab0_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	return true
}
func (z *russianStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *russianStemmer) r_perfective_gerund() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(rus_a_0)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("\u0430")) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			if !(z.eqSB("\u044F")) {
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	}
	return true
}
func (z *russianStemmer) r_adjective() bool {
	z.ket = z.cursor
	if z.findAmongB(rus_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *russianStemmer) r_adjectival() bool {
	var among_var int
	if !z.r_adjective() {
		return false
	}
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(rus_a_2)
		if among_var == 0 {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			{
				v_2 := z.limit - z.cursor
				{
					if !(z.eqSB("\u0430")) {
						goto lab2_end
					}
					goto lab1_end
				}
			lab2_end:
				;
				z.cursor = z.limit - v_2
				if !(z.eqSB("\u044F")) {
					z.cursor = z.limit - v_1
					goto lab0_end
				}
			}
		lab1_end:
			;
			z.sliceDel()
			break
		case 2:
			z.sliceDel()
			break
		}
	}
lab0_end:
	;
	return true
}
func (z *russianStemmer) r_reflexive() bool {
	z.ket = z.cursor
	if z.findAmongB(rus_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *russianStemmer) r_verb() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(rus_a_4)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		{
			v_1 := z.limit - z.cursor
			{
				if !(z.eqSB("\u0430")) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_1
			if !(z.eqSB("\u044F")) {
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	}
	return true
}
func (z *russianStemmer) r_noun() bool {
	z.ket = z.cursor
	if z.findAmongB(rus_a_5) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *russianStemmer) r_derivational() bool {
	z.ket = z.cursor
	if z.findAmongB(rus_a_6) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R2() {
		return false
	}
	z.sliceDel()
	return true
}
func (z *russianStemmer) r_tidy_up() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(rus_a_7)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceDel()
		z.ket = z.cursor
		if !(z.eqSB("\u043D")) {
			return false
		}
		z.bra = z.cursor
		if !(z.eqSB("\u043D")) {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !(z.eqSB("\u043D")) {
			return false
		}
		z.sliceDel()
		break
	case 3:
		z.sliceDel()
		break
	}
	return true
}
func (z *russianStemmer) stem() {
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
						if !(z.eqS("\u0451")) {
							goto lab3_end
						}
						z.ket = z.cursor
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
				z.sliceFrom("\u0435")
				continue
			}
		lab1_end:
			;
			z.cursor = v_2
			break
		}
	}
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	if z.cursor < z.I_pV {
		return
	}
	v_4 := z.limitBackward
	z.limitBackward = z.I_pV
	v_5 := z.limit - z.cursor
	{
		{
			v_6 := z.limit - z.cursor
			{
				if !z.r_perfective_gerund() {
					goto lab6_end
				}
				goto lab5_end
			}
		lab6_end:
			;
			z.cursor = z.limit - v_6
			v_7 := z.limit - z.cursor
			{
				if !z.r_reflexive() {
					z.cursor = z.limit - v_7
					goto lab7_end
				}
			}
		lab7_end:
			;
			{
				v_8 := z.limit - z.cursor
				{
					if !z.r_adjectival() {
						goto lab9_end
					}
					goto lab8_end
				}
			lab9_end:
				;
				z.cursor = z.limit - v_8
				{
					if !z.r_verb() {
						goto lab10_end
					}
					goto lab8_end
				}
			lab10_end:
				;
				z.cursor = z.limit - v_8
				if !z.r_noun() {
					goto lab4_end
				}
			}
		lab8_end:
		}
	lab5_end:
	}
lab4_end:
	;
	z.cursor = z.limit - v_5
	v_9 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("\u0438")) {
			z.cursor = z.limit - v_9
			goto lab11_end
		}
		z.bra = z.cursor
		z.sliceDel()
	}
lab11_end:
	;
	v_10 := z.limit - z.cursor
	z.r_derivational()
	z.cursor = z.limit - v_10
	v_11 := z.limit - z.cursor
	z.r_tidy_up()
	z.cursor = z.limit - v_11
	z.limitBackward = v_4
	z.cursor = z.limitBackward
	return
}

type serbianStemmer struct {
	snowball
	I_p1, I_p2      int
	S_ch, S_x       []rune
	LS_ch, LS_x     int
	B_no_diacritics bool
}

func newSerbian() *serbianStemmer { return &serbianStemmer{} }

var ser_a_0 = []among{
	{"\u0430", -1, 1, ""},
	{"\u0431", -1, 2, ""},
	{"\u0432", -1, 3, ""},
	{"\u0433", -1, 4, ""},
	{"\u0434", -1, 5, ""},
	{"\u0435", -1, 7, ""},
	{"\u0436", -1, 8, ""},
	{"\u0437", -1, 9, ""},
	{"\u0438", -1, 10, ""},
	{"\u043A", -1, 12, ""},
	{"\u043B", -1, 13, ""},
	{"\u043C", -1, 15, ""},
	{"\u043D", -1, 16, ""},
	{"\u043E", -1, 18, ""},
	{"\u043F", -1, 19, ""},
	{"\u0440", -1, 20, ""},
	{"\u0441", -1, 21, ""},
	{"\u0442", -1, 22, ""},
	{"\u0443", -1, 24, ""},
	{"\u0444", -1, 25, ""},
	{"\u0445", -1, 26, ""},
	{"\u0446", -1, 27, ""},
	{"\u0447", -1, 28, ""},
	{"\u0448", -1, 30, ""},
	{"\u0452", -1, 6, ""},
	{"\u0458", -1, 11, ""},
	{"\u0459", -1, 14, ""},
	{"\u045A", -1, 17, ""},
	{"\u045B", -1, 23, ""},
	{"\u045F", -1, 29, ""},
}
var ser_a_1 = []among{
	{"daba", -1, 73, ""},
	{"ajaca", -1, 12, ""},
	{"ejaca", -1, 14, ""},
	{"ljaca", -1, 13, ""},
	{"njaca", -1, 85, ""},
	{"ojaca", -1, 15, ""},
	{"alaca", -1, 82, ""},
	{"elaca", -1, 83, ""},
	{"olaca", -1, 84, ""},
	{"maca", -1, 75, ""},
	{"naca", -1, 76, ""},
	{"raca", -1, 81, ""},
	{"saca", -1, 80, ""},
	{"vaca", -1, 79, ""},
	{"\u0161aca", -1, 18, ""},
	{"aoca", -1, 82, ""},
	{"acaka", -1, 55, ""},
	{"ajaka", -1, 16, ""},
	{"ojaka", -1, 17, ""},
	{"anaka", -1, 78, ""},
	{"ataka", -1, 58, ""},
	{"etaka", -1, 59, ""},
	{"itaka", -1, 60, ""},
	{"otaka", -1, 61, ""},
	{"utaka", -1, 62, ""},
	{"a\u010Daka", -1, 54, ""},
	{"esama", -1, 67, ""},
	{"izama", -1, 87, ""},
	{"jacima", -1, 5, ""},
	{"nicima", -1, 23, ""},
	{"ticima", -1, 24, ""},
	{"teticima", 30, 21, ""},
	{"zicima", -1, 25, ""},
	{"atcima", -1, 58, ""},
	{"utcima", -1, 62, ""},
	{"\u010Dcima", -1, 74, ""},
	{"pesima", -1, 2, ""},
	{"inzima", -1, 19, ""},
	{"lozima", -1, 1, ""},
	{"metara", -1, 68, ""},
	{"centara", -1, 69, ""},
	{"istara", -1, 70, ""},
	{"ekata", -1, 86, ""},
	{"anata", -1, 53, ""},
	{"nstava", -1, 22, ""},
	{"kustava", -1, 29, ""},
	{"ajac", -1, 12, ""},
	{"ejac", -1, 14, ""},
	{"ljac", -1, 13, ""},
	{"njac", -1, 85, ""},
	{"anjac", 49, 11, ""},
	{"ojac", -1, 15, ""},
	{"alac", -1, 82, ""},
	{"elac", -1, 83, ""},
	{"olac", -1, 84, ""},
	{"mac", -1, 75, ""},
	{"nac", -1, 76, ""},
	{"rac", -1, 81, ""},
	{"sac", -1, 80, ""},
	{"vac", -1, 79, ""},
	{"\u0161ac", -1, 18, ""},
	{"jebe", -1, 88, ""},
	{"olce", -1, 84, ""},
	{"kuse", -1, 27, ""},
	{"rave", -1, 42, ""},
	{"save", -1, 52, ""},
	{"\u0161ave", -1, 51, ""},
	{"baci", -1, 89, ""},
	{"jaci", -1, 5, ""},
	{"tvenici", -1, 20, ""},
	{"snici", -1, 26, ""},
	{"tetici", -1, 21, ""},
	{"bojci", -1, 4, ""},
	{"vojci", -1, 3, ""},
	{"ojsci", -1, 66, ""},
	{"atci", -1, 58, ""},
	{"itci", -1, 60, ""},
	{"utci", -1, 62, ""},
	{"\u010Dci", -1, 74, ""},
	{"pesi", -1, 2, ""},
	{"inzi", -1, 19, ""},
	{"lozi", -1, 1, ""},
	{"acak", -1, 55, ""},
	{"usak", -1, 57, ""},
	{"atak", -1, 58, ""},
	{"etak", -1, 59, ""},
	{"itak", -1, 60, ""},
	{"otak", -1, 61, ""},
	{"utak", -1, 62, ""},
	{"a\u010Dak", -1, 54, ""},
	{"u\u0161ak", -1, 56, ""},
	{"izam", -1, 87, ""},
	{"tican", -1, 65, ""},
	{"cajan", -1, 7, ""},
	{"\u010Dajan", -1, 6, ""},
	{"voljan", -1, 77, ""},
	{"eskan", -1, 63, ""},
	{"alan", -1, 40, ""},
	{"bilan", -1, 33, ""},
	{"gilan", -1, 37, ""},
	{"nilan", -1, 39, ""},
	{"rilan", -1, 38, ""},
	{"silan", -1, 36, ""},
	{"tilan", -1, 34, ""},
	{"avilan", -1, 35, ""},
	{"laran", -1, 9, ""},
	{"eran", -1, 8, ""},
	{"asan", -1, 91, ""},
	{"esan", -1, 10, ""},
	{"dusan", -1, 31, ""},
	{"kusan", -1, 28, ""},
	{"atan", -1, 47, ""},
	{"pletan", -1, 50, ""},
	{"tetan", -1, 49, ""},
	{"antan", -1, 32, ""},
	{"pravan", -1, 44, ""},
	{"stavan", -1, 43, ""},
	{"sivan", -1, 46, ""},
	{"tivan", -1, 45, ""},
	{"ozan", -1, 41, ""},
	{"ti\u010Dan", -1, 64, ""},
	{"a\u0161an", -1, 90, ""},
	{"du\u0161an", -1, 30, ""},
	{"metar", -1, 68, ""},
	{"centar", -1, 69, ""},
	{"istar", -1, 70, ""},
	{"ekat", -1, 86, ""},
	{"enat", -1, 48, ""},
	{"oscu", -1, 72, ""},
	{"o\u0161\u0107u", -1, 71, ""},
}
var ser_a_2 = []among{
	{"aca", -1, 124, ""},
	{"eca", -1, 125, ""},
	{"uca", -1, 126, ""},
	{"ga", -1, 20, ""},
	{"acega", 3, 124, ""},
	{"ecega", 3, 125, ""},
	{"ucega", 3, 126, ""},
	{"anjijega", 3, 84, ""},
	{"enjijega", 3, 85, ""},
	{"snjijega", 3, 122, ""},
	{"\u0161njijega", 3, 86, ""},
	{"kijega", 3, 95, ""},
	{"skijega", 11, 1, ""},
	{"\u0161kijega", 11, 2, ""},
	{"elijega", 3, 83, ""},
	{"nijega", 3, 13, ""},
	{"osijega", 3, 123, ""},
	{"atijega", 3, 120, ""},
	{"evitijega", 3, 92, ""},
	{"ovitijega", 3, 93, ""},
	{"astijega", 3, 94, ""},
	{"avijega", 3, 77, ""},
	{"evijega", 3, 78, ""},
	{"ivijega", 3, 79, ""},
	{"ovijega", 3, 80, ""},
	{"o\u0161ijega", 3, 91, ""},
	{"anjega", 3, 84, ""},
	{"enjega", 3, 85, ""},
	{"snjega", 3, 122, ""},
	{"\u0161njega", 3, 86, ""},
	{"kega", 3, 95, ""},
	{"skega", 30, 1, ""},
	{"\u0161kega", 30, 2, ""},
	{"elega", 3, 83, ""},
	{"nega", 3, 13, ""},
	{"anega", 34, 10, ""},
	{"enega", 34, 87, ""},
	{"snega", 34, 159, ""},
	{"\u0161nega", 34, 88, ""},
	{"osega", 3, 123, ""},
	{"atega", 3, 120, ""},
	{"evitega", 3, 92, ""},
	{"ovitega", 3, 93, ""},
	{"astega", 3, 94, ""},
	{"avega", 3, 77, ""},
	{"evega", 3, 78, ""},
	{"ivega", 3, 79, ""},
	{"ovega", 3, 80, ""},
	{"a\u0107ega", 3, 14, ""},
	{"e\u0107ega", 3, 15, ""},
	{"u\u0107ega", 3, 16, ""},
	{"o\u0161ega", 3, 91, ""},
	{"acoga", 3, 124, ""},
	{"ecoga", 3, 125, ""},
	{"ucoga", 3, 126, ""},
	{"anjoga", 3, 84, ""},
	{"enjoga", 3, 85, ""},
	{"snjoga", 3, 122, ""},
	{"\u0161njoga", 3, 86, ""},
	{"koga", 3, 95, ""},
	{"skoga", 59, 1, ""},
	{"\u0161koga", 59, 2, ""},
	{"loga", 3, 19, ""},
	{"eloga", 62, 83, ""},
	{"noga", 3, 13, ""},
	{"cinoga", 64, 137, ""},
	{"\u010Dinoga", 64, 89, ""},
	{"osoga", 3, 123, ""},
	{"atoga", 3, 120, ""},
	{"evitoga", 3, 92, ""},
	{"ovitoga", 3, 93, ""},
	{"astoga", 3, 94, ""},
	{"avoga", 3, 77, ""},
	{"evoga", 3, 78, ""},
	{"ivoga", 3, 79, ""},
	{"ovoga", 3, 80, ""},
	{"a\u0107oga", 3, 14, ""},
	{"e\u0107oga", 3, 15, ""},
	{"u\u0107oga", 3, 16, ""},
	{"o\u0161oga", 3, 91, ""},
	{"uga", 3, 18, ""},
	{"aja", -1, 109, ""},
	{"caja", 81, 26, ""},
	{"laja", 81, 30, ""},
	{"raja", 81, 31, ""},
	{"\u0107aja", 81, 28, ""},
	{"\u010Daja", 81, 27, ""},
	{"\u0111aja", 81, 29, ""},
	{"bija", -1, 32, ""},
	{"cija", -1, 33, ""},
	{"dija", -1, 34, ""},
	{"fija", -1, 40, ""},
	{"gija", -1, 39, ""},
	{"anjija", -1, 84, ""},
	{"enjija", -1, 85, ""},
	{"snjija", -1, 122, ""},
	{"\u0161njija", -1, 86, ""},
	{"kija", -1, 95, ""},
	{"skija", 97, 1, ""},
	{"\u0161kija", 97, 2, ""},
	{"lija", -1, 24, ""},
	{"elija", 100, 83, ""},
	{"mija", -1, 37, ""},
	{"nija", -1, 13, ""},
	{"ganija", 103, 9, ""},
	{"manija", 103, 6, ""},
	{"panija", 103, 7, ""},
	{"ranija", 103, 8, ""},
	{"tanija", 103, 5, ""},
	{"pija", -1, 41, ""},
	{"rija", -1, 42, ""},
	{"rarija", 110, 21, ""},
	{"sija", -1, 23, ""},
	{"osija", 112, 123, ""},
	{"tija", -1, 44, ""},
	{"atija", 114, 120, ""},
	{"evitija", 114, 92, ""},
	{"ovitija", 114, 93, ""},
	{"otija", 114, 22, ""},
	{"astija", 114, 94, ""},
	{"avija", -1, 77, ""},
	{"evija", -1, 78, ""},
	{"ivija", -1, 79, ""},
	{"ovija", -1, 80, ""},
	{"zija", -1, 45, ""},
	{"o\u0161ija", -1, 91, ""},
	{"\u017Eija", -1, 38, ""},
	{"anja", -1, 84, ""},
	{"enja", -1, 85, ""},
	{"snja", -1, 122, ""},
	{"\u0161nja", -1, 86, ""},
	{"ka", -1, 95, ""},
	{"ska", 131, 1, ""},
	{"\u0161ka", 131, 2, ""},
	{"ala", -1, 104, ""},
	{"acala", 134, 128, ""},
	{"astajala", 134, 106, ""},
	{"istajala", 134, 107, ""},
	{"ostajala", 134, 108, ""},
	{"ijala", 134, 47, ""},
	{"injala", 134, 114, ""},
	{"nala", 134, 46, ""},
	{"irala", 134, 100, ""},
	{"urala", 134, 105, ""},
	{"tala", 134, 113, ""},
	{"astala", 144, 110, ""},
	{"istala", 144, 111, ""},
	{"ostala", 144, 112, ""},
	{"avala", 134, 97, ""},
	{"evala", 134, 96, ""},
	{"ivala", 134, 98, ""},
	{"ovala", 134, 76, ""},
	{"uvala", 134, 99, ""},
	{"a\u010Dala", 134, 102, ""},
	{"ela", -1, 83, ""},
	{"ila", -1, 116, ""},
	{"acila", 155, 124, ""},
	{"lucila", 155, 121, ""},
	{"nila", 155, 103, ""},
	{"astanila", 158, 110, ""},
	{"istanila", 158, 111, ""},
	{"ostanila", 158, 112, ""},
	{"rosila", 155, 127, ""},
	{"jetila", 155, 118, ""},
	{"ozila", 155, 48, ""},
	{"a\u010Dila", 155, 101, ""},
	{"lu\u010Dila", 155, 117, ""},
	{"ro\u0161ila", 155, 90, ""},
	{"ola", -1, 50, ""},
	{"asla", -1, 115, ""},
	{"nula", -1, 13, ""},
	{"gama", -1, 20, ""},
	{"logama", 171, 19, ""},
	{"ugama", 171, 18, ""},
	{"ajama", -1, 109, ""},
	{"cajama", 174, 26, ""},
	{"lajama", 174, 30, ""},
	{"rajama", 174, 31, ""},
	{"\u0107ajama", 174, 28, ""},
	{"\u010Dajama", 174, 27, ""},
	{"\u0111ajama", 174, 29, ""},
	{"bijama", -1, 32, ""},
	{"cijama", -1, 33, ""},
	{"dijama", -1, 34, ""},
	{"fijama", -1, 40, ""},
	{"gijama", -1, 39, ""},
	{"lijama", -1, 35, ""},
	{"mijama", -1, 37, ""},
	{"nijama", -1, 36, ""},
	{"ganijama", 188, 9, ""},
	{"manijama", 188, 6, ""},
	{"panijama", 188, 7, ""},
	{"ranijama", 188, 8, ""},
	{"tanijama", 188, 5, ""},
	{"pijama", -1, 41, ""},
	{"rijama", -1, 42, ""},
	{"sijama", -1, 43, ""},
	{"tijama", -1, 44, ""},
	{"zijama", -1, 45, ""},
	{"\u017Eijama", -1, 38, ""},
	{"alama", -1, 104, ""},
	{"ijalama", 200, 47, ""},
	{"nalama", 200, 46, ""},
	{"elama", -1, 119, ""},
	{"ilama", -1, 116, ""},
	{"ramama", -1, 52, ""},
	{"lemama", -1, 51, ""},
	{"inama", -1, 11, ""},
	{"cinama", 207, 137, ""},
	{"\u010Dinama", 207, 89, ""},
	{"rama", -1, 52, ""},
	{"arama", 210, 53, ""},
	{"drama", 210, 54, ""},
	{"erama", 210, 55, ""},
	{"orama", 210, 56, ""},
	{"basama", -1, 135, ""},
	{"gasama", -1, 131, ""},
	{"jasama", -1, 129, ""},
	{"kasama", -1, 133, ""},
	{"nasama", -1, 132, ""},
	{"tasama", -1, 130, ""},
	{"vasama", -1, 134, ""},
	{"esama", -1, 152, ""},
	{"isama", -1, 154, ""},
	{"etama", -1, 70, ""},
	{"estama", -1, 71, ""},
	{"istama", -1, 72, ""},
	{"kstama", -1, 73, ""},
	{"ostama", -1, 74, ""},
	{"avama", -1, 77, ""},
	{"evama", -1, 78, ""},
	{"ivama", -1, 79, ""},
	{"ba\u0161ama", -1, 63, ""},
	{"ga\u0161ama", -1, 64, ""},
	{"ja\u0161ama", -1, 61, ""},
	{"ka\u0161ama", -1, 62, ""},
	{"na\u0161ama", -1, 60, ""},
	{"ta\u0161ama", -1, 59, ""},
	{"va\u0161ama", -1, 65, ""},
	{"e\u0161ama", -1, 66, ""},
	{"i\u0161ama", -1, 67, ""},
	{"lema", -1, 51, ""},
	{"acima", -1, 124, ""},
	{"ecima", -1, 125, ""},
	{"ucima", -1, 126, ""},
	{"ajima", -1, 109, ""},
	{"cajima", 245, 26, ""},
	{"lajima", 245, 30, ""},
	{"rajima", 245, 31, ""},
	{"\u0107ajima", 245, 28, ""},
	{"\u010Dajima", 245, 27, ""},
	{"\u0111ajima", 245, 29, ""},
	{"bijima", -1, 32, ""},
	{"cijima", -1, 33, ""},
	{"dijima", -1, 34, ""},
	{"fijima", -1, 40, ""},
	{"gijima", -1, 39, ""},
	{"anjijima", -1, 84, ""},
	{"enjijima", -1, 85, ""},
	{"snjijima", -1, 122, ""},
	{"\u0161njijima", -1, 86, ""},
	{"kijima", -1, 95, ""},
	{"skijima", 261, 1, ""},
	{"\u0161kijima", 261, 2, ""},
	{"lijima", -1, 35, ""},
	{"elijima", 264, 83, ""},
	{"mijima", -1, 37, ""},
	{"nijima", -1, 13, ""},
	{"ganijima", 267, 9, ""},
	{"manijima", 267, 6, ""},
	{"panijima", 267, 7, ""},
	{"ranijima", 267, 8, ""},
	{"tanijima", 267, 5, ""},
	{"pijima", -1, 41, ""},
	{"rijima", -1, 42, ""},
	{"sijima", -1, 43, ""},
	{"osijima", 275, 123, ""},
	{"tijima", -1, 44, ""},
	{"atijima", 277, 120, ""},
	{"evitijima", 277, 92, ""},
	{"ovitijima", 277, 93, ""},
	{"astijima", 277, 94, ""},
	{"avijima", -1, 77, ""},
	{"evijima", -1, 78, ""},
	{"ivijima", -1, 79, ""},
	{"ovijima", -1, 80, ""},
	{"zijima", -1, 45, ""},
	{"o\u0161ijima", -1, 91, ""},
	{"\u017Eijima", -1, 38, ""},
	{"anjima", -1, 84, ""},
	{"enjima", -1, 85, ""},
	{"snjima", -1, 122, ""},
	{"\u0161njima", -1, 86, ""},
	{"kima", -1, 95, ""},
	{"skima", 293, 1, ""},
	{"\u0161kima", 293, 2, ""},
	{"alima", -1, 104, ""},
	{"ijalima", 296, 47, ""},
	{"nalima", 296, 46, ""},
	{"elima", -1, 83, ""},
	{"ilima", -1, 116, ""},
	{"ozilima", 300, 48, ""},
	{"olima", -1, 50, ""},
	{"lemima", -1, 51, ""},
	{"nima", -1, 13, ""},
	{"anima", 304, 10, ""},
	{"inima", 304, 11, ""},
	{"cinima", 306, 137, ""},
	{"\u010Dinima", 306, 89, ""},
	{"onima", 304, 12, ""},
	{"arima", -1, 53, ""},
	{"drima", -1, 54, ""},
	{"erima", -1, 55, ""},
	{"orima", -1, 56, ""},
	{"basima", -1, 135, ""},
	{"gasima", -1, 131, ""},
	{"jasima", -1, 129, ""},
	{"kasima", -1, 133, ""},
	{"nasima", -1, 132, ""},
	{"tasima", -1, 130, ""},
	{"vasima", -1, 134, ""},
	{"esima", -1, 57, ""},
	{"isima", -1, 58, ""},
	{"osima", -1, 123, ""},
	{"atima", -1, 120, ""},
	{"ikatima", 324, 68, ""},
	{"latima", 324, 69, ""},
	{"etima", -1, 70, ""},
	{"evitima", -1, 92, ""},
	{"ovitima", -1, 93, ""},
	{"astima", -1, 94, ""},
	{"estima", -1, 71, ""},
	{"istima", -1, 72, ""},
	{"kstima", -1, 73, ""},
	{"ostima", -1, 74, ""},
	{"i\u0161tima", -1, 75, ""},
	{"avima", -1, 77, ""},
	{"evima", -1, 78, ""},
	{"ajevima", 337, 109, ""},
	{"cajevima", 338, 26, ""},
	{"lajevima", 338, 30, ""},
	{"rajevima", 338, 31, ""},
	{"\u0107ajevima", 338, 28, ""},
	{"\u010Dajevima", 338, 27, ""},
	{"\u0111ajevima", 338, 29, ""},
	{"ivima", -1, 79, ""},
	{"ovima", -1, 80, ""},
	{"govima", 346, 20, ""},
	{"ugovima", 347, 17, ""},
	{"lovima", 346, 82, ""},
	{"olovima", 349, 49, ""},
	{"movima", 346, 81, ""},
	{"onovima", 346, 12, ""},
	{"stvima", -1, 3, ""},
	{"\u0161tvima", -1, 4, ""},
	{"a\u0107ima", -1, 14, ""},
	{"e\u0107ima", -1, 15, ""},
	{"u\u0107ima", -1, 16, ""},
	{"ba\u0161ima", -1, 63, ""},
	{"ga\u0161ima", -1, 64, ""},
	{"ja\u0161ima", -1, 61, ""},
	{"ka\u0161ima", -1, 62, ""},
	{"na\u0161ima", -1, 60, ""},
	{"ta\u0161ima", -1, 59, ""},
	{"va\u0161ima", -1, 65, ""},
	{"e\u0161ima", -1, 66, ""},
	{"i\u0161ima", -1, 67, ""},
	{"o\u0161ima", -1, 91, ""},
	{"na", -1, 13, ""},
	{"ana", 368, 10, ""},
	{"acana", 369, 128, ""},
	{"urana", 369, 105, ""},
	{"tana", 369, 113, ""},
	{"avana", 369, 97, ""},
	{"evana", 369, 96, ""},
	{"ivana", 369, 98, ""},
	{"uvana", 369, 99, ""},
	{"a\u010Dana", 369, 102, ""},
	{"acena", 368, 124, ""},
	{"lucena", 368, 121, ""},
	{"a\u010Dena", 368, 101, ""},
	{"lu\u010Dena", 368, 117, ""},
	{"ina", 368, 11, ""},
	{"cina", 382, 137, ""},
	{"anina", 382, 10, ""},
	{"\u010Dina", 382, 89, ""},
	{"ona", 368, 12, ""},
	{"ara", -1, 53, ""},
	{"dra", -1, 54, ""},
	{"era", -1, 55, ""},
	{"ora", -1, 56, ""},
	{"basa", -1, 135, ""},
	{"gasa", -1, 131, ""},
	{"jasa", -1, 129, ""},
	{"kasa", -1, 133, ""},
	{"nasa", -1, 132, ""},
	{"tasa", -1, 130, ""},
	{"vasa", -1, 134, ""},
	{"esa", -1, 57, ""},
	{"isa", -1, 58, ""},
	{"osa", -1, 123, ""},
	{"ata", -1, 120, ""},
	{"ikata", 401, 68, ""},
	{"lata", 401, 69, ""},
	{"eta", -1, 70, ""},
	{"evita", -1, 92, ""},
	{"ovita", -1, 93, ""},
	{"asta", -1, 94, ""},
	{"esta", -1, 71, ""},
	{"ista", -1, 72, ""},
	{"ksta", -1, 73, ""},
	{"osta", -1, 74, ""},
	{"nuta", -1, 13, ""},
	{"i\u0161ta", -1, 75, ""},
	{"ava", -1, 77, ""},
	{"eva", -1, 78, ""},
	{"ajeva", 415, 109, ""},
	{"cajeva", 416, 26, ""},
	{"lajeva", 416, 30, ""},
	{"rajeva", 416, 31, ""},
	{"\u0107ajeva", 416, 28, ""},
	{"\u010Dajeva", 416, 27, ""},
	{"\u0111ajeva", 416, 29, ""},
	{"iva", -1, 79, ""},
	{"ova", -1, 80, ""},
	{"gova", 424, 20, ""},
	{"ugova", 425, 17, ""},
	{"lova", 424, 82, ""},
	{"olova", 427, 49, ""},
	{"mova", 424, 81, ""},
	{"onova", 424, 12, ""},
	{"stva", -1, 3, ""},
	{"\u0161tva", -1, 4, ""},
	{"a\u0107a", -1, 14, ""},
	{"e\u0107a", -1, 15, ""},
	{"u\u0107a", -1, 16, ""},
	{"ba\u0161a", -1, 63, ""},
	{"ga\u0161a", -1, 64, ""},
	{"ja\u0161a", -1, 61, ""},
	{"ka\u0161a", -1, 62, ""},
	{"na\u0161a", -1, 60, ""},
	{"ta\u0161a", -1, 59, ""},
	{"va\u0161a", -1, 65, ""},
	{"e\u0161a", -1, 66, ""},
	{"i\u0161a", -1, 67, ""},
	{"o\u0161a", -1, 91, ""},
	{"ace", -1, 124, ""},
	{"ece", -1, 125, ""},
	{"uce", -1, 126, ""},
	{"luce", 448, 121, ""},
	{"astade", -1, 110, ""},
	{"istade", -1, 111, ""},
	{"ostade", -1, 112, ""},
	{"ge", -1, 20, ""},
	{"loge", 453, 19, ""},
	{"uge", 453, 18, ""},
	{"aje", -1, 104, ""},
	{"caje", 456, 26, ""},
	{"laje", 456, 30, ""},
	{"raje", 456, 31, ""},
	{"astaje", 456, 106, ""},
	{"istaje", 456, 107, ""},
	{"ostaje", 456, 108, ""},
	{"\u0107aje", 456, 28, ""},
	{"\u010Daje", 456, 27, ""},
	{"\u0111aje", 456, 29, ""},
	{"ije", -1, 116, ""},
	{"bije", 466, 32, ""},
	{"cije", 466, 33, ""},
	{"dije", 466, 34, ""},
	{"fije", 466, 40, ""},
	{"gije", 466, 39, ""},
	{"anjije", 466, 84, ""},
	{"enjije", 466, 85, ""},
	{"snjije", 466, 122, ""},
	{"\u0161njije", 466, 86, ""},
	{"kije", 466, 95, ""},
	{"skije", 476, 1, ""},
	{"\u0161kije", 476, 2, ""},
	{"lije", 466, 35, ""},
	{"elije", 479, 83, ""},
	{"mije", 466, 37, ""},
	{"nije", 466, 13, ""},
	{"ganije", 482, 9, ""},
	{"manije", 482, 6, ""},
	{"panije", 482, 7, ""},
	{"ranije", 482, 8, ""},
	{"tanije", 482, 5, ""},
	{"pije", 466, 41, ""},
	{"rije", 466, 42, ""},
	{"sije", 466, 43, ""},
	{"osije", 490, 123, ""},
	{"tije", 466, 44, ""},
	{"atije", 492, 120, ""},
	{"evitije", 492, 92, ""},
	{"ovitije", 492, 93, ""},
	{"astije", 492, 94, ""},
	{"avije", 466, 77, ""},
	{"evije", 466, 78, ""},
	{"ivije", 466, 79, ""},
	{"ovije", 466, 80, ""},
	{"zije", 466, 45, ""},
	{"o\u0161ije", 466, 91, ""},
	{"\u017Eije", 466, 38, ""},
	{"anje", -1, 84, ""},
	{"enje", -1, 85, ""},
	{"snje", -1, 122, ""},
	{"\u0161nje", -1, 86, ""},
	{"uje", -1, 25, ""},
	{"lucuje", 508, 121, ""},
	{"iruje", 508, 100, ""},
	{"lu\u010Duje", 508, 117, ""},
	{"ke", -1, 95, ""},
	{"ske", 512, 1, ""},
	{"\u0161ke", 512, 2, ""},
	{"ale", -1, 104, ""},
	{"acale", 515, 128, ""},
	{"astajale", 515, 106, ""},
	{"istajale", 515, 107, ""},
	{"ostajale", 515, 108, ""},
	{"ijale", 515, 47, ""},
	{"injale", 515, 114, ""},
	{"nale", 515, 46, ""},
	{"irale", 515, 100, ""},
	{"urale", 515, 105, ""},
	{"tale", 515, 113, ""},
	{"astale", 525, 110, ""},
	{"istale", 525, 111, ""},
	{"ostale", 525, 112, ""},
	{"avale", 515, 97, ""},
	{"evale", 515, 96, ""},
	{"ivale", 515, 98, ""},
	{"ovale", 515, 76, ""},
	{"uvale", 515, 99, ""},
	{"a\u010Dale", 515, 102, ""},
	{"ele", -1, 83, ""},
	{"ile", -1, 116, ""},
	{"acile", 536, 124, ""},
	{"lucile", 536, 121, ""},
	{"nile", 536, 103, ""},
	{"rosile", 536, 127, ""},
	{"jetile", 536, 118, ""},
	{"ozile", 536, 48, ""},
	{"a\u010Dile", 536, 101, ""},
	{"lu\u010Dile", 536, 117, ""},
	{"ro\u0161ile", 536, 90, ""},
	{"ole", -1, 50, ""},
	{"asle", -1, 115, ""},
	{"nule", -1, 13, ""},
	{"rame", -1, 52, ""},
	{"leme", -1, 51, ""},
	{"acome", -1, 124, ""},
	{"ecome", -1, 125, ""},
	{"ucome", -1, 126, ""},
	{"anjome", -1, 84, ""},
	{"enjome", -1, 85, ""},
	{"snjome", -1, 122, ""},
	{"\u0161njome", -1, 86, ""},
	{"kome", -1, 95, ""},
	{"skome", 558, 1, ""},
	{"\u0161kome", 558, 2, ""},
	{"elome", -1, 83, ""},
	{"nome", -1, 13, ""},
	{"cinome", 562, 137, ""},
	{"\u010Dinome", 562, 89, ""},
	{"osome", -1, 123, ""},
	{"atome", -1, 120, ""},
	{"evitome", -1, 92, ""},
	{"ovitome", -1, 93, ""},
	{"astome", -1, 94, ""},
	{"avome", -1, 77, ""},
	{"evome", -1, 78, ""},
	{"ivome", -1, 79, ""},
	{"ovome", -1, 80, ""},
	{"a\u0107ome", -1, 14, ""},
	{"e\u0107ome", -1, 15, ""},
	{"u\u0107ome", -1, 16, ""},
	{"o\u0161ome", -1, 91, ""},
	{"ne", -1, 13, ""},
	{"ane", 578, 10, ""},
	{"acane", 579, 128, ""},
	{"urane", 579, 105, ""},
	{"tane", 579, 113, ""},
	{"astane", 582, 110, ""},
	{"istane", 582, 111, ""},
	{"ostane", 582, 112, ""},
	{"avane", 579, 97, ""},
	{"evane", 579, 96, ""},
	{"ivane", 579, 98, ""},
	{"uvane", 579, 99, ""},
	{"a\u010Dane", 579, 102, ""},
	{"acene", 578, 124, ""},
	{"lucene", 578, 121, ""},
	{"a\u010Dene", 578, 101, ""},
	{"lu\u010Dene", 578, 117, ""},
	{"ine", 578, 11, ""},
	{"cine", 595, 137, ""},
	{"anine", 595, 10, ""},
	{"\u010Dine", 595, 89, ""},
	{"one", 578, 12, ""},
	{"are", -1, 53, ""},
	{"dre", -1, 54, ""},
	{"ere", -1, 55, ""},
	{"ore", -1, 56, ""},
	{"ase", -1, 161, ""},
	{"base", 604, 135, ""},
	{"acase", 604, 128, ""},
	{"gase", 604, 131, ""},
	{"jase", 604, 129, ""},
	{"astajase", 608, 138, ""},
	{"istajase", 608, 139, ""},
	{"ostajase", 608, 140, ""},
	{"injase", 608, 150, ""},
	{"kase", 604, 133, ""},
	{"nase", 604, 132, ""},
	{"irase", 604, 155, ""},
	{"urase", 604, 156, ""},
	{"tase", 604, 130, ""},
	{"vase", 604, 134, ""},
	{"avase", 618, 144, ""},
	{"evase", 618, 145, ""},
	{"ivase", 618, 146, ""},
	{"ovase", 618, 148, ""},
	{"uvase", 618, 147, ""},
	{"ese", -1, 57, ""},
	{"ise", -1, 58, ""},
	{"acise", 625, 124, ""},
	{"lucise", 625, 121, ""},
	{"rosise", 625, 127, ""},
	{"jetise", 625, 149, ""},
	{"ose", -1, 123, ""},
	{"astadose", 630, 141, ""},
	{"istadose", 630, 142, ""},
	{"ostadose", 630, 143, ""},
	{"ate", -1, 104, ""},
	{"acate", 634, 128, ""},
	{"ikate", 634, 68, ""},
	{"late", 634, 69, ""},
	{"irate", 634, 100, ""},
	{"urate", 634, 105, ""},
	{"tate", 634, 113, ""},
	{"avate", 634, 97, ""},
	{"evate", 634, 96, ""},
	{"ivate", 634, 98, ""},
	{"uvate", 634, 99, ""},
	{"a\u010Date", 634, 102, ""},
	{"ete", -1, 70, ""},
	{"astadete", 646, 110, ""},
	{"istadete", 646, 111, ""},
	{"ostadete", 646, 112, ""},
	{"astajete", 646, 106, ""},
	{"istajete", 646, 107, ""},
	{"ostajete", 646, 108, ""},
	{"ijete", 646, 116, ""},
	{"injete", 646, 114, ""},
	{"ujete", 646, 25, ""},
	{"lucujete", 655, 121, ""},
	{"irujete", 655, 100, ""},
	{"lu\u010Dujete", 655, 117, ""},
	{"nete", 646, 13, ""},
	{"astanete", 659, 110, ""},
	{"istanete", 659, 111, ""},
	{"ostanete", 659, 112, ""},
	{"astete", 646, 115, ""},
	{"ite", -1, 116, ""},
	{"acite", 664, 124, ""},
	{"lucite", 664, 121, ""},
	{"nite", 664, 13, ""},
	{"astanite", 667, 110, ""},
	{"istanite", 667, 111, ""},
	{"ostanite", 667, 112, ""},
	{"rosite", 664, 127, ""},
	{"jetite", 664, 118, ""},
	{"astite", 664, 115, ""},
	{"evite", 664, 92, ""},
	{"ovite", 664, 93, ""},
	{"a\u010Dite", 664, 101, ""},
	{"lu\u010Dite", 664, 117, ""},
	{"ro\u0161ite", 664, 90, ""},
	{"ajte", -1, 104, ""},
	{"urajte", 679, 105, ""},
	{"tajte", 679, 113, ""},
	{"astajte", 681, 106, ""},
	{"istajte", 681, 107, ""},
	{"ostajte", 681, 108, ""},
	{"avajte", 679, 97, ""},
	{"evajte", 679, 96, ""},
	{"ivajte", 679, 98, ""},
	{"uvajte", 679, 99, ""},
	{"ijte", -1, 116, ""},
	{"lucujte", -1, 121, ""},
	{"irujte", -1, 100, ""},
	{"lu\u010Dujte", -1, 117, ""},
	{"aste", -1, 94, ""},
	{"acaste", 693, 128, ""},
	{"astajaste", 693, 106, ""},
	{"istajaste", 693, 107, ""},
	{"ostajaste", 693, 108, ""},
	{"injaste", 693, 114, ""},
	{"iraste", 693, 100, ""},
	{"uraste", 693, 105, ""},
	{"taste", 693, 113, ""},
	{"avaste", 693, 97, ""},
	{"evaste", 693, 96, ""},
	{"ivaste", 693, 98, ""},
	{"ovaste", 693, 76, ""},
	{"uvaste", 693, 99, ""},
	{"a\u010Daste", 693, 102, ""},
	{"este", -1, 71, ""},
	{"iste", -1, 72, ""},
	{"aciste", 709, 124, ""},
	{"luciste", 709, 121, ""},
	{"niste", 709, 103, ""},
	{"rosiste", 709, 127, ""},
	{"jetiste", 709, 118, ""},
	{"a\u010Diste", 709, 101, ""},
	{"lu\u010Diste", 709, 117, ""},
	{"ro\u0161iste", 709, 90, ""},
	{"kste", -1, 73, ""},
	{"oste", -1, 74, ""},
	{"astadoste", 719, 110, ""},
	{"istadoste", 719, 111, ""},
	{"ostadoste", 719, 112, ""},
	{"nuste", -1, 13, ""},
	{"i\u0161te", -1, 75, ""},
	{"ave", -1, 77, ""},
	{"eve", -1, 78, ""},
	{"ajeve", 726, 109, ""},
	{"cajeve", 727, 26, ""},
	{"lajeve", 727, 30, ""},
	{"rajeve", 727, 31, ""},
	{"\u0107ajeve", 727, 28, ""},
	{"\u010Dajeve", 727, 27, ""},
	{"\u0111ajeve", 727, 29, ""},
	{"ive", -1, 79, ""},
	{"ove", -1, 80, ""},
	{"gove", 735, 20, ""},
	{"ugove", 736, 17, ""},
	{"love", 735, 82, ""},
	{"olove", 738, 49, ""},
	{"move", 735, 81, ""},
	{"onove", 735, 12, ""},
	{"a\u0107e", -1, 14, ""},
	{"e\u0107e", -1, 15, ""},
	{"u\u0107e", -1, 16, ""},
	{"a\u010De", -1, 101, ""},
	{"lu\u010De", -1, 117, ""},
	{"a\u0161e", -1, 104, ""},
	{"ba\u0161e", 747, 63, ""},
	{"ga\u0161e", 747, 64, ""},
	{"ja\u0161e", 747, 61, ""},
	{"astaja\u0161e", 750, 106, ""},
	{"istaja\u0161e", 750, 107, ""},
	{"ostaja\u0161e", 750, 108, ""},
	{"inja\u0161e", 750, 114, ""},
	{"ka\u0161e", 747, 62, ""},
	{"na\u0161e", 747, 60, ""},
	{"ira\u0161e", 747, 100, ""},
	{"ura\u0161e", 747, 105, ""},
	{"ta\u0161e", 747, 59, ""},
	{"va\u0161e", 747, 65, ""},
	{"ava\u0161e", 760, 97, ""},
	{"eva\u0161e", 760, 96, ""},
	{"iva\u0161e", 760, 98, ""},
	{"ova\u0161e", 760, 76, ""},
	{"uva\u0161e", 760, 99, ""},
	{"a\u010Da\u0161e", 747, 102, ""},
	{"e\u0161e", -1, 66, ""},
	{"i\u0161e", -1, 67, ""},
	{"jeti\u0161e", 768, 118, ""},
	{"a\u010Di\u0161e", 768, 101, ""},
	{"lu\u010Di\u0161e", 768, 117, ""},
	{"ro\u0161i\u0161e", 768, 90, ""},
	{"o\u0161e", -1, 91, ""},
	{"astado\u0161e", 773, 110, ""},
	{"istado\u0161e", 773, 111, ""},
	{"ostado\u0161e", 773, 112, ""},
	{"aceg", -1, 124, ""},
	{"eceg", -1, 125, ""},
	{"uceg", -1, 126, ""},
	{"anjijeg", -1, 84, ""},
	{"enjijeg", -1, 85, ""},
	{"snjijeg", -1, 122, ""},
	{"\u0161njijeg", -1, 86, ""},
	{"kijeg", -1, 95, ""},
	{"skijeg", 784, 1, ""},
	{"\u0161kijeg", 784, 2, ""},
	{"elijeg", -1, 83, ""},
	{"nijeg", -1, 13, ""},
	{"osijeg", -1, 123, ""},
	{"atijeg", -1, 120, ""},
	{"evitijeg", -1, 92, ""},
	{"ovitijeg", -1, 93, ""},
	{"astijeg", -1, 94, ""},
	{"avijeg", -1, 77, ""},
	{"evijeg", -1, 78, ""},
	{"ivijeg", -1, 79, ""},
	{"ovijeg", -1, 80, ""},
	{"o\u0161ijeg", -1, 91, ""},
	{"anjeg", -1, 84, ""},
	{"enjeg", -1, 85, ""},
	{"snjeg", -1, 122, ""},
	{"\u0161njeg", -1, 86, ""},
	{"keg", -1, 95, ""},
	{"eleg", -1, 83, ""},
	{"neg", -1, 13, ""},
	{"aneg", 805, 10, ""},
	{"eneg", 805, 87, ""},
	{"sneg", 805, 159, ""},
	{"\u0161neg", 805, 88, ""},
	{"oseg", -1, 123, ""},
	{"ateg", -1, 120, ""},
	{"aveg", -1, 77, ""},
	{"eveg", -1, 78, ""},
	{"iveg", -1, 79, ""},
	{"oveg", -1, 80, ""},
	{"a\u0107eg", -1, 14, ""},
	{"e\u0107eg", -1, 15, ""},
	{"u\u0107eg", -1, 16, ""},
	{"o\u0161eg", -1, 91, ""},
	{"acog", -1, 124, ""},
	{"ecog", -1, 125, ""},
	{"ucog", -1, 126, ""},
	{"anjog", -1, 84, ""},
	{"enjog", -1, 85, ""},
	{"snjog", -1, 122, ""},
	{"\u0161njog", -1, 86, ""},
	{"kog", -1, 95, ""},
	{"skog", 827, 1, ""},
	{"\u0161kog", 827, 2, ""},
	{"elog", -1, 83, ""},
	{"nog", -1, 13, ""},
	{"cinog", 831, 137, ""},
	{"\u010Dinog", 831, 89, ""},
	{"osog", -1, 123, ""},
	{"atog", -1, 120, ""},
	{"evitog", -1, 92, ""},
	{"ovitog", -1, 93, ""},
	{"astog", -1, 94, ""},
	{"avog", -1, 77, ""},
	{"evog", -1, 78, ""},
	{"ivog", -1, 79, ""},
	{"ovog", -1, 80, ""},
	{"a\u0107og", -1, 14, ""},
	{"e\u0107og", -1, 15, ""},
	{"u\u0107og", -1, 16, ""},
	{"o\u0161og", -1, 91, ""},
	{"ah", -1, 104, ""},
	{"acah", 847, 128, ""},
	{"astajah", 847, 106, ""},
	{"istajah", 847, 107, ""},
	{"ostajah", 847, 108, ""},
	{"injah", 847, 114, ""},
	{"irah", 847, 100, ""},
	{"urah", 847, 105, ""},
	{"tah", 847, 113, ""},
	{"avah", 847, 97, ""},
	{"evah", 847, 96, ""},
	{"ivah", 847, 98, ""},
	{"ovah", 847, 76, ""},
	{"uvah", 847, 99, ""},
	{"a\u010Dah", 847, 102, ""},
	{"ih", -1, 116, ""},
	{"acih", 862, 124, ""},
	{"ecih", 862, 125, ""},
	{"ucih", 862, 126, ""},
	{"lucih", 865, 121, ""},
	{"anjijih", 862, 84, ""},
	{"enjijih", 862, 85, ""},
	{"snjijih", 862, 122, ""},
	{"\u0161njijih", 862, 86, ""},
	{"kijih", 862, 95, ""},
	{"skijih", 871, 1, ""},
	{"\u0161kijih", 871, 2, ""},
	{"elijih", 862, 83, ""},
	{"nijih", 862, 13, ""},
	{"osijih", 862, 123, ""},
	{"atijih", 862, 120, ""},
	{"evitijih", 862, 92, ""},
	{"ovitijih", 862, 93, ""},
	{"astijih", 862, 94, ""},
	{"avijih", 862, 77, ""},
	{"evijih", 862, 78, ""},
	{"ivijih", 862, 79, ""},
	{"ovijih", 862, 80, ""},
	{"o\u0161ijih", 862, 91, ""},
	{"anjih", 862, 84, ""},
	{"enjih", 862, 85, ""},
	{"snjih", 862, 122, ""},
	{"\u0161njih", 862, 86, ""},
	{"kih", 862, 95, ""},
	{"skih", 890, 1, ""},
	{"\u0161kih", 890, 2, ""},
	{"elih", 862, 83, ""},
	{"nih", 862, 13, ""},
	{"cinih", 894, 137, ""},
	{"\u010Dinih", 894, 89, ""},
	{"osih", 862, 123, ""},
	{"rosih", 897, 127, ""},
	{"atih", 862, 120, ""},
	{"jetih", 862, 118, ""},
	{"evitih", 862, 92, ""},
	{"ovitih", 862, 93, ""},
	{"astih", 862, 94, ""},
	{"avih", 862, 77, ""},
	{"evih", 862, 78, ""},
	{"ivih", 862, 79, ""},
	{"ovih", 862, 80, ""},
	{"a\u0107ih", 862, 14, ""},
	{"e\u0107ih", 862, 15, ""},
	{"u\u0107ih", 862, 16, ""},
	{"a\u010Dih", 862, 101, ""},
	{"lu\u010Dih", 862, 117, ""},
	{"o\u0161ih", 862, 91, ""},
	{"ro\u0161ih", 913, 90, ""},
	{"astadoh", -1, 110, ""},
	{"istadoh", -1, 111, ""},
	{"ostadoh", -1, 112, ""},
	{"acuh", -1, 124, ""},
	{"ecuh", -1, 125, ""},
	{"ucuh", -1, 126, ""},
	{"a\u0107uh", -1, 14, ""},
	{"e\u0107uh", -1, 15, ""},
	{"u\u0107uh", -1, 16, ""},
	{"aci", -1, 124, ""},
	{"aceci", -1, 124, ""},
	{"ieci", -1, 162, ""},
	{"ajuci", -1, 161, ""},
	{"irajuci", 927, 155, ""},
	{"urajuci", 927, 156, ""},
	{"astajuci", 927, 138, ""},
	{"istajuci", 927, 139, ""},
	{"ostajuci", 927, 140, ""},
	{"avajuci", 927, 144, ""},
	{"evajuci", 927, 145, ""},
	{"ivajuci", 927, 146, ""},
	{"uvajuci", 927, 147, ""},
	{"ujuci", -1, 157, ""},
	{"lucujuci", 937, 121, ""},
	{"irujuci", 937, 155, ""},
	{"luci", -1, 121, ""},
	{"nuci", -1, 164, ""},
	{"etuci", -1, 153, ""},
	{"astuci", -1, 136, ""},
	{"gi", -1, 20, ""},
	{"ugi", 944, 18, ""},
	{"aji", -1, 109, ""},
	{"caji", 946, 26, ""},
	{"laji", 946, 30, ""},
	{"raji", 946, 31, ""},
	{"\u0107aji", 946, 28, ""},
	{"\u010Daji", 946, 27, ""},
	{"\u0111aji", 946, 29, ""},
	{"biji", -1, 32, ""},
	{"ciji", -1, 33, ""},
	{"diji", -1, 34, ""},
	{"fiji", -1, 40, ""},
	{"giji", -1, 39, ""},
	{"anjiji", -1, 84, ""},
	{"enjiji", -1, 85, ""},
	{"snjiji", -1, 122, ""},
	{"\u0161njiji", -1, 86, ""},
	{"kiji", -1, 95, ""},
	{"skiji", 962, 1, ""},
	{"\u0161kiji", 962, 2, ""},
	{"liji", -1, 35, ""},
	{"eliji", 965, 83, ""},
	{"miji", -1, 37, ""},
	{"niji", -1, 13, ""},
	{"ganiji", 968, 9, ""},
	{"maniji", 968, 6, ""},
	{"paniji", 968, 7, ""},
	{"raniji", 968, 8, ""},
	{"taniji", 968, 5, ""},
	{"piji", -1, 41, ""},
	{"riji", -1, 42, ""},
	{"siji", -1, 43, ""},
	{"osiji", 976, 123, ""},
	{"tiji", -1, 44, ""},
	{"atiji", 978, 120, ""},
	{"evitiji", 978, 92, ""},
	{"ovitiji", 978, 93, ""},
	{"astiji", 978, 94, ""},
	{"aviji", -1, 77, ""},
	{"eviji", -1, 78, ""},
	{"iviji", -1, 79, ""},
	{"oviji", -1, 80, ""},
	{"ziji", -1, 45, ""},
	{"o\u0161iji", -1, 91, ""},
	{"\u017Eiji", -1, 38, ""},
	{"anji", -1, 84, ""},
	{"enji", -1, 85, ""},
	{"snji", -1, 122, ""},
	{"\u0161nji", -1, 86, ""},
	{"ki", -1, 95, ""},
	{"ski", 994, 1, ""},
	{"\u0161ki", 994, 2, ""},
	{"ali", -1, 104, ""},
	{"acali", 997, 128, ""},
	{"astajali", 997, 106, ""},
	{"istajali", 997, 107, ""},
	{"ostajali", 997, 108, ""},
	{"ijali", 997, 47, ""},
	{"injali", 997, 114, ""},
	{"nali", 997, 46, ""},
	{"irali", 997, 100, ""},
	{"urali", 997, 105, ""},
	{"tali", 997, 113, ""},
	{"astali", 1007, 110, ""},
	{"istali", 1007, 111, ""},
	{"ostali", 1007, 112, ""},
	{"avali", 997, 97, ""},
	{"evali", 997, 96, ""},
	{"ivali", 997, 98, ""},
	{"ovali", 997, 76, ""},
	{"uvali", 997, 99, ""},
	{"a\u010Dali", 997, 102, ""},
	{"eli", -1, 83, ""},
	{"ili", -1, 116, ""},
	{"acili", 1018, 124, ""},
	{"lucili", 1018, 121, ""},
	{"nili", 1018, 103, ""},
	{"rosili", 1018, 127, ""},
	{"jetili", 1018, 118, ""},
	{"ozili", 1018, 48, ""},
	{"a\u010Dili", 1018, 101, ""},
	{"lu\u010Dili", 1018, 117, ""},
	{"ro\u0161ili", 1018, 90, ""},
	{"oli", -1, 50, ""},
	{"asli", -1, 115, ""},
	{"nuli", -1, 13, ""},
	{"rami", -1, 52, ""},
	{"lemi", -1, 51, ""},
	{"ni", -1, 13, ""},
	{"ani", 1033, 10, ""},
	{"acani", 1034, 128, ""},
	{"urani", 1034, 105, ""},
	{"tani", 1034, 113, ""},
	{"avani", 1034, 97, ""},
	{"evani", 1034, 96, ""},
	{"ivani", 1034, 98, ""},
	{"uvani", 1034, 99, ""},
	{"a\u010Dani", 1034, 102, ""},
	{"aceni", 1033, 124, ""},
	{"luceni", 1033, 121, ""},
	{"a\u010Deni", 1033, 101, ""},
	{"lu\u010Deni", 1033, 117, ""},
	{"ini", 1033, 11, ""},
	{"cini", 1047, 137, ""},
	{"\u010Dini", 1047, 89, ""},
	{"oni", 1033, 12, ""},
	{"ari", -1, 53, ""},
	{"dri", -1, 54, ""},
	{"eri", -1, 55, ""},
	{"ori", -1, 56, ""},
	{"basi", -1, 135, ""},
	{"gasi", -1, 131, ""},
	{"jasi", -1, 129, ""},
	{"kasi", -1, 133, ""},
	{"nasi", -1, 132, ""},
	{"tasi", -1, 130, ""},
	{"vasi", -1, 134, ""},
	{"esi", -1, 152, ""},
	{"isi", -1, 154, ""},
	{"osi", -1, 123, ""},
	{"avsi", -1, 161, ""},
	{"acavsi", 1065, 128, ""},
	{"iravsi", 1065, 155, ""},
	{"tavsi", 1065, 160, ""},
	{"etavsi", 1068, 153, ""},
	{"astavsi", 1068, 141, ""},
	{"istavsi", 1068, 142, ""},
	{"ostavsi", 1068, 143, ""},
	{"ivsi", -1, 162, ""},
	{"nivsi", 1073, 158, ""},
	{"rosivsi", 1073, 127, ""},
	{"nuvsi", -1, 164, ""},
	{"ati", -1, 104, ""},
	{"acati", 1077, 128, ""},
	{"astajati", 1077, 106, ""},
	{"istajati", 1077, 107, ""},
	{"ostajati", 1077, 108, ""},
	{"injati", 1077, 114, ""},
	{"ikati", 1077, 68, ""},
	{"lati", 1077, 69, ""},
	{"irati", 1077, 100, ""},
	{"urati", 1077, 105, ""},
	{"tati", 1077, 113, ""},
	{"astati", 1087, 110, ""},
	{"istati", 1087, 111, ""},
	{"ostati", 1087, 112, ""},
	{"avati", 1077, 97, ""},
	{"evati", 1077, 96, ""},
	{"ivati", 1077, 98, ""},
	{"ovati", 1077, 76, ""},
	{"uvati", 1077, 99, ""},
	{"a\u010Dati", 1077, 102, ""},
	{"eti", -1, 70, ""},
	{"iti", -1, 116, ""},
	{"aciti", 1098, 124, ""},
	{"luciti", 1098, 121, ""},
	{"niti", 1098, 103, ""},
	{"rositi", 1098, 127, ""},
	{"jetiti", 1098, 118, ""},
	{"eviti", 1098, 92, ""},
	{"oviti", 1098, 93, ""},
	{"a\u010Diti", 1098, 101, ""},
	{"lu\u010Diti", 1098, 117, ""},
	{"ro\u0161iti", 1098, 90, ""},
	{"asti", -1, 94, ""},
	{"esti", -1, 71, ""},
	{"isti", -1, 72, ""},
	{"ksti", -1, 73, ""},
	{"osti", -1, 74, ""},
	{"nuti", -1, 13, ""},
	{"avi", -1, 77, ""},
	{"evi", -1, 78, ""},
	{"ajevi", 1116, 109, ""},
	{"cajevi", 1117, 26, ""},
	{"lajevi", 1117, 30, ""},
	{"rajevi", 1117, 31, ""},
	{"\u0107ajevi", 1117, 28, ""},
	{"\u010Dajevi", 1117, 27, ""},
	{"\u0111ajevi", 1117, 29, ""},
	{"ivi", -1, 79, ""},
	{"ovi", -1, 80, ""},
	{"govi", 1125, 20, ""},
	{"ugovi", 1126, 17, ""},
	{"lovi", 1125, 82, ""},
	{"olovi", 1128, 49, ""},
	{"movi", 1125, 81, ""},
	{"onovi", 1125, 12, ""},
	{"ie\u0107i", -1, 116, ""},
	{"a\u010De\u0107i", -1, 101, ""},
	{"aju\u0107i", -1, 104, ""},
	{"iraju\u0107i", 1134, 100, ""},
	{"uraju\u0107i", 1134, 105, ""},
	{"astaju\u0107i", 1134, 106, ""},
	{"istaju\u0107i", 1134, 107, ""},
	{"ostaju\u0107i", 1134, 108, ""},
	{"avaju\u0107i", 1134, 97, ""},
	{"evaju\u0107i", 1134, 96, ""},
	{"ivaju\u0107i", 1134, 98, ""},
	{"uvaju\u0107i", 1134, 99, ""},
	{"uju\u0107i", -1, 25, ""},
	{"iruju\u0107i", 1144, 100, ""},
	{"lu\u010Duju\u0107i", 1144, 117, ""},
	{"nu\u0107i", -1, 13, ""},
	{"etu\u0107i", -1, 70, ""},
	{"astu\u0107i", -1, 115, ""},
	{"a\u010Di", -1, 101, ""},
	{"lu\u010Di", -1, 117, ""},
	{"ba\u0161i", -1, 63, ""},
	{"ga\u0161i", -1, 64, ""},
	{"ja\u0161i", -1, 61, ""},
	{"ka\u0161i", -1, 62, ""},
	{"na\u0161i", -1, 60, ""},
	{"ta\u0161i", -1, 59, ""},
	{"va\u0161i", -1, 65, ""},
	{"e\u0161i", -1, 66, ""},
	{"i\u0161i", -1, 67, ""},
	{"o\u0161i", -1, 91, ""},
	{"av\u0161i", -1, 104, ""},
	{"irav\u0161i", 1162, 100, ""},
	{"tav\u0161i", 1162, 113, ""},
	{"etav\u0161i", 1164, 70, ""},
	{"astav\u0161i", 1164, 110, ""},
	{"istav\u0161i", 1164, 111, ""},
	{"ostav\u0161i", 1164, 112, ""},
	{"a\u010Dav\u0161i", 1162, 102, ""},
	{"iv\u0161i", -1, 116, ""},
	{"niv\u0161i", 1170, 103, ""},
	{"ro\u0161iv\u0161i", 1170, 90, ""},
	{"nuv\u0161i", -1, 13, ""},
	{"aj", -1, 104, ""},
	{"uraj", 1174, 105, ""},
	{"taj", 1174, 113, ""},
	{"avaj", 1174, 97, ""},
	{"evaj", 1174, 96, ""},
	{"ivaj", 1174, 98, ""},
	{"uvaj", 1174, 99, ""},
	{"ij", -1, 116, ""},
	{"acoj", -1, 124, ""},
	{"ecoj", -1, 125, ""},
	{"ucoj", -1, 126, ""},
	{"anjijoj", -1, 84, ""},
	{"enjijoj", -1, 85, ""},
	{"snjijoj", -1, 122, ""},
	{"\u0161njijoj", -1, 86, ""},
	{"kijoj", -1, 95, ""},
	{"skijoj", 1189, 1, ""},
	{"\u0161kijoj", 1189, 2, ""},
	{"elijoj", -1, 83, ""},
	{"nijoj", -1, 13, ""},
	{"osijoj", -1, 123, ""},
	{"evitijoj", -1, 92, ""},
	{"ovitijoj", -1, 93, ""},
	{"astijoj", -1, 94, ""},
	{"avijoj", -1, 77, ""},
	{"evijoj", -1, 78, ""},
	{"ivijoj", -1, 79, ""},
	{"ovijoj", -1, 80, ""},
	{"o\u0161ijoj", -1, 91, ""},
	{"anjoj", -1, 84, ""},
	{"enjoj", -1, 85, ""},
	{"snjoj", -1, 122, ""},
	{"\u0161njoj", -1, 86, ""},
	{"koj", -1, 95, ""},
	{"skoj", 1207, 1, ""},
	{"\u0161koj", 1207, 2, ""},
	{"aloj", -1, 104, ""},
	{"eloj", -1, 83, ""},
	{"noj", -1, 13, ""},
	{"cinoj", 1212, 137, ""},
	{"\u010Dinoj", 1212, 89, ""},
	{"osoj", -1, 123, ""},
	{"atoj", -1, 120, ""},
	{"evitoj", -1, 92, ""},
	{"ovitoj", -1, 93, ""},
	{"astoj", -1, 94, ""},
	{"avoj", -1, 77, ""},
	{"evoj", -1, 78, ""},
	{"ivoj", -1, 79, ""},
	{"ovoj", -1, 80, ""},
	{"a\u0107oj", -1, 14, ""},
	{"e\u0107oj", -1, 15, ""},
	{"u\u0107oj", -1, 16, ""},
	{"o\u0161oj", -1, 91, ""},
	{"lucuj", -1, 121, ""},
	{"iruj", -1, 100, ""},
	{"lu\u010Duj", -1, 117, ""},
	{"al", -1, 104, ""},
	{"iral", 1231, 100, ""},
	{"ural", 1231, 105, ""},
	{"el", -1, 119, ""},
	{"il", -1, 116, ""},
	{"am", -1, 104, ""},
	{"acam", 1236, 128, ""},
	{"iram", 1236, 100, ""},
	{"uram", 1236, 105, ""},
	{"tam", 1236, 113, ""},
	{"avam", 1236, 97, ""},
	{"evam", 1236, 96, ""},
	{"ivam", 1236, 98, ""},
	{"uvam", 1236, 99, ""},
	{"a\u010Dam", 1236, 102, ""},
	{"em", -1, 119, ""},
	{"acem", 1246, 124, ""},
	{"ecem", 1246, 125, ""},
	{"ucem", 1246, 126, ""},
	{"astadem", 1246, 110, ""},
	{"istadem", 1246, 111, ""},
	{"ostadem", 1246, 112, ""},
	{"ajem", 1246, 104, ""},
	{"cajem", 1253, 26, ""},
	{"lajem", 1253, 30, ""},
	{"rajem", 1253, 31, ""},
	{"astajem", 1253, 106, ""},
	{"istajem", 1253, 107, ""},
	{"ostajem", 1253, 108, ""},
	{"\u0107ajem", 1253, 28, ""},
	{"\u010Dajem", 1253, 27, ""},
	{"\u0111ajem", 1253, 29, ""},
	{"ijem", 1246, 116, ""},
	{"anjijem", 1263, 84, ""},
	{"enjijem", 1263, 85, ""},
	{"snjijem", 1263, 123, ""},
	{"\u0161njijem", 1263, 86, ""},
	{"kijem", 1263, 95, ""},
	{"skijem", 1268, 1, ""},
	{"\u0161kijem", 1268, 2, ""},
	{"lijem", 1263, 24, ""},
	{"elijem", 1271, 83, ""},
	{"nijem", 1263, 13, ""},
	{"rarijem", 1263, 21, ""},
	{"sijem", 1263, 23, ""},
	{"osijem", 1275, 123, ""},
	{"atijem", 1263, 120, ""},
	{"evitijem", 1263, 92, ""},
	{"ovitijem", 1263, 93, ""},
	{"otijem", 1263, 22, ""},
	{"astijem", 1263, 94, ""},
	{"avijem", 1263, 77, ""},
	{"evijem", 1263, 78, ""},
	{"ivijem", 1263, 79, ""},
	{"ovijem", 1263, 80, ""},
	{"o\u0161ijem", 1263, 91, ""},
	{"anjem", 1246, 84, ""},
	{"enjem", 1246, 85, ""},
	{"injem", 1246, 114, ""},
	{"snjem", 1246, 122, ""},
	{"\u0161njem", 1246, 86, ""},
	{"ujem", 1246, 25, ""},
	{"lucujem", 1292, 121, ""},
	{"irujem", 1292, 100, ""},
	{"lu\u010Dujem", 1292, 117, ""},
	{"kem", 1246, 95, ""},
	{"skem", 1296, 1, ""},
	{"\u0161kem", 1296, 2, ""},
	{"elem", 1246, 83, ""},
	{"nem", 1246, 13, ""},
	{"anem", 1300, 10, ""},
	{"astanem", 1301, 110, ""},
	{"istanem", 1301, 111, ""},
	{"ostanem", 1301, 112, ""},
	{"enem", 1300, 87, ""},
	{"snem", 1300, 159, ""},
	{"\u0161nem", 1300, 88, ""},
	{"basem", 1246, 135, ""},
	{"gasem", 1246, 131, ""},
	{"jasem", 1246, 129, ""},
	{"kasem", 1246, 133, ""},
	{"nasem", 1246, 132, ""},
	{"tasem", 1246, 130, ""},
	{"vasem", 1246, 134, ""},
	{"esem", 1246, 152, ""},
	{"isem", 1246, 154, ""},
	{"osem", 1246, 123, ""},
	{"atem", 1246, 120, ""},
	{"etem", 1246, 70, ""},
	{"evitem", 1246, 92, ""},
	{"ovitem", 1246, 93, ""},
	{"astem", 1246, 94, ""},
	{"istem", 1246, 151, ""},
	{"i\u0161tem", 1246, 75, ""},
	{"avem", 1246, 77, ""},
	{"evem", 1246, 78, ""},
	{"ivem", 1246, 79, ""},
	{"a\u0107em", 1246, 14, ""},
	{"e\u0107em", 1246, 15, ""},
	{"u\u0107em", 1246, 16, ""},
	{"ba\u0161em", 1246, 63, ""},
	{"ga\u0161em", 1246, 64, ""},
	{"ja\u0161em", 1246, 61, ""},
	{"ka\u0161em", 1246, 62, ""},
	{"na\u0161em", 1246, 60, ""},
	{"ta\u0161em", 1246, 59, ""},
	{"va\u0161em", 1246, 65, ""},
	{"e\u0161em", 1246, 66, ""},
	{"i\u0161em", 1246, 67, ""},
	{"o\u0161em", 1246, 91, ""},
	{"im", -1, 116, ""},
	{"acim", 1341, 124, ""},
	{"ecim", 1341, 125, ""},
	{"ucim", 1341, 126, ""},
	{"lucim", 1344, 121, ""},
	{"anjijim", 1341, 84, ""},
	{"enjijim", 1341, 85, ""},
	{"snjijim", 1341, 122, ""},
	{"\u0161njijim", 1341, 86, ""},
	{"kijim", 1341, 95, ""},
	{"skijim", 1350, 1, ""},
	{"\u0161kijim", 1350, 2, ""},
	{"elijim", 1341, 83, ""},
	{"nijim", 1341, 13, ""},
	{"osijim", 1341, 123, ""},
	{"atijim", 1341, 120, ""},
	{"evitijim", 1341, 92, ""},
	{"ovitijim", 1341, 93, ""},
	{"astijim", 1341, 94, ""},
	{"avijim", 1341, 77, ""},
	{"evijim", 1341, 78, ""},
	{"ivijim", 1341, 79, ""},
	{"ovijim", 1341, 80, ""},
	{"o\u0161ijim", 1341, 91, ""},
	{"anjim", 1341, 84, ""},
	{"enjim", 1341, 85, ""},
	{"snjim", 1341, 122, ""},
	{"\u0161njim", 1341, 86, ""},
	{"kim", 1341, 95, ""},
	{"skim", 1369, 1, ""},
	{"\u0161kim", 1369, 2, ""},
	{"elim", 1341, 83, ""},
	{"nim", 1341, 13, ""},
	{"cinim", 1373, 137, ""},
	{"\u010Dinim", 1373, 89, ""},
	{"osim", 1341, 123, ""},
	{"rosim", 1376, 127, ""},
	{"atim", 1341, 120, ""},
	{"jetim", 1341, 118, ""},
	{"evitim", 1341, 92, ""},
	{"ovitim", 1341, 93, ""},
	{"astim", 1341, 94, ""},
	{"avim", 1341, 77, ""},
	{"evim", 1341, 78, ""},
	{"ivim", 1341, 79, ""},
	{"ovim", 1341, 80, ""},
	{"a\u0107im", 1341, 14, ""},
	{"e\u0107im", 1341, 15, ""},
	{"u\u0107im", 1341, 16, ""},
	{"a\u010Dim", 1341, 101, ""},
	{"lu\u010Dim", 1341, 117, ""},
	{"o\u0161im", 1341, 91, ""},
	{"ro\u0161im", 1392, 90, ""},
	{"acom", -1, 124, ""},
	{"ecom", -1, 125, ""},
	{"ucom", -1, 126, ""},
	{"gom", -1, 20, ""},
	{"logom", 1397, 19, ""},
	{"ugom", 1397, 18, ""},
	{"bijom", -1, 32, ""},
	{"cijom", -1, 33, ""},
	{"dijom", -1, 34, ""},
	{"fijom", -1, 40, ""},
	{"gijom", -1, 39, ""},
	{"lijom", -1, 35, ""},
	{"mijom", -1, 37, ""},
	{"nijom", -1, 36, ""},
	{"ganijom", 1407, 9, ""},
	{"manijom", 1407, 6, ""},
	{"panijom", 1407, 7, ""},
	{"ranijom", 1407, 8, ""},
	{"tanijom", 1407, 5, ""},
	{"pijom", -1, 41, ""},
	{"rijom", -1, 42, ""},
	{"sijom", -1, 43, ""},
	{"tijom", -1, 44, ""},
	{"zijom", -1, 45, ""},
	{"\u017Eijom", -1, 38, ""},
	{"anjom", -1, 84, ""},
	{"enjom", -1, 85, ""},
	{"snjom", -1, 122, ""},
	{"\u0161njom", -1, 86, ""},
	{"kom", -1, 95, ""},
	{"skom", 1423, 1, ""},
	{"\u0161kom", 1423, 2, ""},
	{"alom", -1, 104, ""},
	{"ijalom", 1426, 47, ""},
	{"nalom", 1426, 46, ""},
	{"elom", -1, 83, ""},
	{"ilom", -1, 116, ""},
	{"ozilom", 1430, 48, ""},
	{"olom", -1, 50, ""},
	{"ramom", -1, 52, ""},
	{"lemom", -1, 51, ""},
	{"nom", -1, 13, ""},
	{"anom", 1435, 10, ""},
	{"inom", 1435, 11, ""},
	{"cinom", 1437, 137, ""},
	{"aninom", 1437, 10, ""},
	{"\u010Dinom", 1437, 89, ""},
	{"onom", 1435, 12, ""},
	{"arom", -1, 53, ""},
	{"drom", -1, 54, ""},
	{"erom", -1, 55, ""},
	{"orom", -1, 56, ""},
	{"basom", -1, 135, ""},
	{"gasom", -1, 131, ""},
	{"jasom", -1, 129, ""},
	{"kasom", -1, 133, ""},
	{"nasom", -1, 132, ""},
	{"tasom", -1, 130, ""},
	{"vasom", -1, 134, ""},
	{"esom", -1, 57, ""},
	{"isom", -1, 58, ""},
	{"osom", -1, 123, ""},
	{"atom", -1, 120, ""},
	{"ikatom", 1456, 68, ""},
	{"latom", 1456, 69, ""},
	{"etom", -1, 70, ""},
	{"evitom", -1, 92, ""},
	{"ovitom", -1, 93, ""},
	{"astom", -1, 94, ""},
	{"estom", -1, 71, ""},
	{"istom", -1, 72, ""},
	{"kstom", -1, 73, ""},
	{"ostom", -1, 74, ""},
	{"avom", -1, 77, ""},
	{"evom", -1, 78, ""},
	{"ivom", -1, 79, ""},
	{"ovom", -1, 80, ""},
	{"lovom", 1470, 82, ""},
	{"movom", 1470, 81, ""},
	{"stvom", -1, 3, ""},
	{"\u0161tvom", -1, 4, ""},
	{"a\u0107om", -1, 14, ""},
	{"e\u0107om", -1, 15, ""},
	{"u\u0107om", -1, 16, ""},
	{"ba\u0161om", -1, 63, ""},
	{"ga\u0161om", -1, 64, ""},
	{"ja\u0161om", -1, 61, ""},
	{"ka\u0161om", -1, 62, ""},
	{"na\u0161om", -1, 60, ""},
	{"ta\u0161om", -1, 59, ""},
	{"va\u0161om", -1, 65, ""},
	{"e\u0161om", -1, 66, ""},
	{"i\u0161om", -1, 67, ""},
	{"o\u0161om", -1, 91, ""},
	{"an", -1, 104, ""},
	{"acan", 1488, 128, ""},
	{"iran", 1488, 100, ""},
	{"uran", 1488, 105, ""},
	{"tan", 1488, 113, ""},
	{"avan", 1488, 97, ""},
	{"evan", 1488, 96, ""},
	{"ivan", 1488, 98, ""},
	{"uvan", 1488, 99, ""},
	{"a\u010Dan", 1488, 102, ""},
	{"acen", -1, 124, ""},
	{"lucen", -1, 121, ""},
	{"a\u010Den", -1, 101, ""},
	{"lu\u010Den", -1, 117, ""},
	{"anin", -1, 10, ""},
	{"ao", -1, 104, ""},
	{"acao", 1503, 128, ""},
	{"astajao", 1503, 106, ""},
	{"istajao", 1503, 107, ""},
	{"ostajao", 1503, 108, ""},
	{"injao", 1503, 114, ""},
	{"irao", 1503, 100, ""},
	{"urao", 1503, 105, ""},
	{"tao", 1503, 113, ""},
	{"astao", 1511, 110, ""},
	{"istao", 1511, 111, ""},
	{"ostao", 1511, 112, ""},
	{"avao", 1503, 97, ""},
	{"evao", 1503, 96, ""},
	{"ivao", 1503, 98, ""},
	{"ovao", 1503, 76, ""},
	{"uvao", 1503, 99, ""},
	{"a\u010Dao", 1503, 102, ""},
	{"go", -1, 20, ""},
	{"ugo", 1521, 18, ""},
	{"io", -1, 116, ""},
	{"acio", 1523, 124, ""},
	{"lucio", 1523, 121, ""},
	{"lio", 1523, 24, ""},
	{"nio", 1523, 103, ""},
	{"rario", 1523, 21, ""},
	{"sio", 1523, 23, ""},
	{"rosio", 1529, 127, ""},
	{"jetio", 1523, 118, ""},
	{"otio", 1523, 22, ""},
	{"a\u010Dio", 1523, 101, ""},
	{"lu\u010Dio", 1523, 117, ""},
	{"ro\u0161io", 1523, 90, ""},
	{"bijo", -1, 32, ""},
	{"cijo", -1, 33, ""},
	{"dijo", -1, 34, ""},
	{"fijo", -1, 40, ""},
	{"gijo", -1, 39, ""},
	{"lijo", -1, 35, ""},
	{"mijo", -1, 37, ""},
	{"nijo", -1, 36, ""},
	{"pijo", -1, 41, ""},
	{"rijo", -1, 42, ""},
	{"sijo", -1, 43, ""},
	{"tijo", -1, 44, ""},
	{"zijo", -1, 45, ""},
	{"\u017Eijo", -1, 38, ""},
	{"anjo", -1, 84, ""},
	{"enjo", -1, 85, ""},
	{"snjo", -1, 122, ""},
	{"\u0161njo", -1, 86, ""},
	{"ko", -1, 95, ""},
	{"sko", 1554, 1, ""},
	{"\u0161ko", 1554, 2, ""},
	{"alo", -1, 104, ""},
	{"acalo", 1557, 128, ""},
	{"astajalo", 1557, 106, ""},
	{"istajalo", 1557, 107, ""},
	{"ostajalo", 1557, 108, ""},
	{"ijalo", 1557, 47, ""},
	{"injalo", 1557, 114, ""},
	{"nalo", 1557, 46, ""},
	{"iralo", 1557, 100, ""},
	{"uralo", 1557, 105, ""},
	{"talo", 1557, 113, ""},
	{"astalo", 1567, 110, ""},
	{"istalo", 1567, 111, ""},
	{"ostalo", 1567, 112, ""},
	{"avalo", 1557, 97, ""},
	{"evalo", 1557, 96, ""},
	{"ivalo", 1557, 98, ""},
	{"ovalo", 1557, 76, ""},
	{"uvalo", 1557, 99, ""},
	{"a\u010Dalo", 1557, 102, ""},
	{"elo", -1, 83, ""},
	{"ilo", -1, 116, ""},
	{"acilo", 1578, 124, ""},
	{"lucilo", 1578, 121, ""},
	{"nilo", 1578, 103, ""},
	{"rosilo", 1578, 127, ""},
	{"jetilo", 1578, 118, ""},
	{"a\u010Dilo", 1578, 101, ""},
	{"lu\u010Dilo", 1578, 117, ""},
	{"ro\u0161ilo", 1578, 90, ""},
	{"aslo", -1, 115, ""},
	{"nulo", -1, 13, ""},
	{"amo", -1, 104, ""},
	{"acamo", 1589, 128, ""},
	{"ramo", 1589, 52, ""},
	{"iramo", 1591, 100, ""},
	{"uramo", 1591, 105, ""},
	{"tamo", 1589, 113, ""},
	{"avamo", 1589, 97, ""},
	{"evamo", 1589, 96, ""},
	{"ivamo", 1589, 98, ""},
	{"uvamo", 1589, 99, ""},
	{"a\u010Damo", 1589, 102, ""},
	{"emo", -1, 119, ""},
	{"astademo", 1600, 110, ""},
	{"istademo", 1600, 111, ""},
	{"ostademo", 1600, 112, ""},
	{"astajemo", 1600, 106, ""},
	{"istajemo", 1600, 107, ""},
	{"ostajemo", 1600, 108, ""},
	{"ijemo", 1600, 116, ""},
	{"injemo", 1600, 114, ""},
	{"ujemo", 1600, 25, ""},
	{"lucujemo", 1609, 121, ""},
	{"irujemo", 1609, 100, ""},
	{"lu\u010Dujemo", 1609, 117, ""},
	{"lemo", 1600, 51, ""},
	{"nemo", 1600, 13, ""},
	{"astanemo", 1614, 110, ""},
	{"istanemo", 1614, 111, ""},
	{"ostanemo", 1614, 112, ""},
	{"etemo", 1600, 70, ""},
	{"astemo", 1600, 115, ""},
	{"imo", -1, 116, ""},
	{"acimo", 1620, 124, ""},
	{"lucimo", 1620, 121, ""},
	{"nimo", 1620, 13, ""},
	{"astanimo", 1623, 110, ""},
	{"istanimo", 1623, 111, ""},
	{"ostanimo", 1623, 112, ""},
	{"rosimo", 1620, 127, ""},
	{"etimo", 1620, 70, ""},
	{"jetimo", 1628, 118, ""},
	{"astimo", 1620, 115, ""},
	{"a\u010Dimo", 1620, 101, ""},
	{"lu\u010Dimo", 1620, 117, ""},
	{"ro\u0161imo", 1620, 90, ""},
	{"ajmo", -1, 104, ""},
	{"urajmo", 1634, 105, ""},
	{"tajmo", 1634, 113, ""},
	{"astajmo", 1636, 106, ""},
	{"istajmo", 1636, 107, ""},
	{"ostajmo", 1636, 108, ""},
	{"avajmo", 1634, 97, ""},
	{"evajmo", 1634, 96, ""},
	{"ivajmo", 1634, 98, ""},
	{"uvajmo", 1634, 99, ""},
	{"ijmo", -1, 116, ""},
	{"ujmo", -1, 25, ""},
	{"lucujmo", 1645, 121, ""},
	{"irujmo", 1645, 100, ""},
	{"lu\u010Dujmo", 1645, 117, ""},
	{"asmo", -1, 104, ""},
	{"acasmo", 1649, 128, ""},
	{"astajasmo", 1649, 106, ""},
	{"istajasmo", 1649, 107, ""},
	{"ostajasmo", 1649, 108, ""},
	{"injasmo", 1649, 114, ""},
	{"irasmo", 1649, 100, ""},
	{"urasmo", 1649, 105, ""},
	{"tasmo", 1649, 113, ""},
	{"avasmo", 1649, 97, ""},
	{"evasmo", 1649, 96, ""},
	{"ivasmo", 1649, 98, ""},
	{"ovasmo", 1649, 76, ""},
	{"uvasmo", 1649, 99, ""},
	{"a\u010Dasmo", 1649, 102, ""},
	{"ismo", -1, 116, ""},
	{"acismo", 1664, 124, ""},
	{"lucismo", 1664, 121, ""},
	{"nismo", 1664, 103, ""},
	{"rosismo", 1664, 127, ""},
	{"jetismo", 1664, 118, ""},
	{"a\u010Dismo", 1664, 101, ""},
	{"lu\u010Dismo", 1664, 117, ""},
	{"ro\u0161ismo", 1664, 90, ""},
	{"astadosmo", -1, 110, ""},
	{"istadosmo", -1, 111, ""},
	{"ostadosmo", -1, 112, ""},
	{"nusmo", -1, 13, ""},
	{"no", -1, 13, ""},
	{"ano", 1677, 104, ""},
	{"acano", 1678, 128, ""},
	{"urano", 1678, 105, ""},
	{"tano", 1678, 113, ""},
	{"avano", 1678, 97, ""},
	{"evano", 1678, 96, ""},
	{"ivano", 1678, 98, ""},
	{"uvano", 1678, 99, ""},
	{"a\u010Dano", 1678, 102, ""},
	{"aceno", 1677, 124, ""},
	{"luceno", 1677, 121, ""},
	{"a\u010Deno", 1677, 101, ""},
	{"lu\u010Deno", 1677, 117, ""},
	{"ino", 1677, 11, ""},
	{"cino", 1691, 137, ""},
	{"\u010Dino", 1691, 89, ""},
	{"ato", -1, 120, ""},
	{"ikato", 1694, 68, ""},
	{"lato", 1694, 69, ""},
	{"eto", -1, 70, ""},
	{"evito", -1, 92, ""},
	{"ovito", -1, 93, ""},
	{"asto", -1, 94, ""},
	{"esto", -1, 71, ""},
	{"isto", -1, 72, ""},
	{"ksto", -1, 73, ""},
	{"osto", -1, 74, ""},
	{"nuto", -1, 13, ""},
	{"nuo", -1, 13, ""},
	{"avo", -1, 77, ""},
	{"evo", -1, 78, ""},
	{"ivo", -1, 79, ""},
	{"ovo", -1, 80, ""},
	{"stvo", -1, 3, ""},
	{"\u0161tvo", -1, 4, ""},
	{"as", -1, 161, ""},
	{"acas", 1713, 128, ""},
	{"iras", 1713, 155, ""},
	{"uras", 1713, 156, ""},
	{"tas", 1713, 160, ""},
	{"avas", 1713, 144, ""},
	{"evas", 1713, 145, ""},
	{"ivas", 1713, 146, ""},
	{"uvas", 1713, 147, ""},
	{"es", -1, 163, ""},
	{"astades", 1722, 141, ""},
	{"istades", 1722, 142, ""},
	{"ostades", 1722, 143, ""},
	{"astajes", 1722, 138, ""},
	{"istajes", 1722, 139, ""},
	{"ostajes", 1722, 140, ""},
	{"ijes", 1722, 162, ""},
	{"injes", 1722, 150, ""},
	{"ujes", 1722, 157, ""},
	{"lucujes", 1731, 121, ""},
	{"irujes", 1731, 155, ""},
	{"nes", 1722, 164, ""},
	{"astanes", 1734, 141, ""},
	{"istanes", 1734, 142, ""},
	{"ostanes", 1734, 143, ""},
	{"etes", 1722, 153, ""},
	{"astes", 1722, 136, ""},
	{"is", -1, 162, ""},
	{"acis", 1740, 124, ""},
	{"lucis", 1740, 121, ""},
	{"nis", 1740, 158, ""},
	{"rosis", 1740, 127, ""},
	{"jetis", 1740, 149, ""},
	{"at", -1, 104, ""},
	{"acat", 1746, 128, ""},
	{"astajat", 1746, 106, ""},
	{"istajat", 1746, 107, ""},
	{"ostajat", 1746, 108, ""},
	{"injat", 1746, 114, ""},
	{"irat", 1746, 100, ""},
	{"urat", 1746, 105, ""},
	{"tat", 1746, 113, ""},
	{"astat", 1754, 110, ""},
	{"istat", 1754, 111, ""},
	{"ostat", 1754, 112, ""},
	{"avat", 1746, 97, ""},
	{"evat", 1746, 96, ""},
	{"ivat", 1746, 98, ""},
	{"irivat", 1760, 100, ""},
	{"ovat", 1746, 76, ""},
	{"uvat", 1746, 99, ""},
	{"a\u010Dat", 1746, 102, ""},
	{"it", -1, 116, ""},
	{"acit", 1765, 124, ""},
	{"lucit", 1765, 121, ""},
	{"rosit", 1765, 127, ""},
	{"jetit", 1765, 118, ""},
	{"a\u010Dit", 1765, 101, ""},
	{"lu\u010Dit", 1765, 117, ""},
	{"ro\u0161it", 1765, 90, ""},
	{"nut", -1, 13, ""},
	{"astadu", -1, 110, ""},
	{"istadu", -1, 111, ""},
	{"ostadu", -1, 112, ""},
	{"gu", -1, 20, ""},
	{"logu", 1777, 19, ""},
	{"ugu", 1777, 18, ""},
	{"ahu", -1, 104, ""},
	{"acahu", 1780, 128, ""},
	{"astajahu", 1780, 106, ""},
	{"istajahu", 1780, 107, ""},
	{"ostajahu", 1780, 108, ""},
	{"injahu", 1780, 114, ""},
	{"irahu", 1780, 100, ""},
	{"urahu", 1780, 105, ""},
	{"avahu", 1780, 97, ""},
	{"evahu", 1780, 96, ""},
	{"ivahu", 1780, 98, ""},
	{"ovahu", 1780, 76, ""},
	{"uvahu", 1780, 99, ""},
	{"a\u010Dahu", 1780, 102, ""},
	{"aju", -1, 104, ""},
	{"caju", 1794, 26, ""},
	{"acaju", 1795, 128, ""},
	{"laju", 1794, 30, ""},
	{"raju", 1794, 31, ""},
	{"iraju", 1798, 100, ""},
	{"uraju", 1798, 105, ""},
	{"taju", 1794, 113, ""},
	{"astaju", 1801, 106, ""},
	{"istaju", 1801, 107, ""},
	{"ostaju", 1801, 108, ""},
	{"avaju", 1794, 97, ""},
	{"evaju", 1794, 96, ""},
	{"ivaju", 1794, 98, ""},
	{"uvaju", 1794, 99, ""},
	{"\u0107aju", 1794, 28, ""},
	{"\u010Daju", 1794, 27, ""},
	{"a\u010Daju", 1810, 102, ""},
	{"\u0111aju", 1794, 29, ""},
	{"iju", -1, 116, ""},
	{"biju", 1813, 32, ""},
	{"ciju", 1813, 33, ""},
	{"diju", 1813, 34, ""},
	{"fiju", 1813, 40, ""},
	{"giju", 1813, 39, ""},
	{"anjiju", 1813, 84, ""},
	{"enjiju", 1813, 85, ""},
	{"snjiju", 1813, 122, ""},
	{"\u0161njiju", 1813, 86, ""},
	{"kiju", 1813, 95, ""},
	{"liju", 1813, 24, ""},
	{"eliju", 1824, 83, ""},
	{"miju", 1813, 37, ""},
	{"niju", 1813, 13, ""},
	{"ganiju", 1827, 9, ""},
	{"maniju", 1827, 6, ""},
	{"paniju", 1827, 7, ""},
	{"raniju", 1827, 8, ""},
	{"taniju", 1827, 5, ""},
	{"piju", 1813, 41, ""},
	{"riju", 1813, 42, ""},
	{"rariju", 1834, 21, ""},
	{"siju", 1813, 23, ""},
	{"osiju", 1836, 123, ""},
	{"tiju", 1813, 44, ""},
	{"atiju", 1838, 120, ""},
	{"otiju", 1838, 22, ""},
	{"aviju", 1813, 77, ""},
	{"eviju", 1813, 78, ""},
	{"iviju", 1813, 79, ""},
	{"oviju", 1813, 80, ""},
	{"ziju", 1813, 45, ""},
	{"o\u0161iju", 1813, 91, ""},
	{"\u017Eiju", 1813, 38, ""},
	{"anju", -1, 84, ""},
	{"enju", -1, 85, ""},
	{"snju", -1, 122, ""},
	{"\u0161nju", -1, 86, ""},
	{"uju", -1, 25, ""},
	{"lucuju", 1852, 121, ""},
	{"iruju", 1852, 100, ""},
	{"lu\u010Duju", 1852, 117, ""},
	{"ku", -1, 95, ""},
	{"sku", 1856, 1, ""},
	{"\u0161ku", 1856, 2, ""},
	{"alu", -1, 104, ""},
	{"ijalu", 1859, 47, ""},
	{"nalu", 1859, 46, ""},
	{"elu", -1, 83, ""},
	{"ilu", -1, 116, ""},
	{"ozilu", 1863, 48, ""},
	{"olu", -1, 50, ""},
	{"ramu", -1, 52, ""},
	{"acemu", -1, 124, ""},
	{"ecemu", -1, 125, ""},
	{"ucemu", -1, 126, ""},
	{"anjijemu", -1, 84, ""},
	{"enjijemu", -1, 85, ""},
	{"snjijemu", -1, 122, ""},
	{"\u0161njijemu", -1, 86, ""},
	{"kijemu", -1, 95, ""},
	{"skijemu", 1874, 1, ""},
	{"\u0161kijemu", 1874, 2, ""},
	{"elijemu", -1, 83, ""},
	{"nijemu", -1, 13, ""},
	{"osijemu", -1, 123, ""},
	{"atijemu", -1, 120, ""},
	{"evitijemu", -1, 92, ""},
	{"ovitijemu", -1, 93, ""},
	{"astijemu", -1, 94, ""},
	{"avijemu", -1, 77, ""},
	{"evijemu", -1, 78, ""},
	{"ivijemu", -1, 79, ""},
	{"ovijemu", -1, 80, ""},
	{"o\u0161ijemu", -1, 91, ""},
	{"anjemu", -1, 84, ""},
	{"enjemu", -1, 85, ""},
	{"snjemu", -1, 122, ""},
	{"\u0161njemu", -1, 86, ""},
	{"kemu", -1, 95, ""},
	{"skemu", 1893, 1, ""},
	{"\u0161kemu", 1893, 2, ""},
	{"lemu", -1, 51, ""},
	{"elemu", 1896, 83, ""},
	{"nemu", -1, 13, ""},
	{"anemu", 1898, 10, ""},
	{"enemu", 1898, 87, ""},
	{"snemu", 1898, 159, ""},
	{"\u0161nemu", 1898, 88, ""},
	{"osemu", -1, 123, ""},
	{"atemu", -1, 120, ""},
	{"evitemu", -1, 92, ""},
	{"ovitemu", -1, 93, ""},
	{"astemu", -1, 94, ""},
	{"avemu", -1, 77, ""},
	{"evemu", -1, 78, ""},
	{"ivemu", -1, 79, ""},
	{"ovemu", -1, 80, ""},
	{"a\u0107emu", -1, 14, ""},
	{"e\u0107emu", -1, 15, ""},
	{"u\u0107emu", -1, 16, ""},
	{"o\u0161emu", -1, 91, ""},
	{"acomu", -1, 124, ""},
	{"ecomu", -1, 125, ""},
	{"ucomu", -1, 126, ""},
	{"anjomu", -1, 84, ""},
	{"enjomu", -1, 85, ""},
	{"snjomu", -1, 122, ""},
	{"\u0161njomu", -1, 86, ""},
	{"komu", -1, 95, ""},
	{"skomu", 1923, 1, ""},
	{"\u0161komu", 1923, 2, ""},
	{"elomu", -1, 83, ""},
	{"nomu", -1, 13, ""},
	{"cinomu", 1927, 137, ""},
	{"\u010Dinomu", 1927, 89, ""},
	{"osomu", -1, 123, ""},
	{"atomu", -1, 120, ""},
	{"evitomu", -1, 92, ""},
	{"ovitomu", -1, 93, ""},
	{"astomu", -1, 94, ""},
	{"avomu", -1, 77, ""},
	{"evomu", -1, 78, ""},
	{"ivomu", -1, 79, ""},
	{"ovomu", -1, 80, ""},
	{"a\u0107omu", -1, 14, ""},
	{"e\u0107omu", -1, 15, ""},
	{"u\u0107omu", -1, 16, ""},
	{"o\u0161omu", -1, 91, ""},
	{"nu", -1, 13, ""},
	{"anu", 1943, 10, ""},
	{"astanu", 1944, 110, ""},
	{"istanu", 1944, 111, ""},
	{"ostanu", 1944, 112, ""},
	{"inu", 1943, 11, ""},
	{"cinu", 1948, 137, ""},
	{"aninu", 1948, 10, ""},
	{"\u010Dinu", 1948, 89, ""},
	{"onu", 1943, 12, ""},
	{"aru", -1, 53, ""},
	{"dru", -1, 54, ""},
	{"eru", -1, 55, ""},
	{"oru", -1, 56, ""},
	{"basu", -1, 135, ""},
	{"gasu", -1, 131, ""},
	{"jasu", -1, 129, ""},
	{"kasu", -1, 133, ""},
	{"nasu", -1, 132, ""},
	{"tasu", -1, 130, ""},
	{"vasu", -1, 134, ""},
	{"esu", -1, 57, ""},
	{"isu", -1, 58, ""},
	{"osu", -1, 123, ""},
	{"atu", -1, 120, ""},
	{"ikatu", 1967, 68, ""},
	{"latu", 1967, 69, ""},
	{"etu", -1, 70, ""},
	{"evitu", -1, 92, ""},
	{"ovitu", -1, 93, ""},
	{"astu", -1, 94, ""},
	{"estu", -1, 71, ""},
	{"istu", -1, 72, ""},
	{"kstu", -1, 73, ""},
	{"ostu", -1, 74, ""},
	{"i\u0161tu", -1, 75, ""},
	{"avu", -1, 77, ""},
	{"evu", -1, 78, ""},
	{"ivu", -1, 79, ""},
	{"ovu", -1, 80, ""},
	{"lovu", 1982, 82, ""},
	{"movu", 1982, 81, ""},
	{"stvu", -1, 3, ""},
	{"\u0161tvu", -1, 4, ""},
	{"ba\u0161u", -1, 63, ""},
	{"ga\u0161u", -1, 64, ""},
	{"ja\u0161u", -1, 61, ""},
	{"ka\u0161u", -1, 62, ""},
	{"na\u0161u", -1, 60, ""},
	{"ta\u0161u", -1, 59, ""},
	{"va\u0161u", -1, 65, ""},
	{"e\u0161u", -1, 66, ""},
	{"i\u0161u", -1, 67, ""},
	{"o\u0161u", -1, 91, ""},
	{"avav", -1, 97, ""},
	{"evav", -1, 96, ""},
	{"ivav", -1, 98, ""},
	{"uvav", -1, 99, ""},
	{"kov", -1, 95, ""},
	{"a\u0161", -1, 104, ""},
	{"ira\u0161", 2002, 100, ""},
	{"ura\u0161", 2002, 105, ""},
	{"ta\u0161", 2002, 113, ""},
	{"ava\u0161", 2002, 97, ""},
	{"eva\u0161", 2002, 96, ""},
	{"iva\u0161", 2002, 98, ""},
	{"uva\u0161", 2002, 99, ""},
	{"a\u010Da\u0161", 2002, 102, ""},
	{"e\u0161", -1, 119, ""},
	{"astade\u0161", 2011, 110, ""},
	{"istade\u0161", 2011, 111, ""},
	{"ostade\u0161", 2011, 112, ""},
	{"astaje\u0161", 2011, 106, ""},
	{"istaje\u0161", 2011, 107, ""},
	{"ostaje\u0161", 2011, 108, ""},
	{"ije\u0161", 2011, 116, ""},
	{"inje\u0161", 2011, 114, ""},
	{"uje\u0161", 2011, 25, ""},
	{"iruje\u0161", 2020, 100, ""},
	{"lu\u010Duje\u0161", 2020, 117, ""},
	{"ne\u0161", 2011, 13, ""},
	{"astane\u0161", 2023, 110, ""},
	{"istane\u0161", 2023, 111, ""},
	{"ostane\u0161", 2023, 112, ""},
	{"ete\u0161", 2011, 70, ""},
	{"aste\u0161", 2011, 115, ""},
	{"i\u0161", -1, 116, ""},
	{"ni\u0161", 2029, 103, ""},
	{"jeti\u0161", 2029, 118, ""},
	{"a\u010Di\u0161", 2029, 101, ""},
	{"lu\u010Di\u0161", 2029, 117, ""},
	{"ro\u0161i\u0161", 2029, 90, ""},
}
var ser_a_3 = []among{
	{"a", -1, 1, ""},
	{"oga", 0, 1, ""},
	{"ama", 0, 1, ""},
	{"ima", 0, 1, ""},
	{"ena", 0, 1, ""},
	{"e", -1, 1, ""},
	{"og", -1, 1, ""},
	{"anog", 6, 1, ""},
	{"enog", 6, 1, ""},
	{"anih", -1, 1, ""},
	{"enih", -1, 1, ""},
	{"i", -1, 1, ""},
	{"ani", 11, 1, ""},
	{"eni", 11, 1, ""},
	{"anoj", -1, 1, ""},
	{"enoj", -1, 1, ""},
	{"anim", -1, 1, ""},
	{"enim", -1, 1, ""},
	{"om", -1, 1, ""},
	{"enom", 18, 1, ""},
	{"o", -1, 1, ""},
	{"ano", 20, 1, ""},
	{"eno", 20, 1, ""},
	{"ost", -1, 1, ""},
	{"u", -1, 1, ""},
	{"enu", 24, 1, ""},
}
var ser_g_v = []byte{17, 65, 16}
var ser_g_sa = []byte{65, 4, 0, 0, 0, 0, 0, 0, 0, 0, 0, 4, 0, 0, 128}
var ser_g_ca = []byte{119, 95, 23, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 136, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 0, 0, 0, 16}
var ser_g_rg = []byte{1}

func (z *serbianStemmer) r_cyr_to_lat() bool {
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
						among_var = z.findAmong(ser_a_0)
						if among_var == 0 {
							goto lab3_end
						}
						z.ket = z.cursor
						switch among_var {
						case 1:
							z.sliceFrom("a")
							break
						case 2:
							z.sliceFrom("b")
							break
						case 3:
							z.sliceFrom("v")
							break
						case 4:
							z.sliceFrom("g")
							break
						case 5:
							z.sliceFrom("d")
							break
						case 6:
							z.sliceFrom("\u0111")
							break
						case 7:
							z.sliceFrom("e")
							break
						case 8:
							z.sliceFrom("\u017E")
							break
						case 9:
							z.sliceFrom("z")
							break
						case 10:
							z.sliceFrom("i")
							break
						case 11:
							z.sliceFrom("j")
							break
						case 12:
							z.sliceFrom("k")
							break
						case 13:
							z.sliceFrom("l")
							break
						case 14:
							z.sliceFrom("lj")
							break
						case 15:
							z.sliceFrom("m")
							break
						case 16:
							z.sliceFrom("n")
							break
						case 17:
							z.sliceFrom("nj")
							break
						case 18:
							z.sliceFrom("o")
							break
						case 19:
							z.sliceFrom("p")
							break
						case 20:
							z.sliceFrom("r")
							break
						case 21:
							z.sliceFrom("s")
							break
						case 22:
							z.sliceFrom("t")
							break
						case 23:
							z.sliceFrom("\u0107")
							break
						case 24:
							z.sliceFrom("u")
							break
						case 25:
							z.sliceFrom("f")
							break
						case 26:
							z.sliceFrom("h")
							break
						case 27:
							z.sliceFrom("c")
							break
						case 28:
							z.sliceFrom("\u010D")
							break
						case 29:
							z.sliceFrom("d\u017E")
							break
						case 30:
							z.sliceFrom("\u0161")
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
	return true
}
func (z *serbianStemmer) r_prelude() bool {
	v_1 := z.cursor
	{
		for {
			v_2 := z.cursor
			{
			golab2:
				for {
					v_3 := z.cursor
					{
						if !(z.inGrouping(ser_g_ca, 98, 382)) {
							goto lab3_end
						}
						z.bra = z.cursor
						if !(z.eqS("ije")) {
							goto lab3_end
						}
						z.ket = z.cursor
						if !(z.inGrouping(ser_g_ca, 98, 382)) {
							goto lab3_end
						}
						z.sliceFrom("e")
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
	v_4 := z.cursor
	{
		for {
			v_5 := z.cursor
			{
			golab6:
				for {
					v_6 := z.cursor
					{
						if !(z.inGrouping(ser_g_ca, 98, 382)) {
							goto lab7_end
						}
						z.bra = z.cursor
						if !(z.eqS("je")) {
							goto lab7_end
						}
						z.ket = z.cursor
						if !(z.inGrouping(ser_g_ca, 98, 382)) {
							goto lab7_end
						}
						z.sliceFrom("e")
						z.cursor = v_6
						break golab6
					}
				lab7_end:
					;
					z.cursor = v_6
					if z.cursor >= z.limit {
						goto lab5_end
					}
					z.cursor++
				}
				continue
			}
		lab5_end:
			;
			z.cursor = v_5
			break
		}
	}
	z.cursor = v_4
	v_7 := z.cursor
	{
		for {
			v_8 := z.cursor
			{
			golab10:
				for {
					v_9 := z.cursor
					{
						z.bra = z.cursor
						if !(z.eqS("dj")) {
							goto lab11_end
						}
						z.ket = z.cursor
						z.sliceFrom("\u0111")
						z.cursor = v_9
						break golab10
					}
				lab11_end:
					;
					z.cursor = v_9
					if z.cursor >= z.limit {
						goto lab9_end
					}
					z.cursor++
				}
				continue
			}
		lab9_end:
			;
			z.cursor = v_8
			break
		}
	}
	z.cursor = v_7
	return true
}
func (z *serbianStemmer) r_mark_regions() bool {
	z.B_no_diacritics = true
	v_1 := z.cursor
	{
		if !z.goOutGrouping(ser_g_sa, 263, 382) {
			goto lab0_end
		}
		z.cursor++
		z.B_no_diacritics = false
	}
lab0_end:
	;
	z.cursor = v_1
	z.I_p1 = z.limit
	v_2 := z.cursor
	{
		if !z.goOutGrouping(ser_g_v, 97, 117) {
			goto lab1_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if z.I_p1 >= 2 {
			goto lab1_end
		}
		if !z.goInGrouping(ser_g_v, 97, 117) {
			goto lab1_end
		}
		z.cursor++
		z.I_p1 = z.cursor
	}
lab1_end:
	;
	z.cursor = v_2
	v_3 := z.cursor
	{
	golab3:
		for {
			{
				if !(z.eqS("r")) {
					goto lab4_end
				}
				break golab3
			}
		lab4_end:
			;
			if z.cursor >= z.limit {
				goto lab2_end
			}
			z.cursor++
		}
		{
			v_4 := z.cursor
			{
				if z.cursor < 2 {
					goto lab6_end
				}
				goto lab5_end
			}
		lab6_end:
			;
			z.cursor = v_4
			if !z.goInGrouping(ser_g_rg, 114, 114) {
				goto lab2_end
			}
			z.cursor++
		}
	lab5_end:
		;
		if (z.I_p1 - z.cursor) <= 1 {
			goto lab2_end
		}
		z.I_p1 = z.cursor
	}
lab2_end:
	;
	z.cursor = v_3
	return true
}
func (z *serbianStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *serbianStemmer) r_Step_1() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ser_a_1)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("loga")
		break
	case 2:
		z.sliceFrom("peh")
		break
	case 3:
		z.sliceFrom("vojka")
		break
	case 4:
		z.sliceFrom("bojka")
		break
	case 5:
		z.sliceFrom("jak")
		break
	case 6:
		z.sliceFrom("\u010Dajni")
		break
	case 7:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("cajni")
		break
	case 8:
		z.sliceFrom("erni")
		break
	case 9:
		z.sliceFrom("larni")
		break
	case 10:
		z.sliceFrom("esni")
		break
	case 11:
		z.sliceFrom("anjca")
		break
	case 12:
		z.sliceFrom("ajca")
		break
	case 13:
		z.sliceFrom("ljca")
		break
	case 14:
		z.sliceFrom("ejca")
		break
	case 15:
		z.sliceFrom("ojca")
		break
	case 16:
		z.sliceFrom("ajka")
		break
	case 17:
		z.sliceFrom("ojka")
		break
	case 18:
		z.sliceFrom("\u0161ca")
		break
	case 19:
		z.sliceFrom("ing")
		break
	case 20:
		z.sliceFrom("tvenik")
		break
	case 21:
		z.sliceFrom("tetika")
		break
	case 22:
		z.sliceFrom("nstva")
		break
	case 23:
		z.sliceFrom("nik")
		break
	case 24:
		z.sliceFrom("tik")
		break
	case 25:
		z.sliceFrom("zik")
		break
	case 26:
		z.sliceFrom("snik")
		break
	case 27:
		z.sliceFrom("kusi")
		break
	case 28:
		z.sliceFrom("kusni")
		break
	case 29:
		z.sliceFrom("kustva")
		break
	case 30:
		z.sliceFrom("du\u0161ni")
		break
	case 31:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("dusni")
		break
	case 32:
		z.sliceFrom("antni")
		break
	case 33:
		z.sliceFrom("bilni")
		break
	case 34:
		z.sliceFrom("tilni")
		break
	case 35:
		z.sliceFrom("avilni")
		break
	case 36:
		z.sliceFrom("silni")
		break
	case 37:
		z.sliceFrom("gilni")
		break
	case 38:
		z.sliceFrom("rilni")
		break
	case 39:
		z.sliceFrom("nilni")
		break
	case 40:
		z.sliceFrom("alni")
		break
	case 41:
		z.sliceFrom("ozni")
		break
	case 42:
		z.sliceFrom("ravi")
		break
	case 43:
		z.sliceFrom("stavni")
		break
	case 44:
		z.sliceFrom("pravni")
		break
	case 45:
		z.sliceFrom("tivni")
		break
	case 46:
		z.sliceFrom("sivni")
		break
	case 47:
		z.sliceFrom("atni")
		break
	case 48:
		z.sliceFrom("enta")
		break
	case 49:
		z.sliceFrom("tetni")
		break
	case 50:
		z.sliceFrom("pletni")
		break
	case 51:
		z.sliceFrom("\u0161avi")
		break
	case 52:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("savi")
		break
	case 53:
		z.sliceFrom("anta")
		break
	case 54:
		z.sliceFrom("a\u010Dka")
		break
	case 55:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("acka")
		break
	case 56:
		z.sliceFrom("u\u0161ka")
		break
	case 57:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("uska")
		break
	case 58:
		z.sliceFrom("atka")
		break
	case 59:
		z.sliceFrom("etka")
		break
	case 60:
		z.sliceFrom("itka")
		break
	case 61:
		z.sliceFrom("otka")
		break
	case 62:
		z.sliceFrom("utka")
		break
	case 63:
		z.sliceFrom("eskna")
		break
	case 64:
		z.sliceFrom("ti\u010Dni")
		break
	case 65:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ticni")
		break
	case 66:
		z.sliceFrom("ojska")
		break
	case 67:
		z.sliceFrom("esma")
		break
	case 68:
		z.sliceFrom("metra")
		break
	case 69:
		z.sliceFrom("centra")
		break
	case 70:
		z.sliceFrom("istra")
		break
	case 71:
		z.sliceFrom("osti")
		break
	case 72:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("osti")
		break
	case 73:
		z.sliceFrom("dba")
		break
	case 74:
		z.sliceFrom("\u010Dka")
		break
	case 75:
		z.sliceFrom("mca")
		break
	case 76:
		z.sliceFrom("nca")
		break
	case 77:
		z.sliceFrom("voljni")
		break
	case 78:
		z.sliceFrom("anki")
		break
	case 79:
		z.sliceFrom("vca")
		break
	case 80:
		z.sliceFrom("sca")
		break
	case 81:
		z.sliceFrom("rca")
		break
	case 82:
		z.sliceFrom("alca")
		break
	case 83:
		z.sliceFrom("elca")
		break
	case 84:
		z.sliceFrom("olca")
		break
	case 85:
		z.sliceFrom("njca")
		break
	case 86:
		z.sliceFrom("ekta")
		break
	case 87:
		z.sliceFrom("izma")
		break
	case 88:
		z.sliceFrom("jebi")
		break
	case 89:
		z.sliceFrom("baci")
		break
	case 90:
		z.sliceFrom("a\u0161ni")
		break
	case 91:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("asni")
		break
	}
	return true
}
func (z *serbianStemmer) r_Step_2() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(ser_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("sk")
		break
	case 2:
		z.sliceFrom("\u0161k")
		break
	case 3:
		z.sliceFrom("stv")
		break
	case 4:
		z.sliceFrom("\u0161tv")
		break
	case 5:
		z.sliceFrom("tanij")
		break
	case 6:
		z.sliceFrom("manij")
		break
	case 7:
		z.sliceFrom("panij")
		break
	case 8:
		z.sliceFrom("ranij")
		break
	case 9:
		z.sliceFrom("ganij")
		break
	case 10:
		z.sliceFrom("an")
		break
	case 11:
		z.sliceFrom("in")
		break
	case 12:
		z.sliceFrom("on")
		break
	case 13:
		z.sliceFrom("n")
		break
	case 14:
		z.sliceFrom("a\u0107")
		break
	case 15:
		z.sliceFrom("e\u0107")
		break
	case 16:
		z.sliceFrom("u\u0107")
		break
	case 17:
		z.sliceFrom("ugov")
		break
	case 18:
		z.sliceFrom("ug")
		break
	case 19:
		z.sliceFrom("log")
		break
	case 20:
		z.sliceFrom("g")
		break
	case 21:
		z.sliceFrom("rari")
		break
	case 22:
		z.sliceFrom("oti")
		break
	case 23:
		z.sliceFrom("si")
		break
	case 24:
		z.sliceFrom("li")
		break
	case 25:
		z.sliceFrom("uj")
		break
	case 26:
		z.sliceFrom("caj")
		break
	case 27:
		z.sliceFrom("\u010Daj")
		break
	case 28:
		z.sliceFrom("\u0107aj")
		break
	case 29:
		z.sliceFrom("\u0111aj")
		break
	case 30:
		z.sliceFrom("laj")
		break
	case 31:
		z.sliceFrom("raj")
		break
	case 32:
		z.sliceFrom("bij")
		break
	case 33:
		z.sliceFrom("cij")
		break
	case 34:
		z.sliceFrom("dij")
		break
	case 35:
		z.sliceFrom("lij")
		break
	case 36:
		z.sliceFrom("nij")
		break
	case 37:
		z.sliceFrom("mij")
		break
	case 38:
		z.sliceFrom("\u017Eij")
		break
	case 39:
		z.sliceFrom("gij")
		break
	case 40:
		z.sliceFrom("fij")
		break
	case 41:
		z.sliceFrom("pij")
		break
	case 42:
		z.sliceFrom("rij")
		break
	case 43:
		z.sliceFrom("sij")
		break
	case 44:
		z.sliceFrom("tij")
		break
	case 45:
		z.sliceFrom("zij")
		break
	case 46:
		z.sliceFrom("nal")
		break
	case 47:
		z.sliceFrom("ijal")
		break
	case 48:
		z.sliceFrom("ozil")
		break
	case 49:
		z.sliceFrom("olov")
		break
	case 50:
		z.sliceFrom("ol")
		break
	case 51:
		z.sliceFrom("lem")
		break
	case 52:
		z.sliceFrom("ram")
		break
	case 53:
		z.sliceFrom("ar")
		break
	case 54:
		z.sliceFrom("dr")
		break
	case 55:
		z.sliceFrom("er")
		break
	case 56:
		z.sliceFrom("or")
		break
	case 57:
		z.sliceFrom("es")
		break
	case 58:
		z.sliceFrom("is")
		break
	case 59:
		z.sliceFrom("ta\u0161")
		break
	case 60:
		z.sliceFrom("na\u0161")
		break
	case 61:
		z.sliceFrom("ja\u0161")
		break
	case 62:
		z.sliceFrom("ka\u0161")
		break
	case 63:
		z.sliceFrom("ba\u0161")
		break
	case 64:
		z.sliceFrom("ga\u0161")
		break
	case 65:
		z.sliceFrom("va\u0161")
		break
	case 66:
		z.sliceFrom("e\u0161")
		break
	case 67:
		z.sliceFrom("i\u0161")
		break
	case 68:
		z.sliceFrom("ikat")
		break
	case 69:
		z.sliceFrom("lat")
		break
	case 70:
		z.sliceFrom("et")
		break
	case 71:
		z.sliceFrom("est")
		break
	case 72:
		z.sliceFrom("ist")
		break
	case 73:
		z.sliceFrom("kst")
		break
	case 74:
		z.sliceFrom("ost")
		break
	case 75:
		z.sliceFrom("i\u0161t")
		break
	case 76:
		z.sliceFrom("ova")
		break
	case 77:
		z.sliceFrom("av")
		break
	case 78:
		z.sliceFrom("ev")
		break
	case 79:
		z.sliceFrom("iv")
		break
	case 80:
		z.sliceFrom("ov")
		break
	case 81:
		z.sliceFrom("mov")
		break
	case 82:
		z.sliceFrom("lov")
		break
	case 83:
		z.sliceFrom("el")
		break
	case 84:
		z.sliceFrom("anj")
		break
	case 85:
		z.sliceFrom("enj")
		break
	case 86:
		z.sliceFrom("\u0161nj")
		break
	case 87:
		z.sliceFrom("en")
		break
	case 88:
		z.sliceFrom("\u0161n")
		break
	case 89:
		z.sliceFrom("\u010Din")
		break
	case 90:
		z.sliceFrom("ro\u0161i")
		break
	case 91:
		z.sliceFrom("o\u0161")
		break
	case 92:
		z.sliceFrom("evit")
		break
	case 93:
		z.sliceFrom("ovit")
		break
	case 94:
		z.sliceFrom("ast")
		break
	case 95:
		z.sliceFrom("k")
		break
	case 96:
		z.sliceFrom("eva")
		break
	case 97:
		z.sliceFrom("ava")
		break
	case 98:
		z.sliceFrom("iva")
		break
	case 99:
		z.sliceFrom("uva")
		break
	case 100:
		z.sliceFrom("ir")
		break
	case 101:
		z.sliceFrom("a\u010D")
		break
	case 102:
		z.sliceFrom("a\u010Da")
		break
	case 103:
		z.sliceFrom("ni")
		break
	case 104:
		z.sliceFrom("a")
		break
	case 105:
		z.sliceFrom("ur")
		break
	case 106:
		z.sliceFrom("astaj")
		break
	case 107:
		z.sliceFrom("istaj")
		break
	case 108:
		z.sliceFrom("ostaj")
		break
	case 109:
		z.sliceFrom("aj")
		break
	case 110:
		z.sliceFrom("asta")
		break
	case 111:
		z.sliceFrom("ista")
		break
	case 112:
		z.sliceFrom("osta")
		break
	case 113:
		z.sliceFrom("ta")
		break
	case 114:
		z.sliceFrom("inj")
		break
	case 115:
		z.sliceFrom("as")
		break
	case 116:
		z.sliceFrom("i")
		break
	case 117:
		z.sliceFrom("lu\u010D")
		break
	case 118:
		z.sliceFrom("jeti")
		break
	case 119:
		z.sliceFrom("e")
		break
	case 120:
		z.sliceFrom("at")
		break
	case 121:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("luc")
		break
	case 122:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("snj")
		break
	case 123:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("os")
		break
	case 124:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ac")
		break
	case 125:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ec")
		break
	case 126:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("uc")
		break
	case 127:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("rosi")
		break
	case 128:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("aca")
		break
	case 129:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("jas")
		break
	case 130:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("tas")
		break
	case 131:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("gas")
		break
	case 132:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("nas")
		break
	case 133:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("kas")
		break
	case 134:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("vas")
		break
	case 135:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("bas")
		break
	case 136:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("as")
		break
	case 137:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("cin")
		break
	case 138:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("astaj")
		break
	case 139:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("istaj")
		break
	case 140:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ostaj")
		break
	case 141:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("asta")
		break
	case 142:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ista")
		break
	case 143:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("osta")
		break
	case 144:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ava")
		break
	case 145:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("eva")
		break
	case 146:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("iva")
		break
	case 147:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("uva")
		break
	case 148:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ova")
		break
	case 149:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("jeti")
		break
	case 150:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("inj")
		break
	case 151:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ist")
		break
	case 152:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("es")
		break
	case 153:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("et")
		break
	case 154:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("is")
		break
	case 155:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ir")
		break
	case 156:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ur")
		break
	case 157:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("uj")
		break
	case 158:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ni")
		break
	case 159:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("sn")
		break
	case 160:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("ta")
		break
	case 161:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("a")
		break
	case 162:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("i")
		break
	case 163:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("e")
		break
	case 164:
		if !z.B_no_diacritics {
			return false
		}
		z.sliceFrom("n")
		break
	}
	return true
}
func (z *serbianStemmer) r_Step_3() bool {
	z.ket = z.cursor
	if z.findAmongB(ser_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	z.sliceFrom("")
	return true
}
func (z *serbianStemmer) stem() {
	z.r_cyr_to_lat()
	z.r_prelude()
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_Step_1()
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	{
		{
			v_3 := z.limit - z.cursor
			{
				if !z.r_Step_2() {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			if !z.r_Step_3() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	z.cursor = z.limitBackward
	return
}

type spanishStemmer struct {
	snowball
	I_p1, I_p2  int
	S_ch, S_x   []rune
	LS_ch, LS_x int
	I_pV        int
}

func newSpanish() *spanishStemmer { return &spanishStemmer{} }

var spa_a_0 = []among{
	{"", -1, 6, ""},
	{"\u00E1", 0, 1, ""},
	{"\u00E9", 0, 2, ""},
	{"\u00ED", 0, 3, ""},
	{"\u00F3", 0, 4, ""},
	{"\u00FA", 0, 5, ""},
}
var spa_a_1 = []among{
	{"la", -1, -1, ""},
	{"sela", 0, -1, ""},
	{"le", -1, -1, ""},
	{"me", -1, -1, ""},
	{"se", -1, -1, ""},
	{"lo", -1, -1, ""},
	{"selo", 5, -1, ""},
	{"las", -1, -1, ""},
	{"selas", 7, -1, ""},
	{"les", -1, -1, ""},
	{"los", -1, -1, ""},
	{"selos", 10, -1, ""},
	{"nos", -1, -1, ""},
}
var spa_a_2 = []among{
	{"ando", -1, 6, ""},
	{"iendo", -1, 6, ""},
	{"yendo", -1, 7, ""},
	{"\u00E1ndo", -1, 2, ""},
	{"i\u00E9ndo", -1, 1, ""},
	{"ar", -1, 6, ""},
	{"er", -1, 6, ""},
	{"ir", -1, 6, ""},
	{"\u00E1r", -1, 3, ""},
	{"\u00E9r", -1, 4, ""},
	{"\u00EDr", -1, 5, ""},
}
var spa_a_3 = []among{
	{"ic", -1, -1, ""},
	{"ad", -1, -1, ""},
	{"os", -1, -1, ""},
	{"iv", -1, 1, ""},
}
var spa_a_4 = []among{
	{"able", -1, 1, ""},
	{"ible", -1, 1, ""},
	{"ante", -1, 1, ""},
}
var spa_a_5 = []among{
	{"ic", -1, 1, ""},
	{"abil", -1, 1, ""},
	{"iv", -1, 1, ""},
}
var spa_a_6 = []among{
	{"ica", -1, 1, ""},
	{"ancia", -1, 2, ""},
	{"encia", -1, 5, ""},
	{"adora", -1, 2, ""},
	{"osa", -1, 1, ""},
	{"ista", -1, 1, ""},
	{"iva", -1, 9, ""},
	{"anza", -1, 1, ""},
	{"log\u00EDa", -1, 3, ""},
	{"idad", -1, 8, ""},
	{"able", -1, 1, ""},
	{"ible", -1, 1, ""},
	{"ante", -1, 2, ""},
	{"mente", -1, 7, ""},
	{"amente", 13, 6, ""},
	{"acion", -1, 2, ""},
	{"ucion", -1, 4, ""},
	{"aci\u00F3n", -1, 2, ""},
	{"uci\u00F3n", -1, 4, ""},
	{"ico", -1, 1, ""},
	{"ismo", -1, 1, ""},
	{"oso", -1, 1, ""},
	{"amiento", -1, 1, ""},
	{"imiento", -1, 1, ""},
	{"ivo", -1, 9, ""},
	{"ador", -1, 2, ""},
	{"icas", -1, 1, ""},
	{"ancias", -1, 2, ""},
	{"encias", -1, 5, ""},
	{"adoras", -1, 2, ""},
	{"osas", -1, 1, ""},
	{"istas", -1, 1, ""},
	{"ivas", -1, 9, ""},
	{"anzas", -1, 1, ""},
	{"log\u00EDas", -1, 3, ""},
	{"idades", -1, 8, ""},
	{"ables", -1, 1, ""},
	{"ibles", -1, 1, ""},
	{"aciones", -1, 2, ""},
	{"uciones", -1, 4, ""},
	{"adores", -1, 2, ""},
	{"antes", -1, 2, ""},
	{"icos", -1, 1, ""},
	{"ismos", -1, 1, ""},
	{"osos", -1, 1, ""},
	{"amientos", -1, 1, ""},
	{"imientos", -1, 1, ""},
	{"ivos", -1, 9, ""},
}
var spa_a_7 = []among{
	{"ya", -1, 1, ""},
	{"ye", -1, 1, ""},
	{"yan", -1, 1, ""},
	{"yen", -1, 1, ""},
	{"yeron", -1, 1, ""},
	{"yendo", -1, 1, ""},
	{"yo", -1, 1, ""},
	{"yas", -1, 1, ""},
	{"yes", -1, 1, ""},
	{"yais", -1, 1, ""},
	{"yamos", -1, 1, ""},
	{"y\u00F3", -1, 1, ""},
}
var spa_a_8 = []among{
	{"aba", -1, 2, ""},
	{"ada", -1, 2, ""},
	{"ida", -1, 2, ""},
	{"ara", -1, 2, ""},
	{"iera", -1, 2, ""},
	{"\u00EDa", -1, 2, ""},
	{"ar\u00EDa", 5, 2, ""},
	{"er\u00EDa", 5, 2, ""},
	{"ir\u00EDa", 5, 2, ""},
	{"ad", -1, 2, ""},
	{"ed", -1, 2, ""},
	{"id", -1, 2, ""},
	{"ase", -1, 2, ""},
	{"iese", -1, 2, ""},
	{"aste", -1, 2, ""},
	{"iste", -1, 2, ""},
	{"an", -1, 2, ""},
	{"aban", 16, 2, ""},
	{"aran", 16, 2, ""},
	{"ieran", 16, 2, ""},
	{"\u00EDan", 16, 2, ""},
	{"ar\u00EDan", 20, 2, ""},
	{"er\u00EDan", 20, 2, ""},
	{"ir\u00EDan", 20, 2, ""},
	{"en", -1, 1, ""},
	{"asen", 24, 2, ""},
	{"iesen", 24, 2, ""},
	{"aron", -1, 2, ""},
	{"ieron", -1, 2, ""},
	{"ar\u00E1n", -1, 2, ""},
	{"er\u00E1n", -1, 2, ""},
	{"ir\u00E1n", -1, 2, ""},
	{"ado", -1, 2, ""},
	{"ido", -1, 2, ""},
	{"ando", -1, 2, ""},
	{"iendo", -1, 2, ""},
	{"ar", -1, 2, ""},
	{"er", -1, 2, ""},
	{"ir", -1, 2, ""},
	{"as", -1, 2, ""},
	{"abas", 39, 2, ""},
	{"adas", 39, 2, ""},
	{"idas", 39, 2, ""},
	{"aras", 39, 2, ""},
	{"ieras", 39, 2, ""},
	{"\u00EDas", 39, 2, ""},
	{"ar\u00EDas", 45, 2, ""},
	{"er\u00EDas", 45, 2, ""},
	{"ir\u00EDas", 45, 2, ""},
	{"es", -1, 1, ""},
	{"ases", 49, 2, ""},
	{"ieses", 49, 2, ""},
	{"abais", -1, 2, ""},
	{"arais", -1, 2, ""},
	{"ierais", -1, 2, ""},
	{"\u00EDais", -1, 2, ""},
	{"ar\u00EDais", 55, 2, ""},
	{"er\u00EDais", 55, 2, ""},
	{"ir\u00EDais", 55, 2, ""},
	{"aseis", -1, 2, ""},
	{"ieseis", -1, 2, ""},
	{"asteis", -1, 2, ""},
	{"isteis", -1, 2, ""},
	{"\u00E1is", -1, 2, ""},
	{"\u00E9is", -1, 1, ""},
	{"ar\u00E9is", 64, 2, ""},
	{"er\u00E9is", 64, 2, ""},
	{"ir\u00E9is", 64, 2, ""},
	{"ados", -1, 2, ""},
	{"idos", -1, 2, ""},
	{"amos", -1, 2, ""},
	{"\u00E1bamos", 70, 2, ""},
	{"\u00E1ramos", 70, 2, ""},
	{"i\u00E9ramos", 70, 2, ""},
	{"\u00EDamos", 70, 2, ""},
	{"ar\u00EDamos", 74, 2, ""},
	{"er\u00EDamos", 74, 2, ""},
	{"ir\u00EDamos", 74, 2, ""},
	{"emos", -1, 1, ""},
	{"aremos", 78, 2, ""},
	{"eremos", 78, 2, ""},
	{"iremos", 78, 2, ""},
	{"\u00E1semos", 78, 2, ""},
	{"i\u00E9semos", 78, 2, ""},
	{"imos", -1, 2, ""},
	{"ar\u00E1s", -1, 2, ""},
	{"er\u00E1s", -1, 2, ""},
	{"ir\u00E1s", -1, 2, ""},
	{"\u00EDs", -1, 2, ""},
	{"ar\u00E1", -1, 2, ""},
	{"er\u00E1", -1, 2, ""},
	{"ir\u00E1", -1, 2, ""},
	{"ar\u00E9", -1, 2, ""},
	{"er\u00E9", -1, 2, ""},
	{"ir\u00E9", -1, 2, ""},
	{"i\u00F3", -1, 2, ""},
}
var spa_a_9 = []among{
	{"a", -1, 1, ""},
	{"e", -1, 2, ""},
	{"o", -1, 1, ""},
	{"os", -1, 1, ""},
	{"\u00E1", -1, 1, ""},
	{"\u00E9", -1, 2, ""},
	{"\u00ED", -1, 1, ""},
	{"\u00F3", -1, 1, ""},
}
var spa_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 17, 4, 10}

func (z *spanishStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(spa_g_v, 97, 252)) {
					goto lab2_end
				}
				{
					v_3 := z.cursor
					{
						if !(z.outGrouping(spa_g_v, 97, 252)) {
							goto lab4_end
						}
						if !z.goOutGrouping(spa_g_v, 97, 252) {
							goto lab4_end
						}
						z.cursor++
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = v_3
					if !(z.inGrouping(spa_g_v, 97, 252)) {
						goto lab2_end
					}
					if !z.goInGrouping(spa_g_v, 97, 252) {
						goto lab2_end
					}
					z.cursor++
				}
			lab3_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			if !(z.outGrouping(spa_g_v, 97, 252)) {
				goto lab0_end
			}
			{
				v_4 := z.cursor
				{
					if !(z.outGrouping(spa_g_v, 97, 252)) {
						goto lab6_end
					}
					if !z.goOutGrouping(spa_g_v, 97, 252) {
						goto lab6_end
					}
					z.cursor++
					goto lab5_end
				}
			lab6_end:
				;
				z.cursor = v_4
				if !(z.inGrouping(spa_g_v, 97, 252)) {
					goto lab0_end
				}
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		lab5_end:
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_5 := z.cursor
	{
		if !z.goOutGrouping(spa_g_v, 97, 252) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(spa_g_v, 97, 252) {
			goto lab7_end
		}
		z.cursor++
		z.I_p1 = z.cursor
		if !z.goOutGrouping(spa_g_v, 97, 252) {
			goto lab7_end
		}
		z.cursor++
		if !z.goInGrouping(spa_g_v, 97, 252) {
			goto lab7_end
		}
		z.cursor++
		z.I_p2 = z.cursor
	}
lab7_end:
	;
	z.cursor = v_5
	return true
}
func (z *spanishStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(spa_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("a")
				break
			case 2:
				z.sliceFrom("e")
				break
			case 3:
				z.sliceFrom("i")
				break
			case 4:
				z.sliceFrom("o")
				break
			case 5:
				z.sliceFrom("u")
				break
			case 6:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *spanishStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *spanishStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *spanishStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *spanishStemmer) r_attached_pronoun() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(spa_a_1) == 0 {
		return false
	}
	z.bra = z.cursor
	among_var = z.findAmongB(spa_a_2)
	if among_var == 0 {
		return false
	}
	if !z.r_RV() {
		return false
	}
	switch among_var {
	case 1:
		z.bra = z.cursor
		z.sliceFrom("iendo")
		break
	case 2:
		z.bra = z.cursor
		z.sliceFrom("ando")
		break
	case 3:
		z.bra = z.cursor
		z.sliceFrom("ar")
		break
	case 4:
		z.bra = z.cursor
		z.sliceFrom("er")
		break
	case 5:
		z.bra = z.cursor
		z.sliceFrom("ir")
		break
	case 6:
		z.sliceDel()
		break
	case 7:
		if !(z.eqSB("u")) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *spanishStemmer) r_standard_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(spa_a_6)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("ic")) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.sliceDel()
		}
	lab0_end:
		;
		break
	case 3:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("log")
		break
	case 4:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("u")
		break
	case 5:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("ente")
		break
	case 6:
		if !z.r_R1() {
			return false
		}
		z.sliceDel()
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(spa_a_3)
			if among_var == 0 {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_2
				goto lab1_end
			}
			z.sliceDel()
			switch among_var {
			case 1:
				z.ket = z.cursor
				if !(z.eqSB("at")) {
					z.cursor = z.limit - v_2
					goto lab1_end
				}
				z.bra = z.cursor
				if !z.r_R2() {
					z.cursor = z.limit - v_2
					goto lab1_end
				}
				z.sliceDel()
				break
			}
		}
	lab1_end:
		;
		break
	case 7:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_3 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if z.findAmongB(spa_a_4) == 0 {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_3
				goto lab2_end
			}
			z.sliceDel()
		}
	lab2_end:
		;
		break
	case 8:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_4 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if z.findAmongB(spa_a_5) == 0 {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_4
				goto lab3_end
			}
			z.sliceDel()
		}
	lab3_end:
		;
		break
	case 9:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_5 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("at")) {
				z.cursor = z.limit - v_5
				goto lab4_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_5
				goto lab4_end
			}
			z.sliceDel()
		}
	lab4_end:
		;
		break
	}
	return true
}
func (z *spanishStemmer) r_y_verb_suffix() bool {
	if z.cursor < z.I_pV {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	if z.findAmongB(spa_a_7) == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	if !(z.eqSB("u")) {
		return false
	}
	z.sliceDel()
	return true
}
func (z *spanishStemmer) r_verb_suffix() bool {
	var among_var int
	if z.cursor < z.I_pV {
		return false
	}
	v_1 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	among_var = z.findAmongB(spa_a_8)
	if among_var == 0 {
		z.limitBackward = v_1
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_1
	switch among_var {
	case 1:
		v_2 := z.limit - z.cursor
		{
			if !(z.eqSB("u")) {
				z.cursor = z.limit - v_2
				goto lab0_end
			}
			v_3 := z.limit - z.cursor
			if !(z.eqSB("g")) {
				z.cursor = z.limit - v_2
				goto lab0_end
			}
			z.cursor = z.limit - v_3
		}
	lab0_end:
		;
		z.bra = z.cursor
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	}
	return true
}
func (z *spanishStemmer) r_residual_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(spa_a_9)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("u")) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.bra = z.cursor
			v_2 := z.limit - z.cursor
			if !(z.eqSB("g")) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.cursor = z.limit - v_2
			if !z.r_RV() {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.sliceDel()
		}
	lab0_end:
		;
		break
	}
	return true
}
func (z *spanishStemmer) stem() {
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_attached_pronoun()
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	{
		{
			v_3 := z.limit - z.cursor
			{
				if !z.r_standard_suffix() {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			{
				if !z.r_y_verb_suffix() {
					goto lab3_end
				}
				goto lab1_end
			}
		lab3_end:
			;
			z.cursor = z.limit - v_3
			if !z.r_verb_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_2
	v_4 := z.limit - z.cursor
	z.r_residual_suffix()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	v_5 := z.cursor
	z.r_postlude()
	z.cursor = v_5
	return
}

type turkishStemmer struct {
	snowball
	I_p1, I_p2                        int
	S_ch, S_x                         []rune
	LS_ch, LS_x                       int
	B_continue_stemming_noun_suffixes bool
}

func newTurkish() *turkishStemmer { return &turkishStemmer{} }

var tur_a_0 = []among{
	{"m", -1, -1, ""},
	{"n", -1, -1, ""},
	{"miz", -1, -1, ""},
	{"niz", -1, -1, ""},
	{"muz", -1, -1, ""},
	{"nuz", -1, -1, ""},
	{"m\u00FCz", -1, -1, ""},
	{"n\u00FCz", -1, -1, ""},
	{"m\u0131z", -1, -1, ""},
	{"n\u0131z", -1, -1, ""},
}
var tur_a_1 = []among{
	{"leri", -1, -1, ""},
	{"lar\u0131", -1, -1, ""},
}
var tur_a_2 = []among{
	{"ni", -1, -1, ""},
	{"nu", -1, -1, ""},
	{"n\u00FC", -1, -1, ""},
	{"n\u0131", -1, -1, ""},
}
var tur_a_3 = []among{
	{"in", -1, -1, ""},
	{"un", -1, -1, ""},
	{"\u00FCn", -1, -1, ""},
	{"\u0131n", -1, -1, ""},
}
var tur_a_4 = []among{
	{"a", -1, -1, ""},
	{"e", -1, -1, ""},
}
var tur_a_5 = []among{
	{"na", -1, -1, ""},
	{"ne", -1, -1, ""},
}
var tur_a_6 = []among{
	{"da", -1, -1, ""},
	{"ta", -1, -1, ""},
	{"de", -1, -1, ""},
	{"te", -1, -1, ""},
}
var tur_a_7 = []among{
	{"nda", -1, -1, ""},
	{"nde", -1, -1, ""},
}
var tur_a_8 = []among{
	{"dan", -1, -1, ""},
	{"tan", -1, -1, ""},
	{"den", -1, -1, ""},
	{"ten", -1, -1, ""},
}
var tur_a_9 = []among{
	{"ndan", -1, -1, ""},
	{"nden", -1, -1, ""},
}
var tur_a_10 = []among{
	{"la", -1, -1, ""},
	{"le", -1, -1, ""},
}
var tur_a_11 = []among{
	{"ca", -1, -1, ""},
	{"ce", -1, -1, ""},
}
var tur_a_12 = []among{
	{"im", -1, -1, ""},
	{"um", -1, -1, ""},
	{"\u00FCm", -1, -1, ""},
	{"\u0131m", -1, -1, ""},
}
var tur_a_13 = []among{
	{"sin", -1, -1, ""},
	{"sun", -1, -1, ""},
	{"s\u00FCn", -1, -1, ""},
	{"s\u0131n", -1, -1, ""},
}
var tur_a_14 = []among{
	{"iz", -1, -1, ""},
	{"uz", -1, -1, ""},
	{"\u00FCz", -1, -1, ""},
	{"\u0131z", -1, -1, ""},
}
var tur_a_15 = []among{
	{"siniz", -1, -1, ""},
	{"sunuz", -1, -1, ""},
	{"s\u00FCn\u00FCz", -1, -1, ""},
	{"s\u0131n\u0131z", -1, -1, ""},
}
var tur_a_16 = []among{
	{"lar", -1, -1, ""},
	{"ler", -1, -1, ""},
}
var tur_a_17 = []among{
	{"niz", -1, -1, ""},
	{"nuz", -1, -1, ""},
	{"n\u00FCz", -1, -1, ""},
	{"n\u0131z", -1, -1, ""},
}
var tur_a_18 = []among{
	{"dir", -1, -1, ""},
	{"tir", -1, -1, ""},
	{"dur", -1, -1, ""},
	{"tur", -1, -1, ""},
	{"d\u00FCr", -1, -1, ""},
	{"t\u00FCr", -1, -1, ""},
	{"d\u0131r", -1, -1, ""},
	{"t\u0131r", -1, -1, ""},
}
var tur_a_19 = []among{
	{"cas\u0131na", -1, -1, ""},
	{"cesine", -1, -1, ""},
}
var tur_a_20 = []among{
	{"di", -1, -1, ""},
	{"ti", -1, -1, ""},
	{"dik", -1, -1, ""},
	{"tik", -1, -1, ""},
	{"duk", -1, -1, ""},
	{"tuk", -1, -1, ""},
	{"d\u00FCk", -1, -1, ""},
	{"t\u00FCk", -1, -1, ""},
	{"d\u0131k", -1, -1, ""},
	{"t\u0131k", -1, -1, ""},
	{"dim", -1, -1, ""},
	{"tim", -1, -1, ""},
	{"dum", -1, -1, ""},
	{"tum", -1, -1, ""},
	{"d\u00FCm", -1, -1, ""},
	{"t\u00FCm", -1, -1, ""},
	{"d\u0131m", -1, -1, ""},
	{"t\u0131m", -1, -1, ""},
	{"din", -1, -1, ""},
	{"tin", -1, -1, ""},
	{"dun", -1, -1, ""},
	{"tun", -1, -1, ""},
	{"d\u00FCn", -1, -1, ""},
	{"t\u00FCn", -1, -1, ""},
	{"d\u0131n", -1, -1, ""},
	{"t\u0131n", -1, -1, ""},
	{"du", -1, -1, ""},
	{"tu", -1, -1, ""},
	{"d\u00FC", -1, -1, ""},
	{"t\u00FC", -1, -1, ""},
	{"d\u0131", -1, -1, ""},
	{"t\u0131", -1, -1, ""},
}
var tur_a_21 = []among{
	{"sa", -1, -1, ""},
	{"se", -1, -1, ""},
	{"sak", -1, -1, ""},
	{"sek", -1, -1, ""},
	{"sam", -1, -1, ""},
	{"sem", -1, -1, ""},
	{"san", -1, -1, ""},
	{"sen", -1, -1, ""},
}
var tur_a_22 = []among{
	{"mi\u015F", -1, -1, ""},
	{"mu\u015F", -1, -1, ""},
	{"m\u00FC\u015F", -1, -1, ""},
	{"m\u0131\u015F", -1, -1, ""},
}
var tur_a_23 = []among{
	{"b", -1, 1, ""},
	{"c", -1, 2, ""},
	{"d", -1, 3, ""},
	{"\u011F", -1, 4, ""},
}
var tur_g_vowel = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 32, 8, 0, 0, 0, 0, 0, 0, 1}
var tur_g_U = []byte{1, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 0, 0, 0, 0, 0, 1}
var tur_g_vowel1 = []byte{1, 64, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
var tur_g_vowel2 = []byte{17, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 130}
var tur_g_vowel3 = []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1}
var tur_g_vowel4 = []byte{17}
var tur_g_vowel5 = []byte{65}
var tur_g_vowel6 = []byte{65}

func (z *turkishStemmer) r_check_vowel_harmony() bool {
	v_1 := z.limit - z.cursor
	if !z.goOutGroupingB(tur_g_vowel, 97, 305) {
		return false
	}
	{
		v_2 := z.limit - z.cursor
		{
			if !(z.eqSB("a")) {
				goto lab1_end
			}
			if !z.goOutGroupingB(tur_g_vowel1, 97, 305) {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("e")) {
				goto lab2_end
			}
			if !z.goOutGroupingB(tur_g_vowel2, 101, 252) {
				goto lab2_end
			}
			goto lab0_end
		}
	lab2_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("\u0131")) {
				goto lab3_end
			}
			if !z.goOutGroupingB(tur_g_vowel3, 97, 305) {
				goto lab3_end
			}
			goto lab0_end
		}
	lab3_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("i")) {
				goto lab4_end
			}
			if !z.goOutGroupingB(tur_g_vowel4, 101, 105) {
				goto lab4_end
			}
			goto lab0_end
		}
	lab4_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("o")) {
				goto lab5_end
			}
			if !z.goOutGroupingB(tur_g_vowel5, 111, 117) {
				goto lab5_end
			}
			goto lab0_end
		}
	lab5_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("\u00F6")) {
				goto lab6_end
			}
			if !z.goOutGroupingB(tur_g_vowel6, 246, 252) {
				goto lab6_end
			}
			goto lab0_end
		}
	lab6_end:
		;
		z.cursor = z.limit - v_2
		{
			if !(z.eqSB("u")) {
				goto lab7_end
			}
			if !z.goOutGroupingB(tur_g_vowel5, 111, 117) {
				goto lab7_end
			}
			goto lab0_end
		}
	lab7_end:
		;
		z.cursor = z.limit - v_2
		if !(z.eqSB("\u00FC")) {
			return false
		}
		if !z.goOutGroupingB(tur_g_vowel6, 246, 252) {
			return false
		}
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	return true
}
func (z *turkishStemmer) r_mark_suffix_with_optional_n_consonant() bool {
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("n")) {
				goto lab1_end
			}
			v_2 := z.limit - z.cursor
			if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
				goto lab1_end
			}
			z.cursor = z.limit - v_2
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.eqSB("n")) {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
		}
		v_4 := z.limit - z.cursor
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
		if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
			return false
		}
		z.cursor = z.limit - v_4
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_mark_suffix_with_optional_s_consonant() bool {
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("s")) {
				goto lab1_end
			}
			v_2 := z.limit - z.cursor
			if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
				goto lab1_end
			}
			z.cursor = z.limit - v_2
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.eqSB("s")) {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
		}
		v_4 := z.limit - z.cursor
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
		if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
			return false
		}
		z.cursor = z.limit - v_4
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_mark_suffix_with_optional_y_consonant() bool {
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("y")) {
				goto lab1_end
			}
			v_2 := z.limit - z.cursor
			if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
				goto lab1_end
			}
			z.cursor = z.limit - v_2
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.eqSB("y")) {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
		}
		v_4 := z.limit - z.cursor
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
		if !(z.inGroupingB(tur_g_vowel, 97, 305)) {
			return false
		}
		z.cursor = z.limit - v_4
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_mark_suffix_with_optional_U_vowel() bool {
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.inGroupingB(tur_g_U, 105, 305)) {
				goto lab1_end
			}
			v_2 := z.limit - z.cursor
			if !(z.outGroupingB(tur_g_vowel, 97, 305)) {
				goto lab1_end
			}
			z.cursor = z.limit - v_2
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.inGroupingB(tur_g_U, 105, 305)) {
					goto lab2_end
				}
				return false
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
		}
		v_4 := z.limit - z.cursor
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
		if !(z.outGroupingB(tur_g_vowel, 97, 305)) {
			return false
		}
		z.cursor = z.limit - v_4
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_mark_possessives() bool {
	if z.findAmongB(tur_a_0) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_U_vowel()
}
func (z *turkishStemmer) r_mark_sU() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if !(z.inGroupingB(tur_g_U, 105, 305)) {
		return false
	}
	return z.r_mark_suffix_with_optional_s_consonant()
}
func (z *turkishStemmer) r_mark_lArI() bool {
	return z.findAmongB(tur_a_1) != 0
}
func (z *turkishStemmer) r_mark_yU() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if !(z.inGroupingB(tur_g_U, 105, 305)) {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_nU() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_2) != 0
}
func (z *turkishStemmer) r_mark_nUn() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_3) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_n_consonant()
}
func (z *turkishStemmer) r_mark_yA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_4) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_nA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_5) != 0
}
func (z *turkishStemmer) r_mark_DA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_6) != 0
}
func (z *turkishStemmer) r_mark_ndA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_7) != 0
}
func (z *turkishStemmer) r_mark_DAn() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_8) != 0
}
func (z *turkishStemmer) r_mark_ndAn() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_9) != 0
}
func (z *turkishStemmer) r_mark_ylA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_10) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_ki() bool {
	return z.eqSB("ki")
}
func (z *turkishStemmer) r_mark_ncA() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_11) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_n_consonant()
}
func (z *turkishStemmer) r_mark_yUm() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_12) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_sUn() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_13) != 0
}
func (z *turkishStemmer) r_mark_yUz() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_14) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_sUnUz() bool {
	return z.findAmongB(tur_a_15) != 0
}
func (z *turkishStemmer) r_mark_lAr() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_16) != 0
}
func (z *turkishStemmer) r_mark_nUz() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_17) != 0
}
func (z *turkishStemmer) r_mark_DUr() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	return z.findAmongB(tur_a_18) != 0
}
func (z *turkishStemmer) r_mark_cAsInA() bool {
	return z.findAmongB(tur_a_19) != 0
}
func (z *turkishStemmer) r_mark_yDU() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_20) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_ysA() bool {
	if z.findAmongB(tur_a_21) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_ymUs_() bool {
	if !z.r_check_vowel_harmony() {
		return false
	}
	if z.findAmongB(tur_a_22) == 0 {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_mark_yken() bool {
	if !(z.eqSB("ken")) {
		return false
	}
	return z.r_mark_suffix_with_optional_y_consonant()
}
func (z *turkishStemmer) r_stem_nominal_verb_suffixes() bool {
	z.ket = z.cursor
	z.B_continue_stemming_noun_suffixes = true
	{
		v_1 := z.limit - z.cursor
		{
			{
				v_2 := z.limit - z.cursor
				{
					if !z.r_mark_ymUs_() {
						goto lab3_end
					}
					goto lab2_end
				}
			lab3_end:
				;
				z.cursor = z.limit - v_2
				{
					if !z.r_mark_yDU() {
						goto lab4_end
					}
					goto lab2_end
				}
			lab4_end:
				;
				z.cursor = z.limit - v_2
				{
					if !z.r_mark_ysA() {
						goto lab5_end
					}
					goto lab2_end
				}
			lab5_end:
				;
				z.cursor = z.limit - v_2
				if !z.r_mark_yken() {
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
		{
			if !z.r_mark_cAsInA() {
				goto lab6_end
			}
			{
				v_3 := z.limit - z.cursor
				{
					if !z.r_mark_sUnUz() {
						goto lab8_end
					}
					goto lab7_end
				}
			lab8_end:
				;
				z.cursor = z.limit - v_3
				{
					if !z.r_mark_lAr() {
						goto lab9_end
					}
					goto lab7_end
				}
			lab9_end:
				;
				z.cursor = z.limit - v_3
				{
					if !z.r_mark_yUm() {
						goto lab10_end
					}
					goto lab7_end
				}
			lab10_end:
				;
				z.cursor = z.limit - v_3
				{
					if !z.r_mark_sUn() {
						goto lab11_end
					}
					goto lab7_end
				}
			lab11_end:
				;
				z.cursor = z.limit - v_3
				{
					if !z.r_mark_yUz() {
						goto lab12_end
					}
					goto lab7_end
				}
			lab12_end:
				;
				z.cursor = z.limit - v_3
			}
		lab7_end:
			;
			if !z.r_mark_ymUs_() {
				goto lab6_end
			}
			goto lab0_end
		}
	lab6_end:
		;
		z.cursor = z.limit - v_1
		{
			if !z.r_mark_lAr() {
				goto lab13_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_4 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_5 := z.limit - z.cursor
					{
						if !z.r_mark_DUr() {
							goto lab16_end
						}
						goto lab15_end
					}
				lab16_end:
					;
					z.cursor = z.limit - v_5
					{
						if !z.r_mark_yDU() {
							goto lab17_end
						}
						goto lab15_end
					}
				lab17_end:
					;
					z.cursor = z.limit - v_5
					{
						if !z.r_mark_ysA() {
							goto lab18_end
						}
						goto lab15_end
					}
				lab18_end:
					;
					z.cursor = z.limit - v_5
					if !z.r_mark_ymUs_() {
						z.cursor = z.limit - v_4
						goto lab14_end
					}
				}
			lab15_end:
			}
		lab14_end:
			;
			z.B_continue_stemming_noun_suffixes = false
			goto lab0_end
		}
	lab13_end:
		;
		z.cursor = z.limit - v_1
		{
			if !z.r_mark_nUz() {
				goto lab19_end
			}
			{
				v_6 := z.limit - z.cursor
				{
					if !z.r_mark_yDU() {
						goto lab21_end
					}
					goto lab20_end
				}
			lab21_end:
				;
				z.cursor = z.limit - v_6
				if !z.r_mark_ysA() {
					goto lab19_end
				}
			}
		lab20_end:
			;
			goto lab0_end
		}
	lab19_end:
		;
		z.cursor = z.limit - v_1
		{
			{
				v_7 := z.limit - z.cursor
				{
					if !z.r_mark_sUnUz() {
						goto lab24_end
					}
					goto lab23_end
				}
			lab24_end:
				;
				z.cursor = z.limit - v_7
				{
					if !z.r_mark_yUz() {
						goto lab25_end
					}
					goto lab23_end
				}
			lab25_end:
				;
				z.cursor = z.limit - v_7
				{
					if !z.r_mark_sUn() {
						goto lab26_end
					}
					goto lab23_end
				}
			lab26_end:
				;
				z.cursor = z.limit - v_7
				if !z.r_mark_yUm() {
					goto lab22_end
				}
			}
		lab23_end:
			;
			z.bra = z.cursor
			z.sliceDel()
			v_8 := z.limit - z.cursor
			{
				z.ket = z.cursor
				if !z.r_mark_ymUs_() {
					z.cursor = z.limit - v_8
					goto lab27_end
				}
			}
		lab27_end:
			;
			goto lab0_end
		}
	lab22_end:
		;
		z.cursor = z.limit - v_1
		if !z.r_mark_DUr() {
			return false
		}
		z.bra = z.cursor
		z.sliceDel()
		v_9 := z.limit - z.cursor
		{
			z.ket = z.cursor
			{
				v_10 := z.limit - z.cursor
				{
					if !z.r_mark_sUnUz() {
						goto lab30_end
					}
					goto lab29_end
				}
			lab30_end:
				;
				z.cursor = z.limit - v_10
				{
					if !z.r_mark_lAr() {
						goto lab31_end
					}
					goto lab29_end
				}
			lab31_end:
				;
				z.cursor = z.limit - v_10
				{
					if !z.r_mark_yUm() {
						goto lab32_end
					}
					goto lab29_end
				}
			lab32_end:
				;
				z.cursor = z.limit - v_10
				{
					if !z.r_mark_sUn() {
						goto lab33_end
					}
					goto lab29_end
				}
			lab33_end:
				;
				z.cursor = z.limit - v_10
				{
					if !z.r_mark_yUz() {
						goto lab34_end
					}
					goto lab29_end
				}
			lab34_end:
				;
				z.cursor = z.limit - v_10
			}
		lab29_end:
			;
			if !z.r_mark_ymUs_() {
				z.cursor = z.limit - v_9
				goto lab28_end
			}
		}
	lab28_end:
	}
lab0_end:
	;
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *turkishStemmer) r_stem_suffix_chain_before_ki() bool {
	z.ket = z.cursor
	if !z.r_mark_ki() {
		return false
	}
	{
		v_1 := z.limit - z.cursor
		{
			if !z.r_mark_DA() {
				goto lab1_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_2 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_3 := z.limit - z.cursor
					{
						if !z.r_mark_lAr() {
							goto lab4_end
						}
						z.bra = z.cursor
						z.sliceDel()
						v_4 := z.limit - z.cursor
						{
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_4
								goto lab5_end
							}
						}
					lab5_end:
						;
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = z.limit - v_3
					if !z.r_mark_possessives() {
						z.cursor = z.limit - v_2
						goto lab2_end
					}
					z.bra = z.cursor
					z.sliceDel()
					v_5 := z.limit - z.cursor
					{
						z.ket = z.cursor
						if !z.r_mark_lAr() {
							z.cursor = z.limit - v_5
							goto lab6_end
						}
						z.bra = z.cursor
						z.sliceDel()
						if !z.r_stem_suffix_chain_before_ki() {
							z.cursor = z.limit - v_5
							goto lab6_end
						}
					}
				lab6_end:
				}
			lab3_end:
			}
		lab2_end:
			;
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			if !z.r_mark_nUn() {
				goto lab7_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_6 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_7 := z.limit - z.cursor
					{
						if !z.r_mark_lArI() {
							goto lab10_end
						}
						z.bra = z.cursor
						z.sliceDel()
						goto lab9_end
					}
				lab10_end:
					;
					z.cursor = z.limit - v_7
					{
						z.ket = z.cursor
						{
							v_8 := z.limit - z.cursor
							{
								if !z.r_mark_possessives() {
									goto lab13_end
								}
								goto lab12_end
							}
						lab13_end:
							;
							z.cursor = z.limit - v_8
							if !z.r_mark_sU() {
								goto lab11_end
							}
						}
					lab12_end:
						;
						z.bra = z.cursor
						z.sliceDel()
						v_9 := z.limit - z.cursor
						{
							z.ket = z.cursor
							if !z.r_mark_lAr() {
								z.cursor = z.limit - v_9
								goto lab14_end
							}
							z.bra = z.cursor
							z.sliceDel()
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_9
								goto lab14_end
							}
						}
					lab14_end:
						;
						goto lab9_end
					}
				lab11_end:
					;
					z.cursor = z.limit - v_7
					if !z.r_stem_suffix_chain_before_ki() {
						z.cursor = z.limit - v_6
						goto lab8_end
					}
				}
			lab9_end:
			}
		lab8_end:
			;
			goto lab0_end
		}
	lab7_end:
		;
		z.cursor = z.limit - v_1
		if !z.r_mark_ndA() {
			return false
		}
		{
			v_10 := z.limit - z.cursor
			{
				if !z.r_mark_lArI() {
					goto lab16_end
				}
				z.bra = z.cursor
				z.sliceDel()
				goto lab15_end
			}
		lab16_end:
			;
			z.cursor = z.limit - v_10
			{
				if !z.r_mark_sU() {
					goto lab17_end
				}
				z.bra = z.cursor
				z.sliceDel()
				v_11 := z.limit - z.cursor
				{
					z.ket = z.cursor
					if !z.r_mark_lAr() {
						z.cursor = z.limit - v_11
						goto lab18_end
					}
					z.bra = z.cursor
					z.sliceDel()
					if !z.r_stem_suffix_chain_before_ki() {
						z.cursor = z.limit - v_11
						goto lab18_end
					}
				}
			lab18_end:
				;
				goto lab15_end
			}
		lab17_end:
			;
			z.cursor = z.limit - v_10
			if !z.r_stem_suffix_chain_before_ki() {
				return false
			}
		}
	lab15_end:
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_stem_noun_suffixes() bool {
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !z.r_mark_lAr() {
				goto lab1_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_2 := z.limit - z.cursor
			{
				if !z.r_stem_suffix_chain_before_ki() {
					z.cursor = z.limit - v_2
					goto lab2_end
				}
			}
		lab2_end:
			;
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			if !z.r_mark_ncA() {
				goto lab3_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_3 := z.limit - z.cursor
			{
				{
					v_4 := z.limit - z.cursor
					{
						z.ket = z.cursor
						if !z.r_mark_lArI() {
							goto lab6_end
						}
						z.bra = z.cursor
						z.sliceDel()
						goto lab5_end
					}
				lab6_end:
					;
					z.cursor = z.limit - v_4
					{
						z.ket = z.cursor
						{
							v_5 := z.limit - z.cursor
							{
								if !z.r_mark_possessives() {
									goto lab9_end
								}
								goto lab8_end
							}
						lab9_end:
							;
							z.cursor = z.limit - v_5
							if !z.r_mark_sU() {
								goto lab7_end
							}
						}
					lab8_end:
						;
						z.bra = z.cursor
						z.sliceDel()
						v_6 := z.limit - z.cursor
						{
							z.ket = z.cursor
							if !z.r_mark_lAr() {
								z.cursor = z.limit - v_6
								goto lab10_end
							}
							z.bra = z.cursor
							z.sliceDel()
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_6
								goto lab10_end
							}
						}
					lab10_end:
						;
						goto lab5_end
					}
				lab7_end:
					;
					z.cursor = z.limit - v_4
					z.ket = z.cursor
					if !z.r_mark_lAr() {
						z.cursor = z.limit - v_3
						goto lab4_end
					}
					z.bra = z.cursor
					z.sliceDel()
					if !z.r_stem_suffix_chain_before_ki() {
						z.cursor = z.limit - v_3
						goto lab4_end
					}
				}
			lab5_end:
			}
		lab4_end:
			;
			goto lab0_end
		}
	lab3_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			{
				v_7 := z.limit - z.cursor
				{
					if !z.r_mark_ndA() {
						goto lab13_end
					}
					goto lab12_end
				}
			lab13_end:
				;
				z.cursor = z.limit - v_7
				if !z.r_mark_nA() {
					goto lab11_end
				}
			}
		lab12_end:
			;
			{
				v_8 := z.limit - z.cursor
				{
					if !z.r_mark_lArI() {
						goto lab15_end
					}
					z.bra = z.cursor
					z.sliceDel()
					goto lab14_end
				}
			lab15_end:
				;
				z.cursor = z.limit - v_8
				{
					if !z.r_mark_sU() {
						goto lab16_end
					}
					z.bra = z.cursor
					z.sliceDel()
					v_9 := z.limit - z.cursor
					{
						z.ket = z.cursor
						if !z.r_mark_lAr() {
							z.cursor = z.limit - v_9
							goto lab17_end
						}
						z.bra = z.cursor
						z.sliceDel()
						if !z.r_stem_suffix_chain_before_ki() {
							z.cursor = z.limit - v_9
							goto lab17_end
						}
					}
				lab17_end:
					;
					goto lab14_end
				}
			lab16_end:
				;
				z.cursor = z.limit - v_8
				if !z.r_stem_suffix_chain_before_ki() {
					goto lab11_end
				}
			}
		lab14_end:
			;
			goto lab0_end
		}
	lab11_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			{
				v_10 := z.limit - z.cursor
				{
					if !z.r_mark_ndAn() {
						goto lab20_end
					}
					goto lab19_end
				}
			lab20_end:
				;
				z.cursor = z.limit - v_10
				if !z.r_mark_nU() {
					goto lab18_end
				}
			}
		lab19_end:
			;
			{
				v_11 := z.limit - z.cursor
				{
					if !z.r_mark_sU() {
						goto lab22_end
					}
					z.bra = z.cursor
					z.sliceDel()
					v_12 := z.limit - z.cursor
					{
						z.ket = z.cursor
						if !z.r_mark_lAr() {
							z.cursor = z.limit - v_12
							goto lab23_end
						}
						z.bra = z.cursor
						z.sliceDel()
						if !z.r_stem_suffix_chain_before_ki() {
							z.cursor = z.limit - v_12
							goto lab23_end
						}
					}
				lab23_end:
					;
					goto lab21_end
				}
			lab22_end:
				;
				z.cursor = z.limit - v_11
				if !z.r_mark_lArI() {
					goto lab18_end
				}
			}
		lab21_end:
			;
			goto lab0_end
		}
	lab18_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			if !z.r_mark_DAn() {
				goto lab24_end
			}
			z.bra = z.cursor
			z.sliceDel()
			v_13 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_14 := z.limit - z.cursor
					{
						if !z.r_mark_possessives() {
							goto lab27_end
						}
						z.bra = z.cursor
						z.sliceDel()
						v_15 := z.limit - z.cursor
						{
							z.ket = z.cursor
							if !z.r_mark_lAr() {
								z.cursor = z.limit - v_15
								goto lab28_end
							}
							z.bra = z.cursor
							z.sliceDel()
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_15
								goto lab28_end
							}
						}
					lab28_end:
						;
						goto lab26_end
					}
				lab27_end:
					;
					z.cursor = z.limit - v_14
					{
						if !z.r_mark_lAr() {
							goto lab29_end
						}
						z.bra = z.cursor
						z.sliceDel()
						v_16 := z.limit - z.cursor
						{
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_16
								goto lab30_end
							}
						}
					lab30_end:
						;
						goto lab26_end
					}
				lab29_end:
					;
					z.cursor = z.limit - v_14
					if !z.r_stem_suffix_chain_before_ki() {
						z.cursor = z.limit - v_13
						goto lab25_end
					}
				}
			lab26_end:
			}
		lab25_end:
			;
			goto lab0_end
		}
	lab24_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			{
				v_17 := z.limit - z.cursor
				{
					if !z.r_mark_nUn() {
						goto lab33_end
					}
					goto lab32_end
				}
			lab33_end:
				;
				z.cursor = z.limit - v_17
				if !z.r_mark_ylA() {
					goto lab31_end
				}
			}
		lab32_end:
			;
			z.bra = z.cursor
			z.sliceDel()
			v_18 := z.limit - z.cursor
			{
				{
					v_19 := z.limit - z.cursor
					{
						z.ket = z.cursor
						if !z.r_mark_lAr() {
							goto lab36_end
						}
						z.bra = z.cursor
						z.sliceDel()
						if !z.r_stem_suffix_chain_before_ki() {
							goto lab36_end
						}
						goto lab35_end
					}
				lab36_end:
					;
					z.cursor = z.limit - v_19
					{
						z.ket = z.cursor
						{
							v_20 := z.limit - z.cursor
							{
								if !z.r_mark_possessives() {
									goto lab39_end
								}
								goto lab38_end
							}
						lab39_end:
							;
							z.cursor = z.limit - v_20
							if !z.r_mark_sU() {
								goto lab37_end
							}
						}
					lab38_end:
						;
						z.bra = z.cursor
						z.sliceDel()
						v_21 := z.limit - z.cursor
						{
							z.ket = z.cursor
							if !z.r_mark_lAr() {
								z.cursor = z.limit - v_21
								goto lab40_end
							}
							z.bra = z.cursor
							z.sliceDel()
							if !z.r_stem_suffix_chain_before_ki() {
								z.cursor = z.limit - v_21
								goto lab40_end
							}
						}
					lab40_end:
						;
						goto lab35_end
					}
				lab37_end:
					;
					z.cursor = z.limit - v_19
					if !z.r_stem_suffix_chain_before_ki() {
						z.cursor = z.limit - v_18
						goto lab34_end
					}
				}
			lab35_end:
			}
		lab34_end:
			;
			goto lab0_end
		}
	lab31_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			if !z.r_mark_lArI() {
				goto lab41_end
			}
			z.bra = z.cursor
			z.sliceDel()
			goto lab0_end
		}
	lab41_end:
		;
		z.cursor = z.limit - v_1
		{
			if !z.r_stem_suffix_chain_before_ki() {
				goto lab42_end
			}
			goto lab0_end
		}
	lab42_end:
		;
		z.cursor = z.limit - v_1
		{
			z.ket = z.cursor
			{
				v_22 := z.limit - z.cursor
				{
					if !z.r_mark_DA() {
						goto lab45_end
					}
					goto lab44_end
				}
			lab45_end:
				;
				z.cursor = z.limit - v_22
				{
					if !z.r_mark_yU() {
						goto lab46_end
					}
					goto lab44_end
				}
			lab46_end:
				;
				z.cursor = z.limit - v_22
				if !z.r_mark_yA() {
					goto lab43_end
				}
			}
		lab44_end:
			;
			z.bra = z.cursor
			z.sliceDel()
			v_23 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_24 := z.limit - z.cursor
					{
						if !z.r_mark_possessives() {
							goto lab49_end
						}
						z.bra = z.cursor
						z.sliceDel()
						v_25 := z.limit - z.cursor
						{
							z.ket = z.cursor
							if !z.r_mark_lAr() {
								z.cursor = z.limit - v_25
								goto lab50_end
							}
						}
					lab50_end:
						;
						goto lab48_end
					}
				lab49_end:
					;
					z.cursor = z.limit - v_24
					if !z.r_mark_lAr() {
						z.cursor = z.limit - v_23
						goto lab47_end
					}
				}
			lab48_end:
				;
				z.bra = z.cursor
				z.sliceDel()
				z.ket = z.cursor
				if !z.r_stem_suffix_chain_before_ki() {
					z.cursor = z.limit - v_23
					goto lab47_end
				}
			}
		lab47_end:
			;
			goto lab0_end
		}
	lab43_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		{
			v_26 := z.limit - z.cursor
			{
				if !z.r_mark_possessives() {
					goto lab52_end
				}
				goto lab51_end
			}
		lab52_end:
			;
			z.cursor = z.limit - v_26
			if !z.r_mark_sU() {
				return false
			}
		}
	lab51_end:
		;
		z.bra = z.cursor
		z.sliceDel()
		v_27 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !z.r_mark_lAr() {
				z.cursor = z.limit - v_27
				goto lab53_end
			}
			z.bra = z.cursor
			z.sliceDel()
			if !z.r_stem_suffix_chain_before_ki() {
				z.cursor = z.limit - v_27
				goto lab53_end
			}
		}
	lab53_end:
	}
lab0_end:
	;
	return true
}
func (z *turkishStemmer) r_post_process_last_consonants() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(tur_a_23)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("p")
		break
	case 2:
		z.sliceFrom("\u00E7")
		break
	case 3:
		z.sliceFrom("t")
		break
	case 4:
		z.sliceFrom("k")
		break
	}
	return true
}
func (z *turkishStemmer) r_append_U_to_stems_ending_with_d_or_g() bool {
	z.ket = z.cursor
	z.bra = z.cursor
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("d")) {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		if !(z.eqSB("g")) {
			return false
		}
	}
lab0_end:
	;
	if !z.goOutGroupingB(tur_g_vowel, 97, 305) {
		return false
	}
	{
		v_2 := z.limit - z.cursor
		{
			{
				v_3 := z.limit - z.cursor
				{
					if !(z.eqSB("a")) {
						goto lab5_end
					}
					goto lab4_end
				}
			lab5_end:
				;
				z.cursor = z.limit - v_3
				if !(z.eqSB("\u0131")) {
					goto lab3_end
				}
			}
		lab4_end:
			;
			z.sliceFrom("\u0131")
			goto lab2_end
		}
	lab3_end:
		;
		z.cursor = z.limit - v_2
		{
			{
				v_4 := z.limit - z.cursor
				{
					if !(z.eqSB("e")) {
						goto lab8_end
					}
					goto lab7_end
				}
			lab8_end:
				;
				z.cursor = z.limit - v_4
				if !(z.eqSB("i")) {
					goto lab6_end
				}
			}
		lab7_end:
			;
			z.sliceFrom("i")
			goto lab2_end
		}
	lab6_end:
		;
		z.cursor = z.limit - v_2
		{
			{
				v_5 := z.limit - z.cursor
				{
					if !(z.eqSB("o")) {
						goto lab11_end
					}
					goto lab10_end
				}
			lab11_end:
				;
				z.cursor = z.limit - v_5
				if !(z.eqSB("u")) {
					goto lab9_end
				}
			}
		lab10_end:
			;
			z.sliceFrom("u")
			goto lab2_end
		}
	lab9_end:
		;
		z.cursor = z.limit - v_2
		{
			v_6 := z.limit - z.cursor
			{
				if !(z.eqSB("\u00F6")) {
					goto lab13_end
				}
				goto lab12_end
			}
		lab13_end:
			;
			z.cursor = z.limit - v_6
			if !(z.eqSB("\u00FC")) {
				return false
			}
		}
	lab12_end:
		;
		z.sliceFrom("\u00FC")
	}
lab2_end:
	;
	return true
}
func (z *turkishStemmer) r_is_reserved_word() bool {
	if !(z.eqSB("ad")) {
		return false
	}
	v_1 := z.limit - z.cursor
	{
		if !(z.eqSB("soy")) {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
	}
lab0_end:
	;
	if z.cursor > z.limitBackward {
		return false
	}
	return true
}
func (z *turkishStemmer) r_remove_proper_noun_suffix() bool {
	v_1 := z.cursor
	{
		z.bra = z.cursor
	golab1:
		for {
			v_2 := z.cursor
			{
				{
					v_3 := z.cursor
					{
						if !(z.eqS("'")) {
							goto lab3_end
						}
						goto lab2_end
					}
				lab3_end:
					;
					z.cursor = v_3
				}
				z.cursor = v_2
				break golab1
			}
		lab2_end:
			;
			z.cursor = v_2
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		}
		z.ket = z.cursor
		z.sliceDel()
	}
lab0_end:
	;
	z.cursor = v_1
	v_4 := z.cursor
	{
		{
			c := z.cursor + 2
			if c > z.limit {
				goto lab4_end
			}
			z.cursor = c
		}
	golab5:
		for {
			v_5 := z.cursor
			{
				if !(z.eqS("'")) {
					goto lab6_end
				}
				z.cursor = v_5
				break golab5
			}
		lab6_end:
			;
			z.cursor = v_5
			if z.cursor >= z.limit {
				goto lab4_end
			}
			z.cursor++
		}
		z.bra = z.cursor
		z.cursor = z.limit
		z.ket = z.cursor
		z.sliceDel()
	}
lab4_end:
	;
	z.cursor = v_4
	return true
}
func (z *turkishStemmer) r_more_than_one_syllable_word() bool {
	v_1 := z.cursor
	for v_2 := 2; v_2 > 0; v_2-- {
		if !z.goOutGrouping(tur_g_vowel, 97, 305) {
			return false
		}
		z.cursor++
	}
	z.cursor = v_1
	return true
}
func (z *turkishStemmer) r_postlude() bool {
	z.limitBackward = z.cursor
	z.cursor = z.limit
	{
		v_1 := z.limit - z.cursor
		{
			if !z.r_is_reserved_word() {
				goto lab0_end
			}
			return false
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
	}
	v_2 := z.limit - z.cursor
	z.r_append_U_to_stems_ending_with_d_or_g()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_post_process_last_consonants()
	z.cursor = z.limit - v_3
	z.cursor = z.limitBackward
	return true
}
func (z *turkishStemmer) stem() {
	z.r_remove_proper_noun_suffix()
	if !z.r_more_than_one_syllable_word() {
		return
	}
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_stem_nominal_verb_suffixes()
	z.cursor = z.limit - v_1
	if !z.B_continue_stemming_noun_suffixes {
		return
	}
	v_2 := z.limit - z.cursor
	z.r_stem_noun_suffixes()
	z.cursor = z.limit - v_2
	z.cursor = z.limitBackward
	z.r_postlude()
	return
}

// Snowball 2.2.0 stemmers re-translated from Apache Lucene 10.5.1's
// org.tartarus.snowball.ext sources (Dutch, English, French, German,
// Greek, Hungarian, Lithuanian, Norwegian, Swedish), replacing translations
// of a newer Snowball release that differ from Lucene.

type dutchStemmer struct {
	snowball
	I_p2      int
	I_p1      int
	B_e_found bool
}

func newDutch() *dutchStemmer { return &dutchStemmer{} }

var dut_a_0 = []among{
	{"", -1, 6, ""},
	{"\u00E1", 0, 1, ""},
	{"\u00E4", 0, 1, ""},
	{"\u00E9", 0, 2, ""},
	{"\u00EB", 0, 2, ""},
	{"\u00ED", 0, 3, ""},
	{"\u00EF", 0, 3, ""},
	{"\u00F3", 0, 4, ""},
	{"\u00F6", 0, 4, ""},
	{"\u00FA", 0, 5, ""},
	{"\u00FC", 0, 5, ""},
}
var dut_a_1 = []among{
	{"", -1, 3, ""},
	{"I", 0, 2, ""},
	{"Y", 0, 1, ""},
}
var dut_a_2 = []among{
	{"dd", -1, -1, ""},
	{"kk", -1, -1, ""},
	{"tt", -1, -1, ""},
}
var dut_a_3 = []among{
	{"ene", -1, 2, ""},
	{"se", -1, 3, ""},
	{"en", -1, 2, ""},
	{"heden", 2, 1, ""},
	{"s", -1, 3, ""},
}
var dut_a_4 = []among{
	{"end", -1, 1, ""},
	{"ig", -1, 2, ""},
	{"ing", -1, 1, ""},
	{"lijk", -1, 3, ""},
	{"baar", -1, 4, ""},
	{"bar", -1, 5, ""},
}
var dut_a_5 = []among{
	{"aa", -1, -1, ""},
	{"ee", -1, -1, ""},
	{"oo", -1, -1, ""},
	{"uu", -1, -1, ""},
}
var dut_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}
var dut_g_v_I = []byte{1, 0, 0, 17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}
var dut_g_v_j = []byte{17, 67, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}

func (z *dutchStemmer) r_prelude() bool {
	var among_var int
	v_1 := z.cursor
	for {
		v_2 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(dut_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("a")
				break
			case 2:
				z.sliceFrom("e")
				break
			case 3:
				z.sliceFrom("i")
				break
			case 4:
				z.sliceFrom("o")
				break
			case 5:
				z.sliceFrom("u")
				break
			case 6:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_2
		break
	}
	z.cursor = v_1
	v_3 := z.cursor
	{
		z.bra = z.cursor
		if !(z.eqS("y")) {
			z.cursor = v_3
			goto lab1_end
		}
		z.ket = z.cursor
		z.sliceFrom("Y")
	}
lab1_end:
	;
	for {
		v_4 := z.cursor
		{
		golab3:
			for {
				v_5 := z.cursor
				{
					if !(z.inGrouping(dut_g_v, 97, 232)) {
						goto lab4_end
					}
					z.bra = z.cursor
					{
						v_6 := z.cursor
						{
							if !(z.eqS("i")) {
								goto lab6_end
							}
							z.ket = z.cursor
							if !(z.inGrouping(dut_g_v, 97, 232)) {
								goto lab6_end
							}
							z.sliceFrom("I")
							goto lab5_end
						}
					lab6_end:
						;
						z.cursor = v_6
						if !(z.eqS("y")) {
							goto lab4_end
						}
						z.ket = z.cursor
						z.sliceFrom("Y")
					}
				lab5_end:
					;
					z.cursor = v_5
					break golab3
				}
			lab4_end:
				;
				z.cursor = v_5
				if z.cursor >= z.limit {
					goto lab2_end
				}
				z.cursor++
			}
			continue
		}
	lab2_end:
		;
		z.cursor = v_4
		break
	}
	return true
}
func (z *dutchStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	z.I_p2 = z.limit
golab0:
	for {
		{
			if !(z.inGrouping(dut_g_v, 97, 232)) {
				goto lab1_end
			}
			break golab0
		}
	lab1_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab2:
	for {
		{
			if !(z.outGrouping(dut_g_v, 97, 232)) {
				goto lab3_end
			}
			break golab2
		}
	lab3_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= 3 {
			goto lab4_end
		}
		z.I_p1 = 3
	}
lab4_end:
	;
golab5:
	for {
		{
			if !(z.inGrouping(dut_g_v, 97, 232)) {
				goto lab6_end
			}
			break golab5
		}
	lab6_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab7:
	for {
		{
			if !(z.outGrouping(dut_g_v, 97, 232)) {
				goto lab8_end
			}
			break golab7
		}
	lab8_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p2 = z.cursor
	return true
}
func (z *dutchStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(dut_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("y")
				break
			case 2:
				z.sliceFrom("i")
				break
			case 3:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *dutchStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *dutchStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *dutchStemmer) r_undouble() bool {
	v_1 := z.limit - z.cursor
	if z.findAmongB(dut_a_2) == 0 {
		return false
	}
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if z.cursor <= z.limitBackward {
		return false
	}
	z.cursor--
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *dutchStemmer) r_e_ending() bool {
	z.B_e_found = false
	z.ket = z.cursor
	if !(z.eqSB("e")) {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	v_1 := z.limit - z.cursor
	if !(z.outGroupingB(dut_g_v, 97, 232)) {
		return false
	}
	z.cursor = z.limit - v_1
	z.sliceDel()
	z.B_e_found = true
	if !z.r_undouble() {
		return false
	}
	return true
}
func (z *dutchStemmer) r_en_ending() bool {
	if !z.r_R1() {
		return false
	}
	v_1 := z.limit - z.cursor
	if !(z.outGroupingB(dut_g_v, 97, 232)) {
		return false
	}
	z.cursor = z.limit - v_1
	{
		v_2 := z.limit - z.cursor
		{
			if !(z.eqSB("gem")) {
				goto lab0_end
			}
			return false
		}
	lab0_end:
		;
		z.cursor = z.limit - v_2
	}
	z.sliceDel()
	if !z.r_undouble() {
		return false
	}
	return true
}
func (z *dutchStemmer) r_standard_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(dut_a_3)
		if among_var == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			if !z.r_R1() {
				goto lab0_end
			}
			z.sliceFrom("heid")
			break
		case 2:
			if !z.r_en_ending() {
				goto lab0_end
			}
			break
		case 3:
			if !z.r_R1() {
				goto lab0_end
			}
			if !(z.outGroupingB(dut_g_v_j, 97, 232)) {
				goto lab0_end
			}
			z.sliceDel()
			break
		}
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	z.r_e_ending()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("heid")) {
			goto lab1_end
		}
		z.bra = z.cursor
		if !z.r_R2() {
			goto lab1_end
		}
		{
			v_4 := z.limit - z.cursor
			{
				if !(z.eqSB("c")) {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_4
		}
		z.sliceDel()
		z.ket = z.cursor
		if !(z.eqSB("en")) {
			goto lab1_end
		}
		z.bra = z.cursor
		if !z.r_en_ending() {
			goto lab1_end
		}
	}
lab1_end:
	;
	z.cursor = z.limit - v_3
	v_5 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(dut_a_4)
		if among_var == 0 {
			goto lab3_end
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			if !z.r_R2() {
				goto lab3_end
			}
			z.sliceDel()
			{
				v_6 := z.limit - z.cursor
				{
					z.ket = z.cursor
					if !(z.eqSB("ig")) {
						goto lab5_end
					}
					z.bra = z.cursor
					if !z.r_R2() {
						goto lab5_end
					}
					{
						v_7 := z.limit - z.cursor
						{
							if !(z.eqSB("e")) {
								goto lab6_end
							}
							goto lab5_end
						}
					lab6_end:
						;
						z.cursor = z.limit - v_7
					}
					z.sliceDel()
					goto lab4_end
				}
			lab5_end:
				;
				z.cursor = z.limit - v_6
				if !z.r_undouble() {
					goto lab3_end
				}
			}
		lab4_end:
			;
			break
		case 2:
			if !z.r_R2() {
				goto lab3_end
			}
			{
				v_8 := z.limit - z.cursor
				{
					if !(z.eqSB("e")) {
						goto lab7_end
					}
					goto lab3_end
				}
			lab7_end:
				;
				z.cursor = z.limit - v_8
			}
			z.sliceDel()
			break
		case 3:
			if !z.r_R2() {
				goto lab3_end
			}
			z.sliceDel()
			if !z.r_e_ending() {
				goto lab3_end
			}
			break
		case 4:
			if !z.r_R2() {
				goto lab3_end
			}
			z.sliceDel()
			break
		case 5:
			if !z.r_R2() {
				goto lab3_end
			}
			if !(z.B_e_found) {
				goto lab3_end
			}
			z.sliceDel()
			break
		}
	}
lab3_end:
	;
	z.cursor = z.limit - v_5
	v_9 := z.limit - z.cursor
	{
		if !(z.outGroupingB(dut_g_v_I, 73, 232)) {
			goto lab8_end
		}
		v_10 := z.limit - z.cursor
		if z.findAmongB(dut_a_5) == 0 {
			goto lab8_end
		}
		if !(z.outGroupingB(dut_g_v, 97, 232)) {
			goto lab8_end
		}
		z.cursor = z.limit - v_10
		z.ket = z.cursor
		if z.cursor <= z.limitBackward {
			goto lab8_end
		}
		z.cursor--
		z.bra = z.cursor
		z.sliceDel()
	}
lab8_end:
	;
	z.cursor = z.limit - v_9
	return true
}
func (z *dutchStemmer) stem() {
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	v_2 := z.cursor
	z.r_mark_regions()
	z.cursor = v_2
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.r_standard_suffix()
	z.cursor = z.limitBackward
	v_4 := z.cursor
	z.r_postlude()
	z.cursor = v_4
	return
}

type englishStemmer struct {
	snowball
	B_Y_found bool
	I_p2      int
	I_p1      int
}

func newEnglish() *englishStemmer { return &englishStemmer{} }

var eng_a_0 = []among{
	{"arsen", -1, -1, ""},
	{"commun", -1, -1, ""},
	{"gener", -1, -1, ""},
}
var eng_a_1 = []among{
	{"'", -1, 1, ""},
	{"'s'", 0, 1, ""},
	{"'s", -1, 1, ""},
}
var eng_a_2 = []among{
	{"ied", -1, 2, ""},
	{"s", -1, 3, ""},
	{"ies", 1, 2, ""},
	{"sses", 1, 1, ""},
	{"ss", 1, -1, ""},
	{"us", 1, -1, ""},
}
var eng_a_3 = []among{
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
var eng_a_4 = []among{
	{"ed", -1, 2, ""},
	{"eed", 0, 1, ""},
	{"ing", -1, 2, ""},
	{"edly", -1, 2, ""},
	{"eedly", 3, 1, ""},
	{"ingly", -1, 2, ""},
}
var eng_a_5 = []among{
	{"anci", -1, 3, ""},
	{"enci", -1, 2, ""},
	{"ogi", -1, 13, ""},
	{"li", -1, 15, ""},
	{"bli", 3, 12, ""},
	{"abli", 4, 4, ""},
	{"alli", 3, 8, ""},
	{"fulli", 3, 9, ""},
	{"lessli", 3, 14, ""},
	{"ousli", 3, 10, ""},
	{"entli", 3, 5, ""},
	{"aliti", -1, 8, ""},
	{"biliti", -1, 12, ""},
	{"iviti", -1, 11, ""},
	{"tional", -1, 1, ""},
	{"ational", 14, 7, ""},
	{"alism", -1, 8, ""},
	{"ation", -1, 7, ""},
	{"ization", 17, 6, ""},
	{"izer", -1, 6, ""},
	{"ator", -1, 7, ""},
	{"iveness", -1, 11, ""},
	{"fulness", -1, 9, ""},
	{"ousness", -1, 10, ""},
}
var eng_a_6 = []among{
	{"icate", -1, 4, ""},
	{"ative", -1, 6, ""},
	{"alize", -1, 3, ""},
	{"iciti", -1, 4, ""},
	{"ical", -1, 4, ""},
	{"tional", -1, 1, ""},
	{"ational", 5, 2, ""},
	{"ful", -1, 5, ""},
	{"ness", -1, 5, ""},
}
var eng_a_7 = []among{
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
}
var eng_a_8 = []among{
	{"e", -1, 1, ""},
	{"l", -1, 2, ""},
}
var eng_a_9 = []among{
	{"succeed", -1, -1, ""},
	{"proceed", -1, -1, ""},
	{"exceed", -1, -1, ""},
	{"canning", -1, -1, ""},
	{"inning", -1, -1, ""},
	{"earring", -1, -1, ""},
	{"herring", -1, -1, ""},
	{"outing", -1, -1, ""},
}
var eng_a_10 = []among{
	{"andes", -1, -1, ""},
	{"atlas", -1, -1, ""},
	{"bias", -1, -1, ""},
	{"cosmos", -1, -1, ""},
	{"dying", -1, 3, ""},
	{"early", -1, 9, ""},
	{"gently", -1, 7, ""},
	{"howe", -1, -1, ""},
	{"idly", -1, 6, ""},
	{"lying", -1, 4, ""},
	{"news", -1, -1, ""},
	{"only", -1, 10, ""},
	{"singly", -1, 11, ""},
	{"skies", -1, 2, ""},
	{"skis", -1, 1, ""},
	{"sky", -1, -1, ""},
	{"tying", -1, 5, ""},
	{"ugly", -1, 8, ""},
}
var eng_g_aeo = []byte{17, 64}
var eng_g_v = []byte{17, 65, 16, 1}
var eng_g_v_WXY = []byte{1, 17, 65, 208, 1}
var eng_g_valid_LI = []byte{55, 141, 2}

func (z *englishStemmer) r_prelude() bool {
	z.B_Y_found = false
	v_1 := z.cursor
	{
		z.bra = z.cursor
		if !(z.eqS("'")) {
			goto lab0_end
		}
		z.ket = z.cursor
		z.sliceDel()
	}
lab0_end:
	;
	z.cursor = v_1
	v_2 := z.cursor
	{
		z.bra = z.cursor
		if !(z.eqS("y")) {
			goto lab1_end
		}
		z.ket = z.cursor
		z.sliceFrom("Y")
		z.B_Y_found = true
	}
lab1_end:
	;
	z.cursor = v_2
	v_3 := z.cursor
	{
		for {
			v_4 := z.cursor
			{
			golab4:
				for {
					v_5 := z.cursor
					{
						if !(z.inGrouping(eng_g_v, 97, 121)) {
							goto lab5_end
						}
						z.bra = z.cursor
						if !(z.eqS("y")) {
							goto lab5_end
						}
						z.ket = z.cursor
						z.cursor = v_5
						break golab4
					}
				lab5_end:
					;
					z.cursor = v_5
					if z.cursor >= z.limit {
						goto lab3_end
					}
					z.cursor++
				}
				z.sliceFrom("Y")
				z.B_Y_found = true
				continue
			}
		lab3_end:
			;
			z.cursor = v_4
			break
		}
	}
	z.cursor = v_3
	return true
}
func (z *englishStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if z.findAmong(eng_a_0) == 0 {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
		golab3:
			for {
				{
					if !(z.inGrouping(eng_g_v, 97, 121)) {
						goto lab4_end
					}
					break golab3
				}
			lab4_end:
				;
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		golab5:
			for {
				{
					if !(z.outGrouping(eng_g_v, 97, 121)) {
						goto lab6_end
					}
					break golab5
				}
			lab6_end:
				;
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		}
	lab1_end:
		;
		z.I_p1 = z.cursor
	golab7:
		for {
			{
				if !(z.inGrouping(eng_g_v, 97, 121)) {
					goto lab8_end
				}
				break golab7
			}
		lab8_end:
			;
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		}
	golab9:
		for {
			{
				if !(z.outGrouping(eng_g_v, 97, 121)) {
					goto lab10_end
				}
				break golab9
			}
		lab10_end:
			;
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		}
		z.I_p2 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	return true
}
func (z *englishStemmer) r_shortv() bool {
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.outGroupingB(eng_g_v_WXY, 89, 121)) {
				goto lab1_end
			}
			if !(z.inGroupingB(eng_g_v, 97, 121)) {
				goto lab1_end
			}
			if !(z.outGroupingB(eng_g_v, 97, 121)) {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		if !(z.outGroupingB(eng_g_v, 97, 121)) {
			return false
		}
		if !(z.inGroupingB(eng_g_v, 97, 121)) {
			return false
		}
		if z.cursor > z.limitBackward {
			return false
		}
	}
lab0_end:
	;
	return true
}
func (z *englishStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *englishStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *englishStemmer) r_Step_1a() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(eng_a_1) == 0 {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceDel()
	}
lab0_end:
	;
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("ss")
		break
	case 2:
		{
			v_2 := z.limit - z.cursor
			{
				{
					c := z.cursor - 2
					if c < z.limitBackward {
						goto lab2_end
					}
					z.cursor = c
				}
				z.sliceFrom("i")
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_2
			z.sliceFrom("ie")
		}
	lab1_end:
		;
		break
	case 3:
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
	golab3:
		for {
			{
				if !(z.inGroupingB(eng_g_v, 97, 121)) {
					goto lab4_end
				}
				break golab3
			}
		lab4_end:
			;
			if z.cursor <= z.limitBackward {
				return false
			}
			z.cursor--
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *englishStemmer) r_Step_1b() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_4)
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
				if !(z.inGroupingB(eng_g_v, 97, 121)) {
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
		z.ket = z.cursor
		z.bra = z.cursor
		v_3 := z.limit - z.cursor
		among_var = z.findAmongB(eng_a_3)
		switch among_var {
		case 1:
			z.sliceFrom("e")
			return false
		case 2:
			{
				v_4 := z.limit - z.cursor
				{
					if !(z.inGroupingB(eng_g_aeo, 97, 111)) {
						goto lab2_end
					}
					if z.cursor > z.limitBackward {
						goto lab2_end
					}
					return false
				}
			lab2_end:
				;
				z.cursor = z.limit - v_4
			}
			break
		case 3:
			if z.cursor != z.I_p1 {
				return false
			}
			v_5 := z.limit - z.cursor
			if !z.r_shortv() {
				return false
			}
			z.cursor = z.limit - v_5
			z.sliceFrom("e")
			return false
		}
		z.cursor = z.limit - v_3
		z.ket = z.cursor
		if z.cursor <= z.limitBackward {
			return false
		}
		z.cursor--
		z.bra = z.cursor
		z.sliceDel()
		break
	}
	return true
}
func (z *englishStemmer) r_Step_1c() bool {
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
	if !(z.outGroupingB(eng_g_v, 97, 121)) {
		return false
	}
	{
		if z.cursor > z.limitBackward {
			goto lab2_end
		}
		return false
	}
lab2_end:
	;
	z.sliceFrom("i")
	return true
}
func (z *englishStemmer) r_Step_2() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_5)
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
		z.sliceFrom("ize")
		break
	case 7:
		z.sliceFrom("ate")
		break
	case 8:
		z.sliceFrom("al")
		break
	case 9:
		z.sliceFrom("ful")
		break
	case 10:
		z.sliceFrom("ous")
		break
	case 11:
		z.sliceFrom("ive")
		break
	case 12:
		z.sliceFrom("ble")
		break
	case 13:
		if !(z.eqSB("l")) {
			return false
		}
		z.sliceFrom("og")
		break
	case 14:
		z.sliceFrom("less")
		break
	case 15:
		if !(z.inGroupingB(eng_g_valid_LI, 99, 116)) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *englishStemmer) r_Step_3() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_6)
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
		z.sliceFrom("ate")
		break
	case 3:
		z.sliceFrom("al")
		break
	case 4:
		z.sliceFrom("ic")
		break
	case 5:
		z.sliceDel()
		break
	case 6:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *englishStemmer) r_Step_4() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_7)
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
func (z *englishStemmer) r_Step_5() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(eng_a_8)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
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
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		if !(z.eqSB("l")) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *englishStemmer) r_exception2() bool {
	z.ket = z.cursor
	if z.findAmongB(eng_a_9) == 0 {
		return false
	}
	z.bra = z.cursor
	if z.cursor > z.limitBackward {
		return false
	}
	return true
}
func (z *englishStemmer) r_exception1() bool {
	var among_var int
	z.bra = z.cursor
	among_var = z.findAmong(eng_a_10)
	if among_var == 0 {
		return false
	}
	z.ket = z.cursor
	if z.cursor < z.limit {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("ski")
		break
	case 2:
		z.sliceFrom("sky")
		break
	case 3:
		z.sliceFrom("die")
		break
	case 4:
		z.sliceFrom("lie")
		break
	case 5:
		z.sliceFrom("tie")
		break
	case 6:
		z.sliceFrom("idl")
		break
	case 7:
		z.sliceFrom("gentl")
		break
	case 8:
		z.sliceFrom("ugli")
		break
	case 9:
		z.sliceFrom("earli")
		break
	case 10:
		z.sliceFrom("onli")
		break
	case 11:
		z.sliceFrom("singl")
		break
	}
	return true
}
func (z *englishStemmer) r_postlude() bool {
	if !(z.B_Y_found) {
		return false
	}
	for {
		v_1 := z.cursor
		{
		golab1:
			for {
				v_2 := z.cursor
				{
					z.bra = z.cursor
					if !(z.eqS("Y")) {
						goto lab2_end
					}
					z.ket = z.cursor
					z.cursor = v_2
					break golab1
				}
			lab2_end:
				;
				z.cursor = v_2
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
			z.sliceFrom("y")
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *englishStemmer) stem() {
	{
		v_1 := z.cursor
		{
			if !z.r_exception1() {
				goto lab1_end
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = v_1
		{
			{
				v_2 := z.cursor
				{
					{
						c := z.cursor + 3
						if c > z.limit {
							goto lab3_end
						}
						z.cursor = c
					}
					goto lab2_end
				}
			lab3_end:
				;
				z.cursor = v_2
			}
			goto lab0_end
		}
	lab2_end:
		;
		z.cursor = v_1
		z.r_prelude()
		z.r_mark_regions()
		z.limitBackward = z.cursor
		z.cursor = z.limit
		v_5 := z.limit - z.cursor
		z.r_Step_1a()
		z.cursor = z.limit - v_5
		{
			v_6 := z.limit - z.cursor
			{
				if !z.r_exception2() {
					goto lab5_end
				}
				goto lab4_end
			}
		lab5_end:
			;
			z.cursor = z.limit - v_6
			v_7 := z.limit - z.cursor
			z.r_Step_1b()
			z.cursor = z.limit - v_7
			v_8 := z.limit - z.cursor
			z.r_Step_1c()
			z.cursor = z.limit - v_8
			v_9 := z.limit - z.cursor
			z.r_Step_2()
			z.cursor = z.limit - v_9
			v_10 := z.limit - z.cursor
			z.r_Step_3()
			z.cursor = z.limit - v_10
			v_11 := z.limit - z.cursor
			z.r_Step_4()
			z.cursor = z.limit - v_11
			v_12 := z.limit - z.cursor
			z.r_Step_5()
			z.cursor = z.limit - v_12
		}
	lab4_end:
		;
		z.cursor = z.limitBackward
		v_13 := z.cursor
		z.r_postlude()
		z.cursor = v_13
	}
lab0_end:
	;
	return
}

type frenchStemmer struct {
	snowball
	I_p2 int
	I_p1 int
	I_pV int
}

func newFrench() *frenchStemmer { return &frenchStemmer{} }

var fre_a_0 = []among{
	{"col", -1, -1, ""},
	{"par", -1, -1, ""},
	{"tap", -1, -1, ""},
}
var fre_a_1 = []among{
	{"", -1, 7, ""},
	{"H", 0, 6, ""},
	{"He", 1, 4, ""},
	{"Hi", 1, 5, ""},
	{"I", 0, 1, ""},
	{"U", 0, 2, ""},
	{"Y", 0, 3, ""},
}
var fre_a_2 = []among{
	{"iqU", -1, 3, ""},
	{"abl", -1, 3, ""},
	{"I\u00E8r", -1, 4, ""},
	{"i\u00E8r", -1, 4, ""},
	{"eus", -1, 2, ""},
	{"iv", -1, 1, ""},
}
var fre_a_3 = []among{
	{"ic", -1, 2, ""},
	{"abil", -1, 1, ""},
	{"iv", -1, 3, ""},
}
var fre_a_4 = []among{
	{"iqUe", -1, 1, ""},
	{"atrice", -1, 2, ""},
	{"ance", -1, 1, ""},
	{"ence", -1, 5, ""},
	{"logie", -1, 3, ""},
	{"able", -1, 1, ""},
	{"isme", -1, 1, ""},
	{"euse", -1, 11, ""},
	{"iste", -1, 1, ""},
	{"ive", -1, 8, ""},
	{"if", -1, 8, ""},
	{"usion", -1, 4, ""},
	{"ation", -1, 2, ""},
	{"ution", -1, 4, ""},
	{"ateur", -1, 2, ""},
	{"iqUes", -1, 1, ""},
	{"atrices", -1, 2, ""},
	{"ances", -1, 1, ""},
	{"ences", -1, 5, ""},
	{"logies", -1, 3, ""},
	{"ables", -1, 1, ""},
	{"ismes", -1, 1, ""},
	{"euses", -1, 11, ""},
	{"istes", -1, 1, ""},
	{"ives", -1, 8, ""},
	{"ifs", -1, 8, ""},
	{"usions", -1, 4, ""},
	{"ations", -1, 2, ""},
	{"utions", -1, 4, ""},
	{"ateurs", -1, 2, ""},
	{"ments", -1, 15, ""},
	{"ements", 30, 6, ""},
	{"issements", 31, 12, ""},
	{"it\u00E9s", -1, 7, ""},
	{"ment", -1, 15, ""},
	{"ement", 34, 6, ""},
	{"issement", 35, 12, ""},
	{"amment", 34, 13, ""},
	{"emment", 34, 14, ""},
	{"aux", -1, 10, ""},
	{"eaux", 39, 9, ""},
	{"eux", -1, 1, ""},
	{"it\u00E9", -1, 7, ""},
}
var fre_a_5 = []among{
	{"ira", -1, 1, ""},
	{"ie", -1, 1, ""},
	{"isse", -1, 1, ""},
	{"issante", -1, 1, ""},
	{"i", -1, 1, ""},
	{"irai", 4, 1, ""},
	{"ir", -1, 1, ""},
	{"iras", -1, 1, ""},
	{"ies", -1, 1, ""},
	{"\u00EEmes", -1, 1, ""},
	{"isses", -1, 1, ""},
	{"issantes", -1, 1, ""},
	{"\u00EEtes", -1, 1, ""},
	{"is", -1, 1, ""},
	{"irais", 13, 1, ""},
	{"issais", 13, 1, ""},
	{"irions", -1, 1, ""},
	{"issions", -1, 1, ""},
	{"irons", -1, 1, ""},
	{"issons", -1, 1, ""},
	{"issants", -1, 1, ""},
	{"it", -1, 1, ""},
	{"irait", 21, 1, ""},
	{"issait", 21, 1, ""},
	{"issant", -1, 1, ""},
	{"iraIent", -1, 1, ""},
	{"issaIent", -1, 1, ""},
	{"irent", -1, 1, ""},
	{"issent", -1, 1, ""},
	{"iront", -1, 1, ""},
	{"\u00EEt", -1, 1, ""},
	{"iriez", -1, 1, ""},
	{"issiez", -1, 1, ""},
	{"irez", -1, 1, ""},
	{"issez", -1, 1, ""},
}
var fre_a_6 = []among{
	{"a", -1, 3, ""},
	{"era", 0, 2, ""},
	{"asse", -1, 3, ""},
	{"ante", -1, 3, ""},
	{"\u00E9e", -1, 2, ""},
	{"ai", -1, 3, ""},
	{"erai", 5, 2, ""},
	{"er", -1, 2, ""},
	{"as", -1, 3, ""},
	{"eras", 8, 2, ""},
	{"\u00E2mes", -1, 3, ""},
	{"asses", -1, 3, ""},
	{"antes", -1, 3, ""},
	{"\u00E2tes", -1, 3, ""},
	{"\u00E9es", -1, 2, ""},
	{"ais", -1, 3, ""},
	{"erais", 15, 2, ""},
	{"ions", -1, 1, ""},
	{"erions", 17, 2, ""},
	{"assions", 17, 3, ""},
	{"erons", -1, 2, ""},
	{"ants", -1, 3, ""},
	{"\u00E9s", -1, 2, ""},
	{"ait", -1, 3, ""},
	{"erait", 23, 2, ""},
	{"ant", -1, 3, ""},
	{"aIent", -1, 3, ""},
	{"eraIent", 26, 2, ""},
	{"\u00E8rent", -1, 2, ""},
	{"assent", -1, 3, ""},
	{"eront", -1, 2, ""},
	{"\u00E2t", -1, 3, ""},
	{"ez", -1, 2, ""},
	{"iez", 32, 2, ""},
	{"eriez", 33, 2, ""},
	{"assiez", 33, 3, ""},
	{"erez", 32, 2, ""},
	{"\u00E9", -1, 2, ""},
}
var fre_a_7 = []among{
	{"e", -1, 3, ""},
	{"I\u00E8re", 0, 2, ""},
	{"i\u00E8re", 0, 2, ""},
	{"ion", -1, 1, ""},
	{"Ier", -1, 2, ""},
	{"ier", -1, 2, ""},
}
var fre_a_8 = []among{
	{"ell", -1, -1, ""},
	{"eill", -1, -1, ""},
	{"enn", -1, -1, ""},
	{"onn", -1, -1, ""},
	{"ett", -1, -1, ""},
}
var fre_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128, 130, 103, 8, 5}
var fre_g_keep_with_s = []byte{1, 65, 20, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 128}

func (z *frenchStemmer) r_prelude() bool {
	for {
		v_1 := z.cursor
		{
		golab1:
			for {
				v_2 := z.cursor
				{
					{
						v_3 := z.cursor
						{
							if !(z.inGrouping(fre_g_v, 97, 251)) {
								goto lab4_end
							}
							z.bra = z.cursor
							{
								v_4 := z.cursor
								{
									if !(z.eqS("u")) {
										goto lab6_end
									}
									z.ket = z.cursor
									if !(z.inGrouping(fre_g_v, 97, 251)) {
										goto lab6_end
									}
									z.sliceFrom("U")
									goto lab5_end
								}
							lab6_end:
								;
								z.cursor = v_4
								{
									if !(z.eqS("i")) {
										goto lab7_end
									}
									z.ket = z.cursor
									if !(z.inGrouping(fre_g_v, 97, 251)) {
										goto lab7_end
									}
									z.sliceFrom("I")
									goto lab5_end
								}
							lab7_end:
								;
								z.cursor = v_4
								if !(z.eqS("y")) {
									goto lab4_end
								}
								z.ket = z.cursor
								z.sliceFrom("Y")
							}
						lab5_end:
							;
							goto lab3_end
						}
					lab4_end:
						;
						z.cursor = v_3
						{
							z.bra = z.cursor
							if !(z.eqS("\u00EB")) {
								goto lab8_end
							}
							z.ket = z.cursor
							z.sliceFrom("He")
							goto lab3_end
						}
					lab8_end:
						;
						z.cursor = v_3
						{
							z.bra = z.cursor
							if !(z.eqS("\u00EF")) {
								goto lab9_end
							}
							z.ket = z.cursor
							z.sliceFrom("Hi")
							goto lab3_end
						}
					lab9_end:
						;
						z.cursor = v_3
						{
							z.bra = z.cursor
							if !(z.eqS("y")) {
								goto lab10_end
							}
							z.ket = z.cursor
							if !(z.inGrouping(fre_g_v, 97, 251)) {
								goto lab10_end
							}
							z.sliceFrom("Y")
							goto lab3_end
						}
					lab10_end:
						;
						z.cursor = v_3
						if !(z.eqS("q")) {
							goto lab2_end
						}
						z.bra = z.cursor
						if !(z.eqS("u")) {
							goto lab2_end
						}
						z.ket = z.cursor
						z.sliceFrom("U")
					}
				lab3_end:
					;
					z.cursor = v_2
					break golab1
				}
			lab2_end:
				;
				z.cursor = v_2
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *frenchStemmer) r_mark_regions() bool {
	z.I_pV = z.limit
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		{
			v_2 := z.cursor
			{
				if !(z.inGrouping(fre_g_v, 97, 251)) {
					goto lab2_end
				}
				if !(z.inGrouping(fre_g_v, 97, 251)) {
					goto lab2_end
				}
				if z.cursor >= z.limit {
					goto lab2_end
				}
				z.cursor++
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = v_2
			{
				if z.findAmong(fre_a_0) == 0 {
					goto lab3_end
				}
				goto lab1_end
			}
		lab3_end:
			;
			z.cursor = v_2
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		golab4:
			for {
				{
					if !(z.inGrouping(fre_g_v, 97, 251)) {
						goto lab5_end
					}
					break golab4
				}
			lab5_end:
				;
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
		}
	lab1_end:
		;
		z.I_pV = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	v_4 := z.cursor
	{
	golab7:
		for {
			{
				if !(z.inGrouping(fre_g_v, 97, 251)) {
					goto lab8_end
				}
				break golab7
			}
		lab8_end:
			;
			if z.cursor >= z.limit {
				goto lab6_end
			}
			z.cursor++
		}
	golab9:
		for {
			{
				if !(z.outGrouping(fre_g_v, 97, 251)) {
					goto lab10_end
				}
				break golab9
			}
		lab10_end:
			;
			if z.cursor >= z.limit {
				goto lab6_end
			}
			z.cursor++
		}
		z.I_p1 = z.cursor
	golab11:
		for {
			{
				if !(z.inGrouping(fre_g_v, 97, 251)) {
					goto lab12_end
				}
				break golab11
			}
		lab12_end:
			;
			if z.cursor >= z.limit {
				goto lab6_end
			}
			z.cursor++
		}
	golab13:
		for {
			{
				if !(z.outGrouping(fre_g_v, 97, 251)) {
					goto lab14_end
				}
				break golab13
			}
		lab14_end:
			;
			if z.cursor >= z.limit {
				goto lab6_end
			}
			z.cursor++
		}
		z.I_p2 = z.cursor
	}
lab6_end:
	;
	z.cursor = v_4
	return true
}
func (z *frenchStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(fre_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("i")
				break
			case 2:
				z.sliceFrom("u")
				break
			case 3:
				z.sliceFrom("y")
				break
			case 4:
				z.sliceFrom("\u00EB")
				break
			case 5:
				z.sliceFrom("\u00EF")
				break
			case 6:
				z.sliceDel()
				break
			case 7:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *frenchStemmer) r_RV() bool {
	return z.I_pV <= z.cursor
}
func (z *frenchStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *frenchStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *frenchStemmer) r_standard_suffix() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(fre_a_4)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		break
	case 2:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("ic")) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
			z.bra = z.cursor
			{
				v_2 := z.limit - z.cursor
				{
					if !z.r_R2() {
						goto lab2_end
					}
					z.sliceDel()
					goto lab1_end
				}
			lab2_end:
				;
				z.cursor = z.limit - v_2
				z.sliceFrom("iqU")
			}
		lab1_end:
		}
	lab0_end:
		;
		break
	case 3:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("log")
		break
	case 4:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("u")
		break
	case 5:
		if !z.r_R2() {
			return false
		}
		z.sliceFrom("ent")
		break
	case 6:
		if !z.r_RV() {
			return false
		}
		z.sliceDel()
		v_3 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(fre_a_2)
			if among_var == 0 {
				z.cursor = z.limit - v_3
				goto lab3_end
			}
			z.bra = z.cursor
			switch among_var {
			case 1:
				if !z.r_R2() {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
				z.sliceDel()
				z.ket = z.cursor
				if !(z.eqSB("at")) {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
				z.bra = z.cursor
				if !z.r_R2() {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
				z.sliceDel()
				break
			case 2:
				{
					v_4 := z.limit - z.cursor
					{
						if !z.r_R2() {
							goto lab5_end
						}
						z.sliceDel()
						goto lab4_end
					}
				lab5_end:
					;
					z.cursor = z.limit - v_4
					if !z.r_R1() {
						z.cursor = z.limit - v_3
						goto lab3_end
					}
					z.sliceFrom("eux")
				}
			lab4_end:
				;
				break
			case 3:
				if !z.r_R2() {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
				z.sliceDel()
				break
			case 4:
				if !z.r_RV() {
					z.cursor = z.limit - v_3
					goto lab3_end
				}
				z.sliceFrom("i")
				break
			}
		}
	lab3_end:
		;
		break
	case 7:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_5 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(fre_a_3)
			if among_var == 0 {
				z.cursor = z.limit - v_5
				goto lab6_end
			}
			z.bra = z.cursor
			switch among_var {
			case 1:
				{
					v_6 := z.limit - z.cursor
					{
						if !z.r_R2() {
							goto lab8_end
						}
						z.sliceDel()
						goto lab7_end
					}
				lab8_end:
					;
					z.cursor = z.limit - v_6
					z.sliceFrom("abl")
				}
			lab7_end:
				;
				break
			case 2:
				{
					v_7 := z.limit - z.cursor
					{
						if !z.r_R2() {
							goto lab10_end
						}
						z.sliceDel()
						goto lab9_end
					}
				lab10_end:
					;
					z.cursor = z.limit - v_7
					z.sliceFrom("iqU")
				}
			lab9_end:
				;
				break
			case 3:
				if !z.r_R2() {
					z.cursor = z.limit - v_5
					goto lab6_end
				}
				z.sliceDel()
				break
			}
		}
	lab6_end:
		;
		break
	case 8:
		if !z.r_R2() {
			return false
		}
		z.sliceDel()
		v_8 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("at")) {
				z.cursor = z.limit - v_8
				goto lab11_end
			}
			z.bra = z.cursor
			if !z.r_R2() {
				z.cursor = z.limit - v_8
				goto lab11_end
			}
			z.sliceDel()
			z.ket = z.cursor
			if !(z.eqSB("ic")) {
				z.cursor = z.limit - v_8
				goto lab11_end
			}
			z.bra = z.cursor
			{
				v_9 := z.limit - z.cursor
				{
					if !z.r_R2() {
						goto lab13_end
					}
					z.sliceDel()
					goto lab12_end
				}
			lab13_end:
				;
				z.cursor = z.limit - v_9
				z.sliceFrom("iqU")
			}
		lab12_end:
		}
	lab11_end:
		;
		break
	case 9:
		z.sliceFrom("eau")
		break
	case 10:
		if !z.r_R1() {
			return false
		}
		z.sliceFrom("al")
		break
	case 11:
		{
			v_10 := z.limit - z.cursor
			{
				if !z.r_R2() {
					goto lab15_end
				}
				z.sliceDel()
				goto lab14_end
			}
		lab15_end:
			;
			z.cursor = z.limit - v_10
			if !z.r_R1() {
				return false
			}
			z.sliceFrom("eux")
		}
	lab14_end:
		;
		break
	case 12:
		if !z.r_R1() {
			return false
		}
		if !(z.outGroupingB(fre_g_v, 97, 251)) {
			return false
		}
		z.sliceDel()
		break
	case 13:
		if !z.r_RV() {
			return false
		}
		z.sliceFrom("ant")
		return false
	case 14:
		if !z.r_RV() {
			return false
		}
		z.sliceFrom("ent")
		return false
	case 15:
		v_11 := z.limit - z.cursor
		if !(z.inGroupingB(fre_g_v, 97, 251)) {
			return false
		}
		if !z.r_RV() {
			return false
		}
		z.cursor = z.limit - v_11
		z.sliceDel()
		return false
	}
	return true
}
func (z *frenchStemmer) r_i_verb_suffix() bool {
	if z.cursor < z.I_pV {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	if z.findAmongB(fre_a_5) == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	{
		v_3 := z.limit - z.cursor
		{
			if !(z.eqSB("H")) {
				goto lab0_end
			}
			z.limitBackward = v_2
			return false
		}
	lab0_end:
		;
		z.cursor = z.limit - v_3
	}
	if !(z.outGroupingB(fre_g_v, 97, 251)) {
		z.limitBackward = v_2
		return false
	}
	z.sliceDel()
	z.limitBackward = v_2
	return true
}
func (z *frenchStemmer) r_verb_suffix() bool {
	var among_var int
	if z.cursor < z.I_pV {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	among_var = z.findAmongB(fre_a_6)
	if among_var == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			z.limitBackward = v_2
			return false
		}
		z.sliceDel()
		break
	case 2:
		z.sliceDel()
		break
	case 3:
		z.sliceDel()
		v_3 := z.limit - z.cursor
		{
			z.ket = z.cursor
			if !(z.eqSB("e")) {
				z.cursor = z.limit - v_3
				goto lab0_end
			}
			z.bra = z.cursor
			z.sliceDel()
		}
	lab0_end:
		;
		break
	}
	z.limitBackward = v_2
	return true
}
func (z *frenchStemmer) r_residual_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("s")) {
			z.cursor = z.limit - v_1
			goto lab0_end
		}
		z.bra = z.cursor
		v_2 := z.limit - z.cursor
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.eqSB("Hi")) {
					goto lab2_end
				}
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_3
			if !(z.outGroupingB(fre_g_keep_with_s, 97, 232)) {
				z.cursor = z.limit - v_1
				goto lab0_end
			}
		}
	lab1_end:
		;
		z.cursor = z.limit - v_2
		z.sliceDel()
	}
lab0_end:
	;
	if z.cursor < z.I_pV {
		return false
	}
	v_5 := z.limitBackward
	z.limitBackward = z.I_pV
	z.ket = z.cursor
	among_var = z.findAmongB(fre_a_7)
	if among_var == 0 {
		z.limitBackward = v_5
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		if !z.r_R2() {
			z.limitBackward = v_5
			return false
		}
		{
			v_6 := z.limit - z.cursor
			{
				if !(z.eqSB("s")) {
					goto lab4_end
				}
				goto lab3_end
			}
		lab4_end:
			;
			z.cursor = z.limit - v_6
			if !(z.eqSB("t")) {
				z.limitBackward = v_5
				return false
			}
		}
	lab3_end:
		;
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("i")
		break
	case 3:
		z.sliceDel()
		break
	}
	z.limitBackward = v_5
	return true
}
func (z *frenchStemmer) r_un_double() bool {
	v_1 := z.limit - z.cursor
	if z.findAmongB(fre_a_8) == 0 {
		return false
	}
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if z.cursor <= z.limitBackward {
		return false
	}
	z.cursor--
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *frenchStemmer) r_un_accent() bool {
	{
		v_1 := 1
		for {
			{
				if !(z.outGroupingB(fre_g_v, 97, 251)) {
					goto lab0_end
				}
				v_1--
				continue
			}
		lab0_end:
			;
			break
		}
		if v_1 > 0 {
			return false
		}
	}
	z.ket = z.cursor
	{
		v_3 := z.limit - z.cursor
		{
			if !(z.eqSB("\u00E9")) {
				goto lab2_end
			}
			goto lab1_end
		}
	lab2_end:
		;
		z.cursor = z.limit - v_3
		if !(z.eqSB("\u00E8")) {
			return false
		}
	}
lab1_end:
	;
	z.bra = z.cursor
	z.sliceFrom("e")
	return true
}
func (z *frenchStemmer) stem() {
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	z.r_mark_regions()
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_3 := z.limit - z.cursor
	{
		{
			v_4 := z.limit - z.cursor
			{
				v_5 := z.limit - z.cursor
				{
					v_6 := z.limit - z.cursor
					{
						if !z.r_standard_suffix() {
							goto lab4_end
						}
						goto lab3_end
					}
				lab4_end:
					;
					z.cursor = z.limit - v_6
					{
						if !z.r_i_verb_suffix() {
							goto lab5_end
						}
						goto lab3_end
					}
				lab5_end:
					;
					z.cursor = z.limit - v_6
					if !z.r_verb_suffix() {
						goto lab2_end
					}
				}
			lab3_end:
				;
				z.cursor = z.limit - v_5
				v_7 := z.limit - z.cursor
				{
					z.ket = z.cursor
					{
						v_8 := z.limit - z.cursor
						{
							if !(z.eqSB("Y")) {
								goto lab8_end
							}
							z.bra = z.cursor
							z.sliceFrom("i")
							goto lab7_end
						}
					lab8_end:
						;
						z.cursor = z.limit - v_8
						if !(z.eqSB("\u00E7")) {
							z.cursor = z.limit - v_7
							goto lab6_end
						}
						z.bra = z.cursor
						z.sliceFrom("c")
					}
				lab7_end:
				}
			lab6_end:
				;
				goto lab1_end
			}
		lab2_end:
			;
			z.cursor = z.limit - v_4
			if !z.r_residual_suffix() {
				goto lab0_end
			}
		}
	lab1_end:
	}
lab0_end:
	;
	z.cursor = z.limit - v_3
	v_9 := z.limit - z.cursor
	z.r_un_double()
	z.cursor = z.limit - v_9
	v_10 := z.limit - z.cursor
	z.r_un_accent()
	z.cursor = z.limit - v_10
	z.cursor = z.limitBackward
	v_11 := z.cursor
	z.r_postlude()
	z.cursor = v_11
	return
}

type germanStemmer struct {
	snowball
	I_x  int
	I_p2 int
	I_p1 int
}

func newGerman() *germanStemmer { return &germanStemmer{} }

var ger_a_0 = []among{
	{"", -1, 5, ""},
	{"ae", 0, 2, ""},
	{"oe", 0, 3, ""},
	{"qu", 0, -1, ""},
	{"ue", 0, 4, ""},
	{"\u00DF", 0, 1, ""},
}
var ger_a_1 = []among{
	{"", -1, 5, ""},
	{"U", 0, 2, ""},
	{"Y", 0, 1, ""},
	{"\u00E4", 0, 3, ""},
	{"\u00F6", 0, 4, ""},
	{"\u00FC", 0, 2, ""},
}
var ger_a_2 = []among{
	{"e", -1, 3, ""},
	{"em", -1, 1, ""},
	{"en", -1, 3, ""},
	{"ern", -1, 2, ""},
	{"er", -1, 2, ""},
	{"s", -1, 4, ""},
	{"es", 5, 3, ""},
}
var ger_a_3 = []among{
	{"en", -1, 1, ""},
	{"er", -1, 1, ""},
	{"st", -1, 2, ""},
	{"est", 2, 1, ""},
}
var ger_a_4 = []among{
	{"ig", -1, 1, ""},
	{"lich", -1, 1, ""},
}
var ger_a_5 = []among{
	{"end", -1, 1, ""},
	{"ig", -1, 2, ""},
	{"ung", -1, 1, ""},
	{"lich", -1, 3, ""},
	{"isch", -1, 2, ""},
	{"ik", -1, 2, ""},
	{"heit", -1, 3, ""},
	{"keit", -1, 4, ""},
}
var ger_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 8, 0, 32, 8}
var ger_g_s_ending = []byte{117, 30, 5}
var ger_g_st_ending = []byte{117, 30, 4}

func (z *germanStemmer) r_prelude() bool {
	var among_var int
	v_1 := z.cursor
	for {
		v_2 := z.cursor
		{
		golab1:
			for {
				v_3 := z.cursor
				{
					if !(z.inGrouping(ger_g_v, 97, 252)) {
						goto lab2_end
					}
					z.bra = z.cursor
					{
						v_4 := z.cursor
						{
							if !(z.eqS("u")) {
								goto lab4_end
							}
							z.ket = z.cursor
							if !(z.inGrouping(ger_g_v, 97, 252)) {
								goto lab4_end
							}
							z.sliceFrom("U")
							goto lab3_end
						}
					lab4_end:
						;
						z.cursor = v_4
						if !(z.eqS("y")) {
							goto lab2_end
						}
						z.ket = z.cursor
						if !(z.inGrouping(ger_g_v, 97, 252)) {
							goto lab2_end
						}
						z.sliceFrom("Y")
					}
				lab3_end:
					;
					z.cursor = v_3
					break golab1
				}
			lab2_end:
				;
				z.cursor = v_3
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_2
		break
	}
	z.cursor = v_1
	for {
		v_5 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(ger_a_0)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("ss")
				break
			case 2:
				z.sliceFrom("\u00E4")
				break
			case 3:
				z.sliceFrom("\u00F6")
				break
			case 4:
				z.sliceFrom("\u00FC")
				break
			case 5:
				if z.cursor >= z.limit {
					goto lab5_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab5_end:
		;
		z.cursor = v_5
		break
	}
	return true
}
func (z *germanStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	z.I_p2 = z.limit
	v_1 := z.cursor
	{
		c := z.cursor + 3
		if c > z.limit {
			return false
		}
		z.cursor = c
	}
	z.I_x = z.cursor
	z.cursor = v_1
golab0:
	for {
		{
			if !(z.inGrouping(ger_g_v, 97, 252)) {
				goto lab1_end
			}
			break golab0
		}
	lab1_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab2:
	for {
		{
			if !(z.outGrouping(ger_g_v, 97, 252)) {
				goto lab3_end
			}
			break golab2
		}
	lab3_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= z.I_x {
			goto lab4_end
		}
		z.I_p1 = z.I_x
	}
lab4_end:
	;
golab5:
	for {
		{
			if !(z.inGrouping(ger_g_v, 97, 252)) {
				goto lab6_end
			}
			break golab5
		}
	lab6_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab7:
	for {
		{
			if !(z.outGrouping(ger_g_v, 97, 252)) {
				goto lab8_end
			}
			break golab7
		}
	lab8_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p2 = z.cursor
	return true
}
func (z *germanStemmer) r_postlude() bool {
	var among_var int
	for {
		v_1 := z.cursor
		{
			z.bra = z.cursor
			among_var = z.findAmong(ger_a_1)
			z.ket = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("y")
				break
			case 2:
				z.sliceFrom("u")
				break
			case 3:
				z.sliceFrom("a")
				break
			case 4:
				z.sliceFrom("o")
				break
			case 5:
				if z.cursor >= z.limit {
					goto lab0_end
				}
				z.cursor++
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = v_1
		break
	}
	return true
}
func (z *germanStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *germanStemmer) r_R2() bool {
	return z.I_p2 <= z.cursor
}
func (z *germanStemmer) r_standard_suffix() bool {
	var among_var int
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(ger_a_2)
		if among_var == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		if !z.r_R1() {
			goto lab0_end
		}
		switch among_var {
		case 1:
			{
				v_2 := z.limit - z.cursor
				{
					if !(z.eqSB("syst")) {
						goto lab1_end
					}
					goto lab0_end
				}
			lab1_end:
				;
				z.cursor = z.limit - v_2
			}
			z.sliceDel()
			break
		case 2:
			z.sliceDel()
			break
		case 3:
			z.sliceDel()
			v_3 := z.limit - z.cursor
			{
				z.ket = z.cursor
				if !(z.eqSB("s")) {
					z.cursor = z.limit - v_3
					goto lab2_end
				}
				z.bra = z.cursor
				if !(z.eqSB("nis")) {
					z.cursor = z.limit - v_3
					goto lab2_end
				}
				z.sliceDel()
			}
		lab2_end:
			;
			break
		case 4:
			if !(z.inGroupingB(ger_g_s_ending, 98, 116)) {
				goto lab0_end
			}
			z.sliceDel()
			break
		}
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	v_4 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(ger_a_3)
		if among_var == 0 {
			goto lab3_end
		}
		z.bra = z.cursor
		if !z.r_R1() {
			goto lab3_end
		}
		switch among_var {
		case 1:
			z.sliceDel()
			break
		case 2:
			if !(z.inGroupingB(ger_g_st_ending, 98, 116)) {
				goto lab3_end
			}
			{
				c := z.cursor - 3
				if c < z.limitBackward {
					goto lab3_end
				}
				z.cursor = c
			}
			z.sliceDel()
			break
		}
	}
lab3_end:
	;
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	{
		z.ket = z.cursor
		among_var = z.findAmongB(ger_a_5)
		if among_var == 0 {
			goto lab4_end
		}
		z.bra = z.cursor
		if !z.r_R2() {
			goto lab4_end
		}
		switch among_var {
		case 1:
			z.sliceDel()
			v_6 := z.limit - z.cursor
			{
				z.ket = z.cursor
				if !(z.eqSB("ig")) {
					z.cursor = z.limit - v_6
					goto lab5_end
				}
				z.bra = z.cursor
				{
					v_7 := z.limit - z.cursor
					{
						if !(z.eqSB("e")) {
							goto lab6_end
						}
						z.cursor = z.limit - v_6
						goto lab5_end
					}
				lab6_end:
					;
					z.cursor = z.limit - v_7
				}
				if !z.r_R2() {
					z.cursor = z.limit - v_6
					goto lab5_end
				}
				z.sliceDel()
			}
		lab5_end:
			;
			break
		case 2:
			{
				v_8 := z.limit - z.cursor
				{
					if !(z.eqSB("e")) {
						goto lab7_end
					}
					goto lab4_end
				}
			lab7_end:
				;
				z.cursor = z.limit - v_8
			}
			z.sliceDel()
			break
		case 3:
			z.sliceDel()
			v_9 := z.limit - z.cursor
			{
				z.ket = z.cursor
				{
					v_10 := z.limit - z.cursor
					{
						if !(z.eqSB("er")) {
							goto lab10_end
						}
						goto lab9_end
					}
				lab10_end:
					;
					z.cursor = z.limit - v_10
					if !(z.eqSB("en")) {
						z.cursor = z.limit - v_9
						goto lab8_end
					}
				}
			lab9_end:
				;
				z.bra = z.cursor
				if !z.r_R1() {
					z.cursor = z.limit - v_9
					goto lab8_end
				}
				z.sliceDel()
			}
		lab8_end:
			;
			break
		case 4:
			z.sliceDel()
			v_11 := z.limit - z.cursor
			{
				z.ket = z.cursor
				if z.findAmongB(ger_a_4) == 0 {
					z.cursor = z.limit - v_11
					goto lab11_end
				}
				z.bra = z.cursor
				if !z.r_R2() {
					z.cursor = z.limit - v_11
					goto lab11_end
				}
				z.sliceDel()
			}
		lab11_end:
			;
			break
		}
	}
lab4_end:
	;
	z.cursor = z.limit - v_5
	return true
}
func (z *germanStemmer) stem() {
	v_1 := z.cursor
	z.r_prelude()
	z.cursor = v_1
	v_2 := z.cursor
	z.r_mark_regions()
	z.cursor = v_2
	z.limitBackward = z.cursor
	z.cursor = z.limit
	z.r_standard_suffix()
	z.cursor = z.limitBackward
	v_4 := z.cursor
	z.r_postlude()
	z.cursor = v_4
	return
}

type greekStemmer struct {
	snowball
	B_test1 bool
}

func newGreek() *greekStemmer { return &greekStemmer{} }

var gre_a_0 = []among{
	{"", -1, 25, ""},
	{"\u0386", 0, 1, ""},
	{"\u0388", 0, 5, ""},
	{"\u0389", 0, 7, ""},
	{"\u038A", 0, 9, ""},
	{"\u038C", 0, 15, ""},
	{"\u038E", 0, 20, ""},
	{"\u038F", 0, 24, ""},
	{"\u0390", 0, 7, ""},
	{"\u0391", 0, 1, ""},
	{"\u0392", 0, 2, ""},
	{"\u0393", 0, 3, ""},
	{"\u0394", 0, 4, ""},
	{"\u0395", 0, 5, ""},
	{"\u0396", 0, 6, ""},
	{"\u0397", 0, 7, ""},
	{"\u0398", 0, 8, ""},
	{"\u0399", 0, 9, ""},
	{"\u039A", 0, 10, ""},
	{"\u039B", 0, 11, ""},
	{"\u039C", 0, 12, ""},
	{"\u039D", 0, 13, ""},
	{"\u039E", 0, 14, ""},
	{"\u039F", 0, 15, ""},
	{"\u03A0", 0, 16, ""},
	{"\u03A1", 0, 17, ""},
	{"\u03A3", 0, 18, ""},
	{"\u03A4", 0, 19, ""},
	{"\u03A5", 0, 20, ""},
	{"\u03A6", 0, 21, ""},
	{"\u03A7", 0, 22, ""},
	{"\u03A8", 0, 23, ""},
	{"\u03A9", 0, 24, ""},
	{"\u03AA", 0, 9, ""},
	{"\u03AB", 0, 20, ""},
	{"\u03AC", 0, 1, ""},
	{"\u03AD", 0, 5, ""},
	{"\u03AE", 0, 7, ""},
	{"\u03AF", 0, 9, ""},
	{"\u03B0", 0, 20, ""},
	{"\u03C2", 0, 18, ""},
	{"\u03CA", 0, 7, ""},
	{"\u03CB", 0, 20, ""},
	{"\u03CC", 0, 15, ""},
	{"\u03CD", 0, 20, ""},
	{"\u03CE", 0, 24, ""},
}
var gre_a_1 = []among{
	{"\u03C3\u03BA\u03B1\u03B3\u03B9\u03B1", -1, 2, ""},
	{"\u03C6\u03B1\u03B3\u03B9\u03B1", -1, 1, ""},
	{"\u03BF\u03BB\u03BF\u03B3\u03B9\u03B1", -1, 3, ""},
	{"\u03C3\u03BF\u03B3\u03B9\u03B1", -1, 4, ""},
	{"\u03C4\u03B1\u03C4\u03BF\u03B3\u03B9\u03B1", -1, 5, ""},
	{"\u03BA\u03C1\u03B5\u03B1\u03C4\u03B1", -1, 6, ""},
	{"\u03C0\u03B5\u03C1\u03B1\u03C4\u03B1", -1, 7, ""},
	{"\u03C4\u03B5\u03C1\u03B1\u03C4\u03B1", -1, 8, ""},
	{"\u03B3\u03B5\u03B3\u03BF\u03BD\u03BF\u03C4\u03B1", -1, 11, ""},
	{"\u03BA\u03B1\u03B8\u03B5\u03C3\u03C4\u03C9\u03C4\u03B1", -1, 10, ""},
	{"\u03C6\u03C9\u03C4\u03B1", -1, 9, ""},
	{"\u03C0\u03B5\u03C1\u03B1\u03C4\u03B7", -1, 7, ""},
	{"\u03C3\u03BA\u03B1\u03B3\u03B9\u03C9\u03BD", -1, 2, ""},
	{"\u03C6\u03B1\u03B3\u03B9\u03C9\u03BD", -1, 1, ""},
	{"\u03BF\u03BB\u03BF\u03B3\u03B9\u03C9\u03BD", -1, 3, ""},
	{"\u03C3\u03BF\u03B3\u03B9\u03C9\u03BD", -1, 4, ""},
	{"\u03C4\u03B1\u03C4\u03BF\u03B3\u03B9\u03C9\u03BD", -1, 5, ""},
	{"\u03BA\u03C1\u03B5\u03B1\u03C4\u03C9\u03BD", -1, 6, ""},
	{"\u03C0\u03B5\u03C1\u03B1\u03C4\u03C9\u03BD", -1, 7, ""},
	{"\u03C4\u03B5\u03C1\u03B1\u03C4\u03C9\u03BD", -1, 8, ""},
	{"\u03B3\u03B5\u03B3\u03BF\u03BD\u03BF\u03C4\u03C9\u03BD", -1, 11, ""},
	{"\u03BA\u03B1\u03B8\u03B5\u03C3\u03C4\u03C9\u03C4\u03C9\u03BD", -1, 10, ""},
	{"\u03C6\u03C9\u03C4\u03C9\u03BD", -1, 9, ""},
	{"\u03BA\u03C1\u03B5\u03B1\u03C3", -1, 6, ""},
	{"\u03C0\u03B5\u03C1\u03B1\u03C3", -1, 7, ""},
	{"\u03C4\u03B5\u03C1\u03B1\u03C3", -1, 8, ""},
	{"\u03B3\u03B5\u03B3\u03BF\u03BD\u03BF\u03C3", -1, 11, ""},
	{"\u03BA\u03C1\u03B5\u03B1\u03C4\u03BF\u03C3", -1, 6, ""},
	{"\u03C0\u03B5\u03C1\u03B1\u03C4\u03BF\u03C3", -1, 7, ""},
	{"\u03C4\u03B5\u03C1\u03B1\u03C4\u03BF\u03C3", -1, 8, ""},
	{"\u03B3\u03B5\u03B3\u03BF\u03BD\u03BF\u03C4\u03BF\u03C3", -1, 11, ""},
	{"\u03BA\u03B1\u03B8\u03B5\u03C3\u03C4\u03C9\u03C4\u03BF\u03C3", -1, 10, ""},
	{"\u03C6\u03C9\u03C4\u03BF\u03C3", -1, 9, ""},
	{"\u03BA\u03B1\u03B8\u03B5\u03C3\u03C4\u03C9\u03C3", -1, 10, ""},
	{"\u03C6\u03C9\u03C3", -1, 9, ""},
	{"\u03C3\u03BA\u03B1\u03B3\u03B9\u03BF\u03C5", -1, 2, ""},
	{"\u03C6\u03B1\u03B3\u03B9\u03BF\u03C5", -1, 1, ""},
	{"\u03BF\u03BB\u03BF\u03B3\u03B9\u03BF\u03C5", -1, 3, ""},
	{"\u03C3\u03BF\u03B3\u03B9\u03BF\u03C5", -1, 4, ""},
	{"\u03C4\u03B1\u03C4\u03BF\u03B3\u03B9\u03BF\u03C5", -1, 5, ""},
}
var gre_a_2 = []among{
	{"\u03C0\u03B1", -1, 1, ""},
	{"\u03BE\u03B1\u03BD\u03B1\u03C0\u03B1", 0, 1, ""},
	{"\u03B5\u03C0\u03B1", 0, 1, ""},
	{"\u03C0\u03B5\u03C1\u03B9\u03C0\u03B1", 0, 1, ""},
	{"\u03B1\u03BD\u03B1\u03BC\u03C0\u03B1", 0, 1, ""},
	{"\u03B5\u03BC\u03C0\u03B1", 0, 1, ""},
	{"\u03B2", -1, 2, ""},
	{"\u03B4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B2\u03B1\u03B8\u03C5\u03C1\u03B9", -1, 2, ""},
	{"\u03B2\u03B1\u03C1\u03BA", -1, 2, ""},
	{"\u03BC\u03B1\u03C1\u03BA", -1, 2, ""},
	{"\u03BB", -1, 2, ""},
	{"\u03BC", -1, 2, ""},
	{"\u03BA\u03BF\u03C1\u03BD", -1, 2, ""},
	{"\u03B1\u03B8\u03C1\u03BF", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B1\u03B8\u03C1\u03BF", 14, 1, ""},
	{"\u03C0", -1, 2, ""},
	{"\u03B9\u03BC\u03C0", 16, 2, ""},
	{"\u03C1", -1, 2, ""},
	{"\u03BC\u03B1\u03C1", 18, 2, ""},
	{"\u03B1\u03BC\u03C0\u03B1\u03C1", 18, 2, ""},
	{"\u03B3\u03BA\u03C1", 18, 2, ""},
	{"\u03B2\u03BF\u03BB\u03B2\u03BF\u03C1", 18, 2, ""},
	{"\u03B3\u03BB\u03C5\u03BA\u03BF\u03C1", 18, 2, ""},
	{"\u03C0\u03B9\u03C0\u03B5\u03C1\u03BF\u03C1", 18, 2, ""},
	{"\u03C0\u03C1", 18, 2, ""},
	{"\u03BC\u03C0\u03C1", 25, 2, ""},
	{"\u03B1\u03C1\u03C1", 18, 2, ""},
	{"\u03B3\u03BB\u03C5\u03BA\u03C5\u03C1", 18, 2, ""},
	{"\u03C0\u03BF\u03BB\u03C5\u03C1", 18, 2, ""},
	{"\u03BB\u03BF\u03C5", -1, 2, ""},
}
var gre_a_3 = []among{
	{"\u03B9\u03B6\u03B1", -1, 1, ""},
	{"\u03B9\u03B6\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03BF\u03C5\u03BC\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03BF\u03C5\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03B1\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03B5\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03B6\u03B5\u03B9", -1, 1, ""},
	{"\u03B9\u03B6\u03B1\u03BD", -1, 1, ""},
	{"\u03B9\u03B6\u03BF\u03C5\u03BD", -1, 1, ""},
	{"\u03B9\u03B6\u03B5\u03C3", -1, 1, ""},
	{"\u03B9\u03B6\u03B5\u03B9\u03C3", -1, 1, ""},
	{"\u03B9\u03B6\u03C9", -1, 1, ""},
}
var gre_a_4 = []among{
	{"\u03B2\u03B9", -1, 1, ""},
	{"\u03BB\u03B9", -1, 1, ""},
	{"\u03B1\u03BB", -1, 1, ""},
	{"\u03B5\u03BD", -1, 1, ""},
	{"\u03C3", -1, 1, ""},
	{"\u03C7", -1, 1, ""},
	{"\u03C5\u03C8", -1, 1, ""},
	{"\u03B6\u03C9", -1, 1, ""},
}
var gre_a_5 = []among{
	{"\u03C9\u03B8\u03B7\u03BA\u03B1", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B5", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B1\u03C4\u03B5", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B1\u03BD", -1, 1, ""},
	{"\u03C9\u03B8\u03B7\u03BA\u03B5\u03C3", -1, 1, ""},
}
var gre_a_6 = []among{
	{"\u03BE\u03B1\u03BD\u03B1\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03C0\u03B1", -1, 1, ""},
	{"\u03C0\u03B5\u03C1\u03B9\u03C0\u03B1", -1, 1, ""},
	{"\u03B1\u03BD\u03B1\u03BC\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03BC\u03C0\u03B1", -1, 1, ""},
	{"\u03C7\u03B1\u03C1\u03C4\u03BF\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03BE\u03B1\u03C1\u03C7\u03B1", -1, 1, ""},
	{"\u03B3\u03B5", -1, 2, ""},
	{"\u03B3\u03BA\u03B5", -1, 2, ""},
	{"\u03BA\u03BB\u03B5", -1, 1, ""},
	{"\u03B5\u03BA\u03BB\u03B5", 9, 1, ""},
	{"\u03B1\u03C0\u03B5\u03BA\u03BB\u03B5", 10, 1, ""},
	{"\u03B1\u03C0\u03BF\u03BA\u03BB\u03B5", 9, 1, ""},
	{"\u03B5\u03C3\u03C9\u03BA\u03BB\u03B5", 9, 1, ""},
	{"\u03B4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03C0\u03B5", -1, 1, ""},
	{"\u03B5\u03C0\u03B5", 15, 1, ""},
	{"\u03BC\u03B5\u03C4\u03B5\u03C0\u03B5", 16, 1, ""},
	{"\u03B5\u03C3\u03B5", -1, 1, ""},
	{"\u03B3\u03BA", -1, 2, ""},
	{"\u03BC", -1, 2, ""},
	{"\u03C0\u03BF\u03C5\u03BA\u03B1\u03BC", 20, 2, ""},
	{"\u03BA\u03BF\u03BC", 20, 2, ""},
	{"\u03B1\u03BD", -1, 2, ""},
	{"\u03BF\u03BB\u03BF", -1, 2, ""},
	{"\u03B1\u03B8\u03C1\u03BF", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B1\u03B8\u03C1\u03BF", 25, 1, ""},
	{"\u03C0", -1, 2, ""},
	{"\u03BB\u03B1\u03C1", -1, 2, ""},
	{"\u03B4\u03B7\u03BC\u03BF\u03BA\u03C1\u03B1\u03C4", -1, 2, ""},
	{"\u03B1\u03C6", -1, 2, ""},
	{"\u03B3\u03B9\u03B3\u03B1\u03BD\u03C4\u03BF\u03B1\u03C6", 30, 2, ""},
}
var gre_a_7 = []among{
	{"\u03B9\u03C3\u03B1", -1, 1, ""},
	{"\u03B9\u03C3\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B1\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B1\u03BD", -1, 1, ""},
	{"\u03B9\u03C3\u03B5\u03C3", -1, 1, ""},
}
var gre_a_8 = []among{
	{"\u03BE\u03B1\u03BD\u03B1\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03C0\u03B1", -1, 1, ""},
	{"\u03C0\u03B5\u03C1\u03B9\u03C0\u03B1", -1, 1, ""},
	{"\u03B1\u03BD\u03B1\u03BC\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03BC\u03C0\u03B1", -1, 1, ""},
	{"\u03C7\u03B1\u03C1\u03C4\u03BF\u03C0\u03B1", -1, 1, ""},
	{"\u03B5\u03BE\u03B1\u03C1\u03C7\u03B1", -1, 1, ""},
	{"\u03BA\u03BB\u03B5", -1, 1, ""},
	{"\u03B5\u03BA\u03BB\u03B5", 7, 1, ""},
	{"\u03B1\u03C0\u03B5\u03BA\u03BB\u03B5", 8, 1, ""},
	{"\u03B1\u03C0\u03BF\u03BA\u03BB\u03B5", 7, 1, ""},
	{"\u03B5\u03C3\u03C9\u03BA\u03BB\u03B5", 7, 1, ""},
	{"\u03B4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03C0\u03B5", -1, 1, ""},
	{"\u03B5\u03C0\u03B5", 13, 1, ""},
	{"\u03BC\u03B5\u03C4\u03B5\u03C0\u03B5", 14, 1, ""},
	{"\u03B5\u03C3\u03B5", -1, 1, ""},
	{"\u03B1\u03B8\u03C1\u03BF", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B1\u03B8\u03C1\u03BF", 17, 1, ""},
}
var gre_a_9 = []among{
	{"\u03B9\u03C3\u03BF\u03C5\u03BC\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03BF\u03C5\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B5\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03B5\u03B9", -1, 1, ""},
	{"\u03B9\u03C3\u03BF\u03C5\u03BD", -1, 1, ""},
	{"\u03B9\u03C3\u03B5\u03B9\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03C9", -1, 1, ""},
}
var gre_a_10 = []among{
	{"\u03B1\u03C4\u03B1", -1, 2, ""},
	{"\u03C6\u03B1", -1, 2, ""},
	{"\u03B7\u03C6\u03B1", 1, 2, ""},
	{"\u03BC\u03B5\u03B3", -1, 2, ""},
	{"\u03BB\u03C5\u03B3", -1, 2, ""},
	{"\u03B7\u03B4", -1, 2, ""},
	{"\u03BA\u03BB\u03B5", -1, 1, ""},
	{"\u03B5\u03C3\u03C9\u03BA\u03BB\u03B5", 6, 1, ""},
	{"\u03C0\u03BB\u03B5", -1, 1, ""},
	{"\u03B4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03C3\u03B5", -1, 1, ""},
	{"\u03B1\u03C3\u03B5", 10, 1, ""},
	{"\u03BA\u03B1\u03B8", -1, 2, ""},
	{"\u03B5\u03C7\u03B8", -1, 2, ""},
	{"\u03BA\u03B1\u03BA", -1, 2, ""},
	{"\u03BC\u03B1\u03BA", -1, 2, ""},
	{"\u03C3\u03BA", -1, 2, ""},
	{"\u03C6\u03B9\u03BB", -1, 2, ""},
	{"\u03BA\u03C5\u03BB", -1, 2, ""},
	{"\u03BC", -1, 2, ""},
	{"\u03B3\u03B5\u03BC", 19, 2, ""},
	{"\u03B1\u03C7\u03BD", -1, 2, ""},
	{"\u03C3\u03C5\u03BD\u03B1\u03B8\u03C1\u03BF", -1, 1, ""},
	{"\u03C0", -1, 2, ""},
	{"\u03B1\u03C0", 23, 2, ""},
	{"\u03B5\u03BC\u03C0", 23, 2, ""},
	{"\u03B5\u03C5\u03C0", 23, 2, ""},
	{"\u03B1\u03C1", -1, 2, ""},
	{"\u03B1\u03BF\u03C1", -1, 2, ""},
	{"\u03B3\u03C5\u03C1", -1, 2, ""},
	{"\u03C7\u03C1", -1, 2, ""},
	{"\u03C7\u03C9\u03C1", -1, 2, ""},
	{"\u03BA\u03C4", -1, 2, ""},
	{"\u03B1\u03BA\u03C4", 32, 2, ""},
	{"\u03C7\u03C4", -1, 2, ""},
	{"\u03B1\u03C7\u03C4", 34, 2, ""},
	{"\u03C4\u03B1\u03C7", -1, 2, ""},
	{"\u03C3\u03C7", -1, 2, ""},
	{"\u03B1\u03C3\u03C7", 37, 2, ""},
	{"\u03C5\u03C8", -1, 2, ""},
}
var gre_a_11 = []among{
	{"\u03B9\u03C3\u03C4\u03B1", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03B7", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03BF\u03B9", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03BF", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03B5\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03B7\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03BF\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03BF\u03C5\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03C4\u03BF\u03C5", -1, 1, ""},
}
var gre_a_12 = []among{
	{"\u03B5\u03B3\u03BA\u03BB\u03B5", -1, 1, ""},
	{"\u03B1\u03C0\u03BF\u03BA\u03BB\u03B5", -1, 1, ""},
	{"\u03B4\u03B1\u03BD\u03B5", -1, 2, ""},
	{"\u03B1\u03BD\u03C4\u03B9\u03B4\u03B1\u03BD\u03B5", 2, 2, ""},
	{"\u03C3\u03B5", -1, 1, ""},
	{"\u03BC\u03B5\u03C4\u03B1\u03C3\u03B5", 4, 1, ""},
	{"\u03BC\u03B9\u03BA\u03C1\u03BF\u03C3\u03B5", 4, 1, ""},
}
var gre_a_13 = []among{
	{"\u03B1\u03C4\u03BF\u03BC\u03B9\u03BA", -1, 2, ""},
	{"\u03B5\u03B8\u03BD\u03B9\u03BA", -1, 4, ""},
	{"\u03C4\u03BF\u03C0\u03B9\u03BA", -1, 7, ""},
	{"\u03B5\u03BA\u03BB\u03B5\u03BA\u03C4\u03B9\u03BA", -1, 5, ""},
	{"\u03C3\u03BA\u03B5\u03C0\u03C4\u03B9\u03BA", -1, 6, ""},
	{"\u03B3\u03BD\u03C9\u03C3\u03C4\u03B9\u03BA", -1, 3, ""},
	{"\u03B1\u03B3\u03BD\u03C9\u03C3\u03C4\u03B9\u03BA", 5, 1, ""},
	{"\u03B1\u03BB\u03B5\u03BE\u03B1\u03BD\u03B4\u03C1\u03B9\u03BD", -1, 8, ""},
	{"\u03B8\u03B5\u03B1\u03C4\u03C1\u03B9\u03BD", -1, 10, ""},
	{"\u03B2\u03C5\u03B6\u03B1\u03BD\u03C4\u03B9\u03BD", -1, 9, ""},
}
var gre_a_14 = []among{
	{"\u03B9\u03C3\u03BC\u03BF\u03B9", -1, 1, ""},
	{"\u03B9\u03C3\u03BC\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03C3\u03BC\u03BF", -1, 1, ""},
	{"\u03B9\u03C3\u03BC\u03BF\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03BC\u03BF\u03C5\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03BC\u03BF\u03C5", -1, 1, ""},
}
var gre_a_15 = []among{
	{"\u03C3", -1, 1, ""},
	{"\u03C7", -1, 1, ""},
}
var gre_a_16 = []among{
	{"\u03BF\u03C5\u03B4\u03B1\u03BA\u03B9\u03B1", -1, 1, ""},
	{"\u03B1\u03C1\u03B1\u03BA\u03B9\u03B1", -1, 1, ""},
	{"\u03BF\u03C5\u03B4\u03B1\u03BA\u03B9", -1, 1, ""},
	{"\u03B1\u03C1\u03B1\u03BA\u03B9", -1, 1, ""},
}
var gre_a_17 = []among{
	{"\u03B2", -1, 2, ""},
	{"\u03B2\u03B1\u03BC\u03B2", 0, 1, ""},
	{"\u03C3\u03BB\u03BF\u03B2", 0, 1, ""},
	{"\u03C4\u03C3\u03B5\u03C7\u03BF\u03C3\u03BB\u03BF\u03B2", 2, 1, ""},
	{"\u03BA\u03B1\u03C1\u03B4", -1, 2, ""},
	{"\u03B6", -1, 2, ""},
	{"\u03C4\u03B6", 5, 1, ""},
	{"\u03BA", -1, 1, ""},
	{"\u03BA\u03B1\u03C0\u03B1\u03BA", 7, 1, ""},
	{"\u03C3\u03BF\u03BA", 7, 1, ""},
	{"\u03C3\u03BA", 7, 1, ""},
	{"\u03B2\u03B1\u03BB", -1, 2, ""},
	{"\u03BC\u03B1\u03BB", -1, 1, ""},
	{"\u03B3\u03BB", -1, 2, ""},
	{"\u03C4\u03C1\u03B9\u03C0\u03BF\u03BB", -1, 2, ""},
	{"\u03C0\u03BB", -1, 1, ""},
	{"\u03BB\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03C6\u03C5\u03BB", -1, 1, ""},
	{"\u03BA\u03B1\u03B9\u03BC", -1, 1, ""},
	{"\u03BA\u03BB\u03B9\u03BC", -1, 1, ""},
	{"\u03C6\u03B1\u03C1\u03BC", -1, 1, ""},
	{"\u03B3\u03B9\u03B1\u03BD", -1, 2, ""},
	{"\u03C3\u03C0\u03B1\u03BD", -1, 1, ""},
	{"\u03B7\u03B3\u03BF\u03C5\u03BC\u03B5\u03BD", -1, 2, ""},
	{"\u03BA\u03BF\u03BD", -1, 1, ""},
	{"\u03BC\u03B1\u03BA\u03C1\u03C5\u03BD", -1, 2, ""},
	{"\u03C0", -1, 2, ""},
	{"\u03BA\u03B1\u03C4\u03C1\u03B1\u03C0", 26, 1, ""},
	{"\u03C1", -1, 1, ""},
	{"\u03B2\u03C1", 28, 1, ""},
	{"\u03BB\u03B1\u03B2\u03C1", 29, 1, ""},
	{"\u03B1\u03BC\u03B2\u03C1", 29, 1, ""},
	{"\u03BC\u03B5\u03C1", 28, 1, ""},
	{"\u03C0\u03B1\u03C4\u03B5\u03C1", 28, 2, ""},
	{"\u03B1\u03BD\u03B8\u03C1", 28, 1, ""},
	{"\u03BA\u03BF\u03C1", 28, 1, ""},
	{"\u03C3", -1, 1, ""},
	{"\u03BD\u03B1\u03B3\u03BA\u03B1\u03C3", 36, 1, ""},
	{"\u03C4\u03BF\u03C3", 36, 2, ""},
	{"\u03BC\u03BF\u03C5\u03C3\u03C4", -1, 1, ""},
	{"\u03C1\u03C5", -1, 1, ""},
	{"\u03C6", -1, 1, ""},
	{"\u03C3\u03C6", 41, 1, ""},
	{"\u03B1\u03BB\u03B9\u03C3\u03C6", 42, 1, ""},
	{"\u03BD\u03C5\u03C6", 41, 2, ""},
	{"\u03C7", -1, 1, ""},
}
var gre_a_18 = []among{
	{"\u03B1\u03BA\u03B9\u03B1", -1, 1, ""},
	{"\u03B1\u03C1\u03B1\u03BA\u03B9\u03B1", 0, 1, ""},
	{"\u03B9\u03C4\u03C3\u03B1", -1, 1, ""},
	{"\u03B1\u03BA\u03B9", -1, 1, ""},
	{"\u03B1\u03C1\u03B1\u03BA\u03B9", 3, 1, ""},
	{"\u03B9\u03C4\u03C3\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03C4\u03C3\u03B1\u03C3", -1, 1, ""},
	{"\u03B9\u03C4\u03C3\u03B5\u03C3", -1, 1, ""},
}
var gre_a_19 = []among{
	{"\u03C8\u03B1\u03BB", -1, 1, ""},
	{"\u03B1\u03B9\u03C6\u03BD", -1, 1, ""},
	{"\u03BF\u03BB\u03BF", -1, 1, ""},
	{"\u03B9\u03C1", -1, 1, ""},
}
var gre_a_20 = []among{
	{"\u03B5", -1, 1, ""},
	{"\u03C0\u03B1\u03B9\u03C7\u03BD", -1, 1, ""},
}
var gre_a_21 = []among{
	{"\u03B9\u03B4\u03B9\u03B1", -1, 1, ""},
	{"\u03B9\u03B4\u03B9\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03B4\u03B9\u03BF", -1, 1, ""},
}
var gre_a_22 = []among{
	{"\u03B9\u03B2", -1, 1, ""},
	{"\u03B4", -1, 1, ""},
	{"\u03C6\u03C1\u03B1\u03B3\u03BA", -1, 1, ""},
	{"\u03BB\u03C5\u03BA", -1, 1, ""},
	{"\u03BF\u03B2\u03B5\u03BB", -1, 1, ""},
	{"\u03BC\u03B7\u03BD", -1, 1, ""},
	{"\u03C1", -1, 1, ""},
}
var gre_a_23 = []among{
	{"\u03B9\u03C3\u03BA\u03B5", -1, 1, ""},
	{"\u03B9\u03C3\u03BA\u03BF", -1, 1, ""},
	{"\u03B9\u03C3\u03BA\u03BF\u03C3", -1, 1, ""},
	{"\u03B9\u03C3\u03BA\u03BF\u03C5", -1, 1, ""},
}
var gre_a_24 = []among{
	{"\u03B1\u03B4\u03C9\u03BD", -1, 1, ""},
	{"\u03B1\u03B4\u03B5\u03C3", -1, 1, ""},
}
var gre_a_25 = []among{
	{"\u03B3\u03B9\u03B1\u03B3\u03B9", -1, -1, ""},
	{"\u03B8\u03B5\u03B9", -1, -1, ""},
	{"\u03BF\u03BA", -1, -1, ""},
	{"\u03BC\u03B1\u03BC", -1, -1, ""},
	{"\u03BC\u03B1\u03BD", -1, -1, ""},
	{"\u03BC\u03C0\u03B1\u03BC\u03C0", -1, -1, ""},
	{"\u03C0\u03B5\u03B8\u03B5\u03C1", -1, -1, ""},
	{"\u03C0\u03B1\u03C4\u03B5\u03C1", -1, -1, ""},
	{"\u03BA\u03C5\u03C1", -1, -1, ""},
	{"\u03BD\u03C4\u03B1\u03BD\u03C4", -1, -1, ""},
}
var gre_a_26 = []among{
	{"\u03B5\u03B4\u03C9\u03BD", -1, 1, ""},
	{"\u03B5\u03B4\u03B5\u03C3", -1, 1, ""},
}
var gre_a_27 = []among{
	{"\u03BC\u03B9\u03BB", -1, 1, ""},
	{"\u03B4\u03B1\u03C0", -1, 1, ""},
	{"\u03B3\u03B7\u03C0", -1, 1, ""},
	{"\u03B9\u03C0", -1, 1, ""},
	{"\u03B5\u03BC\u03C0", -1, 1, ""},
	{"\u03BF\u03C0", -1, 1, ""},
	{"\u03BA\u03C1\u03B1\u03C3\u03C0", -1, 1, ""},
	{"\u03C5\u03C0", -1, 1, ""},
}
var gre_a_28 = []among{
	{"\u03BF\u03C5\u03B4\u03C9\u03BD", -1, 1, ""},
	{"\u03BF\u03C5\u03B4\u03B5\u03C3", -1, 1, ""},
}
var gre_a_29 = []among{
	{"\u03C4\u03C1\u03B1\u03B3", -1, 1, ""},
	{"\u03C6\u03B5", -1, 1, ""},
	{"\u03BA\u03B1\u03BB\u03B9\u03B1\u03BA", -1, 1, ""},
	{"\u03B1\u03C1\u03BA", -1, 1, ""},
	{"\u03C3\u03BA", -1, 1, ""},
	{"\u03C0\u03B5\u03C4\u03B1\u03BB", -1, 1, ""},
	{"\u03B2\u03B5\u03BB", -1, 1, ""},
	{"\u03BB\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03C6\u03BB", -1, 1, ""},
	{"\u03C7\u03BD", -1, 1, ""},
	{"\u03C0\u03BB\u03B5\u03BE", -1, 1, ""},
	{"\u03C3\u03C0", -1, 1, ""},
	{"\u03C6\u03C1", -1, 1, ""},
	{"\u03C3", -1, 1, ""},
	{"\u03BB\u03B9\u03C7", -1, 1, ""},
}
var gre_a_30 = []among{
	{"\u03B5\u03C9\u03BD", -1, 1, ""},
	{"\u03B5\u03C9\u03C3", -1, 1, ""},
}
var gre_a_31 = []among{
	{"\u03B4", -1, 1, ""},
	{"\u03B9\u03B4", 0, 1, ""},
	{"\u03B8", -1, 1, ""},
	{"\u03B3\u03B1\u03BB", -1, 1, ""},
	{"\u03B5\u03BB", -1, 1, ""},
	{"\u03BD", -1, 1, ""},
	{"\u03C0", -1, 1, ""},
	{"\u03C0\u03B1\u03C1", -1, 1, ""},
}
var gre_a_32 = []among{
	{"\u03B9\u03B1", -1, 1, ""},
	{"\u03B9\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03BF\u03C5", -1, 1, ""},
}
var gre_a_33 = []among{
	{"\u03B9\u03BA\u03B1", -1, 1, ""},
	{"\u03B9\u03BA\u03C9\u03BD", -1, 1, ""},
	{"\u03B9\u03BA\u03BF", -1, 1, ""},
	{"\u03B9\u03BA\u03BF\u03C5", -1, 1, ""},
}
var gre_a_34 = []among{
	{"\u03B1\u03B4", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B1\u03B4", 0, 1, ""},
	{"\u03BA\u03B1\u03C4\u03B1\u03B4", 0, 1, ""},
	{"\u03B1\u03BD\u03C4\u03B9\u03B4", -1, 1, ""},
	{"\u03B5\u03BD\u03B4", -1, 1, ""},
	{"\u03C6\u03C5\u03BB\u03BF\u03B4", -1, 1, ""},
	{"\u03C5\u03C0\u03BF\u03B4", -1, 1, ""},
	{"\u03C0\u03C1\u03C9\u03C4\u03BF\u03B4", -1, 1, ""},
	{"\u03B5\u03BE\u03C9\u03B4", -1, 1, ""},
	{"\u03B7\u03B8", -1, 1, ""},
	{"\u03B1\u03BD\u03B7\u03B8", 9, 1, ""},
	{"\u03BE\u03B9\u03BA", -1, 1, ""},
	{"\u03B1\u03BB", -1, 1, ""},
	{"\u03B1\u03BC\u03BC\u03BF\u03C7\u03B1\u03BB", 12, 1, ""},
	{"\u03C3\u03C5\u03BD\u03BF\u03BC\u03B7\u03BB", -1, 1, ""},
	{"\u03BC\u03C0\u03BF\u03BB", -1, 1, ""},
	{"\u03BC\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03C4\u03C3\u03B1\u03BC", -1, 1, ""},
	{"\u03B2\u03C1\u03C9\u03BC", -1, 1, ""},
	{"\u03B1\u03BC\u03B1\u03BD", -1, 1, ""},
	{"\u03BC\u03C0\u03B1\u03BD", -1, 1, ""},
	{"\u03BA\u03B1\u03BB\u03BB\u03B9\u03BD", -1, 1, ""},
	{"\u03C0\u03BF\u03C3\u03C4\u03B5\u03BB\u03BD", -1, 1, ""},
	{"\u03C6\u03B9\u03BB\u03BF\u03BD", -1, 1, ""},
	{"\u03BA\u03B1\u03BB\u03C0", -1, 1, ""},
	{"\u03B3\u03B5\u03C1", -1, 1, ""},
	{"\u03C7\u03B1\u03C3", -1, 1, ""},
	{"\u03BC\u03C0\u03BF\u03C3", -1, 1, ""},
	{"\u03C0\u03BB\u03B9\u03B1\u03C4\u03C3", -1, 1, ""},
	{"\u03C0\u03B5\u03C4\u03C3", -1, 1, ""},
	{"\u03C0\u03B9\u03C4\u03C3", -1, 1, ""},
	{"\u03C6\u03C5\u03C3", -1, 1, ""},
	{"\u03BC\u03C0\u03B1\u03B3\u03B9\u03B1\u03C4", -1, 1, ""},
	{"\u03BD\u03B9\u03C4", -1, 1, ""},
	{"\u03C0\u03B9\u03BA\u03B1\u03BD\u03C4", -1, 1, ""},
	{"\u03C3\u03B5\u03C1\u03C4", -1, 1, ""},
}
var gre_a_35 = []among{
	{"\u03B1\u03B3\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03B7\u03BA\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B1\u03BC\u03B5", 1, 1, ""},
	{"\u03B7\u03C3\u03B1\u03BC\u03B5", -1, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B1\u03BC\u03B5", -1, 1, ""},
}
var gre_a_36 = []among{
	{"\u03B2\u03BF\u03C5\u03B2", -1, 1, ""},
	{"\u03BE\u03B5\u03B8", -1, 1, ""},
	{"\u03C0\u03B5\u03B8", -1, 1, ""},
	{"\u03B1\u03C0\u03BF\u03B8", -1, 1, ""},
	{"\u03B1\u03C0\u03BF\u03BA", -1, 1, ""},
	{"\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03B1\u03BD\u03B1\u03C0", -1, 1, ""},
	{"\u03C0\u03B9\u03BA\u03C1", -1, 1, ""},
	{"\u03C0\u03BF\u03C4", -1, 1, ""},
	{"\u03B1\u03C0\u03BF\u03C3\u03C4", -1, 1, ""},
	{"\u03C7", -1, 1, ""},
	{"\u03C3\u03B9\u03C7", 10, 1, ""},
}
var gre_a_37 = []among{
	{"\u03C4\u03C1", -1, 1, ""},
	{"\u03C4\u03C3", -1, 1, ""},
}
var gre_a_38 = []among{
	{"\u03B1\u03B3\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B7\u03BA\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B1\u03BD\u03B5", 1, 1, ""},
	{"\u03B7\u03C3\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03BF\u03BD\u03C4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03BF\u03BD\u03C4\u03B1\u03BD\u03B5", 5, 1, ""},
	{"\u03BF\u03C5\u03BD\u03C4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03BF\u03C5\u03BD\u03C4\u03B1\u03BD\u03B5", 7, 1, ""},
	{"\u03BF\u03C4\u03B1\u03BD\u03B5", -1, 1, ""},
	{"\u03B9\u03BF\u03C4\u03B1\u03BD\u03B5", 9, 1, ""},
}
var gre_a_39 = []among{
	{"\u03C4\u03B1\u03B2", -1, 1, ""},
	{"\u03BD\u03C4\u03B1\u03B2", 0, 1, ""},
	{"\u03C8\u03B7\u03BB\u03BF\u03C4\u03B1\u03B2", 0, 1, ""},
	{"\u03BB\u03B9\u03B2", -1, 1, ""},
	{"\u03BA\u03BB\u03B9\u03B2", 3, 1, ""},
	{"\u03BE\u03B7\u03C1\u03BF\u03BA\u03BB\u03B9\u03B2", 4, 1, ""},
	{"\u03B3", -1, 1, ""},
	{"\u03B1\u03B3", 6, 1, ""},
	{"\u03C4\u03C1\u03B1\u03B3", 7, 1, ""},
	{"\u03C4\u03C3\u03B1\u03B3", 7, 1, ""},
	{"\u03B1\u03B8\u03B9\u03B3\u03B3", 6, 1, ""},
	{"\u03C4\u03C3\u03B9\u03B3\u03B3", 6, 1, ""},
	{"\u03B1\u03C4\u03C3\u03B9\u03B3\u03B3", 11, 1, ""},
	{"\u03C3\u03C4\u03B5\u03B3", 6, 1, ""},
	{"\u03B1\u03C0\u03B7\u03B3", 6, 1, ""},
	{"\u03C3\u03B9\u03B3", 6, 1, ""},
	{"\u03B1\u03BD\u03BF\u03C1\u03B3", 6, 1, ""},
	{"\u03B5\u03BD\u03BF\u03C1\u03B3", 6, 1, ""},
	{"\u03BA\u03B1\u03BB\u03C0\u03BF\u03C5\u03B6", -1, 1, ""},
	{"\u03B8", -1, 1, ""},
	{"\u03BC\u03C9\u03B1\u03BC\u03B5\u03B8", 19, 1, ""},
	{"\u03C0\u03B9\u03B8", 19, 1, ""},
	{"\u03B1\u03C0\u03B9\u03B8", 21, 1, ""},
	{"\u03B4\u03B5\u03BA", -1, 1, ""},
	{"\u03C0\u03B5\u03BB\u03B5\u03BA", -1, 1, ""},
	{"\u03B9\u03BA", -1, 1, ""},
	{"\u03B1\u03BD\u03B9\u03BA", 25, 1, ""},
	{"\u03B2\u03BF\u03C5\u03BB\u03BA", -1, 1, ""},
	{"\u03B2\u03B1\u03C3\u03BA", -1, 1, ""},
	{"\u03B2\u03C1\u03B1\u03C7\u03C5\u03BA", -1, 1, ""},
	{"\u03B3\u03B1\u03BB", -1, 1, ""},
	{"\u03BA\u03B1\u03C4\u03B1\u03B3\u03B1\u03BB", 30, 1, ""},
	{"\u03BF\u03BB\u03BF\u03B3\u03B1\u03BB", 30, 1, ""},
	{"\u03B2\u03B1\u03B8\u03C5\u03B3\u03B1\u03BB", 30, 1, ""},
	{"\u03BC\u03B5\u03BB", -1, 1, ""},
	{"\u03BA\u03B1\u03C3\u03C4\u03B5\u03BB", -1, 1, ""},
	{"\u03C0\u03BF\u03C1\u03C4\u03BF\u03BB", -1, 1, ""},
	{"\u03C0\u03BB", -1, 1, ""},
	{"\u03B4\u03B9\u03C0\u03BB", 37, 1, ""},
	{"\u03BB\u03B1\u03BF\u03C0\u03BB", 37, 1, ""},
	{"\u03C8\u03C5\u03C7\u03BF\u03C0\u03BB", 37, 1, ""},
	{"\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03BC", -1, 1, ""},
	{"\u03BF\u03BB\u03B9\u03B3\u03BF\u03B4\u03B1\u03BC", 42, 1, ""},
	{"\u03BC\u03BF\u03C5\u03C3\u03BF\u03C5\u03BB\u03BC", 42, 1, ""},
	{"\u03B4\u03C1\u03B1\u03B4\u03BF\u03C5\u03BC", 42, 1, ""},
	{"\u03B2\u03C1\u03B1\u03C7\u03BC", 42, 1, ""},
	{"\u03BD", -1, 1, ""},
	{"\u03B1\u03BC\u03B5\u03C1\u03B9\u03BA\u03B1\u03BD", 47, 1, ""},
	{"\u03C0", -1, 1, ""},
	{"\u03B1\u03B4\u03B1\u03C0", 49, 1, ""},
	{"\u03C7\u03B1\u03BC\u03B7\u03BB\u03BF\u03B4\u03B1\u03C0", 49, 1, ""},
	{"\u03C0\u03BF\u03BB\u03C5\u03B4\u03B1\u03C0", 49, 1, ""},
	{"\u03BA\u03BF\u03C0", 49, 1, ""},
	{"\u03C5\u03C0\u03BF\u03BA\u03BF\u03C0", 53, 1, ""},
	{"\u03C4\u03C3\u03BF\u03C0", 49, 1, ""},
	{"\u03C3\u03C0", 49, 1, ""},
	{"\u03B5\u03C1", -1, 1, ""},
	{"\u03B3\u03B5\u03C1", 57, 1, ""},
	{"\u03B2\u03B5\u03C4\u03B5\u03C1", 57, 1, ""},
	{"\u03BB\u03BF\u03C5\u03B8\u03B7\u03C1", -1, 1, ""},
	{"\u03BA\u03BF\u03C1\u03BC\u03BF\u03C1", -1, 1, ""},
	{"\u03C0\u03B5\u03C1\u03B9\u03C4\u03C1", -1, 1, ""},
	{"\u03BF\u03C5\u03C1", -1, 1, ""},
	{"\u03C3", -1, 1, ""},
	{"\u03B2\u03B1\u03C3", 64, 1, ""},
	{"\u03C0\u03BF\u03BB\u03B9\u03C3", 64, 1, ""},
	{"\u03C3\u03B1\u03C1\u03B1\u03BA\u03B1\u03C4\u03C3", 64, 1, ""},
	{"\u03B8\u03C5\u03C3", 64, 1, ""},
	{"\u03B4\u03B9\u03B1\u03C4", -1, 1, ""},
	{"\u03C0\u03BB\u03B1\u03C4", -1, 1, ""},
	{"\u03C4\u03C3\u03B1\u03C1\u03BB\u03B1\u03C4", -1, 1, ""},
	{"\u03C4\u03B5\u03C4", -1, 1, ""},
	{"\u03C0\u03BF\u03C5\u03C1\u03B9\u03C4", -1, 1, ""},
	{"\u03C3\u03BF\u03C5\u03BB\u03C4", -1, 1, ""},
	{"\u03BC\u03B1\u03B9\u03BD\u03C4", -1, 1, ""},
	{"\u03B6\u03C9\u03BD\u03C4", -1, 1, ""},
	{"\u03BA\u03B1\u03C3\u03C4", -1, 1, ""},
	{"\u03C6", -1, 1, ""},
	{"\u03B4\u03B9\u03B1\u03C6", 78, 1, ""},
	{"\u03C3\u03C4\u03B5\u03C6", 78, 1, ""},
	{"\u03C6\u03C9\u03C4\u03BF\u03C3\u03C4\u03B5\u03C6", 80, 1, ""},
	{"\u03C0\u03B5\u03C1\u03B7\u03C6", 78, 1, ""},
	{"\u03C5\u03C0\u03B5\u03C1\u03B7\u03C6", 82, 1, ""},
	{"\u03BA\u03BF\u03B9\u03BB\u03B1\u03C1\u03C6", 78, 1, ""},
	{"\u03C0\u03B5\u03BD\u03C4\u03B1\u03C1\u03C6", 78, 1, ""},
	{"\u03BF\u03C1\u03C6", 78, 1, ""},
	{"\u03C7", -1, 1, ""},
	{"\u03B1\u03BC\u03B7\u03C7", 87, 1, ""},
	{"\u03B2\u03B9\u03BF\u03BC\u03B7\u03C7", 87, 1, ""},
	{"\u03BC\u03B5\u03B3\u03BB\u03BF\u03B2\u03B9\u03BF\u03BC\u03B7\u03C7", 89, 1, ""},
	{"\u03BA\u03B1\u03C0\u03BD\u03BF\u03B2\u03B9\u03BF\u03BC\u03B7\u03C7", 89, 1, ""},
	{"\u03BC\u03B9\u03BA\u03C1\u03BF\u03B2\u03B9\u03BF\u03BC\u03B7\u03C7", 89, 1, ""},
	{"\u03C0\u03BF\u03BB\u03C5\u03BC\u03B7\u03C7", 87, 1, ""},
	{"\u03BB\u03B9\u03C7", 87, 1, ""},
}
var gre_a_40 = []among{
	{"\u03B7\u03C3\u03B5\u03C4\u03B5", -1, 1, ""},
}
var gre_a_41 = []among{
	{"\u03B5\u03BD\u03B4", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B4", -1, 1, ""},
	{"\u03BF\u03B4", -1, 1, ""},
	{"\u03B4\u03B9\u03B1\u03B8", -1, 1, ""},
	{"\u03BA\u03B1\u03B8", -1, 1, ""},
	{"\u03C1\u03B1\u03B8", -1, 1, ""},
	{"\u03C4\u03B1\u03B8", -1, 1, ""},
	{"\u03C4\u03B9\u03B8", -1, 1, ""},
	{"\u03B5\u03BA\u03B8", -1, 1, ""},
	{"\u03B5\u03BD\u03B8", -1, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B8", -1, 1, ""},
	{"\u03C1\u03BF\u03B8", -1, 1, ""},
	{"\u03C5\u03C0\u03B5\u03C1\u03B8", -1, 1, ""},
	{"\u03C3\u03B8", -1, 1, ""},
	{"\u03B5\u03C5\u03B8", -1, 1, ""},
	{"\u03B1\u03C1\u03BA", -1, 1, ""},
	{"\u03C9\u03C6\u03B5\u03BB", -1, 1, ""},
	{"\u03B2\u03BF\u03BB", -1, 1, ""},
	{"\u03B1\u03B9\u03BD", -1, 1, ""},
	{"\u03C0\u03BF\u03BD", -1, 1, ""},
	{"\u03C1\u03BF\u03BD", -1, 1, ""},
	{"\u03C3\u03C5\u03BD", -1, 1, ""},
	{"\u03B2\u03B1\u03C1", -1, 1, ""},
	{"\u03B2\u03C1", -1, 1, ""},
	{"\u03B1\u03B9\u03C1", -1, 1, ""},
	{"\u03C6\u03BF\u03C1", -1, 1, ""},
	{"\u03B5\u03C5\u03C1", -1, 1, ""},
	{"\u03C0\u03C5\u03C1", -1, 1, ""},
	{"\u03C7\u03C9\u03C1", -1, 1, ""},
	{"\u03BD\u03B5\u03C4", -1, 1, ""},
	{"\u03C3\u03C7", -1, 1, ""},
}
var gre_a_42 = []among{
	{"\u03C0\u03B1\u03B3", -1, 1, ""},
	{"\u03B4", -1, 1, ""},
	{"\u03B1\u03B4", 1, 1, ""},
	{"\u03B8", -1, 1, ""},
	{"\u03B1\u03B8", 3, 1, ""},
	{"\u03C4\u03BF\u03BA", -1, 1, ""},
	{"\u03C3\u03BA", -1, 1, ""},
	{"\u03C0\u03B1\u03C1\u03B1\u03BA\u03B1\u03BB", -1, 1, ""},
	{"\u03C3\u03BA\u03B5\u03BB", -1, 1, ""},
	{"\u03B1\u03C0\u03BB", -1, 1, ""},
	{"\u03B5\u03BC", -1, 1, ""},
	{"\u03B1\u03BD", -1, 1, ""},
	{"\u03B2\u03B5\u03BD", -1, 1, ""},
	{"\u03B2\u03B1\u03C1\u03BF\u03BD", -1, 1, ""},
	{"\u03BA\u03BF\u03C0", -1, 1, ""},
	{"\u03C3\u03B5\u03C1\u03C0", -1, 1, ""},
	{"\u03B1\u03B2\u03B1\u03C1", -1, 1, ""},
	{"\u03B5\u03BD\u03B1\u03C1", -1, 1, ""},
	{"\u03B1\u03B2\u03C1", -1, 1, ""},
	{"\u03BC\u03C0\u03BF\u03C1", -1, 1, ""},
	{"\u03B8\u03B1\u03C1\u03C1", -1, 1, ""},
	{"\u03BD\u03C4\u03C1", -1, 1, ""},
	{"\u03C5", -1, 1, ""},
	{"\u03BD\u03B9\u03C6", -1, 1, ""},
	{"\u03C3\u03C5\u03C1\u03C6", -1, 1, ""},
}
var gre_a_43 = []among{
	{"\u03BF\u03BD\u03C4\u03B1\u03C3", -1, 1, ""},
	{"\u03C9\u03BD\u03C4\u03B1\u03C3", -1, 1, ""},
}
var gre_a_44 = []among{
	{"\u03BF\u03BC\u03B1\u03C3\u03C4\u03B5", -1, 1, ""},
	{"\u03B9\u03BF\u03BC\u03B1\u03C3\u03C4\u03B5", 0, 1, ""},
}
var gre_a_45 = []among{
	{"\u03C0", -1, 1, ""},
	{"\u03B1\u03C0", 0, 1, ""},
	{"\u03B1\u03BA\u03B1\u03C4\u03B1\u03C0", 1, 1, ""},
	{"\u03C3\u03C5\u03BC\u03C0", 0, 1, ""},
	{"\u03B1\u03C3\u03C5\u03BC\u03C0", 3, 1, ""},
	{"\u03B1\u03BC\u03B5\u03C4\u03B1\u03BC\u03C6", -1, 1, ""},
}
var gre_a_46 = []among{
	{"\u03B6", -1, 1, ""},
	{"\u03B1\u03BB", -1, 1, ""},
	{"\u03C0\u03B1\u03C1\u03B1\u03BA\u03B1\u03BB", 1, 1, ""},
	{"\u03B5\u03BA\u03C4\u03B5\u03BB", -1, 1, ""},
	{"\u03BC", -1, 1, ""},
	{"\u03BE", -1, 1, ""},
	{"\u03C0\u03C1\u03BF", -1, 1, ""},
	{"\u03B1\u03C1", -1, 1, ""},
	{"\u03BD\u03B9\u03C3", -1, 1, ""},
}
var gre_a_47 = []among{
	{"\u03B7\u03B8\u03B7\u03BA\u03B1", -1, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B5", -1, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B5\u03C3", -1, 1, ""},
}
var gre_a_48 = []among{
	{"\u03C0\u03B9\u03B8", -1, 1, ""},
	{"\u03BF\u03B8", -1, 1, ""},
	{"\u03BD\u03B1\u03C1\u03B8", -1, 1, ""},
	{"\u03C3\u03BA\u03BF\u03C5\u03BB", -1, 1, ""},
	{"\u03C3\u03BA\u03C9\u03BB", -1, 1, ""},
	{"\u03C3\u03C6", -1, 1, ""},
}
var gre_a_49 = []among{
	{"\u03B8", -1, 1, ""},
	{"\u03B4\u03B9\u03B1\u03B8", 0, 1, ""},
	{"\u03C0\u03B1\u03C1\u03B1\u03BA\u03B1\u03C4\u03B1\u03B8", 0, 1, ""},
	{"\u03C3\u03C5\u03BD\u03B8", 0, 1, ""},
	{"\u03C0\u03C1\u03BF\u03C3\u03B8", 0, 1, ""},
}
var gre_a_50 = []among{
	{"\u03B7\u03BA\u03B1", -1, 1, ""},
	{"\u03B7\u03BA\u03B5", -1, 1, ""},
	{"\u03B7\u03BA\u03B5\u03C3", -1, 1, ""},
}
var gre_a_51 = []among{
	{"\u03C6\u03B1\u03B3", -1, 1, ""},
	{"\u03BB\u03B7\u03B3", -1, 1, ""},
	{"\u03C6\u03C1\u03C5\u03B4", -1, 1, ""},
	{"\u03BC\u03B1\u03BD\u03C4\u03B9\u03BB", -1, 1, ""},
	{"\u03BC\u03B1\u03BB\u03BB", -1, 1, ""},
	{"\u03BF\u03BC", -1, 1, ""},
	{"\u03B2\u03BB\u03B5\u03C0", -1, 1, ""},
	{"\u03C0\u03BF\u03B4\u03B1\u03C1", -1, 1, ""},
	{"\u03BA\u03C5\u03BC\u03B1\u03C4", -1, 1, ""},
	{"\u03C0\u03C1\u03C9\u03C4", -1, 1, ""},
	{"\u03BB\u03B1\u03C7", -1, 1, ""},
	{"\u03C0\u03B1\u03BD\u03C4\u03B1\u03C7", -1, 1, ""},
}
var gre_a_52 = []among{
	{"\u03C4\u03C3\u03B1", -1, 1, ""},
	{"\u03C7\u03B1\u03B4", -1, 1, ""},
	{"\u03BC\u03B5\u03B4", -1, 1, ""},
	{"\u03BB\u03B1\u03BC\u03C0\u03B9\u03B4", -1, 1, ""},
	{"\u03B4\u03B5", -1, 1, ""},
	{"\u03C0\u03BB\u03B5", -1, 1, ""},
	{"\u03BC\u03B5\u03C3\u03B1\u03B6", -1, 1, ""},
	{"\u03B4\u03B5\u03C3\u03C0\u03BF\u03B6", -1, 1, ""},
	{"\u03B1\u03B9\u03B8", -1, 1, ""},
	{"\u03C6\u03B1\u03C1\u03BC\u03B1\u03BA", -1, 1, ""},
	{"\u03B1\u03B3\u03BA", -1, 1, ""},
	{"\u03B1\u03BD\u03B7\u03BA", -1, 1, ""},
	{"\u03BB", -1, 1, ""},
	{"\u03BC", -1, 1, ""},
	{"\u03B1\u03BC", 13, 1, ""},
	{"\u03B2\u03C1\u03BF\u03BC", 13, 1, ""},
	{"\u03C5\u03C0\u03BF\u03C4\u03B5\u03B9\u03BD", -1, 1, ""},
	{"\u03B5\u03BA\u03BB\u03B9\u03C0", -1, 1, ""},
	{"\u03C1", -1, 1, ""},
	{"\u03B5\u03BD\u03B4\u03B9\u03B1\u03C6\u03B5\u03C1", 18, 1, ""},
	{"\u03B1\u03BD\u03B1\u03C1\u03C1", 18, 1, ""},
	{"\u03C0\u03B1\u03C4", -1, 1, ""},
	{"\u03BA\u03B1\u03B8\u03B1\u03C1\u03B5\u03C5", -1, 1, ""},
	{"\u03B4\u03B5\u03C5\u03C4\u03B5\u03C1\u03B5\u03C5", -1, 1, ""},
	{"\u03BB\u03B5\u03C7", -1, 1, ""},
}
var gre_a_53 = []among{
	{"\u03BF\u03C5\u03C3\u03B1", -1, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B5", -1, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B5\u03C3", -1, 1, ""},
}
var gre_a_54 = []among{
	{"\u03C0\u03B5\u03BB", -1, 1, ""},
	{"\u03BB\u03BB", -1, 1, ""},
	{"\u03C3\u03BC\u03B7\u03BD", -1, 1, ""},
	{"\u03C1\u03C0", -1, 1, ""},
	{"\u03C0\u03C1", -1, 1, ""},
	{"\u03C6\u03C1", -1, 1, ""},
	{"\u03C7\u03BF\u03C1\u03C4", -1, 1, ""},
	{"\u03BF\u03C6", -1, 1, ""},
	{"\u03C8\u03BF\u03C6", 7, -1, ""},
	{"\u03C3\u03C6", -1, 1, ""},
	{"\u03BB\u03BF\u03C7", -1, 1, ""},
	{"\u03BD\u03B1\u03C5\u03BB\u03BF\u03C7", 10, -1, ""},
}
var gre_a_55 = []among{
	{"\u03B1\u03BC\u03B1\u03BB\u03BB\u03B9", -1, 1, ""},
	{"\u03BB", -1, 1, ""},
	{"\u03B1\u03BC\u03B1\u03BB", 1, 1, ""},
	{"\u03BC", -1, 1, ""},
	{"\u03BF\u03C5\u03BB\u03B1\u03BC", 3, 1, ""},
	{"\u03B5\u03BD", -1, 1, ""},
	{"\u03B4\u03B5\u03C1\u03B2\u03B5\u03BD", 5, 1, ""},
	{"\u03C0", -1, 1, ""},
	{"\u03B1\u03B5\u03B9\u03C0", 7, 1, ""},
	{"\u03B1\u03C1\u03C4\u03B9\u03C0", 7, 1, ""},
	{"\u03C3\u03C5\u03BC\u03C0", 7, 1, ""},
	{"\u03BD\u03B5\u03BF\u03C0", 7, 1, ""},
	{"\u03BA\u03C1\u03BF\u03BA\u03B1\u03BB\u03BF\u03C0", 7, 1, ""},
	{"\u03BF\u03BB\u03BF\u03C0", 7, 1, ""},
	{"\u03C0\u03C1\u03BF\u03C3\u03C9\u03C0\u03BF\u03C0", 7, 1, ""},
	{"\u03C3\u03B9\u03B4\u03B7\u03C1\u03BF\u03C0", 7, 1, ""},
	{"\u03B4\u03C1\u03BF\u03C3\u03BF\u03C0", 7, 1, ""},
	{"\u03B1\u03C3\u03C0", 7, 1, ""},
	{"\u03B1\u03BD\u03C5\u03C0", 7, 1, ""},
	{"\u03C1", -1, 1, ""},
	{"\u03B1\u03C3\u03C0\u03B1\u03C1", 19, 1, ""},
	{"\u03C7\u03B1\u03C1", 19, 1, ""},
	{"\u03B1\u03C7\u03B1\u03C1", 21, 1, ""},
	{"\u03B1\u03C0\u03B5\u03C1", 19, 1, ""},
	{"\u03C4\u03C1", 19, 1, ""},
	{"\u03BF\u03C5\u03C1", 19, 1, ""},
	{"\u03C4", -1, 1, ""},
	{"\u03B4\u03B9\u03B1\u03C4", 26, 1, ""},
	{"\u03B5\u03C0\u03B9\u03C4", 26, 1, ""},
	{"\u03C3\u03C5\u03BD\u03C4", 26, 1, ""},
	{"\u03BF\u03BC\u03BF\u03C4", 26, 1, ""},
	{"\u03BD\u03BF\u03BC\u03BF\u03C4", 30, 1, ""},
	{"\u03B1\u03C0\u03BF\u03C4", 26, 1, ""},
	{"\u03C5\u03C0\u03BF\u03C4", 26, 1, ""},
	{"\u03B1\u03B2\u03B1\u03C3\u03C4", 26, 1, ""},
	{"\u03B1\u03B9\u03BC\u03BF\u03C3\u03C4", 26, 1, ""},
	{"\u03C0\u03C1\u03BF\u03C3\u03C4", 26, 1, ""},
	{"\u03B1\u03BD\u03C5\u03C3\u03C4", 26, 1, ""},
	{"\u03BD\u03B1\u03C5", -1, 1, ""},
	{"\u03B1\u03C6", -1, 1, ""},
	{"\u03BE\u03B5\u03C6", -1, 1, ""},
	{"\u03B1\u03B4\u03B7\u03C6", -1, 1, ""},
	{"\u03C0\u03B1\u03BC\u03C6", -1, 1, ""},
	{"\u03C0\u03BF\u03BB\u03C5\u03C6", -1, 1, ""},
}
var gre_a_56 = []among{
	{"\u03B1\u03B3\u03B1", -1, 1, ""},
	{"\u03B1\u03B3\u03B5", -1, 1, ""},
	{"\u03B1\u03B3\u03B5\u03C3", -1, 1, ""},
}
var gre_a_57 = []among{
	{"\u03B7\u03C3\u03B1", -1, 1, ""},
	{"\u03B7\u03C3\u03B5", -1, 1, ""},
	{"\u03B7\u03C3\u03BF\u03C5", -1, 1, ""},
}
var gre_a_58 = []among{
	{"\u03BD", -1, 1, ""},
	{"\u03B4\u03C9\u03B4\u03B5\u03BA\u03B1\u03BD", 0, 1, ""},
	{"\u03B5\u03C0\u03C4\u03B1\u03BD", 0, 1, ""},
	{"\u03BC\u03B5\u03B3\u03B1\u03BB\u03BF\u03BD", 0, 1, ""},
	{"\u03B5\u03C1\u03B7\u03BC\u03BF\u03BD", 0, 1, ""},
	{"\u03C7\u03B5\u03C1\u03C3\u03BF\u03BD", 0, 1, ""},
}
var gre_a_59 = []among{
	{"\u03B7\u03C3\u03C4\u03B5", -1, 1, ""},
}
var gre_a_60 = []among{
	{"\u03C3\u03B2", -1, 1, ""},
	{"\u03B1\u03C3\u03B2", 0, 1, ""},
	{"\u03B1\u03C0\u03BB", -1, 1, ""},
	{"\u03B1\u03B5\u03B9\u03BC\u03BD", -1, 1, ""},
	{"\u03C7\u03C1", -1, 1, ""},
	{"\u03B1\u03C7\u03C1", 4, 1, ""},
	{"\u03BA\u03BF\u03B9\u03BD\u03BF\u03C7\u03C1", 4, 1, ""},
	{"\u03B4\u03C5\u03C3\u03C7\u03C1", 4, 1, ""},
	{"\u03B5\u03C5\u03C7\u03C1", 4, 1, ""},
	{"\u03C0\u03B1\u03BB\u03B9\u03BC\u03C8", -1, 1, ""},
}
var gre_a_61 = []among{
	{"\u03BF\u03C5\u03BD\u03B5", -1, 1, ""},
	{"\u03B7\u03B8\u03BF\u03C5\u03BD\u03B5", 0, 1, ""},
	{"\u03B7\u03C3\u03BF\u03C5\u03BD\u03B5", 0, 1, ""},
}
var gre_a_62 = []among{
	{"\u03C3\u03C0\u03B9", -1, 1, ""},
	{"\u03BD", -1, 1, ""},
	{"\u03B5\u03BE\u03C9\u03BD", 1, 1, ""},
	{"\u03C1", -1, 1, ""},
	{"\u03C3\u03C4\u03C1\u03B1\u03B2\u03BF\u03BC\u03BF\u03C5\u03C4\u03C3", -1, 1, ""},
	{"\u03BA\u03B1\u03BA\u03BF\u03BC\u03BF\u03C5\u03C4\u03C3", -1, 1, ""},
}
var gre_a_63 = []among{
	{"\u03BF\u03C5\u03BC\u03B5", -1, 1, ""},
	{"\u03B7\u03B8\u03BF\u03C5\u03BC\u03B5", 0, 1, ""},
	{"\u03B7\u03C3\u03BF\u03C5\u03BC\u03B5", 0, 1, ""},
}
var gre_a_64 = []among{
	{"\u03B1\u03B6", -1, 1, ""},
	{"\u03C9\u03C1\u03B9\u03BF\u03C0\u03BB", -1, 1, ""},
	{"\u03B1\u03C3\u03BF\u03C5\u03C3", -1, 1, ""},
	{"\u03C0\u03B1\u03C1\u03B1\u03C3\u03BF\u03C5\u03C3", 2, 1, ""},
	{"\u03B1\u03BB\u03BB\u03BF\u03C3\u03BF\u03C5\u03C3", -1, 1, ""},
	{"\u03C6", -1, 1, ""},
	{"\u03C7", -1, 1, ""},
}
var gre_a_65 = []among{
	{"\u03BC\u03B1\u03C4\u03B1", -1, 1, ""},
	{"\u03BC\u03B1\u03C4\u03C9\u03BD", -1, 1, ""},
	{"\u03BC\u03B1\u03C4\u03BF\u03C3", -1, 1, ""},
}
var gre_a_66 = []among{
	{"\u03B1", -1, 1, ""},
	{"\u03B9\u03BF\u03C5\u03BC\u03B1", 0, 1, ""},
	{"\u03BF\u03BC\u03BF\u03C5\u03BD\u03B1", 0, 1, ""},
	{"\u03B9\u03BF\u03BC\u03BF\u03C5\u03BD\u03B1", 2, 1, ""},
	{"\u03BF\u03C3\u03BF\u03C5\u03BD\u03B1", 0, 1, ""},
	{"\u03B9\u03BF\u03C3\u03BF\u03C5\u03BD\u03B1", 4, 1, ""},
	{"\u03B5", -1, 1, ""},
	{"\u03B1\u03B3\u03B1\u03C4\u03B5", 6, 1, ""},
	{"\u03B7\u03BA\u03B1\u03C4\u03B5", 6, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B1\u03C4\u03B5", 8, 1, ""},
	{"\u03B7\u03C3\u03B1\u03C4\u03B5", 6, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B1\u03C4\u03B5", 6, 1, ""},
	{"\u03B5\u03B9\u03C4\u03B5", 6, 1, ""},
	{"\u03B7\u03B8\u03B5\u03B9\u03C4\u03B5", 12, 1, ""},
	{"\u03B9\u03B5\u03BC\u03B1\u03C3\u03C4\u03B5", 6, 1, ""},
	{"\u03BF\u03C5\u03BC\u03B1\u03C3\u03C4\u03B5", 6, 1, ""},
	{"\u03B9\u03BF\u03C5\u03BC\u03B1\u03C3\u03C4\u03B5", 15, 1, ""},
	{"\u03B9\u03B5\u03C3\u03B1\u03C3\u03C4\u03B5", 6, 1, ""},
	{"\u03BF\u03C3\u03B1\u03C3\u03C4\u03B5", 6, 1, ""},
	{"\u03B9\u03BF\u03C3\u03B1\u03C3\u03C4\u03B5", 18, 1, ""},
	{"\u03B7", -1, 1, ""},
	{"\u03B9", -1, 1, ""},
	{"\u03B1\u03BC\u03B1\u03B9", 21, 1, ""},
	{"\u03B9\u03B5\u03BC\u03B1\u03B9", 21, 1, ""},
	{"\u03BF\u03BC\u03B1\u03B9", 21, 1, ""},
	{"\u03BF\u03C5\u03BC\u03B1\u03B9", 21, 1, ""},
	{"\u03B1\u03C3\u03B1\u03B9", 21, 1, ""},
	{"\u03B5\u03C3\u03B1\u03B9", 21, 1, ""},
	{"\u03B9\u03B5\u03C3\u03B1\u03B9", 27, 1, ""},
	{"\u03B1\u03C4\u03B1\u03B9", 21, 1, ""},
	{"\u03B5\u03C4\u03B1\u03B9", 21, 1, ""},
	{"\u03B9\u03B5\u03C4\u03B1\u03B9", 30, 1, ""},
	{"\u03BF\u03BD\u03C4\u03B1\u03B9", 21, 1, ""},
	{"\u03BF\u03C5\u03BD\u03C4\u03B1\u03B9", 21, 1, ""},
	{"\u03B9\u03BF\u03C5\u03BD\u03C4\u03B1\u03B9", 33, 1, ""},
	{"\u03B5\u03B9", 21, 1, ""},
	{"\u03B1\u03B5\u03B9", 35, 1, ""},
	{"\u03B7\u03B8\u03B5\u03B9", 35, 1, ""},
	{"\u03B7\u03C3\u03B5\u03B9", 35, 1, ""},
	{"\u03BF\u03B9", 21, 1, ""},
	{"\u03B1\u03BD", -1, 1, ""},
	{"\u03B1\u03B3\u03B1\u03BD", 40, 1, ""},
	{"\u03B7\u03BA\u03B1\u03BD", 40, 1, ""},
	{"\u03B7\u03B8\u03B7\u03BA\u03B1\u03BD", 42, 1, ""},
	{"\u03B7\u03C3\u03B1\u03BD", 40, 1, ""},
	{"\u03BF\u03C5\u03C3\u03B1\u03BD", 40, 1, ""},
	{"\u03BF\u03BD\u03C4\u03BF\u03C5\u03C3\u03B1\u03BD", 45, 1, ""},
	{"\u03B9\u03BF\u03BD\u03C4\u03BF\u03C5\u03C3\u03B1\u03BD", 46, 1, ""},
	{"\u03BF\u03BD\u03C4\u03B1\u03BD", 40, 1, ""},
	{"\u03B9\u03BF\u03BD\u03C4\u03B1\u03BD", 48, 1, ""},
	{"\u03BF\u03C5\u03BD\u03C4\u03B1\u03BD", 40, 1, ""},
	{"\u03B9\u03BF\u03C5\u03BD\u03C4\u03B1\u03BD", 50, 1, ""},
	{"\u03BF\u03C4\u03B1\u03BD", 40, 1, ""},
	{"\u03B9\u03BF\u03C4\u03B1\u03BD", 52, 1, ""},
	{"\u03BF\u03BC\u03B1\u03C3\u03C4\u03B1\u03BD", 40, 1, ""},
	{"\u03B9\u03BF\u03BC\u03B1\u03C3\u03C4\u03B1\u03BD", 54, 1, ""},
	{"\u03BF\u03C3\u03B1\u03C3\u03C4\u03B1\u03BD", 40, 1, ""},
	{"\u03B9\u03BF\u03C3\u03B1\u03C3\u03C4\u03B1\u03BD", 56, 1, ""},
	{"\u03BF\u03C5\u03BD", -1, 1, ""},
	{"\u03B7\u03B8\u03BF\u03C5\u03BD", 58, 1, ""},
	{"\u03BF\u03BC\u03BF\u03C5\u03BD", 58, 1, ""},
	{"\u03B9\u03BF\u03BC\u03BF\u03C5\u03BD", 60, 1, ""},
	{"\u03B7\u03C3\u03BF\u03C5\u03BD", 58, 1, ""},
	{"\u03BF\u03C3\u03BF\u03C5\u03BD", 58, 1, ""},
	{"\u03B9\u03BF\u03C3\u03BF\u03C5\u03BD", 63, 1, ""},
	{"\u03C9\u03BD", -1, 1, ""},
	{"\u03B7\u03B4\u03C9\u03BD", 65, 1, ""},
	{"\u03BF", -1, 1, ""},
	{"\u03B1\u03C3", -1, 1, ""},
	{"\u03B5\u03C3", -1, 1, ""},
	{"\u03B7\u03B4\u03B5\u03C3", 69, 1, ""},
	{"\u03B7\u03C3\u03B5\u03C3", 69, 1, ""},
	{"\u03B7\u03C3", -1, 1, ""},
	{"\u03B5\u03B9\u03C3", -1, 1, ""},
	{"\u03B7\u03B8\u03B5\u03B9\u03C3", 73, 1, ""},
	{"\u03BF\u03C3", -1, 1, ""},
	{"\u03C5\u03C3", -1, 1, ""},
	{"\u03BF\u03C5\u03C3", 76, 1, ""},
	{"\u03C5", -1, 1, ""},
	{"\u03BF\u03C5", 78, 1, ""},
	{"\u03C9", -1, 1, ""},
	{"\u03B1\u03C9", 80, 1, ""},
	{"\u03B7\u03B8\u03C9", 80, 1, ""},
	{"\u03B7\u03C3\u03C9", 80, 1, ""},
}
var gre_a_67 = []among{
	{"\u03BF\u03C4\u03B5\u03C1", -1, 1, ""},
	{"\u03B5\u03C3\u03C4\u03B5\u03C1", -1, 1, ""},
	{"\u03C5\u03C4\u03B5\u03C1", -1, 1, ""},
	{"\u03C9\u03C4\u03B5\u03C1", -1, 1, ""},
	{"\u03BF\u03C4\u03B1\u03C4", -1, 1, ""},
	{"\u03B5\u03C3\u03C4\u03B1\u03C4", -1, 1, ""},
	{"\u03C5\u03C4\u03B1\u03C4", -1, 1, ""},
	{"\u03C9\u03C4\u03B1\u03C4", -1, 1, ""},
}
var gre_g_v = []byte{81, 65, 16, 1}
var gre_g_v2 = []byte{81, 65, 0, 1}

func (z *greekStemmer) r_has_min_length() bool {
	return z.length >= 3
}
func (z *greekStemmer) r_tolower() bool {
	var among_var int
	for {
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			among_var = z.findAmongB(gre_a_0)
			z.bra = z.cursor
			switch among_var {
			case 1:
				z.sliceFrom("\u03B1")
				break
			case 2:
				z.sliceFrom("\u03B2")
				break
			case 3:
				z.sliceFrom("\u03B3")
				break
			case 4:
				z.sliceFrom("\u03B4")
				break
			case 5:
				z.sliceFrom("\u03B5")
				break
			case 6:
				z.sliceFrom("\u03B6")
				break
			case 7:
				z.sliceFrom("\u03B7")
				break
			case 8:
				z.sliceFrom("\u03B8")
				break
			case 9:
				z.sliceFrom("\u03B9")
				break
			case 10:
				z.sliceFrom("\u03BA")
				break
			case 11:
				z.sliceFrom("\u03BB")
				break
			case 12:
				z.sliceFrom("\u03BC")
				break
			case 13:
				z.sliceFrom("\u03BD")
				break
			case 14:
				z.sliceFrom("\u03BE")
				break
			case 15:
				z.sliceFrom("\u03BF")
				break
			case 16:
				z.sliceFrom("\u03C0")
				break
			case 17:
				z.sliceFrom("\u03C1")
				break
			case 18:
				z.sliceFrom("\u03C3")
				break
			case 19:
				z.sliceFrom("\u03C4")
				break
			case 20:
				z.sliceFrom("\u03C5")
				break
			case 21:
				z.sliceFrom("\u03C6")
				break
			case 22:
				z.sliceFrom("\u03C7")
				break
			case 23:
				z.sliceFrom("\u03C8")
				break
			case 24:
				z.sliceFrom("\u03C9")
				break
			case 25:
				if z.cursor <= z.limitBackward {
					goto lab0_end
				}
				z.cursor--
				break
			}
			continue
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
		break
	}
	return true
}
func (z *greekStemmer) r_step1() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(gre_a_1)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("\u03C6\u03B1")
		break
	case 2:
		z.sliceFrom("\u03C3\u03BA\u03B1")
		break
	case 3:
		z.sliceFrom("\u03BF\u03BB\u03BF")
		break
	case 4:
		z.sliceFrom("\u03C3\u03BF")
		break
	case 5:
		z.sliceFrom("\u03C4\u03B1\u03C4\u03BF")
		break
	case 6:
		z.sliceFrom("\u03BA\u03C1\u03B5")
		break
	case 7:
		z.sliceFrom("\u03C0\u03B5\u03C1")
		break
	case 8:
		z.sliceFrom("\u03C4\u03B5\u03C1")
		break
	case 9:
		z.sliceFrom("\u03C6\u03C9")
		break
	case 10:
		z.sliceFrom("\u03BA\u03B1\u03B8\u03B5\u03C3\u03C4")
		break
	case 11:
		z.sliceFrom("\u03B3\u03B5\u03B3\u03BF\u03BD")
		break
	}
	z.B_test1 = false
	return true
}
func (z *greekStemmer) r_steps1() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	among_var = z.findAmongB(gre_a_2)
	if among_var == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("\u03B9")
		break
	case 2:
		z.sliceFrom("\u03B9\u03B6")
		break
	}
	return true
}
func (z *greekStemmer) r_steps2() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_5) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_4) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03C9\u03BD")
	return true
}
func (z *greekStemmer) r_steps3() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_7) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			if !(z.eqSB("\u03B9\u03C3\u03B1")) {
				goto lab1_end
			}
			if z.cursor > z.limitBackward {
				goto lab1_end
			}
			z.sliceFrom("\u03B9\u03C3")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		z.bra = z.cursor
		among_var = z.findAmongB(gre_a_6)
		if among_var == 0 {
			return false
		}
		if z.cursor > z.limitBackward {
			return false
		}
		switch among_var {
		case 1:
			z.sliceFrom("\u03B9")
			break
		case 2:
			z.sliceFrom("\u03B9\u03C3")
			break
		}
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_steps4() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_9) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_8) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B9")
	return true
}
func (z *greekStemmer) r_steps5() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_11) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	among_var = z.findAmongB(gre_a_10)
	if among_var == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("\u03B9")
		break
	case 2:
		z.sliceFrom("\u03B9\u03C3\u03C4")
		break
	}
	return true
}
func (z *greekStemmer) r_steps6() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_14) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			among_var = z.findAmongB(gre_a_12)
			if among_var == 0 {
				goto lab1_end
			}
			if z.cursor > z.limitBackward {
				goto lab1_end
			}
			switch among_var {
			case 1:
				z.sliceFrom("\u03B9\u03C3\u03BC")
				break
			case 2:
				z.sliceFrom("\u03B9")
				break
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		among_var = z.findAmongB(gre_a_13)
		if among_var == 0 {
			return false
		}
		z.bra = z.cursor
		switch among_var {
		case 1:
			z.sliceFrom("\u03B1\u03B3\u03BD\u03C9\u03C3\u03C4")
			break
		case 2:
			z.sliceFrom("\u03B1\u03C4\u03BF\u03BC")
			break
		case 3:
			z.sliceFrom("\u03B3\u03BD\u03C9\u03C3\u03C4")
			break
		case 4:
			z.sliceFrom("\u03B5\u03B8\u03BD")
			break
		case 5:
			z.sliceFrom("\u03B5\u03BA\u03BB\u03B5\u03BA\u03C4")
			break
		case 6:
			z.sliceFrom("\u03C3\u03BA\u03B5\u03C0\u03C4")
			break
		case 7:
			z.sliceFrom("\u03C4\u03BF\u03C0")
			break
		case 8:
			z.sliceFrom("\u03B1\u03BB\u03B5\u03BE\u03B1\u03BD\u03B4\u03C1")
			break
		case 9:
			z.sliceFrom("\u03B2\u03C5\u03B6\u03B1\u03BD\u03C4")
			break
		case 10:
			z.sliceFrom("\u03B8\u03B5\u03B1\u03C4\u03C1")
			break
		}
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_steps7() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_16) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_15) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B1\u03C1\u03B1\u03BA")
	return true
}
func (z *greekStemmer) r_steps8() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_18) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			among_var = z.findAmongB(gre_a_17)
			if among_var == 0 {
				goto lab1_end
			}
			if z.cursor > z.limitBackward {
				goto lab1_end
			}
			switch among_var {
			case 1:
				z.sliceFrom("\u03B1\u03BA")
				break
			case 2:
				z.sliceFrom("\u03B9\u03C4\u03C3")
				break
			}
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		z.bra = z.cursor
		if !(z.eqSB("\u03BA\u03BF\u03C1")) {
			return false
		}
		z.sliceFrom("\u03B9\u03C4\u03C3")
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_steps9() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_21) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if z.findAmongB(gre_a_19) == 0 {
				goto lab1_end
			}
			if z.cursor > z.limitBackward {
				goto lab1_end
			}
			z.sliceFrom("\u03B9\u03B4")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		z.bra = z.cursor
		if z.findAmongB(gre_a_20) == 0 {
			return false
		}
		z.sliceFrom("\u03B9\u03B4")
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_steps10() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_23) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_22) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B9\u03C3\u03BA")
	return true
}
func (z *greekStemmer) r_step2a() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_24) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	{
		v_1 := z.limit - z.cursor
		{
			if z.findAmongB(gre_a_25) == 0 {
				goto lab0_end
			}
			return false
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
	}
	{
		c := z.cursor
		z.insert(z.cursor, z.cursor, "\u03B1\u03B4")
		z.cursor = c
	}
	return true
}
func (z *greekStemmer) r_step2b() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_26) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_27) == 0 {
		return false
	}
	z.sliceFrom("\u03B5\u03B4")
	return true
}
func (z *greekStemmer) r_step2c() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_28) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_29) == 0 {
		return false
	}
	z.sliceFrom("\u03BF\u03C5\u03B4")
	return true
}
func (z *greekStemmer) r_step2d() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_30) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_31) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B5")
	return true
}
func (z *greekStemmer) r_step3() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_32) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if !(z.inGroupingB(gre_g_v, 945, 969)) {
		return false
	}
	z.sliceFrom("\u03B9")
	return true
}
func (z *greekStemmer) r_step4() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_33) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if !(z.inGroupingB(gre_g_v, 945, 969)) {
				goto lab1_end
			}
			z.sliceFrom("\u03B9\u03BA")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
	}
lab0_end:
	;
	z.bra = z.cursor
	if z.findAmongB(gre_a_34) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B9\u03BA")
	return true
}
func (z *greekStemmer) r_step5a() bool {
	v_1 := z.limit - z.cursor
	{
		if !(z.eqSB("\u03B1\u03B3\u03B1\u03BC\u03B5")) {
			goto lab0_end
		}
		if z.cursor > z.limitBackward {
			goto lab0_end
		}
		z.sliceFrom("\u03B1\u03B3\u03B1\u03BC")
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	v_2 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(gre_a_35) == 0 {
			goto lab1_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_test1 = false
	}
lab1_end:
	;
	z.cursor = z.limit - v_2
	z.ket = z.cursor
	if !(z.eqSB("\u03B1\u03BC\u03B5")) {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_36) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B1\u03BC")
	return true
}
func (z *greekStemmer) r_step5b() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(gre_a_38) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_test1 = false
		z.ket = z.cursor
		z.bra = z.cursor
		if z.findAmongB(gre_a_37) == 0 {
			goto lab0_end
		}
		if z.cursor > z.limitBackward {
			goto lab0_end
		}
		z.sliceFrom("\u03B1\u03B3\u03B1\u03BD")
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if !(z.eqSB("\u03B1\u03BD\u03B5")) {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if !(z.inGroupingB(gre_g_v2, 945, 969)) {
				goto lab2_end
			}
			z.sliceFrom("\u03B1\u03BD")
			goto lab1_end
		}
	lab2_end:
		;
		z.cursor = z.limit - v_2
		z.ket = z.cursor
	}
lab1_end:
	;
	z.bra = z.cursor
	if z.findAmongB(gre_a_39) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B1\u03BD")
	return true
}
func (z *greekStemmer) r_step5c() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(gre_a_40) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_test1 = false
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if !(z.eqSB("\u03B5\u03C4\u03B5")) {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if !(z.inGroupingB(gre_g_v2, 945, 969)) {
				goto lab2_end
			}
			z.sliceFrom("\u03B5\u03C4")
			goto lab1_end
		}
	lab2_end:
		;
		z.cursor = z.limit - v_2
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if z.findAmongB(gre_a_41) == 0 {
				goto lab3_end
			}
			z.sliceFrom("\u03B5\u03C4")
			goto lab1_end
		}
	lab3_end:
		;
		z.cursor = z.limit - v_2
		z.ket = z.cursor
	}
lab1_end:
	;
	z.bra = z.cursor
	if z.findAmongB(gre_a_42) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B5\u03C4")
	return true
}
func (z *greekStemmer) r_step5d() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_43) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if !(z.eqSB("\u03B1\u03C1\u03C7")) {
				goto lab1_end
			}
			if z.cursor > z.limitBackward {
				goto lab1_end
			}
			z.sliceFrom("\u03BF\u03BD\u03C4")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		z.bra = z.cursor
		if !(z.eqSB("\u03BA\u03C1\u03B5")) {
			return false
		}
		z.sliceFrom("\u03C9\u03BD\u03C4")
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_step5e() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_44) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if !(z.eqSB("\u03BF\u03BD")) {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03BF\u03BC\u03B1\u03C3\u03C4")
	return true
}
func (z *greekStemmer) r_step5f() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if !(z.eqSB("\u03B9\u03B5\u03C3\u03C4\u03B5")) {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_test1 = false
		z.ket = z.cursor
		z.bra = z.cursor
		if z.findAmongB(gre_a_45) == 0 {
			goto lab0_end
		}
		if z.cursor > z.limitBackward {
			goto lab0_end
		}
		z.sliceFrom("\u03B9\u03B5\u03C3\u03C4")
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if !(z.eqSB("\u03B5\u03C3\u03C4\u03B5")) {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_46) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B9\u03B5\u03C3\u03C4")
	return true
}
func (z *greekStemmer) r_step5g() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(gre_a_47) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceDel()
		z.B_test1 = false
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	z.ket = z.cursor
	if z.findAmongB(gre_a_50) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_2 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if z.findAmongB(gre_a_48) == 0 {
				goto lab2_end
			}
			z.sliceFrom("\u03B7\u03BA")
			goto lab1_end
		}
	lab2_end:
		;
		z.cursor = z.limit - v_2
		z.ket = z.cursor
		z.bra = z.cursor
		if z.findAmongB(gre_a_49) == 0 {
			return false
		}
		if z.cursor > z.limitBackward {
			return false
		}
		z.sliceFrom("\u03B7\u03BA")
	}
lab1_end:
	;
	return true
}
func (z *greekStemmer) r_step5h() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_53) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if z.findAmongB(gre_a_51) == 0 {
				goto lab1_end
			}
			z.sliceFrom("\u03BF\u03C5\u03C3")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		z.ket = z.cursor
		z.bra = z.cursor
		if z.findAmongB(gre_a_52) == 0 {
			return false
		}
		if z.cursor > z.limitBackward {
			return false
		}
		z.sliceFrom("\u03BF\u03C5\u03C3")
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_step5i() bool {
	var among_var int
	z.ket = z.cursor
	if z.findAmongB(gre_a_56) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	{
		v_1 := z.limit - z.cursor
		{
			z.ket = z.cursor
			z.bra = z.cursor
			if !(z.eqSB("\u03BA\u03BF\u03BB\u03BB")) {
				goto lab1_end
			}
			z.sliceFrom("\u03B1\u03B3")
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = z.limit - v_1
		{
			v_2 := z.limit - z.cursor
			{
				z.ket = z.cursor
				z.bra = z.cursor
				among_var = z.findAmongB(gre_a_54)
				if among_var == 0 {
					goto lab3_end
				}
				switch among_var {
				case 1:
					z.sliceFrom("\u03B1\u03B3")
					break
				}
				goto lab2_end
			}
		lab3_end:
			;
			z.cursor = z.limit - v_2
			z.ket = z.cursor
			z.bra = z.cursor
			if z.findAmongB(gre_a_55) == 0 {
				return false
			}
			if z.cursor > z.limitBackward {
				return false
			}
			z.sliceFrom("\u03B1\u03B3")
		}
	lab2_end:
	}
lab0_end:
	;
	return true
}
func (z *greekStemmer) r_step5j() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_57) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_58) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B7\u03C3")
	return true
}
func (z *greekStemmer) r_step5k() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_59) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_60) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03B7\u03C3\u03C4")
	return true
}
func (z *greekStemmer) r_step5l() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_61) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_62) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03BF\u03C5\u03BD")
	return true
}
func (z *greekStemmer) r_step5m() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_63) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	z.B_test1 = false
	z.ket = z.cursor
	z.bra = z.cursor
	if z.findAmongB(gre_a_64) == 0 {
		return false
	}
	if z.cursor > z.limitBackward {
		return false
	}
	z.sliceFrom("\u03BF\u03C5\u03BC")
	return true
}
func (z *greekStemmer) r_step6() bool {
	v_1 := z.limit - z.cursor
	{
		z.ket = z.cursor
		if z.findAmongB(gre_a_65) == 0 {
			goto lab0_end
		}
		z.bra = z.cursor
		z.sliceFrom("\u03BC\u03B1")
	}
lab0_end:
	;
	z.cursor = z.limit - v_1
	if !(z.B_test1) {
		return false
	}
	z.ket = z.cursor
	if z.findAmongB(gre_a_66) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *greekStemmer) r_step7() bool {
	z.ket = z.cursor
	if z.findAmongB(gre_a_67) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *greekStemmer) stem() {
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_1 := z.limit - z.cursor
	z.r_tolower()
	z.cursor = z.limit - v_1
	if !z.r_has_min_length() {
		return
	}
	z.B_test1 = true
	v_2 := z.limit - z.cursor
	z.r_step1()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_steps1()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_steps2()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_steps3()
	z.cursor = z.limit - v_5
	v_6 := z.limit - z.cursor
	z.r_steps4()
	z.cursor = z.limit - v_6
	v_7 := z.limit - z.cursor
	z.r_steps5()
	z.cursor = z.limit - v_7
	v_8 := z.limit - z.cursor
	z.r_steps6()
	z.cursor = z.limit - v_8
	v_9 := z.limit - z.cursor
	z.r_steps7()
	z.cursor = z.limit - v_9
	v_10 := z.limit - z.cursor
	z.r_steps8()
	z.cursor = z.limit - v_10
	v_11 := z.limit - z.cursor
	z.r_steps9()
	z.cursor = z.limit - v_11
	v_12 := z.limit - z.cursor
	z.r_steps10()
	z.cursor = z.limit - v_12
	v_13 := z.limit - z.cursor
	z.r_step2a()
	z.cursor = z.limit - v_13
	v_14 := z.limit - z.cursor
	z.r_step2b()
	z.cursor = z.limit - v_14
	v_15 := z.limit - z.cursor
	z.r_step2c()
	z.cursor = z.limit - v_15
	v_16 := z.limit - z.cursor
	z.r_step2d()
	z.cursor = z.limit - v_16
	v_17 := z.limit - z.cursor
	z.r_step3()
	z.cursor = z.limit - v_17
	v_18 := z.limit - z.cursor
	z.r_step4()
	z.cursor = z.limit - v_18
	v_19 := z.limit - z.cursor
	z.r_step5a()
	z.cursor = z.limit - v_19
	v_20 := z.limit - z.cursor
	z.r_step5b()
	z.cursor = z.limit - v_20
	v_21 := z.limit - z.cursor
	z.r_step5c()
	z.cursor = z.limit - v_21
	v_22 := z.limit - z.cursor
	z.r_step5d()
	z.cursor = z.limit - v_22
	v_23 := z.limit - z.cursor
	z.r_step5e()
	z.cursor = z.limit - v_23
	v_24 := z.limit - z.cursor
	z.r_step5f()
	z.cursor = z.limit - v_24
	v_25 := z.limit - z.cursor
	z.r_step5g()
	z.cursor = z.limit - v_25
	v_26 := z.limit - z.cursor
	z.r_step5h()
	z.cursor = z.limit - v_26
	v_27 := z.limit - z.cursor
	z.r_step5j()
	z.cursor = z.limit - v_27
	v_28 := z.limit - z.cursor
	z.r_step5i()
	z.cursor = z.limit - v_28
	v_29 := z.limit - z.cursor
	z.r_step5k()
	z.cursor = z.limit - v_29
	v_30 := z.limit - z.cursor
	z.r_step5l()
	z.cursor = z.limit - v_30
	v_31 := z.limit - z.cursor
	z.r_step5m()
	z.cursor = z.limit - v_31
	v_32 := z.limit - z.cursor
	z.r_step6()
	z.cursor = z.limit - v_32
	v_33 := z.limit - z.cursor
	z.r_step7()
	z.cursor = z.limit - v_33
	z.cursor = z.limitBackward
	return
}

type hungarianStemmer struct {
	snowball
	I_p1 int
}

func newHungarian() *hungarianStemmer { return &hungarianStemmer{} }

var hun_a_0 = []among{
	{"cs", -1, -1, ""},
	{"dzs", -1, -1, ""},
	{"gy", -1, -1, ""},
	{"ly", -1, -1, ""},
	{"ny", -1, -1, ""},
	{"sz", -1, -1, ""},
	{"ty", -1, -1, ""},
	{"zs", -1, -1, ""},
}
var hun_a_1 = []among{
	{"\u00E1", -1, 1, ""},
	{"\u00E9", -1, 2, ""},
}
var hun_a_2 = []among{
	{"bb", -1, -1, ""},
	{"cc", -1, -1, ""},
	{"dd", -1, -1, ""},
	{"ff", -1, -1, ""},
	{"gg", -1, -1, ""},
	{"jj", -1, -1, ""},
	{"kk", -1, -1, ""},
	{"ll", -1, -1, ""},
	{"mm", -1, -1, ""},
	{"nn", -1, -1, ""},
	{"pp", -1, -1, ""},
	{"rr", -1, -1, ""},
	{"ccs", -1, -1, ""},
	{"ss", -1, -1, ""},
	{"zzs", -1, -1, ""},
	{"tt", -1, -1, ""},
	{"vv", -1, -1, ""},
	{"ggy", -1, -1, ""},
	{"lly", -1, -1, ""},
	{"nny", -1, -1, ""},
	{"tty", -1, -1, ""},
	{"ssz", -1, -1, ""},
	{"zz", -1, -1, ""},
}
var hun_a_3 = []among{
	{"al", -1, 1, ""},
	{"el", -1, 1, ""},
}
var hun_a_4 = []among{
	{"ba", -1, -1, ""},
	{"ra", -1, -1, ""},
	{"be", -1, -1, ""},
	{"re", -1, -1, ""},
	{"ig", -1, -1, ""},
	{"nak", -1, -1, ""},
	{"nek", -1, -1, ""},
	{"val", -1, -1, ""},
	{"vel", -1, -1, ""},
	{"ul", -1, -1, ""},
	{"n\u00E1l", -1, -1, ""},
	{"n\u00E9l", -1, -1, ""},
	{"b\u00F3l", -1, -1, ""},
	{"r\u00F3l", -1, -1, ""},
	{"t\u00F3l", -1, -1, ""},
	{"\u00FCl", -1, -1, ""},
	{"b\u0151l", -1, -1, ""},
	{"r\u0151l", -1, -1, ""},
	{"t\u0151l", -1, -1, ""},
	{"n", -1, -1, ""},
	{"an", 19, -1, ""},
	{"ban", 20, -1, ""},
	{"en", 19, -1, ""},
	{"ben", 22, -1, ""},
	{"k\u00E9ppen", 22, -1, ""},
	{"on", 19, -1, ""},
	{"\u00F6n", 19, -1, ""},
	{"k\u00E9pp", -1, -1, ""},
	{"kor", -1, -1, ""},
	{"t", -1, -1, ""},
	{"at", 29, -1, ""},
	{"et", 29, -1, ""},
	{"k\u00E9nt", 29, -1, ""},
	{"ank\u00E9nt", 32, -1, ""},
	{"enk\u00E9nt", 32, -1, ""},
	{"onk\u00E9nt", 32, -1, ""},
	{"ot", 29, -1, ""},
	{"\u00E9rt", 29, -1, ""},
	{"\u00F6t", 29, -1, ""},
	{"hez", -1, -1, ""},
	{"hoz", -1, -1, ""},
	{"h\u00F6z", -1, -1, ""},
	{"v\u00E1", -1, -1, ""},
	{"v\u00E9", -1, -1, ""},
}
var hun_a_5 = []among{
	{"\u00E1n", -1, 2, ""},
	{"\u00E9n", -1, 1, ""},
	{"\u00E1nk\u00E9nt", -1, 2, ""},
}
var hun_a_6 = []among{
	{"stul", -1, 1, ""},
	{"astul", 0, 1, ""},
	{"\u00E1stul", 0, 2, ""},
	{"st\u00FCl", -1, 1, ""},
	{"est\u00FCl", 3, 1, ""},
	{"\u00E9st\u00FCl", 3, 3, ""},
}
var hun_a_7 = []among{
	{"\u00E1", -1, 1, ""},
	{"\u00E9", -1, 1, ""},
}
var hun_a_8 = []among{
	{"k", -1, 3, ""},
	{"ak", 0, 3, ""},
	{"ek", 0, 3, ""},
	{"ok", 0, 3, ""},
	{"\u00E1k", 0, 1, ""},
	{"\u00E9k", 0, 2, ""},
	{"\u00F6k", 0, 3, ""},
}
var hun_a_9 = []among{
	{"\u00E9i", -1, 1, ""},
	{"\u00E1\u00E9i", 0, 3, ""},
	{"\u00E9\u00E9i", 0, 2, ""},
	{"\u00E9", -1, 1, ""},
	{"k\u00E9", 3, 1, ""},
	{"ak\u00E9", 4, 1, ""},
	{"ek\u00E9", 4, 1, ""},
	{"ok\u00E9", 4, 1, ""},
	{"\u00E1k\u00E9", 4, 3, ""},
	{"\u00E9k\u00E9", 4, 2, ""},
	{"\u00F6k\u00E9", 4, 1, ""},
	{"\u00E9\u00E9", 3, 2, ""},
}
var hun_a_10 = []among{
	{"a", -1, 1, ""},
	{"ja", 0, 1, ""},
	{"d", -1, 1, ""},
	{"ad", 2, 1, ""},
	{"ed", 2, 1, ""},
	{"od", 2, 1, ""},
	{"\u00E1d", 2, 2, ""},
	{"\u00E9d", 2, 3, ""},
	{"\u00F6d", 2, 1, ""},
	{"e", -1, 1, ""},
	{"je", 9, 1, ""},
	{"nk", -1, 1, ""},
	{"unk", 11, 1, ""},
	{"\u00E1nk", 11, 2, ""},
	{"\u00E9nk", 11, 3, ""},
	{"\u00FCnk", 11, 1, ""},
	{"uk", -1, 1, ""},
	{"juk", 16, 1, ""},
	{"\u00E1juk", 17, 2, ""},
	{"\u00FCk", -1, 1, ""},
	{"j\u00FCk", 19, 1, ""},
	{"\u00E9j\u00FCk", 20, 3, ""},
	{"m", -1, 1, ""},
	{"am", 22, 1, ""},
	{"em", 22, 1, ""},
	{"om", 22, 1, ""},
	{"\u00E1m", 22, 2, ""},
	{"\u00E9m", 22, 3, ""},
	{"o", -1, 1, ""},
	{"\u00E1", -1, 2, ""},
	{"\u00E9", -1, 3, ""},
}
var hun_a_11 = []among{
	{"id", -1, 1, ""},
	{"aid", 0, 1, ""},
	{"jaid", 1, 1, ""},
	{"eid", 0, 1, ""},
	{"jeid", 3, 1, ""},
	{"\u00E1id", 0, 2, ""},
	{"\u00E9id", 0, 3, ""},
	{"i", -1, 1, ""},
	{"ai", 7, 1, ""},
	{"jai", 8, 1, ""},
	{"ei", 7, 1, ""},
	{"jei", 10, 1, ""},
	{"\u00E1i", 7, 2, ""},
	{"\u00E9i", 7, 3, ""},
	{"itek", -1, 1, ""},
	{"eitek", 14, 1, ""},
	{"jeitek", 15, 1, ""},
	{"\u00E9itek", 14, 3, ""},
	{"ik", -1, 1, ""},
	{"aik", 18, 1, ""},
	{"jaik", 19, 1, ""},
	{"eik", 18, 1, ""},
	{"jeik", 21, 1, ""},
	{"\u00E1ik", 18, 2, ""},
	{"\u00E9ik", 18, 3, ""},
	{"ink", -1, 1, ""},
	{"aink", 25, 1, ""},
	{"jaink", 26, 1, ""},
	{"eink", 25, 1, ""},
	{"jeink", 28, 1, ""},
	{"\u00E1ink", 25, 2, ""},
	{"\u00E9ink", 25, 3, ""},
	{"aitok", -1, 1, ""},
	{"jaitok", 32, 1, ""},
	{"\u00E1itok", -1, 2, ""},
	{"im", -1, 1, ""},
	{"aim", 35, 1, ""},
	{"jaim", 36, 1, ""},
	{"eim", 35, 1, ""},
	{"jeim", 38, 1, ""},
	{"\u00E1im", 35, 2, ""},
	{"\u00E9im", 35, 3, ""},
}
var hun_g_v = []byte{17, 65, 16, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 17, 36, 10, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 1, 0, 0, 0, 1}

func (z *hungarianStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	{
		v_1 := z.cursor
		{
			if !(z.inGrouping(hun_g_v, 97, 369)) {
				goto lab1_end
			}
		golab2:
			for {
				v_2 := z.cursor
				{
					if !(z.outGrouping(hun_g_v, 97, 369)) {
						goto lab3_end
					}
					z.cursor = v_2
					break golab2
				}
			lab3_end:
				;
				z.cursor = v_2
				if z.cursor >= z.limit {
					goto lab1_end
				}
				z.cursor++
			}
			{
				v_3 := z.cursor
				{
					if z.findAmong(hun_a_0) == 0 {
						goto lab5_end
					}
					goto lab4_end
				}
			lab5_end:
				;
				z.cursor = v_3
				if z.cursor >= z.limit {
					goto lab1_end
				}
				z.cursor++
			}
		lab4_end:
			;
			z.I_p1 = z.cursor
			goto lab0_end
		}
	lab1_end:
		;
		z.cursor = v_1
		if !(z.outGrouping(hun_g_v, 97, 369)) {
			return false
		}
	golab6:
		for {
			{
				if !(z.inGrouping(hun_g_v, 97, 369)) {
					goto lab7_end
				}
				break golab6
			}
		lab7_end:
			;
			if z.cursor >= z.limit {
				return false
			}
			z.cursor++
		}
		z.I_p1 = z.cursor
	}
lab0_end:
	;
	return true
}
func (z *hungarianStemmer) r_R1() bool {
	return z.I_p1 <= z.cursor
}
func (z *hungarianStemmer) r_v_ending() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_1)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("a")
		break
	case 2:
		z.sliceFrom("e")
		break
	}
	return true
}
func (z *hungarianStemmer) r_double() bool {
	v_1 := z.limit - z.cursor
	if z.findAmongB(hun_a_2) == 0 {
		return false
	}
	z.cursor = z.limit - v_1
	return true
}
func (z *hungarianStemmer) r_undouble() bool {
	if z.cursor <= z.limitBackward {
		return false
	}
	z.cursor--
	z.ket = z.cursor
	{
		c := z.cursor - 1
		if c < z.limitBackward {
			return false
		}
		z.cursor = c
	}
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *hungarianStemmer) r_instrum() bool {
	z.ket = z.cursor
	if z.findAmongB(hun_a_3) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	if !z.r_double() {
		return false
	}
	z.sliceDel()
	if !z.r_undouble() {
		return false
	}
	return true
}
func (z *hungarianStemmer) r_case() bool {
	z.ket = z.cursor
	if z.findAmongB(hun_a_4) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	z.sliceDel()
	if !z.r_v_ending() {
		return false
	}
	return true
}
func (z *hungarianStemmer) r_case_special() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_5)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("e")
		break
	case 2:
		z.sliceFrom("a")
		break
	}
	return true
}
func (z *hungarianStemmer) r_case_other() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_6)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("a")
		break
	case 3:
		z.sliceFrom("e")
		break
	}
	return true
}
func (z *hungarianStemmer) r_factive() bool {
	z.ket = z.cursor
	if z.findAmongB(hun_a_7) == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	if !z.r_double() {
		return false
	}
	z.sliceDel()
	if !z.r_undouble() {
		return false
	}
	return true
}
func (z *hungarianStemmer) r_plural() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_8)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceFrom("a")
		break
	case 2:
		z.sliceFrom("e")
		break
	case 3:
		z.sliceDel()
		break
	}
	return true
}
func (z *hungarianStemmer) r_owned() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_9)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("e")
		break
	case 3:
		z.sliceFrom("a")
		break
	}
	return true
}
func (z *hungarianStemmer) r_sing_owner() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_10)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("a")
		break
	case 3:
		z.sliceFrom("e")
		break
	}
	return true
}
func (z *hungarianStemmer) r_plur_owner() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(hun_a_11)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	if !z.r_R1() {
		return false
	}
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		z.sliceFrom("a")
		break
	case 3:
		z.sliceFrom("e")
		break
	}
	return true
}
func (z *hungarianStemmer) stem() {
	v_1 := z.cursor
	z.r_mark_regions()
	z.cursor = v_1
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_instrum()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_case()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_case_special()
	z.cursor = z.limit - v_4
	v_5 := z.limit - z.cursor
	z.r_case_other()
	z.cursor = z.limit - v_5
	v_6 := z.limit - z.cursor
	z.r_factive()
	z.cursor = z.limit - v_6
	v_7 := z.limit - z.cursor
	z.r_owned()
	z.cursor = z.limit - v_7
	v_8 := z.limit - z.cursor
	z.r_sing_owner()
	z.cursor = z.limit - v_8
	v_9 := z.limit - z.cursor
	z.r_plur_owner()
	z.cursor = z.limit - v_9
	v_10 := z.limit - z.cursor
	z.r_plural()
	z.cursor = z.limit - v_10
	z.cursor = z.limitBackward
	return
}

type lithuanianStemmer struct {
	snowball
	I_p1 int
}

func newLithuanian() *lithuanianStemmer { return &lithuanianStemmer{} }

var lit_a_0 = []among{
	{"a", -1, -1, ""},
	{"ia", 0, -1, ""},
	{"eria", 1, -1, ""},
	{"osna", 0, -1, ""},
	{"iosna", 3, -1, ""},
	{"uosna", 3, -1, ""},
	{"iuosna", 5, -1, ""},
	{"ysna", 0, -1, ""},
	{"\u0117sna", 0, -1, ""},
	{"e", -1, -1, ""},
	{"ie", 9, -1, ""},
	{"enie", 10, -1, ""},
	{"erie", 10, -1, ""},
	{"oje", 9, -1, ""},
	{"ioje", 13, -1, ""},
	{"uje", 9, -1, ""},
	{"iuje", 15, -1, ""},
	{"yje", 9, -1, ""},
	{"enyje", 17, -1, ""},
	{"eryje", 17, -1, ""},
	{"\u0117je", 9, -1, ""},
	{"ame", 9, -1, ""},
	{"iame", 21, -1, ""},
	{"sime", 9, -1, ""},
	{"ome", 9, -1, ""},
	{"\u0117me", 9, -1, ""},
	{"tum\u0117me", 25, -1, ""},
	{"ose", 9, -1, ""},
	{"iose", 27, -1, ""},
	{"uose", 27, -1, ""},
	{"iuose", 29, -1, ""},
	{"yse", 9, -1, ""},
	{"enyse", 31, -1, ""},
	{"eryse", 31, -1, ""},
	{"\u0117se", 9, -1, ""},
	{"ate", 9, -1, ""},
	{"iate", 35, -1, ""},
	{"ite", 9, -1, ""},
	{"kite", 37, -1, ""},
	{"site", 37, -1, ""},
	{"ote", 9, -1, ""},
	{"tute", 9, -1, ""},
	{"\u0117te", 9, -1, ""},
	{"tum\u0117te", 42, -1, ""},
	{"i", -1, -1, ""},
	{"ai", 44, -1, ""},
	{"iai", 45, -1, ""},
	{"eriai", 46, -1, ""},
	{"ei", 44, -1, ""},
	{"tumei", 48, -1, ""},
	{"ki", 44, -1, ""},
	{"imi", 44, -1, ""},
	{"erimi", 51, -1, ""},
	{"umi", 44, -1, ""},
	{"iumi", 53, -1, ""},
	{"si", 44, -1, ""},
	{"asi", 55, -1, ""},
	{"iasi", 56, -1, ""},
	{"esi", 55, -1, ""},
	{"iesi", 58, -1, ""},
	{"siesi", 59, -1, ""},
	{"isi", 55, -1, ""},
	{"aisi", 61, -1, ""},
	{"eisi", 61, -1, ""},
	{"tumeisi", 63, -1, ""},
	{"uisi", 61, -1, ""},
	{"osi", 55, -1, ""},
	{"\u0117josi", 66, -1, ""},
	{"uosi", 66, -1, ""},
	{"iuosi", 68, -1, ""},
	{"siuosi", 69, -1, ""},
	{"usi", 55, -1, ""},
	{"ausi", 71, -1, ""},
	{"\u010Diausi", 72, -1, ""},
	{"\u0105si", 55, -1, ""},
	{"\u0117si", 55, -1, ""},
	{"\u0173si", 55, -1, ""},
	{"t\u0173si", 76, -1, ""},
	{"ti", 44, -1, ""},
	{"enti", 78, -1, ""},
	{"inti", 78, -1, ""},
	{"oti", 78, -1, ""},
	{"ioti", 81, -1, ""},
	{"uoti", 81, -1, ""},
	{"iuoti", 83, -1, ""},
	{"auti", 78, -1, ""},
	{"iauti", 85, -1, ""},
	{"yti", 78, -1, ""},
	{"\u0117ti", 78, -1, ""},
	{"tel\u0117ti", 88, -1, ""},
	{"in\u0117ti", 88, -1, ""},
	{"ter\u0117ti", 88, -1, ""},
	{"ui", 44, -1, ""},
	{"iui", 92, -1, ""},
	{"eniui", 93, -1, ""},
	{"oj", -1, -1, ""},
	{"\u0117j", -1, -1, ""},
	{"k", -1, -1, ""},
	{"am", -1, -1, ""},
	{"iam", 98, -1, ""},
	{"iem", -1, -1, ""},
	{"im", -1, -1, ""},
	{"sim", 101, -1, ""},
	{"om", -1, -1, ""},
	{"tum", -1, -1, ""},
	{"\u0117m", -1, -1, ""},
	{"tum\u0117m", 105, -1, ""},
	{"an", -1, -1, ""},
	{"on", -1, -1, ""},
	{"ion", 108, -1, ""},
	{"un", -1, -1, ""},
	{"iun", 110, -1, ""},
	{"\u0117n", -1, -1, ""},
	{"o", -1, -1, ""},
	{"io", 113, -1, ""},
	{"enio", 114, -1, ""},
	{"\u0117jo", 113, -1, ""},
	{"uo", 113, -1, ""},
	{"s", -1, -1, ""},
	{"as", 118, -1, ""},
	{"ias", 119, -1, ""},
	{"es", 118, -1, ""},
	{"ies", 121, -1, ""},
	{"is", 118, -1, ""},
	{"ais", 123, -1, ""},
	{"iais", 124, -1, ""},
	{"tumeis", 123, -1, ""},
	{"imis", 123, -1, ""},
	{"enimis", 127, -1, ""},
	{"omis", 123, -1, ""},
	{"iomis", 129, -1, ""},
	{"umis", 123, -1, ""},
	{"\u0117mis", 123, -1, ""},
	{"enis", 123, -1, ""},
	{"asis", 123, -1, ""},
	{"ysis", 123, -1, ""},
	{"ams", 118, -1, ""},
	{"iams", 136, -1, ""},
	{"iems", 118, -1, ""},
	{"ims", 118, -1, ""},
	{"enims", 139, -1, ""},
	{"erims", 139, -1, ""},
	{"oms", 118, -1, ""},
	{"ioms", 142, -1, ""},
	{"ums", 118, -1, ""},
	{"\u0117ms", 118, -1, ""},
	{"ens", 118, -1, ""},
	{"os", 118, -1, ""},
	{"ios", 147, -1, ""},
	{"uos", 147, -1, ""},
	{"iuos", 149, -1, ""},
	{"ers", 118, -1, ""},
	{"us", 118, -1, ""},
	{"aus", 152, -1, ""},
	{"iaus", 153, -1, ""},
	{"ius", 152, -1, ""},
	{"ys", 118, -1, ""},
	{"enys", 156, -1, ""},
	{"erys", 156, -1, ""},
	{"\u0105s", 118, -1, ""},
	{"i\u0105s", 159, -1, ""},
	{"\u0117s", 118, -1, ""},
	{"am\u0117s", 161, -1, ""},
	{"iam\u0117s", 162, -1, ""},
	{"im\u0117s", 161, -1, ""},
	{"kim\u0117s", 164, -1, ""},
	{"sim\u0117s", 164, -1, ""},
	{"om\u0117s", 161, -1, ""},
	{"\u0117m\u0117s", 161, -1, ""},
	{"tum\u0117m\u0117s", 168, -1, ""},
	{"at\u0117s", 161, -1, ""},
	{"iat\u0117s", 170, -1, ""},
	{"sit\u0117s", 161, -1, ""},
	{"ot\u0117s", 161, -1, ""},
	{"\u0117t\u0117s", 161, -1, ""},
	{"tum\u0117t\u0117s", 174, -1, ""},
	{"\u012Fs", 118, -1, ""},
	{"\u016Bs", 118, -1, ""},
	{"t\u0173s", 118, -1, ""},
	{"at", -1, -1, ""},
	{"iat", 179, -1, ""},
	{"it", -1, -1, ""},
	{"sit", 181, -1, ""},
	{"ot", -1, -1, ""},
	{"\u0117t", -1, -1, ""},
	{"tum\u0117t", 184, -1, ""},
	{"u", -1, -1, ""},
	{"au", 186, -1, ""},
	{"iau", 187, -1, ""},
	{"\u010Diau", 188, -1, ""},
	{"iu", 186, -1, ""},
	{"eniu", 190, -1, ""},
	{"siu", 190, -1, ""},
	{"y", -1, -1, ""},
	{"\u0105", -1, -1, ""},
	{"i\u0105", 194, -1, ""},
	{"\u0117", -1, -1, ""},
	{"\u0119", -1, -1, ""},
	{"\u012F", -1, -1, ""},
	{"en\u012F", 198, -1, ""},
	{"er\u012F", 198, -1, ""},
	{"\u0173", -1, -1, ""},
	{"i\u0173", 201, -1, ""},
	{"er\u0173", 201, -1, ""},
}
var lit_a_1 = []among{
	{"ing", -1, -1, ""},
	{"aj", -1, -1, ""},
	{"iaj", 1, -1, ""},
	{"iej", -1, -1, ""},
	{"oj", -1, -1, ""},
	{"ioj", 4, -1, ""},
	{"uoj", 4, -1, ""},
	{"iuoj", 6, -1, ""},
	{"auj", -1, -1, ""},
	{"\u0105j", -1, -1, ""},
	{"i\u0105j", 9, -1, ""},
	{"\u0117j", -1, -1, ""},
	{"\u0173j", -1, -1, ""},
	{"i\u0173j", 12, -1, ""},
	{"ok", -1, -1, ""},
	{"iok", 14, -1, ""},
	{"iuk", -1, -1, ""},
	{"uliuk", 16, -1, ""},
	{"u\u010Diuk", 16, -1, ""},
	{"i\u0161k", -1, -1, ""},
	{"iul", -1, -1, ""},
	{"yl", -1, -1, ""},
	{"\u0117l", -1, -1, ""},
	{"am", -1, -1, ""},
	{"dam", 23, -1, ""},
	{"jam", 23, -1, ""},
	{"zgan", -1, -1, ""},
	{"ain", -1, -1, ""},
	{"esn", -1, -1, ""},
	{"op", -1, -1, ""},
	{"iop", 29, -1, ""},
	{"ias", -1, -1, ""},
	{"ies", -1, -1, ""},
	{"ais", -1, -1, ""},
	{"iais", 33, -1, ""},
	{"os", -1, -1, ""},
	{"ios", 35, -1, ""},
	{"uos", 35, -1, ""},
	{"iuos", 37, -1, ""},
	{"aus", -1, -1, ""},
	{"iaus", 39, -1, ""},
	{"\u0105s", -1, -1, ""},
	{"i\u0105s", 41, -1, ""},
	{"\u0119s", -1, -1, ""},
	{"ut\u0117ait", -1, -1, ""},
	{"ant", -1, -1, ""},
	{"iant", 45, -1, ""},
	{"siant", 46, -1, ""},
	{"int", -1, -1, ""},
	{"ot", -1, -1, ""},
	{"uot", 49, -1, ""},
	{"iuot", 50, -1, ""},
	{"yt", -1, -1, ""},
	{"\u0117t", -1, -1, ""},
	{"yk\u0161t", -1, -1, ""},
	{"iau", -1, -1, ""},
	{"dav", -1, -1, ""},
	{"sv", -1, -1, ""},
	{"\u0161v", -1, -1, ""},
	{"yk\u0161\u010D", -1, -1, ""},
	{"\u0119", -1, -1, ""},
	{"\u0117j\u0119", 60, -1, ""},
}
var lit_a_2 = []among{
	{"ojime", -1, 7, ""},
	{"\u0117jime", -1, 3, ""},
	{"avime", -1, 6, ""},
	{"okate", -1, 8, ""},
	{"aite", -1, 1, ""},
	{"uote", -1, 2, ""},
	{"asius", -1, 5, ""},
	{"okat\u0117s", -1, 8, ""},
	{"ait\u0117s", -1, 1, ""},
	{"uot\u0117s", -1, 2, ""},
	{"esiu", -1, 4, ""},
}
var lit_a_3 = []among{
	{"\u010D", -1, 1, ""},
	{"d\u017E", -1, 2, ""},
}
var lit_a_4 = []among{
	{"gd", -1, 1, ""},
}
var lit_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 16, 0, 64, 1, 0, 64, 0, 0, 0, 0, 0, 0, 0, 4, 4}

func (z *lithuanianStemmer) r_step1() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(lit_a_0) == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	z.sliceDel()
	return true
}
func (z *lithuanianStemmer) r_step2() bool {
	for {
		v_1 := z.limit - z.cursor
		{
			if z.cursor < z.I_p1 {
				goto lab0_end
			}
			v_3 := z.limitBackward
			z.limitBackward = z.I_p1
			z.ket = z.cursor
			if z.findAmongB(lit_a_1) == 0 {
				z.limitBackward = v_3
				goto lab0_end
			}
			z.bra = z.cursor
			z.limitBackward = v_3
			z.sliceDel()
			continue
		}
	lab0_end:
		;
		z.cursor = z.limit - v_1
		break
	}
	return true
}
func (z *lithuanianStemmer) r_fix_conflicts() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(lit_a_2)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("ait\u0117")
		break
	case 2:
		z.sliceFrom("uot\u0117")
		break
	case 3:
		z.sliceFrom("\u0117jimas")
		break
	case 4:
		z.sliceFrom("esys")
		break
	case 5:
		z.sliceFrom("asys")
		break
	case 6:
		z.sliceFrom("avimas")
		break
	case 7:
		z.sliceFrom("ojimas")
		break
	case 8:
		z.sliceFrom("okat\u0117")
		break
	}
	return true
}
func (z *lithuanianStemmer) r_fix_chdz() bool {
	var among_var int
	z.ket = z.cursor
	among_var = z.findAmongB(lit_a_3)
	if among_var == 0 {
		return false
	}
	z.bra = z.cursor
	switch among_var {
	case 1:
		z.sliceFrom("t")
		break
	case 2:
		z.sliceFrom("d")
		break
	}
	return true
}
func (z *lithuanianStemmer) r_fix_gd() bool {
	z.ket = z.cursor
	if z.findAmongB(lit_a_4) == 0 {
		return false
	}
	z.bra = z.cursor
	z.sliceFrom("g")
	return true
}
func (z *lithuanianStemmer) stem() {
	z.I_p1 = z.limit
	v_1 := z.cursor
	{
		v_2 := z.cursor
		{
			v_3 := z.cursor
			if !(z.eqS("a")) {
				z.cursor = v_2
				goto lab1_end
			}
			z.cursor = v_3
			if z.length <= 6 {
				z.cursor = v_2
				goto lab1_end
			}
			{
				c := z.cursor + 1
				if c > z.limit {
					z.cursor = v_2
					goto lab1_end
				}
				z.cursor = c
			}
		}
	lab1_end:
		;
	golab2:
		for {
			{
				if !(z.inGrouping(lit_g_v, 97, 371)) {
					goto lab3_end
				}
				break golab2
			}
		lab3_end:
			;
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		}
	golab4:
		for {
			{
				if !(z.outGrouping(lit_g_v, 97, 371)) {
					goto lab5_end
				}
				break golab4
			}
		lab5_end:
			;
			if z.cursor >= z.limit {
				goto lab0_end
			}
			z.cursor++
		}
		z.I_p1 = z.cursor
	}
lab0_end:
	;
	z.cursor = v_1
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_6 := z.limit - z.cursor
	z.r_fix_conflicts()
	z.cursor = z.limit - v_6
	v_7 := z.limit - z.cursor
	z.r_step1()
	z.cursor = z.limit - v_7
	v_8 := z.limit - z.cursor
	z.r_fix_chdz()
	z.cursor = z.limit - v_8
	v_9 := z.limit - z.cursor
	z.r_step2()
	z.cursor = z.limit - v_9
	v_10 := z.limit - z.cursor
	z.r_fix_chdz()
	z.cursor = z.limit - v_10
	v_11 := z.limit - z.cursor
	z.r_fix_gd()
	z.cursor = z.limit - v_11
	z.cursor = z.limitBackward
	return
}

type norwegianStemmer struct {
	snowball
	I_x  int
	I_p1 int
}

func newNorwegian() *norwegianStemmer { return &norwegianStemmer{} }

var nor_a_0 = []among{
	{"a", -1, 1, ""},
	{"e", -1, 1, ""},
	{"ede", 1, 1, ""},
	{"ande", 1, 1, ""},
	{"ende", 1, 1, ""},
	{"ane", 1, 1, ""},
	{"ene", 1, 1, ""},
	{"hetene", 6, 1, ""},
	{"erte", 1, 3, ""},
	{"en", -1, 1, ""},
	{"heten", 9, 1, ""},
	{"ar", -1, 1, ""},
	{"er", -1, 1, ""},
	{"heter", 12, 1, ""},
	{"s", -1, 2, ""},
	{"as", 14, 1, ""},
	{"es", 14, 1, ""},
	{"edes", 16, 1, ""},
	{"endes", 16, 1, ""},
	{"enes", 16, 1, ""},
	{"hetenes", 19, 1, ""},
	{"ens", 14, 1, ""},
	{"hetens", 21, 1, ""},
	{"ers", 14, 1, ""},
	{"ets", 14, 1, ""},
	{"et", -1, 1, ""},
	{"het", 25, 1, ""},
	{"ert", -1, 3, ""},
	{"ast", -1, 1, ""},
}
var nor_a_1 = []among{
	{"dt", -1, -1, ""},
	{"vt", -1, -1, ""},
}
var nor_a_2 = []among{
	{"leg", -1, 1, ""},
	{"eleg", 0, 1, ""},
	{"ig", -1, 1, ""},
	{"eig", 2, 1, ""},
	{"lig", 2, 1, ""},
	{"elig", 4, 1, ""},
	{"els", -1, 1, ""},
	{"lov", -1, 1, ""},
	{"elov", 7, 1, ""},
	{"slov", 7, 1, ""},
	{"hetslov", 9, 1, ""},
}
var nor_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 48, 0, 128}
var nor_g_s_ending = []byte{119, 125, 149, 1}

func (z *norwegianStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	v_1 := z.cursor
	{
		c := z.cursor + 3
		if c > z.limit {
			return false
		}
		z.cursor = c
	}
	z.I_x = z.cursor
	z.cursor = v_1
golab0:
	for {
		v_2 := z.cursor
		{
			if !(z.inGrouping(nor_g_v, 97, 248)) {
				goto lab1_end
			}
			z.cursor = v_2
			break golab0
		}
	lab1_end:
		;
		z.cursor = v_2
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab2:
	for {
		{
			if !(z.outGrouping(nor_g_v, 97, 248)) {
				goto lab3_end
			}
			break golab2
		}
	lab3_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= z.I_x {
			goto lab4_end
		}
		z.I_p1 = z.I_x
	}
lab4_end:
	;
	return true
}
func (z *norwegianStemmer) r_main_suffix() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(nor_a_0)
	if among_var == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		{
			v_3 := z.limit - z.cursor
			{
				if !(z.inGroupingB(nor_g_s_ending, 98, 122)) {
					goto lab1_end
				}
				goto lab0_end
			}
		lab1_end:
			;
			z.cursor = z.limit - v_3
			if !(z.eqSB("k")) {
				return false
			}
			if !(z.outGroupingB(nor_g_v, 97, 248)) {
				return false
			}
		}
	lab0_end:
		;
		z.sliceDel()
		break
	case 3:
		z.sliceFrom("er")
		break
	}
	return true
}
func (z *norwegianStemmer) r_consonant_pair() bool {
	v_1 := z.limit - z.cursor
	if z.cursor < z.I_p1 {
		return false
	}
	v_3 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(nor_a_1) == 0 {
		z.limitBackward = v_3
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_3
	z.cursor = z.limit - v_1
	if z.cursor <= z.limitBackward {
		return false
	}
	z.cursor--
	z.bra = z.cursor
	z.sliceDel()
	return true
}
func (z *norwegianStemmer) r_other_suffix() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	if z.findAmongB(nor_a_2) == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	z.sliceDel()
	return true
}
func (z *norwegianStemmer) stem() {
	v_1 := z.cursor
	z.r_mark_regions()
	z.cursor = v_1
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_main_suffix()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_consonant_pair()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_other_suffix()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	return
}

type swedishStemmer struct {
	snowball
	I_x  int
	I_p1 int
}

func newSwedish() *swedishStemmer { return &swedishStemmer{} }

var swe_a_0 = []among{
	{"a", -1, 1, ""},
	{"arna", 0, 1, ""},
	{"erna", 0, 1, ""},
	{"heterna", 2, 1, ""},
	{"orna", 0, 1, ""},
	{"ad", -1, 1, ""},
	{"e", -1, 1, ""},
	{"ade", 6, 1, ""},
	{"ande", 6, 1, ""},
	{"arne", 6, 1, ""},
	{"are", 6, 1, ""},
	{"aste", 6, 1, ""},
	{"en", -1, 1, ""},
	{"anden", 12, 1, ""},
	{"aren", 12, 1, ""},
	{"heten", 12, 1, ""},
	{"ern", -1, 1, ""},
	{"ar", -1, 1, ""},
	{"er", -1, 1, ""},
	{"heter", 18, 1, ""},
	{"or", -1, 1, ""},
	{"s", -1, 2, ""},
	{"as", 21, 1, ""},
	{"arnas", 22, 1, ""},
	{"ernas", 22, 1, ""},
	{"ornas", 22, 1, ""},
	{"es", 21, 1, ""},
	{"ades", 26, 1, ""},
	{"andes", 26, 1, ""},
	{"ens", 21, 1, ""},
	{"arens", 29, 1, ""},
	{"hetens", 29, 1, ""},
	{"erns", 21, 1, ""},
	{"at", -1, 1, ""},
	{"andet", -1, 1, ""},
	{"het", -1, 1, ""},
	{"ast", -1, 1, ""},
}
var swe_a_1 = []among{
	{"dd", -1, -1, ""},
	{"gd", -1, -1, ""},
	{"nn", -1, -1, ""},
	{"dt", -1, -1, ""},
	{"gt", -1, -1, ""},
	{"kt", -1, -1, ""},
	{"tt", -1, -1, ""},
}
var swe_a_2 = []among{
	{"ig", -1, 1, ""},
	{"lig", 0, 1, ""},
	{"els", -1, 1, ""},
	{"fullt", -1, 3, ""},
	{"\u00F6st", -1, 2, ""},
}
var swe_g_v = []byte{17, 65, 16, 1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 24, 0, 32}
var swe_g_s_ending = []byte{119, 127, 149}
var swe_g_ost_ending = []byte{173, 58}

func (z *swedishStemmer) r_mark_regions() bool {
	z.I_p1 = z.limit
	v_1 := z.cursor
	{
		c := z.cursor + 3
		if c > z.limit {
			return false
		}
		z.cursor = c
	}
	z.I_x = z.cursor
	z.cursor = v_1
golab0:
	for {
		v_2 := z.cursor
		{
			if !(z.inGrouping(swe_g_v, 97, 246)) {
				goto lab1_end
			}
			z.cursor = v_2
			break golab0
		}
	lab1_end:
		;
		z.cursor = v_2
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
golab2:
	for {
		{
			if !(z.outGrouping(swe_g_v, 97, 246)) {
				goto lab3_end
			}
			break golab2
		}
	lab3_end:
		;
		if z.cursor >= z.limit {
			return false
		}
		z.cursor++
	}
	z.I_p1 = z.cursor
	{
		if z.I_p1 >= z.I_x {
			goto lab4_end
		}
		z.I_p1 = z.I_x
	}
lab4_end:
	;
	return true
}
func (z *swedishStemmer) r_main_suffix() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(swe_a_0)
	if among_var == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		if !(z.inGroupingB(swe_g_s_ending, 98, 121)) {
			return false
		}
		z.sliceDel()
		break
	}
	return true
}
func (z *swedishStemmer) r_consonant_pair() bool {
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	v_3 := z.limit - z.cursor
	if z.findAmongB(swe_a_1) == 0 {
		z.limitBackward = v_2
		return false
	}
	z.cursor = z.limit - v_3
	z.ket = z.cursor
	if z.cursor <= z.limitBackward {
		z.limitBackward = v_2
		return false
	}
	z.cursor--
	z.bra = z.cursor
	z.sliceDel()
	z.limitBackward = v_2
	return true
}
func (z *swedishStemmer) r_other_suffix() bool {
	var among_var int
	if z.cursor < z.I_p1 {
		return false
	}
	v_2 := z.limitBackward
	z.limitBackward = z.I_p1
	z.ket = z.cursor
	among_var = z.findAmongB(swe_a_2)
	if among_var == 0 {
		z.limitBackward = v_2
		return false
	}
	z.bra = z.cursor
	z.limitBackward = v_2
	switch among_var {
	case 1:
		z.sliceDel()
		break
	case 2:
		if !(z.inGroupingB(swe_g_ost_ending, 105, 118)) {
			return false
		}
		z.sliceFrom("\u00F6s")
		break
	case 3:
		z.sliceFrom("full")
		break
	}
	return true
}
func (z *swedishStemmer) stem() {
	v_1 := z.cursor
	z.r_mark_regions()
	z.cursor = v_1
	z.limitBackward = z.cursor
	z.cursor = z.limit
	v_2 := z.limit - z.cursor
	z.r_main_suffix()
	z.cursor = z.limit - v_2
	v_3 := z.limit - z.cursor
	z.r_consonant_pair()
	z.cursor = z.limit - v_3
	v_4 := z.limit - z.cursor
	z.r_other_suffix()
	z.cursor = z.limit - v_4
	z.cursor = z.limitBackward
	return
}
