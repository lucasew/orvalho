package hostobject

import "errors"

var (
	ErrNil                = errors.New("hostobject: nil object")
	ErrNoRuntime          = errors.New("hostobject: nil runtime")
	ErrEmptyName          = errors.New("hostobject: empty property name")
	ErrNameClash          = errors.New("hostobject: property clash")
	ErrMethodsNil         = errors.New("hostobject: Methods needs a non-nil pointer")
	ErrMethodsNeedPointer = errors.New("hostobject: Methods needs a pointer")
)
