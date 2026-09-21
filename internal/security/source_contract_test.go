// SPDX-License-Identifier: Apache-2.0

package security_test

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestProductionSourceKeepsSecretHandlingOffShellAndLogs(t *testing.T) {
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate source-contract test")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(filename), "..", ".."))

	forbiddenImports := map[string]string{
		"os/exec":  "shell/process execution",
		"log":      "standard logging",
		"log/slog": "structured standard logging",
	}

	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if entry.Name() == ".git" || entry.Name() == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if parseErr != nil {
			t.Errorf("parse %s: %v", path, parseErr)
			return nil
		}

		for _, spec := range file.Imports {
			value, unquoteErr := strconv.Unquote(spec.Path.Value)
			if unquoteErr != nil {
				continue
			}
			if reason, forbidden := forbiddenImports[value]; forbidden {
				t.Errorf("%s imports %q (%s); secret-bearing paths must not use it", path, value, reason)
			}
		}

		ast.Inspect(file, func(node ast.Node) bool {
			switch value := node.(type) {
			case *ast.SelectorExpr:
				if value.Sel.Name == "GetSecrets" {
					t.Errorf("%s references NetworkManager GetSecrets", path)
				}
			case *ast.BasicLit:
				if value.Kind != token.STRING {
					break
				}
				literal, unquoteErr := strconv.Unquote(value.Value)
				if unquoteErr == nil && strings.Contains(literal, "GetSecrets") {
					t.Errorf("%s contains a GetSecrets D-Bus method literal", path)
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walk production source: %v", err)
	}
}
