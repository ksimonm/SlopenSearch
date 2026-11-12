package main

// A port of the Snowball runtime (SnowballProgram.java) used by the
// generated stemmers in snowball_generated.go. Working on []rune keeps the
// algorithms readable; all supported languages are in the BMP.

type among struct {
	s          string
	substringI int
	result     int
	method     string
}

type snowball struct {
	current       []rune
	cursor        int
	length        int
	limit         int
	limitBackward int
	bra           int
	ket           int
	methods       map[string]func() bool
}

func (z *snowball) callAmongMethod(name string) bool {
	if z.methods == nil {
		return false
	}
	if f, ok := z.methods[name]; ok {
		return f()
	}
	return false
}

func (z *snowball) setCurrent(s string) {
	z.current = []rune(s)
	z.cursor = 0
	z.length = len(z.current)
	z.limit = z.length
	z.limitBackward = 0
	z.bra = z.cursor
	z.ket = z.limit
}

func (z *snowball) getCurrent() string { return string(z.current[:z.length]) }

func (z *snowball) inGrouping(s []byte, min, max int) bool {
	if z.cursor >= z.limit {
		return false
	}
	ch := int(z.current[z.cursor])
	if ch > max || ch < min {
		return false
	}
	ch -= min
	if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
		return false
	}
	z.cursor++
	return true
}

func (z *snowball) goInGrouping(s []byte, min, max int) bool {
	for z.cursor < z.limit {
		ch := int(z.current[z.cursor])
		if ch > max || ch < min {
			return true
		}
		ch -= min
		if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
			return true
		}
		z.cursor++
	}
	return false
}

func (z *snowball) inGroupingB(s []byte, min, max int) bool {
	if z.cursor <= z.limitBackward {
		return false
	}
	ch := int(z.current[z.cursor-1])
	if ch > max || ch < min {
		return false
	}
	ch -= min
	if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
		return false
	}
	z.cursor--
	return true
}

func (z *snowball) goInGroupingB(s []byte, min, max int) bool {
	for z.cursor > z.limitBackward {
		ch := int(z.current[z.cursor-1])
		if ch > max || ch < min {
			return true
		}
		ch -= min
		if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
			return true
		}
		z.cursor--
	}
	return false
}

func (z *snowball) outGrouping(s []byte, min, max int) bool {
	if z.cursor >= z.limit {
		return false
	}
	ch := int(z.current[z.cursor])
	if ch > max || ch < min {
		z.cursor++
		return true
	}
	ch -= min
	if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
		z.cursor++
		return true
	}
	return false
}

func (z *snowball) goOutGrouping(s []byte, min, max int) bool {
	for z.cursor < z.limit {
		ch := int(z.current[z.cursor])
		if ch <= max && ch >= min {
			ch -= min
			if s[ch>>3]&(1<<(uint(ch)&7)) != 0 {
				return true
			}
		}
		z.cursor++
	}
	return false
}

func (z *snowball) outGroupingB(s []byte, min, max int) bool {
	if z.cursor <= z.limitBackward {
		return false
	}
	ch := int(z.current[z.cursor-1])
	if ch > max || ch < min {
		z.cursor--
		return true
	}
	ch -= min
	if s[ch>>3]&(1<<(uint(ch)&7)) == 0 {
		z.cursor--
		return true
	}
	return false
}

func (z *snowball) goOutGroupingB(s []byte, min, max int) bool {
	for z.cursor > z.limitBackward {
		ch := int(z.current[z.cursor-1])
		if ch <= max && ch >= min {
			ch -= min
			if s[ch>>3]&(1<<(uint(ch)&7)) != 0 {
				return true
			}
		}
		z.cursor--
	}
	return false
}

func (z *snowball) eqS(s string) bool {
	rs := []rune(s)
	if z.limit-z.cursor < len(rs) {
		return false
	}
	for i := 0; i != len(rs); i++ {
		if z.current[z.cursor+i] != rs[i] {
			return false
		}
	}
	z.cursor += len(rs)
	return true
}

func (z *snowball) eqSB(s string) bool {
	rs := []rune(s)
	if z.cursor-z.limitBackward < len(rs) {
		return false
	}
	for i := 0; i != len(rs); i++ {
		if z.current[z.cursor-len(rs)+i] != rs[i] {
			return false
		}
	}
	z.cursor -= len(rs)
	return true
}

func (z *snowball) findAmong(v []among) int {
	i, j := 0, len(v)
	c := z.cursor
	l := z.limit
	commonI, commonJ := 0, 0
	firstKeyInspected := false
	for {
		k := i + ((j - i) >> 1)
		diff := 0
		common := commonI
		if commonJ < commonI {
			common = commonJ
		}
		w := v[k]
		wrs := []rune(w.s)
		i2 := common
		for ; i2 < len(wrs); i2++ {
			if c+common == l {
				diff = -1
				break
			}
			diff = int(z.current[c+common]) - int(wrs[i2])
			if diff != 0 {
				break
			}
			common++
		}
		if diff < 0 {
			j = k
			commonJ = common
		} else {
			i = k
			commonI = common
		}
		if j-i <= 1 {
			if i > 0 {
				break
			}
			if j == i {
				break
			}
			if firstKeyInspected {
				break
			}
			firstKeyInspected = true
		}
	}
	for {
		w := v[i]
		wrs := []rune(w.s)
		if commonI >= len(wrs) {
			z.cursor = c + len(wrs)
			if w.method == "" {
				return w.result
			}
			if z.callAmongMethod(w.method) {
				z.cursor = c + len(wrs)
				return w.result
			}
		}
		i = w.substringI
		if i < 0 {
			return 0
		}
	}
}

func (z *snowball) findAmongB(v []among) int {
	i, j := 0, len(v)
	c := z.cursor
	lb := z.limitBackward
	commonI, commonJ := 0, 0
	firstKeyInspected := false
	for {
		k := i + ((j - i) >> 1)
		diff := 0
		common := commonI
		if commonJ < commonI {
			common = commonJ
		}
		w := v[k]
		wrs := []rune(w.s)
		for i2 := len(wrs) - 1 - common; i2 >= 0; i2-- {
			if c-common == lb {
				diff = -1
				break
			}
			diff = int(z.current[c-1-common]) - int(wrs[i2])
			if diff != 0 {
				break
			}
			common++
		}
		if diff < 0 {
			j = k
			commonJ = common
		} else {
			i = k
			commonI = common
		}
		if j-i <= 1 {
			if i > 0 {
				break
			}
			if j == i {
				break
			}
			if firstKeyInspected {
				break
			}
			firstKeyInspected = true
		}
	}
	for {
		w := v[i]
		wrs := []rune(w.s)
		if commonI >= len(wrs) {
			z.cursor = c - len(wrs)
			if w.method == "" {
				return w.result
			}
			if z.callAmongMethod(w.method) {
				z.cursor = c - len(wrs)
				return w.result
			}
		}
		i = w.substringI
		if i < 0 {
			return 0
		}
	}
}

func (z *snowball) replaceS(cBra, cKet int, s string) int {
	rs := []rune(s)
	adjustment := len(rs) - (cKet - cBra)
	newLength := z.length + adjustment
	if newLength > len(z.current) {
		grown := make([]rune, newLength)
		copy(grown, z.current)
		z.current = grown
	}
	if adjustment != 0 && cKet < z.length {
		copy(z.current[cBra+len(rs):], z.current[cKet:z.length])
	}
	copy(z.current[cBra:], rs)
	z.length += adjustment
	z.limit += adjustment
	if z.cursor >= cKet {
		z.cursor += adjustment
	} else if z.cursor > cBra {
		z.cursor = cBra
	}
	return adjustment
}

func (z *snowball) sliceCheck() {}

func (z *snowball) sliceFrom(s string) {
	z.sliceCheck()
	z.replaceS(z.bra, z.ket, s)
	z.ket = z.bra + len([]rune(s))
}

func (z *snowball) sliceDel() { z.sliceFrom("") }

func (z *snowball) insert(cBra, cKet int, s string) {
	adjustment := z.replaceS(cBra, cKet, s)
	if cBra <= z.bra {
		z.bra += adjustment
	}
	if cBra <= z.ket {
		z.ket += adjustment
	}
}

// stemSnowball runs a generated stemmer over a term.
func stemSnowball(z interface {
	setCurrent(string)
	stem()
	getCurrent() string
}, term string) string {
	z.setCurrent(term)
	z.stem()
	return z.getCurrent()
}
