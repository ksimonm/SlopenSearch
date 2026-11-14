package mapping

import (
	"encoding/json"
	"strings"
	"testing"

	"slopensearch/internal/apierr"
	"slopensearch/internal/jsonv"
	"slopensearch/internal/model"
)

// Expected values come from tests/reference (OpenSearch 3.9 answers):
// GET _mapping after indexing a document into a fresh index
// (test_70_mappings.jsonl, test_71_field_types.jsonl, test_21_documents.jsonl,
// test_61_concurrency.jsonl), PUT _mapping conflicts (test_11_index_admin.jsonl,
// test_71_field_types.jsonl) and _simulate_index template composition.

func decode(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := jsonv.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("decode %s: %v", s, err)
	}
	return m
}

// canon renders v as sorted-key JSON so int 256 and json.Number("256") compare equal.
func canon(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var x any
	if err := json.Unmarshal(b, &x); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	b, _ = json.Marshal(x)
	return string(b)
}

// dynamicProps maps every top-level field of doc dynamically, the way
// httpapi.mergeMapping does for undeclared fields of a fresh index (the
// document is decoded with UseNumber, so numbers keep their token kind).
func dynamicProps(t *testing.T, mappings, doc string) (map[string]any, *apierr.Error) {
	t.Helper()
	idx := &model.Index{Mappings: decode(t, mappings), Settings: map[string]any{}}
	expanded, err := ExpandDottedObject(decode(t, doc))
	if err != nil {
		return nil, err
	}
	props := map[string]any{}
	for rawK, v := range expanded {
		k := strings.TrimRight(rawK, ".")
		if k == "" {
			k = rawK
		}
		def, _, err := DynamicDefFor(idx, k, k, v)
		if err != nil {
			return nil, err
		}
		if def != nil {
			props[k] = def
		}
	}
	return props, nil
}

const textKw = `{"type":"text","fields":{"keyword":{"type":"keyword","ignore_above":256}}}`

func TestDynamicMapping(t *testing.T) {
	cases := []struct {
		name     string
		mappings string
		doc      string
		want     string // expected properties
	}{
		{"infers types (test_dynamic_mapping_infers_types)", `{}`,
			`{"s":"hello","i":3,"f":1.5,"b":true,"dt":"2024-01-01","o":{"k":"v"}}`,
			`{"b":{"type":"boolean"},"dt":{"type":"date"},"f":{"type":"float"},"i":{"type":"long"},"o":{"properties":{"k":` + textKw + `}},"s":` + textKw + `}`},
		{"auto-created index (test_index_auto_creates_index_with_dynamic_mapping)", `{}`,
			`{"s":"x","n":1,"f":1.5,"b":true,"d":"2020-01-01","o":{"k":1}}`,
			`{"b":{"type":"boolean"},"d":{"type":"date"},"f":{"type":"float"},"n":{"type":"long"},"o":{"properties":{"k":{"type":"long"}}},"s":` + textKw + `}`},
		{"0.0 is a float token (test_parallel_dynamic_mapping_same_new_field)", `{}`,
			`{"shared":0.0}`, `{"shared":{"type":"float"}}`},
		{"date_detection off (test_date_detection_disabled)", `{"date_detection":false}`,
			`{"d":"2024-01-01"}`, `{"d":` + textKw + `}`},
		{"numeric_detection (test_numeric_detection)", `{"numeric_detection":true}`,
			`{"i":"42","f":"4.2","s":"4x","neg":"-7","e":"1e3"}`,
			`{"e":{"type":"float"},"f":{"type":"float"},"i":{"type":"long"},"neg":{"type":"long"},"s":` + textKw + `}`},
		{"numeric strings stay text (test_numeric_detection_off_by_default)", `{}`,
			`{"i":"42"}`, `{"i":` + textKw + `}`},
		{"dynamic_date_formats (test_dynamic_date_formats)", `{"dynamic_date_formats":["yyyy/MM/dd","dd.MM.yyyy"]}`,
			`{"a":"2024/01/15","b":"2024-01-15","c":"15.01.2024"}`,
			`{"a":{"type":"date","format":"yyyy/MM/dd","print_format":"yyyy/MM/dd"},"b":` + textKw + `,"c":{"type":"date","format":"dd.MM.yyyy","print_format":"dd.MM.yyyy"}}`},
		{"default detection: datetime", `{}`, `{"f":"2024-01-15T10:00:00Z"}`, `{"f":{"type":"date"}}`},
		{"default detection: year-month", `{}`, `{"f":"2024-01"}`, `{"f":{"type":"date"}}`},
		{"default detection: slashes", `{}`, `{"f":"2024/01/15"}`,
			`{"f":{"type":"date","format":"yyyy/MM/dd HH:mm:ss||yyyy/MM/dd||epoch_millis","print_format":"yyyy/MM/dd HH:mm:ss"}}`},
		{"default detection: year only", `{}`, `{"f":"2024"}`, `{"f":` + textKw + `}`},
		{"default detection: space separator", `{}`, `{"f":"2024-01-15 10:00:00"}`, `{"f":` + textKw + `}`},
		{"default detection: epoch string", `{}`, `{"f":"1705312800000"}`, `{"f":` + textKw + `}`},
		{"null adds no mapping", `{}`, `{"n":null}`, `{}`},
		{"empty array adds no mapping", `{}`, `{"n":[]}`, `{}`},
		{"dotted names expand (test_dotted_field_names_expand_to_objects)", `{}`,
			`{"a.b.c":"x","a.d":1}`,
			`{"a":{"properties":{"b":{"properties":{"c":` + textKw + `}},"d":{"type":"long"}}}}`},
		{"trailing dot stripped (test_trailing_dot_field_name_is_stripped)", `{}`,
			`{"a.":1}`, `{"a":{"type":"long"}}`},
		{"dynamic template placeholders (test_dynamic_template_placeholders)",
			`{"dynamic_templates":[{"dyn":{"match":"dyn_*","mapping":{"doc_values":false,"type":"{dynamic_type}"}}},{"named":{"match":"nm_*","mapping":{"copy_to":"all_{name}","type":"keyword"}}}]}`,
			`{"dyn_l":5,"dyn_s":"str","dyn_d":1.5,"dyn_b":true,"dyn_dt":"2024-01-01","nm_t":"zz"}`,
			`{"dyn_b":{"type":"boolean","doc_values":false},"dyn_d":{"type":"float","doc_values":false},"dyn_dt":{"type":"date","doc_values":false},"dyn_l":{"type":"long","doc_values":false},"dyn_s":{"type":"text"},"nm_t":{"type":"keyword","copy_to":["all_nm_t"]}}`},
		{"dynamic template resolution (test_dynamic_templates_resolution)",
			`{"dynamic_templates":[{"ints":{"match_mapping_type":"long","mapping":{"type":"integer"}}},{"kw":{"match":"kw_*","unmatch":"*_text","mapping":{"type":"keyword"}}},{"path":{"path_match":"meta.*","path_unmatch":"meta.skip","mapping":{"index":false,"type":"keyword"}}},{"re":{"match":"^num_\\d+$","match_pattern":"regex","mapping":{"type":"double"}}},{"strs":{"match_mapping_type":"string","mapping":{"analyzer":"whitespace","type":"text"}}},{"never":{"match":"dup_*","mapping":{"type":"keyword"}}},{"objs":{"match":"obj_*","match_mapping_type":"object","mapping":{"type":"nested"}}}]}`,
			`{"cnt":5,"kw_a":"X","kw_text":"Y","meta":{"a":"m","skip":"s"},"num_12":"3","num_x":"3","plain":"Hello World","dup_v":"7","obj_n":{"q":1},"dbl":1.5,"bool":true,"dt":"2024-01-01"}`,
			`{"bool":{"type":"boolean"},"cnt":{"type":"integer"},"dbl":{"type":"float"},"dt":{"type":"date"},"dup_v":{"type":"text","analyzer":"whitespace"},"kw_a":{"type":"keyword"},"kw_text":{"type":"text","analyzer":"whitespace"},"meta":{"properties":{"a":{"type":"keyword","index":false},"skip":{"type":"text","analyzer":"whitespace"}}},"num_12":{"type":"double"},"num_x":{"type":"text","analyzer":"whitespace"},"obj_n":{"type":"nested","properties":{"q":{"type":"integer"}}},"plain":{"type":"text","analyzer":"whitespace"}}`},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			props, err := dynamicProps(t, c.mappings, c.doc)
			if err != nil {
				t.Fatalf("unexpected error: %s", err.Reason)
			}
			if got, want := canon(t, props), canon(t, decode(t, c.want)); got != want {
				t.Fatalf("properties\n got: %s\nwant: %s", got, want)
			}
		})
	}
}

func TestDynamicMappingMixedArrays(t *testing.T) {
	// test_dynamic_mixed_type_array_rejected, test_mixed_scalar_and_object_array_rejected
	cases := []struct {
		doc, reason string
	}{
		{`{"f":[1,"two",3.5]}`, "mapper [f] cannot be changed from type [long] to [text]"},
		{`{"f":["two",1]}`, "mapper [f] cannot be changed from type [text] to [long]"},
		{`{"f":[1.5,1]}`, "mapper [f] cannot be changed from type [float] to [long]"},
		{`{"f":[1,1.5]}`, "mapper [f] cannot be changed from type [long] to [float]"},
		{`{"f":["1",2]}`, "mapper [f] cannot be changed from type [text] to [long]"},
		{`{"f":[true,"x"]}`, "mapper [f] cannot be changed from type [boolean] to [text]"},
		{`{"f":[{"a":1},5]}`, "can't merge a non object mapping [f] with an object mapping"},
		{`{"arr":[1,{"x":1}]}`, "can't merge a non object mapping [arr] with an object mapping"},
	}
	for _, c := range cases {
		t.Run(c.doc, func(t *testing.T) {
			_, err := dynamicProps(t, `{}`, c.doc)
			if err == nil {
				t.Fatalf("no error, want %q", c.reason)
			}
			if err.Type != "illegal_argument_exception" || err.Status != 400 || err.Reason != c.reason {
				t.Fatalf("got %s/%d %q, want illegal_argument_exception/400 %q", err.Type, err.Status, err.Reason, c.reason)
			}
		})
	}
}

func TestMergeChecked(t *testing.T) {
	const ia11 = `{"title":{"type":"text","fields":{"raw":{"type":"keyword"}}},"user":{"properties":{"name":{"type":"keyword"},"age":{"type":"integer"}}},"price":{"type":"float"}}`
	const t71 = `{"t":{"type":"text","analyzer":"standard"},"plain":{"type":"text"},"k":{"type":"keyword"},"n":{"type":"integer"},"o":{"properties":{"x":{"type":"keyword"}}},"ne":{"type":"nested"}}`
	cases := []struct {
		name, existing, incoming string
		errType                  string
		status                   int
		reason                   string
	}{
		{"type change", ia11, `{"title":{"type":"keyword"}}`, "illegal_argument_exception", 400,
			"mapper [title] cannot be changed from type [text] to [keyword]"},
		{"numeric type change", ia11, `{"price":{"type":"integer"}}`, "illegal_argument_exception", 400,
			"mapper [price] cannot be changed from type [float] to [integer]"},
		{"object to leaf", ia11, `{"user":{"type":"keyword"}}`, "illegal_argument_exception", 400,
			"can't merge a non object mapping [user] with an object mapping"},
		{"analyzer from default", ia11, `{"title":{"type":"text","analyzer":"keyword"}}`, "illegal_argument_exception", 400,
			"Mapper for [title] conflicts with existing mapper:\n\tCannot update parameter [analyzer] from [default] to [keyword]"},
		{"keyword to long", t71, `{"k":{"type":"long"}}`, "illegal_argument_exception", 400,
			"mapper [k] cannot be changed from type [keyword] to [long]"},
		{"integer to long", t71, `{"n":{"type":"long"}}`, "illegal_argument_exception", 400,
			"mapper [n] cannot be changed from type [integer] to [long]"},
		{"analyzer change", t71, `{"t":{"type":"text","analyzer":"whitespace"}}`, "illegal_argument_exception", 400,
			"Mapper for [t] conflicts with existing mapper:\n\tCannot update parameter [analyzer] from [standard] to [whitespace]"},
		{"omitted analyzer is default", t71, `{"t":{"type":"text","fields":{"raw":{"type":"keyword"}}}}`, "illegal_argument_exception", 400,
			"Mapper for [t] conflicts with existing mapper:\n\tCannot update parameter [analyzer] from [standard] to [default]"},
		{"index flag", t71, `{"k":{"type":"keyword","index":false}}`, "illegal_argument_exception", 400,
			"Mapper for [k] conflicts with existing mapper:\n\tCannot update parameter [index] from [true] to [false]"},
		{"doc_values flag", t71, `{"k":{"type":"keyword","doc_values":false}}`, "illegal_argument_exception", 400,
			"Mapper for [k] conflicts with existing mapper:\n\tCannot update parameter [doc_values] from [true] to [false]"},
		{"normalizer", t71, `{"k":{"type":"keyword","normalizer":"lowercase"}}`, "illegal_argument_exception", 400,
			"Mapper for [k] conflicts with existing mapper:\n\tCannot update parameter [normalizer] from [default] to [lowercase]"},
		{"null_value", t71, `{"n":{"type":"integer","null_value":0}}`, "illegal_argument_exception", 400,
			"Mapper for [n] conflicts with existing mapper:\n\tCannot update parameter [null_value] from [null] to [0]"},
		{"index_options", t71, `{"plain":{"type":"text","index_options":"docs"}}`, "illegal_argument_exception", 400,
			"Mapper for [plain] conflicts with existing mapper:\n\tCannot update parameter [index_options] from [positions] to [docs]"},
		{"object to nested", t71, `{"o":{"type":"nested"}}`, "illegal_argument_exception", 400,
			"cannot change object mapping from non-nested to nested"},
		{"nested to object", t71, `{"ne":{"type":"object"}}`, "illegal_argument_exception", 400,
			"cannot change object mapping from nested to non-nested"},
		{"object to keyword", t71, `{"o":{"type":"keyword"}}`, "illegal_argument_exception", 400,
			"can't merge a non object mapping [o] with an object mapping"},
		{"leaf to object", t71, `{"k":{"properties":{"x":{"type":"keyword"}}}}`, "illegal_argument_exception", 400,
			"can't merge a non object mapping [k] with an object mapping"},
		{"multi-field type change", `{"t":{"type":"text","fields":{"raw":{"type":"keyword"}}}}`,
			`{"t":{"type":"text","fields":{"raw":{"type":"long"}}}}`, "illegal_argument_exception", 400,
			"mapper [t.raw] cannot be changed from type [keyword] to [long]"},
		{"object enabled", `{"o":{"type":"object","enabled":false}}`, `{"o":{"type":"object","enabled":true}}`,
			"mapper_exception", 500, "the [enabled] parameter can't be updated for the object mapping [o]"},
		{"alias to field", `{"x":{"type":"long"},"a":{"type":"alias","path":"x"}}`, `{"a":{"type":"keyword"}}`,
			"illegal_argument_exception", 400,
			"Cannot merge a field alias mapping [a] with a mapping that is not for a field alias."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := MergeChecked(decode(t, c.existing), decode(t, c.incoming))
			if err == nil {
				t.Fatalf("no error, want %q", c.reason)
			}
			if err.Type != c.errType || err.Status != c.status || err.Reason != c.reason {
				t.Fatalf("got %s/%d %q\nwant %s/%d %q", err.Type, err.Status, err.Reason, c.errType, c.status, c.reason)
			}
		})
	}
}

func TestMergeCheckedAccepts(t *testing.T) {
	// test_alias_path_can_be_updated_but_not_type: the alias path may change.
	existing := decode(t, `{"x":{"type":"long"},"a":{"type":"alias","path":"x"}}`)
	if err := MergeChecked(existing, decode(t, `{"y":{"type":"long"},"a":{"type":"alias","path":"y"}}`)); err != nil {
		t.Fatalf("unexpected error: %s", err.Reason)
	}
	want := `{"a":{"path":"y","type":"alias"},"x":{"type":"long"},"y":{"type":"long"}}`
	if got := canon(t, existing); got != want {
		t.Fatalf("merged\n got: %s\nwant: %s", got, want)
	}
	// Adding a sub-field to an object mapping merges recursively.
	existing = decode(t, `{"user":{"properties":{"name":{"type":"keyword"}}}}`)
	if err := MergeChecked(existing, decode(t, `{"user":{"properties":{"age":{"type":"integer"}}}}`)); err != nil {
		t.Fatalf("unexpected error: %s", err.Reason)
	}
	want = `{"user":{"properties":{"age":{"type":"integer"},"name":{"type":"keyword"}}}}`
	if got := canon(t, existing); got != want {
		t.Fatalf("merged\n got: %s\nwant: %s", got, want)
	}
}

func TestMergeDeepTemplateComposition(t *testing.T) {
	// test_simulate_index: component ct-a, then ct-b, then the index template;
	// later field definitions win.
	merged := map[string]any{}
	for _, m := range []string{
		`{"properties":{"a":{"type":"keyword"}}}`,
		`{"properties":{"b":{"type":"long"},"a":{"type":"text"}}}`,
		`{"properties":{"c":{"type":"date"}}}`,
	} {
		MergeDeep(merged, decode(t, m))
	}
	want := `{"properties":{"a":{"type":"text"},"b":{"type":"long"},"c":{"type":"date"}}}`
	if got := canon(t, merged); got != want {
		t.Fatalf("merged\n got: %s\nwant: %s", got, want)
	}
}

func TestExpandDottedMapping(t *testing.T) {
	// test_dotted_names_in_explicit_mapping
	m := decode(t, `{"properties":{"user.name":{"type":"keyword"},"user.age":{"type":"integer"}}}`)
	if err := ExpandDottedMapping(m); err != nil {
		t.Fatalf("unexpected error: %s", err.Reason)
	}
	want := `{"properties":{"user":{"properties":{"age":{"type":"integer"},"name":{"type":"keyword"}}}}}`
	if got := canon(t, m); got != want {
		t.Fatalf("expanded\n got: %s\nwant: %s", got, want)
	}
}

func TestFieldDefAndAliases(t *testing.T) {
	idx := &model.Index{Mappings: decode(t, `{"properties":{
		"title":{"type":"text","fields":{"raw":{"type":"keyword"}}},
		"user":{"properties":{"name":{"type":"keyword"},"age":{"type":"integer"}}},
		"x":{"type":"long"},
		"a":{"type":"alias","path":"x"},
		"b":{"type":"alias","path":"user.name"}}}`)}
	cases := []struct {
		field, wantType string
	}{
		{"title", "text"},
		{"title.raw", "keyword"},
		{"user.name", "keyword"},
		{"user.age", "integer"},
		{"user", ""}, // object: found, no type
		{"a", "alias"},
		{"missing", "-"},
		{"user.missing", "-"},
		{"title.raw.deeper", "-"},
	}
	for _, c := range cases {
		def, ok := FieldDef(idx, c.field)
		if c.wantType == "-" {
			if ok {
				t.Errorf("FieldDef(%q) found %v, want missing", c.field, def)
			}
			continue
		}
		if !ok {
			t.Errorf("FieldDef(%q) missing", c.field)
			continue
		}
		if typ, _ := def["type"].(string); typ != c.wantType {
			t.Errorf("FieldDef(%q) type %q, want %q", c.field, typ, c.wantType)
		}
	}
	for field, want := range map[string]string{"a": "x", "b": "user.name", "x": "x", "missing": "missing"} {
		if got := ResolveAliasField(idx, field); got != want {
			t.Errorf("ResolveAliasField(%q) = %q, want %q", field, got, want)
		}
	}
	// test_field_mapping_across_all_indices: GET _mapping/field/user.name
	got := canon(t, FieldMappings(idx, "user.name", false))
	want := `{"user.name":{"full_name":"user.name","mapping":{"name":{"type":"keyword"}}}}`
	if got != want {
		t.Errorf("FieldMappings\n got: %s\nwant: %s", got, want)
	}
}
