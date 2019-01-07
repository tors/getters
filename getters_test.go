package main

import (
	"go/parser"
	"go/token"
	"testing"

	"github.com/google/go-cmp/cmp"
)

func TestTmpl(t *testing.T) {
	suffix := "_gen.go"

	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, "./testpkg", sourceFilter(suffix), 0)
	if err != nil {
		t.Errorf("parser.ParseDir returned an error %v", err)
	}

	for pkgName, pkg := range pkgs {
		tmpl := &Tmpl{
			filename: pkgName + suffix,
			Package:  pkgName,
			Imports:  make(map[string]string),
			ignoreStructs: map[string]bool{
				"Pony": true,
			},
			ignoreMethods: map[string]bool{
				"Narwhal.GetAge": true,
			},
		}

		for _, f := range pkg.Files {
			if err := tmpl.processAST(f); err != nil {
				t.Errorf("processAST returned an error %v", err)
			}
		}

		getters := []*getter{
			newGetter("Narwhal", "Name", "string", `""`, false),
			newGetter("Narwhal", "Horn", "Horn", "nil", true),
			newGetter("Horn", "Length", "float64", "0", false),
			newGetter("Horn", "Color", "string", `""`, false),
		}

		if diff := cmp.Diff(tmpl.Getters, getters); diff != "" {
			t.Errorf("Tmpl.Getters (+got -want)\n%s", diff)
		}

		if tmpl.filename != tmpl.Package+suffix {
			t.Errorf("Tmpl.filename got %v, want %v", tmpl.filename, tmpl.Package+suffix)
		}
	}
}

func TestGetConfigFile(t *testing.T) {
	conf := getConfigFile([]string{"testpkg/.getters.yml"})

	want := &ConfigFile{
		Suffix: "_gen.go",
		Ignore: &ignore{
			Structs:  []string{"Pony"},
			Methods:  []string{"Narwhal.GetAge"},
			Packages: []string{"main"},
		},
	}

	if diff := cmp.Diff(conf, want); diff != "" {
		t.Errorf("ConfigFile (+got -want)\n%s", diff)
	}
}
