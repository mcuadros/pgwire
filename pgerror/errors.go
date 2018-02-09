// Copyright 2016 The Cockroach Authors.
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

package pgerror

import (
	"fmt"

	"github.com/pkg/errors"
)

type Error struct {
	Code       string
	Message    string
	Detail     string
	Hint       string
	StackTrace *StackTrace
}

func (pg *Error) Error() string {
	return pg.Message
}

// NewErrorWithDepthf creates an Error and extracts the context
// information at the specified depth level.
func NewErrorWithDepthf(depth int, code string, format string, args ...interface{}) *Error {
	st := NewStackTrace(depth + 1)
	return &Error{
		Message:    fmt.Sprintf(format, args...),
		Code:       code,
		StackTrace: &st,
	}
}

// NewError creates an Error.
func NewError(code string, msg string) *Error {
	return NewErrorWithDepthf(1, code, "%s", msg)
}

// NewErrorWithDepth creates an Error with context extracted from the
// specified depth.
func NewErrorWithDepth(depth int, code string, msg string) *Error {
	return NewErrorWithDepthf(depth+1, code, "%s", msg)
}

// NewErrorf creates an Error with a format string.
func NewErrorf(code string, format string, args ...interface{}) *Error {
	return NewErrorWithDepthf(1, code, format, args...)
}

// SetHintf annotates an Error object with a hint.
func (pg *Error) SetHintf(f string, args ...interface{}) *Error {
	pg.Hint = fmt.Sprintf(f, args...)
	return pg
}

// SetDetailf annotates an Error object with details.
func (pg *Error) SetDetailf(f string, args ...interface{}) *Error {
	pg.Detail = fmt.Sprintf(f, args...)
	return pg
}

// GetPGCause returns an unwrapped Error.
func GetPGCause(err error) (*Error, bool) {
	switch pgErr := errors.Cause(err).(type) {
	case *Error:
		return pgErr, true

	default:
		return nil, false
	}
}
