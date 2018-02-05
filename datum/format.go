package datum

import (
	"bytes"
	"fmt"

	"github.com/cockroachdb/cockroach/pkg/sql/coltypes"
	"github.com/mcuadros/pgwire/types"
)

// FmtFlags carries options for the pretty-printer.
type FmtFlags int

// FmtCtx is suitable for passing to Format() methods.
// It also exposes the underlying bytes.Buffer interface for
// convenience.
type FmtCtx struct {
	*bytes.Buffer
	// The flags to use for pretty-printing.
	flags FmtFlags
	// indexedVarFormat is an optional interceptor for
	// IndexedVarContainer.IndexedVarFormat calls; it can be used to
	// customize the formatting of IndexedVars.
	indexedVarFormat func(ctx *FmtCtx, idx int)
	// tableNameFormatter will be called on all NormalizableTableNames if it is
	// non-nil.
	//tableNameFormatter func(*FmtCtx, *NormalizableTableName)
	// placeholderFormat is an optional interceptor for Placeholder.Format calls;
	// it can be used to format placeholders differently than normal.
	//placeholderFormat func(ctx *FmtCtx, p *Placeholder)
}

// MakeFmtCtx creates a FmtCtx from an existing buffer and flags.
func MakeFmtCtx(buf *bytes.Buffer, f FmtFlags) FmtCtx {
	return FmtCtx{Buffer: buf, flags: f}
}

// HasFlags returns true iff the given flags are set in the formatter context.
func (ctx *FmtCtx) HasFlags(f FmtFlags) bool {
	return ctx.flags.HasFlags(f)
}

// Printf calls fmt.Fprintf on the linked bytes.Buffer. It is provided
// for convenience, to avoid having to call fmt.Fprintf(ctx.Buffer, ...).
//
// Note: DO NOT USE THIS TO INTERPOLATE %s ON NodeFormatter OBJECTS.
// This would call the String() method on them and would fail to reuse
// the same bytes buffer (and waste allocations). Instead use
// ctx.FormatNode().
func (ctx *FmtCtx) Printf(f string, args ...interface{}) {
	fmt.Fprintf(ctx.Buffer, f, args...)
}

// FormatNode recurses into a node for pretty-printing.
// Flag-driven special cases can hook into this.
func (ctx *FmtCtx) FormatNode(n Datum) {
	f := ctx.flags
	if f.HasFlags(FmtShowTypes) {
		if te, ok := n.(TypedExpr); ok {
			ctx.WriteByte('(')
			ctx.formatNodeOrHideConstants(n)
			ctx.WriteString(")[")
			if rt := te.ResolvedType(); rt == nil {
				// An attempt is made to pretty-print an expression that was
				// not assigned a type yet. This should not happen, so we make
				// it clear in the output this needs to be investigated
				// further.
				fmt.Fprintf(ctx.Buffer, "??? %v", te)
			} else {
				ctx.WriteString(rt.String())
			}
			ctx.WriteByte(']')
			return
		}
	}
	if f.HasFlags(FmtAlwaysGroupExprs) {
		if _, ok := n.(Expr); ok {
			ctx.WriteByte('(')
		}
	}
	ctx.formatNodeOrHideConstants(n)
	if f.HasFlags(FmtAlwaysGroupExprs) {
		if _, ok := n.(Expr); ok {
			ctx.WriteByte(')')
		}
	}
	if f.HasFlags(fmtDisambiguateDatumTypes) {
		var typ types.T
		if d, isDatum := n.(Datum); isDatum {
			if p, isPlaceholder := d.(*Placeholder); isPlaceholder {
				// p.typ will be nil if the placeholder has not been type-checked yet.
				typ = p.typ
			} else if d.AmbiguousFormat() {
				typ = d.ResolvedType()
			}
		}
		if typ != nil {
			ctx.WriteString(":::")
			colType, err := coltypes.DatumTypeToColumnType(typ)
			if err != nil {
				panic(err)
			}
			colType.Format(ctx.Buffer, f.EncodeFlags())
		}
	}
}
