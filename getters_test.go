package main

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTmpl(t *testing.T) {
	ignore := &ignoreyml{
		Suffix:  "_gen_getters.go",
		Structs: []string{"Kitten"},
		Methods: []string{"Pony.GetWeight"},
	}

	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, "./test", sourceFilter(ignore.Suffix), 0)
	if err != nil {
		t.Errorf("parser.ParseDir returned an error %v", err)
	}

	for pkgName, pkg := range pkgs {
		tmpl := &Tmpl{
			filename: pkgName + ignore.Suffix,
			Package:  pkgName,
			Imports:  make(map[string]string),
		}

		for _, f := range pkg.Files {
			if err := tmpl.processAST(f); err != nil {
				t.Errorf("processAST returned an error %v", err)
			}
		}

		getters := []*getter{
			newGetter("Pony", "Name", "string", `""`, false),
			newGetter("Pony", "Age", "int64", "0", false),
			newGetter("Pony", "Weight", "float64", "0.0", false),
			newGetter("Kitten", "Name", "string", `""`, false),
		}

		if !cmp.Equal(tmpl.Getters, getters) {
			t.Errorf("Tmpl.Getters got %v, want %v", tmpl.Getters, getters)
		}

		if tmpl.filename != tmpl.Package+ignore.Suffix {
			t.Errorf("Tmpl.filename got %v, want %v", tmpl.filename, tmpl.Package+ignore.Suffix)
		}
	}
}

func TestGetYMLConfig(t *testing.T) {
	ignore := getYMLConfig([]string{"test/.getters.yml"})

	want := &ignoreyml{
		Suffix:   "_gen.go",
		Structs:  []string{"Kitten"},
		Methods:  []string{"Pony.GetWeight"},
		Packages: []string{"main"},
	}

	if !cmp.Equal(ignore, want) {
		t.Errorf("ignoreyml returned %v, want %v", ignore, want)
	}
}
