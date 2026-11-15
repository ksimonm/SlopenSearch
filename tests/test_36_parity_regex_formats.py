"""Java regex semantics and non-JSON request bodies.

- Painless regexes are java.util.regex: lookaround, backreferences, possessive
  quantifiers, named groups, flags, Matcher replacement rules and
  PatternSyntaxException texts (an invalid regex constant is a compile error);
- the gsub / split / kv ingest processors use java.util.regex too, grok uses
  Joni (Oniguruma, Ruby syntax) with its own error texts;
- CBOR request bodies are decoded (indefinite lengths, byte strings, floats)
  and answered in CBOR (Jackson writes indefinite-length maps and arrays);
- YAML request bodies are parsed with YAML 1.1 scalar rules; the answer is
  YAML unless Accept asks for JSON; documents must start with `---`.
"""
import base64
import struct

import pytest

pytestmark = pytest.mark.parity


def _execute(client, source):
    return client.post("/_scripts/painless/_execute", json={"script": {"source": source}})


# --------------------------------------------------------------- painless regex


@pytest.mark.parametrize("source,result", [
    (r"'foobar' ==~ /foo(?=bar).*/", "true"),
    (r"def m = /(?<=foo)bar/.matcher('foobar'); m.find() ? m.group() + m.start() : 'no'", "bar3"),
    (r"/(?<!x)y/.matcher('xy ay').replaceAll('Z')", "xy aZ"),
    (r"/(?<=a+)b/.matcher('aab').find()", "true"),
    (r"'abab' ==~ /(ab)\1/", "true"),
    (r"def m = /(\w)\1/.matcher('xaab'); m.find(); m.start(1) + ':' + m.end()", "1:3"),
    (r"/(?=(a+))a*b\1/.matcher('baaabac').find()", "true"),
    (r"'aaa' ==~ /a++a/", "false"),
    (r"/x*+/.split('axxbxc').length", "5"),
    (r"def m = /(?<w>o)/.matcher('foo'); m.find(); m.namedGroup('w')", "o"),
    (r"/(?<n>\d+)/.matcher('x12y3').replaceFirst('<${n}>')", "x<12>y3"),
    (r"'FOO' ==~ /foo/i", "true"),
    (r"'aBc' =~ /(?i)b/", "true"),
    (r"def m = /(?x) a b # c/.matcher('ab'); m.matches()", "true"),
    (r"'a.b' ==~ /a.b/l", "true"),
    (r"/x/.pattern() + /x/i.flags()", "x2"),
    (r"/(\w+)@(\w+)/.matcher('a@b c@d').replaceAll('$2#$1')", "b#a d#c"),
    (r"/(?<x>a)/.matcher('ab').replaceAll('[${x}$1\\$]')", "[aa$]b"),
    (r"/(a)/.matcher('a').replaceAll('$12')", "a2"),
    (r"/(a)(b)?/.matcher('a').replaceAll('<$2>')", "<>"),
    (r"def m = /(a)|b/.matcher('b'); m.find(); m.start(1)", "-1"),
    (r"Arrays.asList(/(?=b)/.split('abab'))", "[a, ba, b]"),
    (r"Arrays.asList(/a/.split('aaxa', 2))", "[, axa]"),
    (r"/b/.splitAsStream('abcbd').collect(Collectors.toList())", "[a, c, d]"),
    (r"'aéb'.replaceAll(/é/, m -> m.start() + '-' + m.end())", "a1-2b"),
    (r"/\p{Lu}+/.matcher('abCDe').replaceAll('_')", "ab_e"),
], ids=["lookahead", "lookbehind", "negative_lookbehind", "variable_lookbehind", "backref",
        "backref_positions", "backref_into_lookahead", "possessive", "possessive_split", "named_group",
        "named_replacement", "flag_i", "inline_i", "comments", "literal", "pattern_flags",
        "replace_groups", "replace_named_and_escape", "replace_two_digit_group", "replace_unset_group",
        "unset_group_start", "split_lookahead", "split_limit", "split_as_stream", "replace_function",
        "unicode_property"])
def test_painless_java_regex(client, source, result):
    r = _execute(client, source)
    assert r.status_code == 200, r.text
    assert r.json() == {"result": result}


def test_painless_regex_in_script_fields(client, index_factory):
    name = index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0},
                         mappings={"properties": {"s": {"type": "keyword"}}})
    r = client.put(f"/{name}/_doc/1", json={"s": "2024-05 and 2025-06"}, params={"refresh": "true"})
    assert r.status_code == 201, r.text
    source = (r"def m = /(?<y>\d{4})-(\d\d)(?=\s|$)/.matcher(doc['s'].value); def out = [];"
              r" while (m.find()) { out.add(m.group(2) + '/' + m.namedGroup('y')) } out")
    r = client.post(f"/{name}/_search", json={
        "script_fields": {"f": {"script": {"source": source}}}, "_source": False})
    assert r.status_code == 200, r.text
    assert r.json()["hits"]["hits"][0]["fields"] == {"f": ["05/2024", "06/2025"]}


def _compile_error(source, offset, start, end, stack, reason, pattern_reason):
    script = {"type": "script_exception", "reason": "compile error", "script_stack": stack,
              "script": source, "lang": "painless", "position": {"offset": offset, "start": start, "end": end}}
    return {"error": {"root_cause": [script], **script, "caused_by": {
        "type": "illegal_argument_exception", "reason": reason,
        "caused_by": {"type": "pattern_syntax_exception", "reason": pattern_reason}}}, "status": 400}


@pytest.mark.parametrize("source,offset,end,stack,pattern,pattern_reason", [
    ("/a(/.matcher('x').matches()", 3, 27, ["/a(/.matcher('x').matches()", "   ^---- HERE"],
     "a(", "Unclosed group near index 2\na("),
    ("/a{2,1}/.matcher('a').find()", 6, 28, ["/a{2,1}/.matcher('a').find()", "      ^---- HERE"],
     "a{2,1}", "Illegal repetition range near index 5\na{2,1}\n     ^"),
    ("/(?<1a>x)/.matcher('a').find()", 4, 29, ["/(?<1a>x)/.matcher('a').find( ...", "    ^---- HERE"],
     "(?<1a>x)", "capturing group name does not start with a Latin letter near index 3\n(?<1a>x)\n   ^"),
    ("/(?<a>x)(?<a>y)/.matcher('a').find()", 12, 36, ["/(?<a>x)(?<a>y)/.matcher('a').find()", "            ^---- HERE"],
     "(?<a>x)(?<a>y)", "Named capturing group <a> is already defined near index 11\n(?<a>x)(?<a>y)\n           ^"),
    ("/[z-a]/.matcher('a').find()", 4, 27, ["/[z-a]/.matcher('a').find()", "    ^---- HERE"],
     "[z-a]", "Illegal character range near index 3\n[z-a]\n   ^"),
    (r"/\p{Foo}/.matcher('a').find()", 7, 29, [r"/\p{Foo}/.matcher('a').find()", "       ^---- HERE"],
     r"\p{Foo}", "Unknown character property name {Foo} near index 6\n\\p{Foo}\n      ^"),
], ids=["unclosed_group", "repetition_range", "group_name", "duplicate_group", "char_range", "property"])
def test_painless_invalid_regex_constant(client, source, offset, end, stack, pattern, pattern_reason):
    r = _execute(client, source)
    assert r.status_code == 400
    reason = f"invalid regular expression: could not compile regex constant [{pattern}] with flags []"
    assert r.json() == _compile_error(source, offset, 0, end, stack, reason, pattern_reason)


def test_painless_pattern_compile_is_not_whitelisted(client):
    r = _execute(client, "Pattern.compile('[a-').matcher('x').matches()")
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["reason"] == "compile error"
    assert err["caused_by"] == {"type": "illegal_argument_exception",
                                "reason": "static method [java.util.regex.Pattern, compile/1] not found"}


@pytest.mark.parametrize("source,frames,etype,reason", [
    ("/(a)/.matcher('a').replaceAll('$2')",
     ["checkGroup(Matcher.java:1845)", "start(Matcher.java:523)", "appendExpandedReplacement(Matcher.java:1134)",
      "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "index_out_of_bounds_exception", "No group 2"),
    ("/(a)/.matcher('a').replaceFirst('$2')",
     ["checkGroup(Matcher.java:1845)", "start(Matcher.java:523)", "appendExpandedReplacement(Matcher.java:1134)",
      "appendReplacement(Matcher.java:1041)", "replaceFirst(Matcher.java:1444)"],
     "index_out_of_bounds_exception", "No group 2"),
    ("/(a)/.matcher('a').replaceAll('$')",
     ["appendExpandedReplacement(Matcher.java:1073)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "Illegal group reference: group index is missing"),
    ("/(a)/.matcher('a').replaceAll('$a')",
     ["appendExpandedReplacement(Matcher.java:1111)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "Illegal group reference"),
    ("/(a)/.matcher('a').replaceAll('${x}')",
     ["appendExpandedReplacement(Matcher.java:1103)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "No group with name {x}"),
    ("/(a)/.matcher('a').replaceAll('${1x}')",
     ["appendExpandedReplacement(Matcher.java:1098)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "capturing group name {1x} starts with digit character"),
    ("/(a)/.matcher('a').replaceAll('${x')",
     ["appendExpandedReplacement(Matcher.java:1094)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "named capturing group is missing trailing '}'"),
    ("/(a)/.matcher('a').replaceAll('\\\\')",
     ["appendExpandedReplacement(Matcher.java:1063)", "appendReplacement(Matcher.java:1041)", "replaceAll(Matcher.java:1227)"],
     "illegal_argument_exception", "character to be escaped is missing"),
], ids=["no_group", "no_group_first", "missing_index", "illegal_reference", "unknown_name", "digit_name",
        "unclosed_name", "dangling_escape"])
def test_painless_replacement_errors(client, source, frames, etype, reason):
    r = _execute(client, source)
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["reason"] == "runtime error"
    assert err["script_stack"] == ["java.base/java.util.regex.Matcher." + f for f in frames] + [
        source, "                  ^---- HERE"]
    assert err["position"] == {"offset": 18, "start": 0, "end": len(source)}
    assert err["caused_by"] == {"type": etype, "reason": reason}


@pytest.mark.parametrize("source,offset,frames,etype,reason", [
    ("def m = /a/.matcher('a'); m.group()", 27,
     ["checkMatch(Matcher.java:1850)", "group(Matcher.java:685)", "group(Matcher.java:643)"],
     "illegal_state_exception", "No match found"),
    ("def m = /a/.matcher('a'); m.group(1)", 27,
     ["checkMatch(Matcher.java:1850)", "group(Matcher.java:685)"], "illegal_state_exception", "No match found"),
    ("def m = /a/.matcher('a'); m.start()", 27,
     ["checkMatch(Matcher.java:1850)", "start(Matcher.java:493)"], "illegal_state_exception", "No match found"),
    ("def m = /a/.matcher('a'); m.find(); m.group(3)", 37,
     ["checkGroup(Matcher.java:1845)", "group(Matcher.java:686)"], "index_out_of_bounds_exception", "No group 3"),
    ("def m = /a/.matcher('a'); m.find(); m.start(2)", 37,
     ["checkGroup(Matcher.java:1845)", "start(Matcher.java:523)"], "index_out_of_bounds_exception", "No group 2"),
    ("def m = /a/.matcher('xa'); m.find(5)", 28,
     ["find(Matcher.java:810)"], "index_out_of_bounds_exception", "Illegal start index"),
    ("def m = /(?<w>o)/.matcher('foo'); m.find(); m.namedGroup('zz')", 45,
     ["getMatchedGroupIndex(Matcher.java:1839)", "group(Matcher.java:720)"],
     "illegal_argument_exception", "No group with name <zz>"),
], ids=["group_no_match", "group_n_no_match", "start_no_match", "no_group", "start_no_group",
        "find_start", "named_missing"])
def test_painless_matcher_errors(client, source, offset, frames, etype, reason):
    r = _execute(client, source)
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["script_stack"][:-2] == ["java.base/java.util.regex.Matcher." + f for f in frames]
    assert err["position"]["offset"] == offset
    assert err["caused_by"] == {"type": etype, "reason": reason}


def test_painless_group_by_name_is_not_whitelisted(client):
    r = _execute(client, "def m = /(?<w>o)/.matcher('foo'); m.find(); m.group('w')")
    assert r.status_code == 400
    assert r.json()["error"]["caused_by"] == {
        "type": "wrong_method_type_exception",
        "reason": "cannot convert MethodHandle(Matcher,int)String to (Object,String)Object"}
    r = _execute(client, "Matcher m = /(?<w>o)/.matcher('foo'); m.find(); m.group('w')")
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["reason"] == "compile error"
    assert err["position"] == {"offset": 56, "start": 31, "end": 60}
    assert err["caused_by"] == {"type": "class_cast_exception", "reason": "Cannot cast from [java.lang.String] to [int]."}


# --------------------------------------------------------------- ingest regex


def _simulate(client, processor, source):
    return client.post("/_ingest/pipeline/_simulate", json={
        "pipeline": {"processors": [processor]}, "docs": [{"_source": source}]})


def _sim_source(client, processor, source):
    r = _simulate(client, processor, source)
    assert r.status_code == 200, r.text
    doc = r.json()["docs"][0]
    assert "doc" in doc, doc
    return doc["doc"]["_source"]


def _sim_error(client, processor, source):
    r = _simulate(client, processor, source)
    assert r.status_code == 200, r.text
    return r.json()["docs"][0]["error"]


@pytest.mark.parametrize("processor,source,expected", [
    ({"gsub": {"field": "f", "pattern": "(?<=a)b", "replacement": "X"}}, {"f": "abcb"}, {"f": "aXcb"}),
    ({"gsub": {"field": "f", "pattern": r"(\w)\1", "replacement": "<$1>"}}, {"f": "aabcc"}, {"f": "<a>b<c>"}),
    ({"gsub": {"field": "f", "pattern": r"(?<w>\d+)", "replacement": "[${w}]"}}, {"f": "a12b3"}, {"f": "a[12]b[3]"}),
    ({"gsub": {"field": "f", "pattern": "a++a", "replacement": "X"}}, {"f": "aaa"}, {"f": "aaa"}),
    ({"gsub": {"field": "f", "pattern": "a", "replacement": "\\"}}, {"f": "b"}, {"f": "b"}),
    ({"split": {"field": "f", "separator": "(?<=,)"}}, {"f": "a,b,c"}, {"f": ["a,", "b,", "c"]}),
    ({"split": {"field": "f", "separator": "(?=b)"}}, {"f": "abab"}, {"f": ["a", "ba", "b"]}),
    ({"split": {"field": "f", "separator": "x*+"}}, {"f": "axxbxc"}, {"f": ["a", "", "b", "", "c"]}),
    ({"split": {"field": "f", "separator": ","}}, {"f": ""}, {"f": [""]}),
    ({"split": {"field": "f", "separator": ",", "preserve_trailing": True}}, {"f": "a,,"}, {"f": ["a", "", ""]}),
    ({"kv": {"field": "f", "field_split": r"(?<=\d)\s", "value_split": "="}}, {"f": "a=1 b=2 c=x d=3"},
     {"a": "1", "b": "2", "c": "x d=3", "f": "a=1 b=2 c=x d=3"}),
    ({"kv": {"field": "f", "field_split": "&", "value_split": r"(?<!\\)="}}, {"f": "a=1&b\\=x=2"},
     {"a": "1", "b\\=x": "2", "f": "a=1&b\\=x=2"}),
], ids=["gsub_lookbehind", "gsub_backref", "gsub_named", "gsub_possessive", "gsub_no_match_bad_replacement",
        "split_lookbehind", "split_lookahead", "split_possessive", "split_empty", "split_preserve_trailing",
        "kv_lookbehind_field_split", "kv_negative_lookbehind_value_split"])
def test_ingest_java_regex(client, processor, source, expected):
    assert _sim_source(client, processor, source) == expected


@pytest.mark.parametrize("processor,source,etype,reason", [
    ({"gsub": {"field": "f", "pattern": "a", "replacement": "$2"}}, {"f": "a"},
     "index_out_of_bounds_exception", "No group 2"),
    ({"gsub": {"field": "f", "pattern": "a", "replacement": "${x}"}}, {"f": "a"},
     "illegal_argument_exception", "No group with name {x}"),
    ({"gsub": {"field": "f", "pattern": "a", "replacement": "\\"}}, {"f": "a"},
     "illegal_argument_exception", "character to be escaped is missing"),
    ({"split": {"field": "f", "separator": "a("}}, {"f": "a"},
     "pattern_syntax_exception", "Unclosed group near index 2\na("),
    ({"kv": {"field": "f", "field_split": "(", "value_split": "="}}, {"f": "a=1"},
     "pattern_syntax_exception", "Unclosed group near index 1\n("),
    ({"kv": {"field": "f", "field_split": "&", "value_split": "="}}, {"f": "a=1&&b=2"},
     "illegal_argument_exception", "field [f] does not contain value_split [=]"),
], ids=["gsub_no_group", "gsub_no_name", "gsub_escape", "split_invalid", "kv_invalid", "kv_empty_pair"])
def test_ingest_regex_runtime_errors(client, processor, source, etype, reason):
    err = _sim_error(client, processor, source)
    assert err == {"root_cause": [{"type": etype, "reason": reason}], "type": etype, "reason": reason}


def _config_error(reason, processor_type, prop):
    e = {"type": "parse_exception", "reason": reason, "processor_type": processor_type, "property_name": prop}
    return {"error": {"root_cause": [e], **e}, "status": 400}


@pytest.mark.parametrize("pattern,message", [
    ("a(", "Unclosed group near index 2\na("),
    ("[a-", "Illegal character range near index 3\n[a-"),
    ("[", "Unclosed character class near index 0\n[\n^"),
], ids=["unclosed_group", "char_range", "char_class"])
def test_gsub_invalid_pattern(client, pattern, message):
    r = _simulate(client, {"gsub": {"field": "f", "pattern": pattern, "replacement": "X"}}, {"f": "a"})
    assert r.status_code == 400
    assert r.json() == _config_error("[pattern] Invalid regex pattern. " + message, "gsub", "pattern")


@pytest.mark.parametrize("patterns,defs,source,expected", [
    ([r"(?<x>\w+)(?=-)"], None, "ab-cd", {"x": "ab"}),
    ([r"%{WORD:a} (?<b>\w)\k<b>"], None, "hi xx", {"a": "hi", "b": "x"}),
    (["%{FOO:x}"], {"FOO": "a(?=b)"}, "ab", {"x": "a"}),
    ([r"(?<a_b>x)\k<a_b>"], None, "xx ab", {"a_b": "x"}),
    (["(?<a.b>x)"], None, "xx", {"a": {"b": "x"}}),
    (["%{WORD:a.b} %{NUMBER:n:int}"], None, "hi 12", {"a": {"b": "hi"}, "n": 12}),
    (["^(?<x>b)$"], None, "a\nb\nc", {"x": "b"}),
    (["(?m)a(?<x>.)"], None, "a\nb", {"x": "\n"}),
    ([r"(?<x>\h+)"], None, "zz 1f", {"x": "1f"}),
    (["(?<x>(?<=a)b|(?<=xy)c)"], None, "xyc", {"x": "c"}),
    (["(?<x>a)|(?<x>b)"], None, "b", {"x": "b"}),
    (["a++b"], None, "ab", {}),
], ids=["lookahead", "named_backref", "custom_definition", "underscore_name", "dotted_name", "typed",
        "line_anchors", "ruby_m_is_dotall", "hex_class", "fixed_lookbehind_alternatives", "duplicate_names",
        "possessive"])
def test_grok_oniguruma(client, patterns, defs, source, expected):
    proc = {"grok": {"field": "f", "patterns": patterns}}
    if defs:
        proc["grok"]["pattern_definitions"] = defs
    out = _sim_source(client, proc, {"f": source})
    assert out == {"f": source, **expected}


@pytest.mark.parametrize("pattern,message", [
    ("(?<x>a", "end pattern with unmatched parenthesis"),
    ("a)", "unmatched close parenthesis"),
    ("[a", "premature end of char-class"),
    ("*a", "target of repeat operator is not specified"),
    ("a{2,1}", "upper is smaller than lower in repeat range"),
    ("[z-a]", "empty range in char class"),
    (r"\p{Foo}", "invalid character property name <Foo>"),
    ("(?<1x>a)", "invalid group name <1x>"),
    ("(?<>a)", "group name is empty"),
    (r"\k<zz>", "undefined name <zz> reference"),
    (r"\k<1>", "invalid backref number/name"),
    ("x(?<=a+)", "invalid pattern in look-behind"),
], ids=["unclosed", "unmatched_close", "char_class", "dangling_repeat", "range", "empty_range", "property",
        "group_name", "empty_name", "undefined_backref", "numeric_backref", "variable_lookbehind"])
def test_grok_invalid_pattern(client, pattern, message):
    r = _simulate(client, {"grok": {"field": "f", "patterns": [pattern]}}, {"f": "a"})
    assert r.status_code == 400
    assert r.json() == _config_error(
        f"[patterns] Invalid regex pattern found in: [{pattern}]. {message}", "grok", "patterns")


# --------------------------------------------------------------- CBOR


def _head(major, n):
    if n < 24:
        return bytes([major << 5 | n])
    for ai, fmt in ((24, ">B"), (25, ">H"), (26, ">I"), (27, ">Q")):
        if n < 1 << (8 * struct.calcsize(fmt)):
            return bytes([major << 5 | ai]) + struct.pack(fmt, n)
    raise ValueError(n)


def cbor(v):
    """A tiny CBOR encoder (definite lengths)."""
    if v is None:
        return b"\xf6"
    if v is True or v is False:
        return b"\xf5" if v else b"\xf4"
    if isinstance(v, int):
        return _head(0, v) if v >= 0 else _head(1, -1 - v)
    if isinstance(v, float):
        return b"\xfb" + struct.pack(">d", v)
    if isinstance(v, str):
        b = v.encode()
        return _head(3, len(b)) + b
    if isinstance(v, bytes):
        return _head(2, len(v)) + v
    if isinstance(v, (list, tuple)):
        return _head(4, len(v)) + b"".join(cbor(x) for x in v)
    if isinstance(v, dict):
        return _head(5, len(v)) + b"".join(cbor(k) + cbor(x) for k, x in v.items())
    raise TypeError(v)


def uncbor(data):
    """A tiny CBOR decoder; maps and arrays must be of indefinite length,
    as Jackson writes them."""
    pos = 0

    def item():
        nonlocal pos
        b = data[pos]
        pos += 1
        major, ai = b >> 5, b & 0x1f
        if major == 7:
            if ai in (20, 21, 22):
                return {20: False, 21: True, 22: None}[ai]
            size, fmt = {25: (2, ">e"), 26: (4, ">f"), 27: (8, ">d")}[ai]
            v = struct.unpack(fmt, data[pos:pos + size])[0]
            pos += size
            return v
        if ai == 31:
            assert major in (4, 5), "indefinite strings are not expected"
            out = [] if major == 4 else {}
            while data[pos] != 0xff:
                if major == 4:
                    out.append(item())
                else:
                    k = item()
                    out[k] = item()
            pos += 1
            return out
        assert major not in (4, 5), "Jackson writes indefinite-length containers"
        if ai < 24:
            n = ai
        else:
            size = 1 << (ai - 24)
            n = int.from_bytes(data[pos:pos + size], "big")
            pos += size
        if major == 0:
            return n
        if major == 1:
            return -1 - n
        raw = data[pos:pos + n]
        pos += n
        return raw.decode() if major == 3 else raw

    v = item()
    assert pos == len(data)
    return v


CBOR = {"Content-Type": "application/cbor"}


@pytest.fixture
def cbor_index(client, index_factory):
    return index_factory(settings={"number_of_shards": 1, "number_of_replicas": 0})


def test_cbor_index_and_get(client, cbor_index):
    source = {"a": 1, "b": "x", "c": [1.5, 2.5], "d": {"e": -3, "f": None, "g": True}, "big": 2 ** 40,
              "bin": b"\x01\x02"}
    r = client.put(f"/{cbor_index}/_doc/1", content=cbor(source), headers=CBOR, params={"refresh": "true"})
    assert r.status_code == 201
    assert r.headers["content-type"] == "application/cbor"
    assert r.content[:1] == b"\xbf"
    assert uncbor(r.content) == {"_index": cbor_index, "_id": "1", "_version": 1, "result": "created",
                                 "forced_refresh": True, "_shards": {"total": 1, "successful": 1, "failed": 0},
                                 "_seq_no": 0, "_primary_term": 1}
    r = client.get(f"/{cbor_index}/_doc/1")
    assert r.status_code == 200
    assert r.headers["content-type"].startswith("application/json")
    assert r.json()["_source"] == {"a": 1, "b": "x", "c": [1.5, 2.5], "d": {"e": -3, "f": None, "g": True},
                                   "big": 1099511627776, "bin": base64.b64encode(b"\x01\x02").decode()}


def test_cbor_indefinite_lengths_and_small_floats(client, cbor_index):
    # {_"a": 1, "x": 1.1f, "y": 1.0 (half)} with an indefinite-length map
    body = b"\xbf\x61a\x01\x61x\xfa\x3f\x8c\xcc\xcd\x61y\xf9\x3c\x00\xff"
    r = client.put(f"/{cbor_index}/_doc/2", content=body, headers=CBOR, params={"refresh": "true"})
    assert r.status_code == 201
    r = client.get(f"/{cbor_index}/_doc/2")
    assert r.json()["_source"] == {"a": 1, "x": 1.1, "y": 1.0}
    assert '"_source":{"a":1,"x":1.1,"y":1.0}' in r.text


def test_cbor_search_answers_cbor(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", json={"b": "x"}, params={"refresh": "true"})
    assert r.status_code == 201
    body = cbor({"query": {"match": {"b": "x"}}, "_source": False})
    r = client.post(f"/{cbor_index}/_search", content=body, headers=CBOR)
    assert r.status_code == 200
    assert r.headers["content-type"] == "application/cbor"
    out = uncbor(r.content)
    assert out["hits"]["total"] == {"value": 1, "relation": "eq"}
    assert [h["_id"] for h in out["hits"]["hits"]] == ["1"]
    r = client.post(f"/{cbor_index}/_search", content=body, headers={**CBOR, "Accept": "application/json"})
    assert r.status_code == 200
    assert r.headers["content-type"].startswith("application/json")
    assert r.json()["hits"]["total"] == {"value": 1, "relation": "eq"}


def test_cbor_response_by_accept_and_format(client, cbor_index):
    for kw in ({"headers": {"Accept": "application/cbor"}}, {"params": {"format": "cbor"}}):
        r = client.get(f"/{cbor_index}/_doc/missing", **kw)
        assert r.status_code == 404
        assert r.headers["content-type"] == "application/cbor"
        assert uncbor(r.content) == {"_index": cbor_index, "_id": "missing", "found": False}


def test_cbor_error_answers_cbor(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", content=cbor({"a": 1}), headers=CBOR, params={"refresh": "bad"})
    assert r.status_code == 400
    assert r.headers["content-type"] == "application/cbor"
    cause = {"type": "illegal_argument_exception", "reason": "Unknown value for refresh: [bad]."}
    assert uncbor(r.content) == {"error": {"root_cause": [cause], **cause}, "status": 400}


def test_cbor_truncated_body(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", content=b"\xa1\x61", headers=CBOR)
    assert r.status_code == 400
    err = uncbor(r.content)["error"]
    assert err["type"] == "mapper_parsing_exception"
    assert err["reason"] == "failed to parse"
    assert err["caused_by"]["type"] == "json_parse_exception"


# --------------------------------------------------------------- YAML


YAML = {"Content-Type": "application/yaml"}

YAML_DOC = b"""---
a: 1
b:
  c: [x, 'y z']
  d:
    - 1.5
    - 2.5
e: "q"
f: yes
g: 0x1F
h: 1e3
i: ~
j: 2024-01-01
k: |
  line1
  line2
l: {m: n}
n: 1.0
o: -0
p: 012
q: .5
r: 1_000
s: True
t: 'it''s'
u: "\\u00e9\\t"
"""


def test_yaml_document(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", content=YAML_DOC, headers=YAML, params={"refresh": "true"})
    assert r.status_code == 201
    assert r.headers["content-type"] == "application/yaml"
    assert r.text.startswith("---\n_index: ")
    assert 'result: "created"\n' in r.text
    r = client.get(f"/{cbor_index}/_doc/1")
    assert r.json()["_source"] == {
        "a": 1, "b": {"c": ["x", "y z"], "d": [1.5, 2.5]}, "e": "q", "f": True, "g": 31, "h": 1000.0,
        "i": None, "j": "2024-01-01", "k": "line1\nline2\n", "l": {"m": "n"}, "n": 1.0, "o": 0, "p": 10,
        "q": 0.5, "r": "1_000", "s": True, "t": "it's", "u": "é\t"}
    assert '"h":1000.0' in r.text


def test_yaml_search(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", json={"a": 1, "tags": ["x", "y"]}, params={"refresh": "true"})
    assert r.status_code == 201
    body = b"query:\n  bool:\n    filter:\n      - term:\n          a: 1\n      - terms:\n          tags: [y, z]\n_source: false\n"
    r = client.post(f"/{cbor_index}/_search", content=body, headers={**YAML, "Accept": "application/json"})
    assert r.status_code == 200, r.text
    assert [h["_id"] for h in r.json()["hits"]["hits"]] == ["1"]
    r = client.post(f"/{cbor_index}/_search", content=body, headers=YAML)
    assert r.status_code == 200
    assert r.headers["content-type"] == "application/yaml"
    assert r.text.startswith("---\n")
    assert "  max_score: 0.0\n" in r.text


def test_yaml_document_needs_document_start(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", content=b"a: 1\n", headers={**YAML, "Accept": "application/json"})
    assert r.status_code == 400
    assert r.json() == {"error": {
        "root_cause": [{"type": "mapper_parsing_exception", "reason": "failed to parse"}],
        "type": "mapper_parsing_exception", "reason": "failed to parse",
        "caused_by": {"type": "not_x_content_exception",
                      "reason": "Compressor detection can only be called on some xcontent bytes or compressed xcontent bytes"}},
        "status": 400}


def test_yaml_document_must_be_an_object(client, cbor_index):
    r = client.put(f"/{cbor_index}/_doc/1", content=b"---\n- 1\n- 2\n", headers={**YAML, "Accept": "application/json"})
    assert r.status_code == 400
    e = {"type": "mapper_parsing_exception", "reason": "Malformed content, must start with an object"}
    assert r.json() == {"error": {"root_cause": [e], **e}, "status": 400}


@pytest.mark.parametrize("body", [b"---\na: [1\nb: 2\n", b"---\na: 1\n  b: 2\n", b"---\na: 1\na: 2\n"],
                         ids=["unclosed_flow", "bad_indent", "duplicate_key"])
def test_yaml_invalid_document(client, cbor_index, body):
    r = client.put(f"/{cbor_index}/_doc/1", content=body, headers={**YAML, "Accept": "application/json"})
    assert r.status_code == 400
    err = r.json()["error"]
    assert err["root_cause"] == [{"type": "mapper_parsing_exception", "reason": "failed to parse"}]
    assert err["type"] == "mapper_parsing_exception"
    assert err["caused_by"]["type"] == "json_parse_exception"
