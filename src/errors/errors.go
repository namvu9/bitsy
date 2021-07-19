package errors

import (
	"errors"
	"fmt"
	"strings"
)

type Op string

func (op Op) String() string {
	return string(op)
}

type Kind int

const (
	Internal    Kind = 1
	IO               = 2
	Network          = 3
	BadArgument      = 4
)

func (k Kind) String() string {
	switch k {
	case IO:
		return "IO Error"
	case Network:
		return "Network Error"
	case BadArgument:
		return "Bad arguments"
	default:
		return "Internal Error"
	}
}

type Error struct {
	err  error
	op   Op
	kind Kind
}

func (e Error) Error() string {
	return e.err.Error()
}

type Errors []error

func (errs Errors) Error() string {
	var sb strings.Builder

	for i, err := range errs {
		sb.WriteString(err.Error())

		if i < len(errs)-1 {
			sb.WriteString(", ")
		}
	}

	return sb.String()
}

func Ops(e error) []string {
	var out []string

	err, ok := e.(Error)
	if !ok {
		return out
	}

	out = append(out, string(err.op))
	out = append(out, Ops(err.err)...)

	return out
}

func Wrap(e error, args ...interface{}) error {
	err := Error{err: e, kind: Internal}

	if _err, ok := e.(Error); ok {
		err.kind = _err.kind
	}

	for _, arg := range args {
		switch v := arg.(type) {
		case Kind:
			err.kind = v
		case Op:
			err.op = v
		}
	}

	return err
}

func New(e string) error {
	return Error{err: errors.New(e)}
}

func Newf(fmtStr string, args ...interface{}) error {
	return fmt.Errorf(fmtStr, args...)
}
