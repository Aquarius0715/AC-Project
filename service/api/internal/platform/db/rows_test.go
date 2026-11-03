package db_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TestRowLoopsCheckErr: pgx reports a query's server error (a timeout, a deadlock, a failed cast) only through
// Rows.Err once Next returns false, so a manual row loop that does not check it takes a failed read for a short
// result (IR484). Every `for rows.Next()` of the service code must be followed by a rows.Err() check in its function;
// pgx.CollectRows and pgx.ForEachRow check it themselves.
func TestRowLoopsCheckErr(t *testing.T) {
	t.Parallel()

	fset := token.NewFileSet()
	loops := 0

	for _, dir := range []string{filepath.Join("..", ".."), filepath.Join("..", "..", "..", "cmd")} {
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") {
				return err
			}

			found, err := checkRowLoops(t, fset, path)
			loops += found

			return err
		})
		if err != nil {
			t.Fatal(err)
		}
	}

	if loops == 0 {
		t.Fatal("no row loops found: the walk missed the service code")
	}
}

// checkRowLoops reports each row loop of the file without an Err check after it and returns how many loops it saw.
func checkRowLoops(t *testing.T, fset *token.FileSet, path string) (int, error) {
	t.Helper()

	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		return 0, apperr.Wrap(err, "parser.ParseFile")
	}

	loops := 0

	ast.Inspect(file, func(n ast.Node) bool {
		var body *ast.BlockStmt

		switch fn := n.(type) {
		case *ast.FuncDecl:
			body = fn.Body
		case *ast.FuncLit:
			body = fn.Body
		}

		if body == nil {
			return true
		}

		for _, loop := range rowLoops(body) {
			loops++

			if !errChecked(body, loop.rows, loop.stmt.End()) {
				t.Errorf(
					"%s: the loop over %s.Next() has no %s.Err() check after it",
					fset.Position(loop.stmt.Pos()),
					loop.rows,
					loop.rows,
				)
			}
		}

		return true
	})

	return loops, nil
}

// rowLoop is a `for x.Next() { … }` loop and the name of its rows variable x.
type rowLoop struct {
	stmt *ast.ForStmt
	rows string
}

// rowLoops returns the `for x.Next() { … }` loops of a function body, leaving out its nested function literals
// (they are checked as functions of their own).
func rowLoops(body *ast.BlockStmt) []rowLoop {
	var out []rowLoop

	ast.Inspect(body, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.FuncLit:
			return false
		case *ast.ForStmt:
			if c, ok := node.Cond.(*ast.CallExpr); ok && len(c.Args) == 0 {
				if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Next" {
					if id, ok := s.X.(*ast.Ident); ok {
						out = append(out, rowLoop{stmt: node, rows: id.Name})
					}
				}
			}
		}

		return true
	})

	return out
}

// errChecked reports whether the body calls name.Err() after pos.
func errChecked(body *ast.BlockStmt, name string, after token.Pos) bool {
	found := false

	ast.Inspect(body, func(n ast.Node) bool {
		if c, ok := n.(*ast.CallExpr); ok && c.Pos() > after {
			if s, ok := c.Fun.(*ast.SelectorExpr); ok && s.Sel.Name == "Err" {
				if id, ok := s.X.(*ast.Ident); ok && id.Name == name {
					found = true
				}
			}
		}

		return !found
	})

	return found
}
