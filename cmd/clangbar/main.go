package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"play/clangbar/proginfo"
	"strings"

	"github.com/go-clang/clang-v3.9/clang"
	"github.com/mewkiz/pkg/jsonutil"
	"github.com/mewkiz/pkg/pathutil"
	"github.com/mewkiz/pkg/term"
	"github.com/mewspring/cc"
	"github.com/pkg/errors"
)

var (
	// dbg is a logger with the "clangbar:" prefix which logs debug messages to
	// standard error.
	dbg = log.New(os.Stderr, term.MagentaBold("clangbar:")+" ", 0)
	// warn is a logger with the "clangbar:" prefix which logs warning messages
	// to standard error.
	warn = log.New(os.Stderr, term.RedBold("clangbar:")+" ", 0)
)

const (
	outputDir = "_dump_"
)

func main() {
	flag.Parse()
	for _, srcPath := range flag.Args() {
		// Parse source file into AST.
		//dbg.Printf("=== [ %v ] ===\n", srcPath)
		dbg.Printf("parsing %q\n", srcPath)
		file, err := cc.ParseFile(srcPath, "-D DEVILUTION_STUB", "-D DEVILUTION_ENGINE", "-I./SourceS", "-I/usr/lib/clang/8.0.1/include")
		if err != nil {
			warn.Printf("%+v", err)
			// continue with partial AST.
		}
		defer file.Close()
		funcUses := analyze(file.Root)
		funcUses = filter(funcUses, srcPath)
		//for _, funcUse := range funcUses {
		//	dbg.Printf("uses in function %v (at %q)\n", funcUse.Func.Body.Spelling(), funcUse.Func.Loc)
		//	pretty.Println(funcUse.Uses)
		//}
		srcName := pathutil.FileName(srcPath)
		jsonName := fmt.Sprintf("%s.json", srcName)
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			log.Fatalf("%+v", errors.WithStack(err))
		}
		jsonPath := filepath.Join(outputDir, jsonName)
		dbg.Printf("creating %q", jsonPath)
		if err := jsonutil.WriteFile(jsonPath, funcUses); err != nil {
			log.Fatalf("%+v", err)
		}
	}
}

func filter(funcUses []*proginfo.FuncUse, srcPath string) []*proginfo.FuncUse {
	var after []*proginfo.FuncUse
	for _, funcUse := range funcUses {
		if pathutil.TrimExt(funcUse.Func.Loc.File) != pathutil.TrimExt(srcPath) {
			// Skip functions not part of source file.
			continue
		}
		after = append(after, funcUse)
	}
	return after
}

func analyze(root *cc.Node) []*proginfo.FuncUse {
	var globals []*cc.Node
	cc.Walk(root, recordNodes(clang.Cursor_VarDecl, &globals))
	//dbg.Println("globals:")
	//for _, g := range globals {
	//	dbg.Printf("   %v (at %q)\n", g.Body.Spelling(), g.Loc)
	//}
	var funcs []*cc.Node
	cc.Walk(root, recordNodes(clang.Cursor_FunctionDecl, &funcs))
	//dbg.Println("functions:")
	//for _, f := range funcs {
	//	dbg.Printf("   %v (at %q)\n", f.Body.Spelling(), f.Loc)
	//}
	var funcUses []*proginfo.FuncUse
	for _, f := range funcs {
		if !isDef(f) {
			// Skip function declarations.
			//dbg.Printf("skipping function declaration %q", f.Body.Spelling())
			continue
		}
		uses := findUses(f)
		funcUse := &proginfo.FuncUse{
			Func:     f,
			FuncName: f.Body.Spelling(),
			FuncLoc:  f.Loc,
			Uses:     uses,
		}
		funcUses = append(funcUses, funcUse)
	}
	// map from global variable and function definition name to associated node.
	defs := make(map[string]*cc.Node)
	for _, global := range globals {
		name := global.Body.Spelling()
		if old, ok := defs[name]; ok {
			if isDef(old) {
				continue
			} else if isDefOrInSrc(global) {
				defs[name] = global
				continue
			}
			warn.Printf("global variable %q already present; old %v, new %v", name, old, global)
			continue
		}
		defs[name] = global
	}
	for _, fn := range funcs {
		name := fn.Body.Spelling()
		if old, ok := defs[name]; ok {
			if isDef(old) {
				continue
			} else if isDefOrInSrc(fn) {
				defs[name] = fn
				continue
			}
			warn.Printf("function %q already present; old %v, new %v", name, old, fn)
			continue
		}
		defs[name] = fn
	}
	for _, funcUse := range funcUses {
		resolveExternalDefs(funcUse, defs)
	}
	return funcUses
}

func isDefOrInSrc(new *cc.Node) bool {
	if isDef(new) {
		return true
	}
	newExt := filepath.Ext(new.Loc.File)
	switch newExt {
	case ".h", ".hpp":
		return false // header file.
	case ".c", ".cpp", ".cxx":
		return true // source file.
	default:
		if strings.Contains(new.Loc.File, "/include/") {
			return false // header file.
		}
		panic(fmt.Errorf("support for extension %q of file %q not yet implemented", newExt, new.Loc.File))
	}
}

// contains reports whether the given AST tree contains a node of the specified
// kind.
func contains(root *cc.Node, kind clang.CursorKind) bool {
	ret := false
	cc.Walk(root, func(n *cc.Node) {
		if n.Body.Kind() == kind {
			ret = true
		}
	})
	return ret
}

func resolveExternalDefs(funcUse *proginfo.FuncUse, defs map[string]*cc.Node) {
	zero := cc.Location{}
	for _, use := range funcUse.Uses {
		if use.DefLoc != zero {
			// Already has location of definition.
			continue
		}
		global, ok := defs[use.Name]
		if !ok {
			if !strings.HasPrefix(use.Name, "__builtin_") {
				warn.Printf("unable to resolve use of external global variable %q, as used in function %q (at %q)", use.Name, funcUse.Func.Body.Spelling(), funcUse.Func.Loc)
			}
			continue
		}
		use.DefLoc = global.Loc
	}
}

func findUses(f *cc.Node) []*proginfo.Use {
	//dbg.Printf("uses in function %v (at %q)\n", f.Body.Spelling(), f.Loc)
	var uses []*proginfo.Use
	cc.Walk(f, func(n *cc.Node) {
		if n.Body.Kind() != clang.Cursor_DeclRefExpr {
			// Early return if not identifier use expression.
			return
		}
		if isExternal(n.Body.Definition()) {
			//dbg.Printf("   external use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// (external) global use.
			use := &proginfo.Use{
				Name:   n.Body.Spelling(),
				UseLoc: n.Loc,
				// Note: external use has no definition, thus no DefLoc.
				//DefLoc: cc.NewLocation(n.Body.Definition().Location()),
			}
			uses = append(uses, use)
		} else if isGlobal(n.Body.Definition()) {
			//dbg.Printf("   global use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// global use.
			use := &proginfo.Use{
				Name:   n.Body.Spelling(),
				UseLoc: n.Loc,
				DefLoc: cc.NewLocation(n.Body.Definition().Location()),
			}
			uses = append(uses, use)
		} else {
			//dbg.Printf("   local use: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			// skip local use.
		}
	})
	return uses
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
		case clang.Cursor_UnexposedDecl, clang.Cursor_Namespace, clang.Cursor_StructDecl, clang.Cursor_ClassTemplate, clang.Cursor_ClassTemplatePartialSpecialization, clang.Cursor_ClassDecl, clang.Cursor_EnumDecl:
			// continue traversing parents.
		case clang.Cursor_TranslationUnit:
			// global
			return true
		case clang.Cursor_FunctionDecl, clang.Cursor_FunctionTemplate, clang.Cursor_Constructor, clang.Cursor_CXXMethod:
			// local variable.
			//dbg.Printf("local: %v (at %q)\n", n.Body.Spelling(), n.Loc)
			return false
		default:
			dbg.Println("n:", n.Spelling())
			panic(fmt.Sprintf("support for kind %v not yet implemented", parent.Kind().Spelling()))
		}
		parent = parent.SemanticParent()
	}
}

// isDef reports whether the given node is a defition.
func isDef(n *cc.Node) bool {
	switch n.Body.Kind() {
	case clang.Cursor_VarDecl:
		// TODO: verify if this may give false positives.
		return len(n.Children) > 0
	case clang.Cursor_FunctionDecl:
		return contains(n, clang.Cursor_CompoundStmt)
	default:
		panic(fmt.Errorf("support for node kind %v not yet implemented", n.Body.Kind().Spelling()))
	}
}
