package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/kr/pretty"
	"github.com/mewkiz/pkg/term"
	"github.com/mewspring/cc"
	"github.com/mewspring/go-clang/clang"
)

var (
	// dbg is a logger with the "clangbar:" prefix which logs debug messages to
	// standard error.
	dbg = log.New(os.Stderr, term.MagentaBold("clangbar:")+" ", 0)
	// warn is a logger with the "clangbar:" prefix which logs warning messages
	// to standard error.
	warn = log.New(os.Stderr, term.RedBold("clangbar:")+" ", 0)
)

func main() {
	flag.Parse()
	for _, srcPath := range flag.Args() {
		// Parse source file into AST.
		dbg.Printf("=== [ %v ] ===\n", srcPath)
		root, err := cc.ParseFile(srcPath, "-D DEVILUTION_STUB", "-D DEVILUTION_ENGINE", "-I./SourceS", "-I/usr/lib/clang/8.0.1/include")
		if err != nil {
			warn.Printf("%+v", err)
		}
		analyze(root)
	}
}

func analyze(root *cc.Node) {
	var globals []*cc.Node
	cc.Walk(root, recordNodes(clang.Cursor_VarDecl, &globals))
	fmt.Println("globals:")
	for _, g := range globals {
		pretty.Printf("   %v (at %q)\n", g.Body.Spelling(), g.Loc)
	}
	var funcs []*cc.Node
	cc.Walk(root, recordNodes(clang.Cursor_FunctionDecl, &funcs))
	fmt.Println("functions:")
	for _, f := range funcs {
		pretty.Printf("   %v (at %q)\n", f.Body.Spelling(), f.Loc)
	}
	var funcUses []*FuncUse
	for _, f := range funcs {
		uses := findUses(f)
		funcUse := &FuncUse{
			fn:   f,
			uses: uses,
		}
		funcUses = append(funcUses, funcUse)
		fmt.Printf("uses in function %v (at %q)\n", f.Body.Spelling(), f.Loc)
		pretty.Println(uses)
	}
	globalDefs := make(map[string]*cc.Node)
	for _, global := range globals {
		name := global.Body.Spelling()
		if old, ok := globalDefs[name]; ok {
			panic(fmt.Errorf("global variable %q already present; old %v, new %v", name, old, global))
		}
		globalDefs[name] = global
	}
	for _, funcUse := range funcUses {
		resolveExternalDefs(funcUse, globalDefs)
	}
	pretty.Println(funcUses)
}

func resolveExternalDefs(funcUse *FuncUse, globalDefs map[string]*cc.Node) {
	zero := cc.Location{}
	for _, use := range funcUse.uses {
		if use.DefLoc != zero {
			// Already has location of definition.
			continue
		}
		global, ok := globalDefs[use.Name]
		if !ok {
			warn.Printf("unable to resolve use of external global variable %q, as used in function %q (at %q)", use.Name, funcUse.fn.Body.Spelling(), funcUse.fn.Loc)
		}
		use.DefLoc = global.Loc
	}
}

type FuncUse struct {
	fn   *cc.Node
	uses []*Use
}

func findUses(f *cc.Node) []*Use {
	fmt.Printf("uses in function %v (at %q)\n", f.Body.Spelling(), f.Loc)
	var uses []*Use
	cc.Walk(f, func(n *cc.Node) {
		if n.Body.Kind() != clang.Cursor_DeclRefExpr {
			// Early return if not identifier use expression.
			return
		}
		if isExternal(n.Body.Definition()) {
			fmt.Printf("   external use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// (external) global use.
			use := &Use{
				Name:   n.Body.Spelling(),
				UseLoc: n.Loc,
				// Note: external use has no definition, thus no DefLoc.
				//DefLoc: cc.NewLocation(n.Body.Definition().Location()),
			}
			uses = append(uses, use)
		} else if isGlobal(n.Body.Definition()) {
			fmt.Printf("   global use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// global use.
			use := &Use{
				Name:   n.Body.Spelling(),
				UseLoc: n.Loc,
				DefLoc: cc.NewLocation(n.Body.Definition().Location()),
			}
			uses = append(uses, use)
		} else {
			fmt.Printf("   local use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// skip local use.
		}
	})
	return uses
}

// Use is a use of an identifier.
type Use struct {
	// Identifier being used.
	Name string
	// Use location.
	UseLoc cc.Location
	// Definition location (or zero if external).
	DefLoc cc.Location
}

func recordNodes(recordKind clang.CursorKind, out *[]*cc.Node) func(*cc.Node) {
	visit := func(n *cc.Node) {
		if n.Body.Kind() != recordKind {
			return
		}
		if isGlobal(n.Body) {
			*out = append(*out, n)
		}
	}
	return visit
}

func isExternal(n clang.Cursor) bool {
	zero := cc.Location{}
	if cc.NewLocation(n.Location()) == zero {
		// Definition not found, thus external (i.e. global).
		return true
	}
	return false
}

func isGlobal(n clang.Cursor) bool {
	// Check scope to see if the identifier is local or global.
	parent := n.SemanticParent()
	for {
		switch parent.Kind() {
		case clang.Cursor_UnexposedDecl, clang.Cursor_Namespace, clang.Cursor_StructDecl, clang.Cursor_ClassTemplate, clang.Cursor_ClassTemplatePartialSpecialization, clang.Cursor_ClassDecl:
			// continue traversing parents.
		case clang.Cursor_TranslationUnit:
			// global
			return true
		case clang.Cursor_FunctionDecl, clang.Cursor_FunctionTemplate, clang.Cursor_Constructor, clang.Cursor_CXXMethod:
			// local variable.
			//fmt.Printf("local: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			return false
		default:
			fmt.Println("n:", n.Spelling())
			panic(fmt.Sprintf("support for kind %v not yet implemented", parent.Kind().Spelling()))
		}
		parent = parent.SemanticParent()
	}
}
