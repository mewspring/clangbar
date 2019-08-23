package main

import (
	"flag"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"play/clangbar/proginfo"
	"sort"
	"strings"

	"github.com/kr/pretty"
	"github.com/mewkiz/pkg/jsonutil"
	"github.com/mewkiz/pkg/pathutil"
	"github.com/mewkiz/pkg/term"
	"github.com/mewspring/cc"
	"github.com/pkg/errors"
)

var (
	// dbg is a logger with the "clangviz:" prefix which logs debug messages to
	// standard error.
	dbg = log.New(os.Stderr, term.MagentaBold("clangviz:")+" ", 0)
	// warn is a logger with the "clangviz:" prefix which logs warning messages
	// to standard error.
	warn = log.New(os.Stderr, term.RedBold("clangviz:")+" ", 0)
)

func main() {
	flag.Parse()
	for _, jsonPath := range flag.Args() {
		if err := visualize(jsonPath); err != nil {
			log.Fatalf("%+v", err)
		}
	}
}

func visualize(jsonPath string) error {
	var funcUses []*proginfo.FuncUse
	dbg.Printf("parsing %q", jsonPath)
	if err := jsonutil.ParseFile(jsonPath, &funcUses); err != nil {
		return errors.WithStack(err)
	}
	graph := genFileInteractionGraph(funcUses)
	dotPath := pathutil.TrimExt(jsonPath) + ".dot"
	dbg.Printf("creating %q", dotPath)
	if err := ioutil.WriteFile(dotPath, []byte(graph), 0644); err != nil {
		return errors.WithStack(err)
	}
	return nil
}

func genFileInteractionGraph(funcUses []*proginfo.FuncUse) string {
	buf := &strings.Builder{}
	buf.WriteString("digraph {\n")
	es := make(map[Edge]bool)
	for _, funcUse := range funcUses {
		funcFileName := pathutil.FileName(funcUse.FuncLoc.File)
		if !strings.HasPrefix(funcUse.FuncLoc.File, "Source/") {
			// Use full filepath if file is not within the main Source directory.
			funcFileName = funcUse.FuncLoc.File
		}
		for _, use := range funcUse.Uses {
			if strings.HasPrefix(use.DefLoc.File, "/usr/include/") || strings.Contains(use.DefLoc.File, "/lib64/gcc/") {
				// Skip standard includes.
				continue
			}
			if strings.HasPrefix(use.Name, "__builtin_") || strings.HasPrefix(use.Name, "__sync_") {
				// Skip builtin identifiers.
				continue
			}
			zero := cc.Location{}
			if use.DefLoc == zero {
				pretty.Println(use)
				panic("builtin identifier?")
			}
			useFileName := pathutil.FileName(use.DefLoc.File)
			if !strings.HasPrefix(use.DefLoc.File, "Source/") {
				// Use full filepath if file is not within the main Source
				// directory.
				useFileName = use.DefLoc.File
			}
			edge := Edge{
				From: funcFileName,
				To:   useFileName,
			}
			es[edge] = true
		}
	}
	var edges []Edge
	for edge := range es {
		edges = append(edges, edge)
	}
	sort.Slice(edges, func(i, j int) bool {
		switch {
		case edges[i].From < edges[j].From:
			return false
		case edges[i].From > edges[j].From:
			return true
		default:
			// edges[i].From == edges[j].From
			return edges[i].To < edges[j].To
		}
	})
	for _, edge := range edges {
		fmt.Fprintf(buf, "\t%q -> %q\n", edge.From, edge.To)
	}
	buf.WriteString("}\n")
	return buf.String()
}

type Edge struct {
	From string
	To   string
}
