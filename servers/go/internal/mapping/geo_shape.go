package mapping

import (
	"strconv"
	"strings"
)

func ParseWktBBox(s string) (top, left, bottom, right float64, ok bool) {
	u := strings.ToUpper(strings.TrimSpace(s))
	if !strings.HasPrefix(u, "BBOX") {
		return 0, 0, 0, 0, false
	}
	body := strings.TrimSpace(strings.TrimPrefix(u, "BBOX"))
	body = strings.TrimPrefix(body, "(")
	body = strings.TrimSuffix(body, ")")
	parts := strings.Split(body, ",")
	if len(parts) != 4 {
		return 0, 0, 0, 0, false
	}
	nums := make([]float64, 4)
	for i, p := range parts {
		f, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			return 0, 0, 0, 0, false
		}
		nums[i] = f
	}
	return nums[2], nums[0], nums[3], nums[1], true
}

type GeoShape struct {
	Typ        string
	Lat, Lon   float64
	RadiusM    float64
	Top, Left  float64
	Bot, Right float64
	Points     [][2]float64 // lon, lat
}

func parseWktNumbers(s string) []float64 {
	repl := strings.NewReplacer("(", " ", ")", " ", ",", " ")
	fields := strings.Fields(repl.Replace(s))
	out := make([]float64, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.ParseFloat(f, 64)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func ParseWktGeoShape(s string) (*GeoShape, bool) {
	u := strings.ToUpper(strings.TrimSpace(s))
	switch {
	case strings.HasPrefix(u, "POINT"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("POINT"):]))
		if len(nums) < 2 {
			return nil, false
		}
		return &GeoShape{Typ: "point", Lon: nums[0], Lat: nums[1]}, true
	case strings.HasPrefix(u, "BBOX"):
		top, left, bottom, right, ok := ParseWktBBox(s)
		if !ok {
			return nil, false
		}
		return &GeoShape{Typ: "envelope", Top: top, Left: left, Bot: bottom, Right: right}, true
	case strings.HasPrefix(u, "MULTIPOINT"):
		body := strings.TrimSpace(s[len("MULTIPOINT"):])
		body = strings.TrimSpace(strings.TrimPrefix(body, "("))
		body = strings.TrimSpace(strings.TrimSuffix(body, ")"))
		gs := &GeoShape{Typ: "multipoint"}
		for _, pt := range strings.Split(body, ",") {
			nums := parseWktNumbers(pt)
			if len(nums) >= 2 {
				gs.Points = append(gs.Points, [2]float64{nums[0], nums[1]})
			}
		}
		return gs, len(gs.Points) > 0
	case strings.HasPrefix(u, "POLYGON"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("POLYGON"):]))
		gs := &GeoShape{Typ: "polygon"}
		for i := 0; i+1 < len(nums); i += 2 {
			gs.Points = append(gs.Points, [2]float64{nums[i], nums[i+1]})
		}
		return gs, len(gs.Points) >= 3
	case strings.HasPrefix(u, "LINESTRING"):
		nums := parseWktNumbers(strings.TrimSpace(s[len("LINESTRING"):]))
		gs := &GeoShape{Typ: "linestring"}
		for i := 0; i+1 < len(nums); i += 2 {
			gs.Points = append(gs.Points, [2]float64{nums[i], nums[i+1]})
		}
		return gs, len(gs.Points) >= 2
	}
	return nil, false
}
