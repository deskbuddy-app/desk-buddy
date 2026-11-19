//go:build ignore

// Read-only guard for pre-existing Go branch conditions and case/action identifiers.
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"os"
	"os/exec"
	"strings"
)

func guards(src []byte) []string {
	fs := token.NewFileSet()
	f, e := parser.ParseFile(fs, "source.go", src, 0)
	if e != nil {
		panic(e)
	}
	var result []string
	ast.Inspect(f, func(n ast.Node) bool {
		var nodes []ast.Node
		switch x := n.(type) {
		case *ast.IfStmt:
			nodes = []ast.Node{x.Cond}
		case *ast.ForStmt:
			if x.Cond != nil {
				nodes = []ast.Node{x.Cond}
			}
		case *ast.SwitchStmt:
			if x.Tag != nil {
				nodes = []ast.Node{x.Tag}
			}
		case *ast.KeyValueExpr:
			if lit, ok := x.Key.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				nodes = []ast.Node{lit}
			}
		case *ast.CaseClause:
			for _, v := range x.List {
				nodes = append(nodes, v)
			}
		}
		for _, x := range nodes {
			var b bytes.Buffer
			printer.Fprint(&b, fs, x)
			result = append(result, fmt.Sprintf("%T:%s", n, b.String()))
		}
		return true
	})
	return result
}
func main() {
	cmd := exec.Command("git", "diff", "--name-only", "--diff-filter=MDT", "7245f1a344efd0584246b96f4d0023ce2666e2c3")
	files, e := cmd.Output()
	if e != nil {
		panic(e)
	}
	type check struct {
		File, Status string
		Guards       int
	}
	var checks []check
	failed := false
	for _, name := range strings.Fields(string(files)) {
		if !strings.HasPrefix(name, "kindle/desk/") || !strings.HasSuffix(name, ".go") {
			continue
		}
		if name == "kindle/desk/gfx.go" {
			continue
		}
		old, e := exec.Command("git", "show", "7245f1a344efd0584246b96f4d0023ce2666e2c3:"+name).Output()
		if e != nil {
			panic(e)
		}
		now, e := os.ReadFile(name)
		if e != nil {
			panic(e)
		}
		a, b := guards(old), guards(now)
		aj, _ := json.Marshal(a)
		bj, _ := json.Marshal(b)
		status := "PASS"
		if !bytes.Equal(aj, bj) {
			status = "FAIL"
			failed = true
		}
		checks = append(checks, check{name, status, len(a)})
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	enc.Encode(checks)
	if failed {
		os.Exit(1)
	}
}
