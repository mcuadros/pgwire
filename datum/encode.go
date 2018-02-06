package datum

import (
	"bytes"
	"unicode/utf8"

	"github.com/cockroachdb/cockroach/pkg/util/stringencoding"
)

// EncodeSQLStringWithFlags writes a string literal to buf. All
// unicode and non-printable characters are escaped. flags controls
// the output format: if encodeBareString is set, the output string
// will not be wrapped in quotes if the strings contains no special
// characters.
func EncodeSQLString(buf *bytes.Buffer, in string) {
	// See http://www.postgresql.org/docs/9.4/static/sql-syntax-lexical.html
	start := 0
	escapedString := false
	// Loop through each unicode code point.
	for i, r := range in {
		ch := byte(r)
		if r >= 0x20 && r < 0x7F {
			if !stringencoding.NeedEscape(ch) && ch != '\'' {
				continue
			}
		}

		if !escapedString {
			buf.WriteString("e'") // begin e'xxx' string
			escapedString = true
		}
		buf.WriteString(in[start:i])
		ln := utf8.RuneLen(r)
		if ln < 0 {
			start = i + 1
		} else {
			start = i + ln
		}
		stringencoding.EncodeEscapedChar(buf, in, r, ch, i, '\'')
	}

	if !escapedString {
		buf.WriteByte('\'') // begin 'xxx' string if nothing was escaped
	}
	buf.WriteString(in[start:])
	if !escapedString {
		buf.WriteByte('\'')
	}
}
