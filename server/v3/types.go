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

package v3

import (
	"context"
	"encoding/hex"
	"math"
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/cockroachdb/cockroach/pkg/util/duration"
	"github.com/cockroachdb/cockroach/pkg/util/timeofday"
	"github.com/cockroachdb/cockroach/pkg/util/timeutil"
	"github.com/lib/pq/oid"
	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
	"gopkg.in/sqle/sqle.v0/sql"

	"github.com/mcuadros/pgwire"
)

// pgType contains type metadata used in RowDescription messages.
type pgType struct {
	oid oid.Oid

	// Variable-size types have size=-1.
	// Note that the protocol has both int16 and int32 size fields,
	// so this attribute is an unsized int and should be cast
	// as needed.
	// This field does *not* correspond to the encoded length of a
	// data type, so it's unclear what, if anything, it is used for.
	// To get the right value, "SELECT oid, typlen FROM pg_type"
	// on a postgres server.
	size int
}

//go:generate stringer -type=pgNumericSign
type pgNumericSign uint16

const (
	pgNumericPos pgNumericSign = 0x0000
	pgNumericNeg pgNumericSign = 0x4000
	// pgNumericNan pgNumericSign = 0xC000
)

// The number of decimal digits per int16 Postgres "digit".
const pgDecDigits = 4

type pgNumeric struct {
	ndigits, weight, dscale int16
	sign                    pgNumericSign
}

func pgTypeForParserType(t sql.Type) pgType {
	size := -1
	if s, variable := pgwire.DatumTypeSize(t); !variable {
		size = int(s)
	}

	o, ok := mappingTypeToOid[t]
	if !ok {
		o = oid.T_unknown
	}

	return pgType{
		oid:  o,
		size: size,
	}
}

var mappingTypeToOid = map[sql.Type]oid.Oid{
	sql.Boolean:               oid.T_bool,
	sql.Integer:               oid.T_int8,
	sql.Float:                 oid.T_float8,
	sql.Decimal:               oid.T_numeric,
	sql.String:                oid.T_text,
	sql.Blob:                  oid.T_bytea,
	sql.Date:                  oid.T_date,
	sql.Time:                  oid.T_time,
	sql.Timestamp:             oid.T_timestamp,
	sql.TimestampWithTimezone: oid.T_timestamptz,
	sql.Interval:              oid.T_interval,
}

const secondsInDay = 24 * 60 * 60

func (b *writeBuffer) writeTextpgwire(ctx context.Context, d pgwire.Datum, sessionLoc *time.Location) {
	log.Debugf("pgwire writing TEXT pgwire of type: %T, %#v", d, d)

	if d == pgwire.DNull {
		// NULL is encoded as -1; all other values have a length prefix.
		b.putInt32(-1)
		return
	}
	switch v := d.(type) {
	case *pgwire.DBool:
		b.putInt32(1)
		if *v {
			b.writeByte('t')
		} else {
			b.writeByte('f')
		}

	case *pgwire.DInt:
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := strconv.AppendInt(b.putbuf[4:4], int64(*v), 10)
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DFloat:
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := strconv.AppendFloat(b.putbuf[4:4], float64(*v), 'f', -1, 64)
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DDecimal:
		b.writeLengthPrefixedpgwire(v)

	case *pgwire.DBytes:
		// http://www.postgresql.org/docs/current/static/datatype-binary.html#AEN5667
		// Code cribbed from github.com/lib/pq.
		result := make([]byte, 2+hex.EncodedLen(len(*v)))
		result[0] = '\\'
		result[1] = 'x'
		hex.Encode(result[2:], []byte(*v))

		b.putInt32(int32(len(result)))
		b.write(result)

	case *pgwire.DString:
		b.writeLengthPrefixedString(string(*v))

	case *pgwire.DCollatedString:
		b.writeLengthPrefixedString(v.Contents)

	case *pgwire.DDate:
		t := timeutil.Unix(int64(*v)*secondsInDay, 0)
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := formatTs(t, nil, b.putbuf[4:4])
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DTime:
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := formatTime(timeofday.TimeOfDay(*v), b.putbuf[4:4])
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DTimestamp:
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := formatTs(v.Time, nil, b.putbuf[4:4])
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DTimestampTZ:
		// Start at offset 4 because `putInt32` clobbers the first 4 bytes.
		s := formatTs(v.Time, sessionLoc, b.putbuf[4:4])
		b.putInt32(int32(len(s)))
		b.write(s)

	case *pgwire.DInterval:
		b.writeLengthPrefixedString(v.Duration.String())
	case *pgwire.DJSON:
		b.writeLengthPrefixedString(v.JSON.String())

	default:
		b.setError(errors.Errorf("unsupported type %T", d))
	}
}

func (b *writeBuffer) writeBinarypgwire(
	ctx context.Context, d pgwire.Datum, sessionLoc *time.Location,
) {
	log.Debugf("pgwire writing BINARY pgwire of type: %T, %#v", d, d)
	if d == pgwire.DNull {
		// NULL is encoded as -1; all other values have a length prefix.
		b.putInt32(-1)
		return
	}
	switch v := d.(type) {
	case *pgwire.DBool:
		b.putInt32(1)
		if *v {
			b.writeByte(1)
		} else {
			b.writeByte(0)
		}

	case *pgwire.DInt:
		b.putInt32(8)
		b.putInt64(int64(*v))

	case *pgwire.DFloat:
		b.putInt32(8)
		b.putInt64(int64(math.Float64bits(float64(*v))))

	case *pgwire.DDecimal:
		alloc := struct {
			pgNum pgNumeric

			bigI big.Int
		}{
			pgNum: pgNumeric{
				// Since we use 2000 as the exponent limits in pgwire.DecimalCtx, this
				// conversion should not overflow.
				dscale: int16(-v.Exponent),
			},
		}

		if v.Sign() >= 0 {
			alloc.pgNum.sign = pgNumericPos
		} else {
			alloc.pgNum.sign = pgNumericNeg
		}

		isZero := func(r rune) bool {
			return r == '0'
		}

		// Mostly cribbed from libpqtypes' str2num.
		digits := strings.TrimLeftFunc(alloc.bigI.Abs(&v.Coeff).String(), isZero)
		dweight := len(digits) - int(alloc.pgNum.dscale) - 1
		digits = strings.TrimRightFunc(digits, isZero)

		if dweight >= 0 {
			alloc.pgNum.weight = int16((dweight+1+pgDecDigits-1)/pgDecDigits - 1)
		} else {
			alloc.pgNum.weight = int16(-((-dweight-1)/pgDecDigits + 1))
		}
		offset := (int(alloc.pgNum.weight)+1)*pgDecDigits - (dweight + 1)
		alloc.pgNum.ndigits = int16((len(digits) + offset + pgDecDigits - 1) / pgDecDigits)

		if len(digits) == 0 {
			offset = 0
			alloc.pgNum.ndigits = 0
			alloc.pgNum.weight = 0
		}

		digitIdx := -offset

		nextDigit := func() int16 {
			var ndigit int16
			for nextDigitIdx := digitIdx + pgDecDigits; digitIdx < nextDigitIdx; digitIdx++ {
				ndigit *= 10
				if digitIdx >= 0 && digitIdx < len(digits) {
					ndigit += int16(digits[digitIdx] - '0')
				}
			}
			return ndigit
		}

		b.putInt32(int32(2 * (4 + alloc.pgNum.ndigits)))
		b.putInt16(alloc.pgNum.ndigits)
		b.putInt16(alloc.pgNum.weight)
		b.putInt16(int16(alloc.pgNum.sign))
		b.putInt16(alloc.pgNum.dscale)

		for digitIdx < len(digits) {
			b.putInt16(nextDigit())
		}

	case *pgwire.DBytes:
		b.putInt32(int32(len(*v)))
		b.write([]byte(*v))

	case *pgwire.DString:
		b.writeLengthPrefixedString(string(*v))

	case *pgwire.DCollatedString:
		b.writeLengthPrefixedString(v.Contents)

	case *pgwire.DTimestamp:
		b.putInt32(8)
		b.putInt64(timeToPgBinary(v.Time, nil))

	case *pgwire.DTimestampTZ:
		b.putInt32(8)
		b.putInt64(timeToPgBinary(v.Time, sessionLoc))

	case *pgwire.DDate:
		b.putInt32(4)
		b.putInt32(dateToPgBinary(v))

	case *pgwire.DTime:
		b.putInt32(8)
		b.putInt64(int64(*v))

	default:
		b.setError(errors.Errorf("unsupported type %T", d))
	}
}

const pgTimeFormat = "15:04:05.999999"
const pgTimeStampFormatNoOffset = "2006-01-02 " + pgTimeFormat
const pgTimeStampFormat = pgTimeStampFormatNoOffset + "-07:00"

// formatTime formats t into a format lib/pq understands, appending to the
// provided tmp buffer and reallocating if needed. The function will then return
// the resulting buffer.
func formatTime(t timeofday.TimeOfDay, tmp []byte) []byte {
	return t.ToTime().AppendFormat(tmp, pgTimeFormat)
}

// formatTs formats t with an optional offset into a format lib/pq understands,
// appending to the provided tmp buffer and reallocating if needed. The function
// will then return the resulting buffer. formatTs is mostly cribbed from
// github.com/lib/pq.
func formatTs(t time.Time, offset *time.Location, tmp []byte) (b []byte) {
	// Need to send dates before 0001 A.D. with " BC" suffix, instead of the
	// minus sign preferred by Go.
	// Beware, "0000" in ISO is "1 BC", "-0001" is "2 BC" and so on
	if offset != nil {
		t = t.In(offset)
	}

	bc := false
	if t.Year() <= 0 {
		// flip year sign, and add 1, e.g: "0" will be "1", and "-10" will be "11"
		t = t.AddDate((-t.Year())*2+1, 0, 0)
		bc = true
	}

	if offset != nil {
		b = t.AppendFormat(tmp, pgTimeStampFormat)
	} else {
		b = t.AppendFormat(tmp, pgTimeStampFormatNoOffset)
	}

	if bc {
		b = append(b, " BC"...)
	}
	return b
}

var (
	pgEpochJDate         = time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	pgEpochJDateFromUnix = int32(pgEpochJDate.Unix() / secondsInDay)
)

// timeToPgBinary calculates the Postgres binary format for a timestamp. The timestamp
// is represented as the number of microseconds between the given time and Jan 1, 2000
// (dubbed the pgEpochJDate), stored within an int64.
func timeToPgBinary(t time.Time, offset *time.Location) int64 {
	if offset != nil {
		t = t.In(offset)
	} else {
		t = t.UTC()
	}
	return duration.DiffMicros(t, pgEpochJDate)
}

// dateToPgBinary calculates the Postgres binary format for a date. The date is
// represented as the number of days between the given date and Jan 1, 2000
// (dubbed the pgEpochJDate), stored within an int32.
func dateToPgBinary(d *pgwire.DDate) int32 {
	return int32(*d) - pgEpochJDateFromUnix
}

const (
	// pgBinaryIPv4family is the pgwire constant for IPv4. It is defined as
	// AF_INET.
	pgBinaryIPv4family byte = 2
	// pgBinaryIPv6family is the pgwire constant for IPv4. It is defined as
	// AF_NET + 1.
	pgBinaryIPv6family byte = 3
)
