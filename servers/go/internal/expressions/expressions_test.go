package expressions

import (
	"strconv"
	"testing"

	"slopensearch/internal/jsonv"
	"slopensearch/internal/painless"
)

// Expected values come from tests/reference/test_43_scripting.jsonl
// (test_expression_script_fields, test_expression_numeric_params,
// test_expression_link_errors, test_expression_compile_error), run against
// the index of that file:
//
//	name: keyword, price: integer, qty: long, rating: double, tags: keyword,
//	nums: integer, date: date, active: boolean
//
// with the documents below (sorted by name: apple, banana, carrot, durian).

type fakeDoc map[string][]any

func (d fakeDoc) Values(field string) []any { return d[field] }

var docs = []fakeDoc{
	{"price": {10.0}, "qty": {3.0}, "rating": {4.5}, "nums": {5.0, 1.0, 3.0},
		"date": {1610668800000.0}, "active": {true}},
	{"price": {20.0}, "qty": {0.0}, "rating": {3.0}, "nums": {2.0},
		"date": {1622505600000.0}, "active": {false}},
	{"price": {5.0}, "qty": {10.0}, "rating": {4.0}, "nums": {},
		"date": {1646870400000.0}, "active": {true}},
	{"price": {50.0}},
}

var fieldKinds = map[string]Kind{
	"name": KindOther, "price": KindNumber, "qty": KindNumber, "rating": KindNumber,
	"tags": KindOther, "nums": KindNumber, "title": KindOther, "date": KindDate,
	"active": KindNumber,
}

func testEnv(params map[string]any) Env {
	return Env{
		Fields: func(field string) (Kind, bool) {
			k, ok := fieldKinds[field]
			return k, ok
		},
		Params:  params,
		ToFloat: jsonv.ToFloat,
	}
}

// render formats a double the way Java (and the responses) print it.
func render(f float64) string { return painless.ToString(f) }

func TestScriptFields(t *testing.T) {
	tests := []struct {
		src    string
		params map[string]any
		want   [4]string // one value per document
	}{
		{"doc['price'].value * 2", nil, [4]string{"20.0", "40.0", "10.0", "100.0"}},
		{"doc['price']", nil, [4]string{"10.0", "20.0", "5.0", "50.0"}},
		{"7 / 2", nil, [4]string{"3.5", "3.5", "3.5", "3.5"}},
		{"doc['price'].value % 3", nil, [4]string{"1.0", "2.0", "2.0", "2.0"}},
		{"doc['nums'].length", nil, [4]string{"3.0", "1.0", "0.0", "0.0"}},
		{"doc['nums'].min()", nil, [4]string{"1.0", "2.0", "0.0", "0.0"}},
		{"doc['nums'].max()", nil, [4]string{"5.0", "2.0", "0.0", "0.0"}},
		{"doc['nums'].sum()", nil, [4]string{"9.0", "2.0", "0.0", "0.0"}},
		{"doc['nums'].avg()", nil, [4]string{"3.0", "2.0", "0.0", "0.0"}},
		{"doc['nums'].median()", nil, [4]string{"3.0", "2.0", "0.0", "0.0"}},
		{"doc['nums'].count()", nil, [4]string{"3.0", "1.0", "0.0", "0.0"}},
		{"doc['nums'].empty", nil, [4]string{"0.0", "0.0", "1.0", "1.0"}},
		{"doc['qty'].value", nil, [4]string{"3.0", "0.0", "10.0", "0.0"}},
		{"doc['date'].value", nil, [4]string{"1.6106688E12", "1.6225056E12", "1.6468704E12", "0.0"}},
		{"doc['date'].date.year", nil, [4]string{"2021.0", "2021.0", "2022.0", "0.0"}},
		{"doc['date'].getYear()", nil, [4]string{"2021.0", "2021.0", "2022.0", "0.0"}},
		{"doc['active'].value", nil, [4]string{"1.0", "0.0", "1.0", "0.0"}},
		{"floor(doc['rating'].value)", nil, [4]string{"4.0", "3.0", "4.0", "0.0"}},
		{"doc['price'].value > 10 ? 1 : 0", nil, [4]string{"0.0", "1.0", "0.0", "1.0"}},
		{"max(doc['price'].value, 15)", nil, [4]string{"15.0", "20.0", "15.0", "50.0"}},
		{"pow(2, 3)", nil, [4]string{"8.0", "8.0", "8.0", "8.0"}},
		{"abs(-3)", nil, [4]string{"3.0", "3.0", "3.0", "3.0"}},
		{"ln(doc['price'].value)", nil, [4]string{"2.302585092994046", "2.995732273553991", "1.6094379124341003", "3.912023005428146"}},
		{"sqrt(doc['price'].value)", nil, [4]string{"3.1622776601683795", "4.47213595499958", "2.23606797749979", "7.0710678118654755"}},
		{"doc['price'].empty ? 0 : 1", nil, [4]string{"1.0", "1.0", "1.0", "1.0"}},
		{"doc['price'].value + x", map[string]any{"x": jsonNumber("1")}, [4]string{"11.0", "21.0", "6.0", "51.0"}},
	}
	for _, tt := range tests {
		s, cerr := Compile(tt.src)
		if cerr != nil {
			t.Errorf("Compile(%q): %v", tt.src, cerr)
			continue
		}
		l, lerr := Link(s, testEnv(tt.params))
		if lerr != nil {
			t.Errorf("Link(%q): %v", tt.src, lerr)
			continue
		}
		for i, d := range docs {
			if got := render(l.Eval(d, 0, nil)); got != tt.want[i] {
				t.Errorf("%q on doc %d = %s, want %s", tt.src, i+1, got, tt.want[i])
			}
		}
	}
}

func jsonNumber(s string) any {
	var v any
	if err := jsonv.Unmarshal([]byte(s), &v); err != nil {
		panic(err)
	}
	return v
}

func TestLinkErrors(t *testing.T) {
	// Offset is where OpenSearch puts the "^---- HERE" marker of the
	// script_stack.
	tests := []struct {
		src    string
		params map[string]any
		reason string
		offset int
	}{
		{"doc['name'].value", nil, "Field [name] must be numeric, date, or geopoint", 5},
		{"doc['tags'].value", nil, "Field [tags] must be numeric, date, or geopoint", 5},
		{"params._source.price", nil, "Unknown variable [params]", 0},
		{"_source.price", nil, "Unknown variable [_source]", 0},
		{"_score", nil, "Unknown variable [_score]", 0},
		{"x", map[string]any{"x": "str"}, "Parameter [x] must be a numeric type", 0},
		{"doc['missingf'].value", nil, "Field [missingf] does not exist in mappings", 5},
	}
	for _, tt := range tests {
		s, cerr := Compile(tt.src)
		if cerr != nil {
			t.Errorf("Compile(%q): %v", tt.src, cerr)
			continue
		}
		_, err := Link(s, testEnv(tt.params))
		if err == nil {
			t.Errorf("Link(%q) succeeded, want %q", tt.src, tt.reason)
			continue
		}
		if !err.Link || err.Reason != tt.reason || err.Offset != tt.offset {
			t.Errorf("Link(%q) = {Link:%v %q @%d}, want {Link:true %q @%d}",
				tt.src, err.Link, err.Reason, err.Offset, tt.reason, tt.offset)
		}
	}
}

func TestCompileError(t *testing.T) {
	// test_expression_compile_error: "unexpected end of expression" with the
	// marker at the end of the source (offset 21).
	src := "doc['price'].value + "
	_, err := Compile(src)
	if err == nil {
		t.Fatalf("Compile(%q) succeeded", src)
	}
	if err.Link || !err.EOF || err.Reason != "unexpected end of expression" || err.Offset != 21 {
		t.Errorf("Compile(%q) = %+v", src, *err)
	}
}

func TestScoreBinding(t *testing.T) {
	// test_expression_script_score: _score is bound in score contexts.
	s, cerr := Compile("_score * 10 + doc['price'].value")
	if cerr != nil {
		t.Fatal(cerr)
	}
	env := testEnv(nil)
	env.AllowScore = true
	l, lerr := Link(s, env)
	if lerr != nil {
		t.Fatal(lerr)
	}
	if got := l.Eval(docs[0], 2.5, nil); got != 35 {
		t.Errorf("Eval = %s, want 35", strconv.FormatFloat(got, 'g', -1, 64))
	}
}
