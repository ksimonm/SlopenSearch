package search

// The geo and date decay functions of score scripts (OpenSearch's
// ScoreScriptUtils DecayGeo* / DecayDate*): decayGeoGauss(origin, scale,
// offset, decay, doc['location'].value) and friends. The numeric ones are in
// ScoreBindings.

import (
	"math"
	"strconv"
	"strings"
	"sync"
	"time"

	"slopensearch/internal/dates"
	"slopensearch/internal/geohash"
	"slopensearch/internal/painless"
)

// decayParams is what a decay function derives from its constant arguments
// once (the Java classes cache it per script): the origin, the offset and the
// scaling factor of the curve.
type decayParams struct {
	lat, lon float64 // geo origin
	origin   int64   // date origin (epoch millis)
	offset   float64
	scaling  float64
	err      *painless.RuntimeError
}

var decayCache sync.Map // "kind\x00origin\x00scale\x00offset\x00decay" -> *decayParams

// decayScaling is the scaling factor of the curve: the distance (scale) at
// which the score is decay.
func decayScaling(curve string, scale, decay float64) float64 {
	switch curve {
	case "Linear":
		return scale / (1.0 - decay)
	case "Exp":
		return math.Log(decay) / scale
	default: // Gauss
		return 0.5 * math.Pow(scale, 2.0) / math.Log(decay)
	}
}

// decayScore is the score of a document at distance (already reduced by the
// offset, never negative).
func decayScore(curve string, scaling, distance float64) float64 {
	switch curve {
	case "Linear":
		return math.Max(0.0, (scaling-distance)/scaling)
	case "Exp":
		return math.Exp(scaling * distance)
	default:
		return math.Exp(0.5 * math.Pow(distance, 2.0) / scaling)
	}
}

func decayRtErr(off int, typ, reason string) *painless.RuntimeError {
	return &painless.RuntimeError{Type: typ, Offset: off, Reason: reason}
}

func decayParamsFor(kind, curve string, a []any, off int) *decayParams {
	originS, scaleS, offsetS := painless.ToString(a[0]), painless.ToString(a[1]), painless.ToString(a[2])
	decay := painless.F64(a[3])
	key := kind + curve + "\x00" + originS + "\x00" + scaleS + "\x00" + offsetS + "\x00" + strconv.FormatFloat(decay, 'g', -1, 64)
	if v, ok := decayCache.Load(key); ok {
		return v.(*decayParams)
	}
	p := &decayParams{}
	if kind == "Geo" {
		lat, lon, err := parseGeoPointString(originS)
		if err != "" {
			p.err = decayRtErr(off, "illegal_argument_exception", err)
		}
		scale, ok1 := parseDistanceMeters(scaleS)
		offset, ok2 := parseDistanceMeters(offsetS)
		if !ok1 || !ok2 {
			bad := scaleS
			if ok1 {
				bad = offsetS
			}
			p.err = decayRtErr(off, "number_format_exception", "For input string: \""+bad+"\"")
		}
		p.lat, p.lon, p.offset, p.scaling = lat, lon, offset, decayScaling(curve, scale, decay)
	} else {
		origin, ok := parseDecayDate(originS)
		if !ok {
			p.err = decayRtErr(off, "illegal_argument_exception",
				"failed to parse date field ["+originS+"] with format [strict_date_optional_time||epoch_millis]")
		}
		name := "DecayDate" + curve
		scale, err1 := decayTimeValueMillis(scaleS, name+".scale")
		offset, err2 := decayTimeValueMillis(offsetS, name+".offset")
		if err1 != "" {
			p.err = decayRtErr(off, "illegal_argument_exception", err1)
		} else if err2 != "" {
			p.err = decayRtErr(off, "illegal_argument_exception", err2)
		}
		p.origin, p.offset, p.scaling = origin, float64(offset), decayScaling(curve, float64(scale), decay)
	}
	if len(key) < 512 {
		decayCache.Store(key, p)
	}
	return p
}

// decayBindings adds decayGeo{Linear,Exp,Gauss} and decayDate{Linear,Exp,Gauss}.
func decayBindings(m map[string]func(args []any, off int) (any, *painless.RuntimeError)) {
	for _, curve := range []string{"Linear", "Exp", "Gauss"} {
		curve := curve
		m["decayGeo"+curve] = func(a []any, off int) (any, *painless.RuntimeError) {
			p := decayParamsFor("Geo", curve, a, off)
			if p.err != nil {
				return nil, p.err
			}
			gp, ok := a[4].(*painless.GeoPoint)
			if !ok {
				return nil, decayRtErr(off, "class_cast_exception",
					"Cannot cast "+painless.JavaClassName(a[4])+" to org.opensearch.common.geo.GeoPoint")
			}
			// doc values hold the point quantized to Lucene's 32-bit encoding
			lat, lon := quantizeLat(gp.Lat), quantizeLon(gp.Lon)
			distance := arcDistanceMeters(p.lat, p.lon, lat, lon)
			distance = math.Max(0.0, distance-p.offset)
			return decayScore(curve, p.scaling, distance), nil
		}
		m["decayDate"+curve] = func(a []any, off int) (any, *painless.RuntimeError) {
			p := decayParamsFor("Date", curve, a, off)
			if p.err != nil {
				return nil, p.err
			}
			t, ok := painless.ZDTTime(a[4])
			if !ok {
				return nil, decayRtErr(off, "class_cast_exception",
					"Cannot cast "+painless.JavaClassName(a[4])+" to org.opensearch.script.JodaCompatibleZonedDateTime")
			}
			doc := t.UnixMilli()
			diff := doc - p.origin
			if diff < 0 {
				diff = -diff
			}
			distance := int64(math.Max(0, float64(diff)-p.offset))
			return decayScore(curve, p.scaling, float64(distance)), nil
		}
	}
}

// arcDistanceMeters is GeoDistance.ARC: Lucene's SloppyMath.haversinMeters.
func arcDistanceMeters(lat1, lon1, lat2, lon2 float64) float64 {
	const toMeters = 6_371_008.7714
	rad := math.Pi / 180
	x1, x2 := lat1*rad, lat2*rad
	h1 := 1 - math.Cos(x1-x2)
	h2 := 1 - math.Cos((lon1-lon2)*rad)
	h := h1 + math.Cos(x1)*math.Cos(x2)*h2
	h = math.Float64frombits(math.Float64bits(h) & 0xFFFFFFFFFFFFFFF8)
	return toMeters * 2 * math.Asin(math.Min(1, math.Sqrt(h*0.5)))
}

const (
	latDecode = 180.0 / (1 << 32)
	lonDecode = 360.0 / (1 << 32)
)

// quantizeLat / quantizeLon round a coordinate to its doc-value encoding
// (GeoEncodingUtils.encodeLatitude / decodeLatitude).
func quantizeLat(lat float64) float64 {
	if lat == 90 {
		lat = math.Nextafter(lat, 0)
	}
	return float64(int32(math.Floor(lat/latDecode))) * latDecode
}

func quantizeLon(lon float64) float64 {
	if lon == 180 {
		lon = math.Nextafter(lon, 0)
	}
	return float64(int32(math.Floor(lon/lonDecode))) * lonDecode
}

// parseGeoPointString is GeoUtils.parseGeoPoint of a string: "lat, lon",
// "POINT (lon lat)" or a geohash.
func parseGeoPointString(s string) (lat, lon float64, err string) {
	t := strings.TrimSpace(s)
	if up := strings.ToUpper(t); strings.HasPrefix(up, "POINT") {
		inner := strings.TrimSpace(t[5:])
		if strings.HasPrefix(inner, "(") && strings.HasSuffix(inner, ")") {
			f := strings.Fields(inner[1 : len(inner)-1])
			if len(f) == 2 {
				lo, e1 := strconv.ParseFloat(f[0], 64)
				la, e2 := strconv.ParseFloat(f[1], 64)
				if e1 == nil && e2 == nil {
					return la, lo, ""
				}
			}
		}
		return 0, 0, "Unable to parse shape [" + s + "]"
	}
	if strings.Contains(t, ",") {
		parts := strings.Split(t, ",")
		if len(parts) > 3 {
			return 0, 0, "failed to parse [" + s + "], expected 2 or 3 coordinates but found: [" + strconv.Itoa(len(parts)) + "]"
		}
		la, e1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
		lo, e2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
		if e1 != nil || e2 != nil {
			return 0, 0, "latitude must be a number"
		}
		if len(parts) == 3 {
			return 0, 0, "Exception parsing coordinates: found Z value [" + strings.TrimSpace(parts[2]) +
				"] but [ignore_z_value] parameter is [false]"
		}
		return la, lo, ""
	}
	return geohash.Point(t)
}

// parseDecayDate parses the origin of a date decay function with
// "strict_date_optional_time||epoch_millis" and date math, in UTC.
func parseDecayDate(s string) (int64, bool) {
	anchor, expr := s, ""
	if strings.HasPrefix(s, "now") {
		anchor, expr = "now", s[3:]
	} else if i := strings.Index(s, "||"); i >= 0 {
		anchor, expr = s[:i], s[i+2:]
	}
	t, _, ok := dates.ParseAnchor(anchor, "strict_date_optional_time||epoch_millis", "", time.UTC)
	if !ok {
		return 0, false
	}
	if expr != "" {
		if t, ok = dates.ApplyMath(t, expr, false); !ok {
			return 0, false
		}
	}
	return t.UnixMilli(), true
}

// decayTimeValueMillis is TimeValue.parseTimeValue(s, name).getMillis().
func decayTimeValueMillis(s, name string) (int64, string) {
	norm := strings.TrimSpace(strings.ToLower(s))
	type unit struct {
		suffix string
		ms     float64
	}
	var u *unit
	switch {
	case strings.HasSuffix(norm, "nanos"):
		u = &unit{"nanos", 1e-6}
	case strings.HasSuffix(norm, "micros"):
		u = &unit{"micros", 1e-3}
	case strings.HasSuffix(norm, "ms"):
		u = &unit{"ms", 1}
	case strings.HasSuffix(norm, "s"):
		u = &unit{"s", 1000}
	case strings.HasSuffix(strings.TrimSpace(s), "m"):
		u = &unit{"m", 60_000}
	case strings.HasSuffix(norm, "h"):
		u = &unit{"h", 3_600_000}
	case strings.HasSuffix(norm, "d"):
		u = &unit{"d", 86_400_000}
	}
	if u == nil {
		if strings.Trim(norm, "0") == "" && norm != "" {
			return 0, ""
		}
		return 0, "failed to parse setting [" + name + "] with value [" + s + "] as a time value: unit is missing or unrecognized"
	}
	num := strings.TrimSpace(norm[:len(norm)-len(u.suffix)])
	n, err := strconv.ParseInt(num, 10, 64)
	if err != nil {
		if _, ferr := strconv.ParseFloat(num, 64); ferr == nil {
			return 0, "failed to parse [" + s + "], fractional time values are not supported"
		}
		return 0, "failed to parse [" + s + "]"
	}
	if n < -1 {
		return 0, "failed to parse setting [" + name + "] with value [" + s + "] as a time value: negative durations are not supported"
	}
	return int64(float64(n) * u.ms), ""
}
