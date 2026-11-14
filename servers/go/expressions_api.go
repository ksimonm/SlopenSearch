package main

import (
	"strings"

	"slopensearch/internal/apierr"
	"slopensearch/internal/expressions"
	"slopensearch/internal/mapping"
	"slopensearch/internal/model"
	"slopensearch/internal/painless"
)

// Wiring of the expression engine (internal/expressions) into scripts.

// expressionScriptErr renders an expression error as a script_exception.
func expressionScriptErr(e *expressions.Error, src string) *plScriptErr {
	se := &plScriptErr{compile: !e.Link, script: src, lang: "expression", causeType: "parse_exception", reason: e.Reason}
	if e.Link {
		se.linkErr = true
	}
	if e.EOF {
		se.inner = &painless.CompileError{Type: "no_viable_alt_exception", NilReason: true}
	}
	off := e.Offset
	if off > len(src) {
		off = len(src)
	}
	se.stack = []string{src, strings.Repeat(" ", off) + "^---- HERE"}
	return se
}

// exPrepare compiles and checks an expression for a context.
func exPrepare(src, ctxName string) (*expressions.Script, *plScriptErr, *apierr.Error) {
	s, err := expressions.Compile(src)
	if err != nil {
		return nil, expressionScriptErr(err, src), nil
	}
	if !expressionContexts[ctxName] {
		return nil, nil, apierr.BadRequest("expression engine does not know how to handle script context [" + ctxName + "]")
	}
	return s, nil, nil
}

// exprFields resolves doc['field'] variables against the mapping of idx.
func exprFields(idx *model.Index) func(field string) (expressions.Kind, bool) {
	if idx == nil {
		return nil
	}
	return func(field string) (expressions.Kind, bool) {
		def, found := mapping.FieldDef(idx, field)
		if !found {
			return expressions.KindOther, false
		}
		ft, _ := def["type"].(string)
		switch {
		case ft == "date" || ft == "date_nanos":
			return expressions.KindDate, true
		case mapping.IsNumericType(ft) || ft == "boolean":
			return expressions.KindNumber, true
		case ft == "geo_point":
			return expressions.KindGeo, true
		}
		return expressions.KindOther, true
	}
}

// exprDoc exposes a document's values to expressions.
type exprDoc struct{ qc *queryCtx }

func (d exprDoc) Values(field string) []any {
	vals, _ := d.qc.values(field)
	return vals
}
