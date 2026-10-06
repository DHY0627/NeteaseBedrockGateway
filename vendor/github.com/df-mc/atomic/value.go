// Copyright (c) 2020 Uber Technologies, Inc.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in
// all copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN
// THE SOFTWARE.

package atomic

import (
	"fmt"
	"sync/atomic"
)

// Value is a wrapper around atomic.Value with a generic API. Note that for basic types such as int, float and bool
// types, using atomic.(U)Int*, atomic.Float* and atomic.Bool is more efficient.
// https://godoc.org/sync/atomic#Value
type Value[T any] struct {
	atomic.Value

	_ nocmp // disallow non-atomic comparison
}

// wrapper is a wrapper struct around an arbitrary type T. This wrapper is required for atomic.Values that want to
// store an interface type, because these are "inconsistently typed".
type wrapper[T any] struct{ val T }

// wrap packs a value of type T into a wrapper.
func wrap[T any](val T) wrapper[T] {
	return wrapper[T]{val: val}
}

// unwrap removes the wrapper of a value and returns the value held.
func unwrap[T any](val any) T {
	w, _ := val.(wrapper[T])
	return w.val
}

// NewValue creates a Value[T] and assigns to it the value passed. NewValue returns a pointer to the Value[T] created.
func NewValue[T any](val T) *Value[T] {
	var v Value[T]
	v.Store(val)
	return &v
}

// Load returns the value set by the most recent Store.
// It returns nil if there has been no call to Store for this Value.
func (v *Value[T]) Load() (val T) {
	return unwrap[T](v.Value.Load())
}

// Store sets the value of the Value to x.
// All calls to Store for a given Value must use values of the same concrete type.
// Store of an inconsistent type panics, as does Store(nil).
func (v *Value[T]) Store(val T) {
	v.Value.Store(wrap(val))
}

// Swap stores new into Value and returns the previous value. It returns nil if
// the Value is empty.
//
// All calls to Swap for a given Value must use values of the same concrete
// type. Swap of an inconsistent type panics, as does Swap(nil).
func (v *Value[T]) Swap(new T) (old T) {
	return unwrap[T](v.Value.Swap(wrap(new)))
}

// CompareAndSwap executes the compare-and-swap operation for the Value.
//
// All calls to CompareAndSwap for a given Value must use values of the same
// concrete type. CompareAndSwap of an inconsistent type panics, as does
// CompareAndSwap(old, nil).
func (v *Value[T]) CompareAndSwap(old, new T) (swapped bool) {
	return v.Value.CompareAndSwap(wrap(old), wrap(new))
}

// String implements fmt.Stringer to return the standard value representation of the underlying value.
func (v *Value[T]) String() string {
	return fmt.Sprint(v.Load())
}

// GoString implements fmt.GoStringer to return a valid Go syntax representation of the underlying value.
func (v *Value[T]) GoString() string {
	return fmt.Sprintf("%#v", v.Load())
}
