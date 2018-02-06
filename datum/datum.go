// Copyright 2015 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License.

package datum

import (
	"bytes"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"time"
	"unsafe"

	"github.com/mcuadros/pgwire/types"

	"github.com/cockroachdb/apd"
	"github.com/cockroachdb/cockroach/pkg/util/duration"
	"github.com/cockroachdb/cockroach/pkg/util/ipaddr"
	"github.com/cockroachdb/cockroach/pkg/util/json"
	"github.com/cockroachdb/cockroach/pkg/util/stringencoding"
	"github.com/cockroachdb/cockroach/pkg/util/timeofday"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/cockroachdb/cockroach/pkg/util/uuid"
	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

var (
	constDBoolTrue  DBool = true
	constDBoolFalse DBool = false

	// DBoolTrue is a pointer to the DBool(true) value and can be used in
	// comparisons against Datum types.
	DBoolTrue = &constDBoolTrue
	// DBoolFalse is a pointer to the DBool(false) value and can be used in
	// comparisons against Datum types.
	DBoolFalse = &constDBoolFalse

	// DNull is the NULL Datum.
	DNull Datum = dNull{}

	// DZero is the zero-valued integer Datum.
	DZero = NewDInt(0)
)

// Datum represents a SQL value.
type Datum interface {
	// ResolvedType provides the type of the TypedExpr, which is the type of Datum
	// that the TypedExpr will return when evaluated.
	ResolvedType() types.T
	// It holds for every Datum d that d.Size().
	Size() uintptr
	// Format performs pretty-printing towards a bytes buffer.
	Format(*bytes.Buffer)
}

// Datums is a slice of Datum values.
type Datums []Datum

// Len returns the number of Datum values.
func (d Datums) Len() int { return len(d) }

// Reverse reverses the order of the Datum values.
func (d Datums) Reverse() {
	for i, j := 0, d.Len()-1; i < j; i, j = i+1, j-1 {
		d[i], d[j] = d[j], d[i]
	}
}

// DBool is the boolean Datum.
type DBool bool

func NewDBool(d bool) *DBool {
	if d {
		return DBoolTrue
	}

	return DBoolFalse
}

// ResolvedType implements the TypedExpr interface.
func (*DBool) ResolvedType() types.T {
	return types.Bool
}

// Size implements the Datum interface.
func (d *DBool) Size() uintptr {
	return unsafe.Sizeof(*d)
}

// Format implements the Datum interface.
func (d *DBool) Format(w *bytes.Buffer) {
	w.WriteString(strconv.FormatBool(bool(*d)))
}

// DInt is the int Datum.
type DInt int64

// NewDInt is a helper routine to create a *DInt initialized from its argument.
func NewDInt(v int64) *DInt {
	di := DInt(v)
	return &di
}

// ResolvedType implements the TypedExpr interface.
func (*DInt) ResolvedType() types.T {
	return types.Int
}

// Size implements the Datum interface.
func (d *DInt) Size() uintptr {
	return unsafe.Sizeof(*d)
}

func (d *DInt) Format(w *bytes.Buffer) {
	w.WriteString(strconv.FormatInt(int64(*d), 10))
}

// DFloat is the float Datum.
type DFloat float64

// NewDFloat is a helper routine to create a *DFloat initialized from its
// argument.
func NewDFloat(d float64) *DFloat {
	df := DFloat(d)
	return &df
}

// ResolvedType implements the TypedExpr interface.
func (*DFloat) ResolvedType() types.T {
	return types.Float
}

// Size implements the Datum interface.
func (d *DFloat) Size() uintptr {
	return unsafe.Sizeof(*d)
}

func (d *DFloat) Format(w *bytes.Buffer) {
	fl := float64(*d)
	if _, frac := math.Modf(fl); frac == 0 && -1000000 < *d && *d < 1000000 {
		// d is a small whole number. Ensure it is printed using a decimal point.
		fmt.Fprintf(w, "%.1f", fl)
	} else {
		fmt.Fprintf(w, "%g", fl)
	}
}

// DDecimal is the decimal Datum.
type DDecimal struct {
	apd.Decimal
}

// ParseDDecimal parses and returns the *DDecimal Datum value represented by the
// provided string, or an error if parsing is unsuccessful.
func ParseDDecimal(s string) (*DDecimal, error) {
	d, _, err := apd.NewFromString(s)
	return &DDecimal{*d}, err
}

// ResolvedType implements the TypedExpr interface.
func (*DDecimal) ResolvedType() types.T {
	return types.Decimal
}

// Size implements the Datum interface.
func (d *DDecimal) Size() uintptr {
	intVal := d.Decimal.Coeff
	return unsafe.Sizeof(*d) + uintptr(cap(intVal.Bits()))*unsafe.Sizeof(big.Word(0))
}

// Format implements the NodeFormatter interface.
func (d *DDecimal) Format(w *bytes.Buffer) {
	w.WriteString(d.Decimal.String())
}

// DString is the string Datum.
type DString string

// NewDString is a helper routine to create a *DString initialized from its
// argument.
func NewDString(d string) *DString {
	r := DString(d)
	return &r
}

// ResolvedType implements the TypedExpr interface.
func (*DString) ResolvedType() types.T {
	return types.String
}

// Size implements the Datum interface.
func (d *DString) Size() uintptr {
	return unsafe.Sizeof(*d) + uintptr(len(*d))
}

func (d *DString) Format(w *bytes.Buffer) {
	EncodeSQLString(w, string(*d))
}

// DCollatedString is the Datum for strings with a locale. The struct members
// are intended to be immutable.
type DCollatedString struct {
	Contents string
	Locale   string
	// Key is the collation key.
	Key []byte
}

// CollationEnvironment stores the state needed by NewDCollatedString to
// construct collation keys efficiently.
type CollationEnvironment struct {
	cache  map[string]collationEnvironmentCacheEntry
	buffer *collate.Buffer
}

type collationEnvironmentCacheEntry struct {
	// locale is interned.
	locale string
	// collator is an expensive factory.
	collator *collate.Collator
}

func (env *CollationEnvironment) getCacheEntry(locale string) collationEnvironmentCacheEntry {
	entry, ok := env.cache[locale]
	if !ok {
		if env.cache == nil {
			env.cache = make(map[string]collationEnvironmentCacheEntry)
		}
		entry = collationEnvironmentCacheEntry{locale, collate.New(language.MustParse(locale))}
		env.cache[locale] = entry
	}
	return entry
}

// NewDCollatedString is a helper routine to create a *DCollatedString. Panics
// if locale is invalid. Not safe for concurrent use.
func NewDCollatedString(
	contents string, locale string, env *CollationEnvironment,
) *DCollatedString {
	entry := env.getCacheEntry(locale)
	if env.buffer == nil {
		env.buffer = &collate.Buffer{}
	}
	key := entry.collator.KeyFromString(env.buffer, contents)
	d := DCollatedString{contents, entry.locale, make([]byte, len(key))}
	copy(d.Key, key)
	env.buffer.Reset()
	return &d
}

// ResolvedType implements the TypedExpr interface.
func (d *DCollatedString) ResolvedType() types.T {
	return types.TCollatedString{Locale: d.Locale}
}

// Size implements the Datum interface.
func (d *DCollatedString) Size() uintptr {
	return unsafe.Sizeof(*d) + uintptr(len(d.Contents)) + uintptr(len(d.Locale)) + uintptr(len(d.Key))
}

func (d *DCollatedString) Format(w *bytes.Buffer) {
	EncodeSQLString(w, d.Contents)
	w.WriteString(" COLLATE ")
	w.WriteString(d.Locale)
}

// DBytes is the bytes Datum. The underlying type is a string because we want
// the immutability, but this may contain arbitrary bytes.
type DBytes string

// NewDBytes is a helper routine to create a *DBytes initialized from its
// argument.
func NewDBytes(d string) *DBytes {
	db := DBytes(d)
	return &db
}

// ResolvedType implements the TypedExpr interface.
func (*DBytes) ResolvedType() types.T {
	return types.Bytes
}

// Size implements the Datum interface.
func (d *DBytes) Size() uintptr {
	return unsafe.Sizeof(*d) + uintptr(len(*d))
}

// Format implements the NodeFormatter interface.
func (d *DBytes) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	w.WriteString("\\x")
	b := string(*d)
	for i := 0; i < len(b); i++ {
		w.Write(stringencoding.RawHexMap[b[i]])
	}
	w.WriteByte('\'')
}

// DUuid is the UUID Datum.
type DUuid struct {
	uuid.UUID
}

// NewDUuid is a helper routine to create a *DUuid initialized from its
// argument.
func NewDUuid(d DUuid) *DUuid {
	return &d
}

// ResolvedType implements the TypedExpr interface.
func (*DUuid) ResolvedType() types.T {
	return types.UUID
}

// Size implements the Datum interface.
func (d *DUuid) Size() uintptr {
	return unsafe.Sizeof(*d)
}

// Format implements the NodeFormatter interface.
func (d *DUuid) Format(w *bytes.Buffer) {
	w.WriteString(d.UUID.String())
}

// DIPAddr is the IPAddr Datum.
type DIPAddr struct {
	ipaddr.IPAddr
}

// NewDIPAddr is a helper routine to create a *DIPAddr initialized from its
// argument.
func NewDIPAddr(d DIPAddr) *DIPAddr {
	return &d
}

// ResolvedType implements the TypedExpr interface.
func (*DIPAddr) ResolvedType() types.T {
	return types.INet
}

// Size implements the Datum interface.
func (d *DIPAddr) Size() uintptr {
	return unsafe.Sizeof(*d)
}

// Format implements the NodeFormatter interface.
func (d *DIPAddr) Format(w *bytes.Buffer) {
	w.WriteString(d.IPAddr.String())
}

// DDate is the date Datum represented as the number of days after
// the Unix epoch.
type DDate int64

const SecondsInDay = 24 * 60 * 60

// NewDDate is a helper routine to create a *DDate initialized from its
// argument.
func NewDDate(v int64) *DDate {
	dd := DDate(v)
	return &dd
}

// NewDDateFromTime constructs a *DDate from a time.Time in the provided time zone.
func NewDDateFromTime(t time.Time, loc *time.Location) *DDate {
	Year, Month, Day := t.In(loc).Date()
	secs := time.Date(Year, Month, Day, 0, 0, 0, 0, time.UTC).Unix()

	d := DDate(secs / SecondsInDay)
	return &d
}

// ResolvedType implements the TypedExpr interface.
func (*DDate) ResolvedType() types.T {
	return types.Date
}

// Size implements the Datum interface.
func (d *DDate) Size() uintptr {
	return unsafe.Sizeof(*d)
}

const dateFormat = "2006-01-02"

func (d *DDate) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	w.WriteString(timeutil.Unix(int64(*d)*SecondsInDay, 0).Format(dateFormat))
	w.WriteByte('\'')
}

// DTime is the time Datum.
type DTime timeofday.TimeOfDay

// MakeDTime creates a DTime from a TimeOfDay.
func MakeDTime(t timeofday.TimeOfDay) *DTime {
	d := DTime(t)
	return &d
}

// ResolvedType implements the TypedExpr interface.
func (*DTime) ResolvedType() types.T {
	return types.Time
}

// Size implements the Datum interface.
func (d *DTime) Size() uintptr {
	return unsafe.Sizeof(*d)
}

// Format implements the NodeFormatter interface.
func (d *DTime) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	w.WriteString(timeofday.TimeOfDay(*d).String())
	w.WriteByte('\'')
}

// DTimestamp is the timestamp Datum.
type DTimestamp struct {
	time.Time
}

// MakeDTimestamp creates a DTimestamp with specified precision.
func MakeDTimestamp(t time.Time, precision time.Duration) *DTimestamp {
	return &DTimestamp{Time: t.Round(precision)}
}

// ResolvedType implements the TypedExpr interface.
func (*DTimestamp) ResolvedType() types.T {
	return types.Timestamp
}

// Size implements the Datum interface.
func (d *DTimestamp) Size() uintptr {
	return unsafe.Sizeof(*d)
}

const TimestampOutputFormat = "2006-01-02 15:04:05.999999-07:00"

func (d *DTimestamp) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	w.WriteString(d.UTC().Format(TimestampOutputFormat))
	w.WriteByte('\'')
}

// DTimestampTZ is the timestamp Datum that is rendered with session offset.
type DTimestampTZ struct {
	time.Time
}

// MakeDTimestampTZ creates a DTimestampTZ with specified precision.
func MakeDTimestampTZ(t time.Time, precision time.Duration) *DTimestampTZ {
	return &DTimestampTZ{Time: t.Round(precision)}
}

// MakeDTimestampTZFromDate creates a DTimestampTZ from a DDate.
func MakeDTimestampTZFromDate(loc *time.Location, d *DDate) *DTimestampTZ {
	year, month, day := timeutil.Unix(int64(*d)*SecondsInDay, 0).Date()
	return MakeDTimestampTZ(time.Date(year, month, day, 0, 0, 0, 0, loc), time.Microsecond)
}

// ResolvedType implements the TypedExpr interface.
func (*DTimestampTZ) ResolvedType() types.T {
	return types.TimestampTZ
}

// Size implements the Datum interface.
func (d *DTimestampTZ) Size() uintptr {
	return unsafe.Sizeof(*d)
}

func (d *DTimestampTZ) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	w.WriteString(d.Time.Format(TimestampOutputFormat))
	w.WriteByte('\'')
}

// DInterval is the interval Datum.
type DInterval struct {
	duration.Duration
}

// DurationField is the type of a postgres duration field.
// https://www.postgresql.org/docs/9.6/static/datatype-datetime.html
type DurationField int

// ResolvedType implements the TypedExpr interface.
func (*DInterval) ResolvedType() types.T {
	return types.Interval
}

// Size implements the Datum interface.
func (d *DInterval) Size() uintptr {
	return unsafe.Sizeof(*d)
}

// Format implements the NodeFormatter interface.
func (d *DInterval) Format(w *bytes.Buffer) {
	w.WriteByte('\'')
	d.Duration.Format(w)
	w.WriteByte('\'')
}

// DJSON is the JSON Datum.
type DJSON struct{ json.JSON }

// NewDJSON is a helper routine to create a DJSON initialized from its argument.
func NewDJSON(j json.JSON) *DJSON {
	return &DJSON{j}
}

// MakeDJSON returns a JSON value given a Go-style representation of JSON.
// * JSON null is Go `nil`,
// * JSON true is Go `true`,
// * JSON false is Go `false`,
// * JSON numbers are json.Number | int | int64 | float64,
// * JSON string is a Go string,
// * JSON array is a Go []interface{},
// * JSON object is a Go map[string]interface{}.
func MakeDJSON(d interface{}) (Datum, error) {
	j, err := json.MakeJSON(d)
	if err != nil {
		return nil, err
	}
	return &DJSON{j}, nil
}

// ResolvedType implements the TypedExpr interface.
func (*DJSON) ResolvedType() types.T {
	return types.JSON
}

// Size implements the Datum interface.
// TODO(justin): is this a frequently-called method? Should we be caching the computed size?
func (d *DJSON) Size() uintptr {
	return unsafe.Sizeof(*d) + d.JSON.Size()
}

// Format implements the NodeFormatter interface.
func (d *DJSON) Format(w *bytes.Buffer) {
	// TODO(justin): ideally the JSON string encoder should know it needs to
	// escape things to be inside SQL strings in order to avoid this allocation.
	s := d.JSON.String()
	EncodeSQLString(w, s)
}

// DTuple is the tuple Datum.
type DTuple struct {
	D Datums

	Sorted bool
}

// NewDTuple creates a *DTuple with the provided datums. When creating a new
// DTuple with Datums that are known to be sorted in ascending order, chain
// this call with DTuple.SetSorted.
func NewDTuple(d ...Datum) *DTuple {
	return &DTuple{D: d}
}

// NewDTupleWithLen creates a *DTuple with the provided length.
func NewDTupleWithLen(l int) *DTuple {
	return &DTuple{D: make(Datums, l)}
}

// NewDTupleWithCap creates a *DTuple with the provided capacity.
func NewDTupleWithCap(c int) *DTuple {
	return &DTuple{D: make(Datums, 0, c)}
}

// ResolvedType implements the TypedExpr interface.
func (d *DTuple) ResolvedType() types.T {
	typ := make(types.TTuple, len(d.D))
	for i, v := range d.D {
		typ[i] = v.ResolvedType()
	}
	return typ
}

// Size implements the Datum interface.
func (d *DTuple) Size() uintptr {
	sz := unsafe.Sizeof(*d)
	for _, e := range d.D {
		dsz := e.Size()
		sz += dsz
	}
	return sz
}

func (d *DTuple) Format(w *bytes.Buffer) {
	//TODO: w.FormatNode(&d.D)
}

type dNull struct{}

// ResolvedType implements the TypedExpr interface.
func (dNull) ResolvedType() types.T {
	return types.Null
}

// Size implements the Datum interface.
func (d dNull) Size() uintptr {
	return unsafe.Sizeof(d)
}

func (dNull) Format(w *bytes.Buffer) {
	w.WriteString("NULL")
}

// DArray is the array Datum. Any Datum inserted into a DArray are treated as
// text during serialization.
type DArray struct {
	ParamTyp types.T
	Array    Datums
	// HasNulls is set to true if any of the datums within the array are null.
	// This is used in the binary array serialization format.
	HasNulls bool
}

// NewDArray returns a DArray containing elements of the specified type.
func NewDArray(paramTyp types.T) *DArray {
	return &DArray{ParamTyp: paramTyp}
}

// ResolvedType implements the TypedExpr interface.
func (d *DArray) ResolvedType() types.T {
	return types.TArray{Typ: d.ParamTyp}
}

// Len returns the length of the Datum array.
func (d *DArray) Len() int {
	return len(d.Array)
}

// Size implements the Datum interface.
func (d *DArray) Size() uintptr {
	sz := unsafe.Sizeof(*d)
	for _, e := range d.Array {
		dsz := e.Size()
		sz += dsz
	}
	return sz
}

func (d *DArray) Format(w *bytes.Buffer) {
	w.WriteString("ARRAY[")
	for i, v := range d.Array {
		if i > 0 {
			w.WriteString(",")
		}
		v.Format(w)
	}
	w.WriteByte(']')
}

// DOid is the Postgres OID datum. It can represent either an OID type or any
// of the reg* types, such as regproc or regclass.
type DOid struct {
	// A DOid embeds a DInt, the underlying integer OID for this OID datum.
	DInt
	// name is set to the resolved name of this OID, if available.
	name string
}

// NewDOid is a helper routine to create a DOid initialized from a int64.
func NewDOid(v int64) *DOid {
	return &DOid{DInt: DInt(v), name: ""}
}

// ResolvedType implements the Datum interface.
func (d *DOid) ResolvedType() types.T {
	return types.Oid
}

// Size implements the Datum interface.
func (d *DOid) Size() uintptr { return unsafe.Sizeof(*d) }

func (d *DOid) Format(w *bytes.Buffer) {
	d.DInt.Format(w)
}

// DatumTypeSize returns a lower bound on the total size of a Datum
// of the given type in bytes, including memory that is
// pointed at (even if shared between Datum instances) but excluding
// allocation overhead.
//
// The second argument indicates whether data of this type have different
// sizes.
//
// It holds for every Datum d that d.Size() >= DatumSize(d.ResolvedType())
func DatumTypeSize(t types.T) (uintptr, bool) {
	// The following are composite types.
	switch ty := t.(type) {
	case types.TOid:
		// Note: we have multiple Type instances of tOid (TypeOid,
		// TypeRegClass, etc). Instead of listing all of them in
		// baseDatumTypeSizes below, we use a single case here.
		return unsafe.Sizeof(DInt(0)), fixedSize

	case types.TOidWrapper:
		return DatumTypeSize(ty.T)

	case types.TCollatedString:
		return unsafe.Sizeof(DCollatedString{"", "", nil}), variableSize

	case types.TTuple:
		sz := uintptr(0)
		variable := false
		for _, typ := range ty {
			typsz, typvariable := DatumTypeSize(typ)
			sz += typsz
			variable = variable || typvariable
		}
		return sz, variable

	case types.TTable:
		sz, _ := DatumTypeSize(ty.Cols)
		return sz, variableSize

	case types.TArray:
		// TODO(jordan,justin): This seems suspicious.
		return unsafe.Sizeof(DString("")), variableSize
	}

	// All the primary types have fixed size information.
	if bSzInfo, ok := baseDatumTypeSizes[t]; ok {
		return bSzInfo.sz, bSzInfo.variable
	}

	panic(fmt.Sprintf("unknown type: %T", t))
}

const (
	fixedSize    = false
	variableSize = true
)

var baseDatumTypeSizes = map[types.T]struct {
	sz       uintptr
	variable bool
}{
	types.Null:        {unsafe.Sizeof(dNull{}), fixedSize},
	types.Bool:        {unsafe.Sizeof(DBool(false)), fixedSize},
	types.Int:         {unsafe.Sizeof(DInt(0)), fixedSize},
	types.Float:       {unsafe.Sizeof(DFloat(0.0)), fixedSize},
	types.Decimal:     {unsafe.Sizeof(DDecimal{}), variableSize},
	types.String:      {unsafe.Sizeof(DString("")), variableSize},
	types.Bytes:       {unsafe.Sizeof(DBytes("")), variableSize},
	types.Date:        {unsafe.Sizeof(DDate(0)), fixedSize},
	types.Time:        {unsafe.Sizeof(DTime(0)), fixedSize},
	types.Timestamp:   {unsafe.Sizeof(DTimestamp{}), fixedSize},
	types.TimestampTZ: {unsafe.Sizeof(DTimestampTZ{}), fixedSize},
	types.Interval:    {unsafe.Sizeof(DInterval{}), fixedSize},
	types.JSON:        {unsafe.Sizeof(DJSON{}), variableSize},
	types.UUID:        {unsafe.Sizeof(DUuid{}), fixedSize},
	types.INet:        {unsafe.Sizeof(DIPAddr{}), fixedSize},
	// TODO(jordan,justin): This seems suspicious.
	types.Any: {unsafe.Sizeof(DString("")), variableSize},
}
