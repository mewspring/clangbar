// Package proginfo records information about a source program.
package proginfo

import (
	"github.com/mewspring/cc"
)

// Use is a use of an identifier.
type Use struct {
	// Identifier being used.
	Name string
	// Use location.
	UseLoc cc.Location
	// Definition location (or zero if external).
	DefLoc cc.Location
}

// FuncUse records identifier uses within a given function.
type FuncUse struct {
	// Function AST node.
	Func *cc.Node `json:"-"`
	// Function name.
	FuncName string
	// Function definition location.
	FuncLoc cc.Location
	// Uses within the function.
	Uses []*Use
}
