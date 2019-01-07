package main

import (
	"bytes"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/parser"
	"go/token"
	"io/ioutil"
	"log"
	"os"
	"sort"
	"strings"
	"text/template"

	yaml "gopkg.in/yaml.v2"
)

var (
	// Flags
	verbose  bool
	suffix   string
	structs  Options
	methods  Options
	packages Options

	// Defaults
	defaultConfigFile = ".getters.yml"
	defaultSuffix     = "_getters.go"

	sourceTmpl = template.Must(template.New("source").Parse(source))
)

func init() {
	flag.StringVar(&suffix, "suffix", "", fmt.Sprintf("suffix file name (default: %v)", defaultSuffix))
	flag.Var(&structs, "s", "ignore structs")
	flag.Var(&methods, "m", "ignore struct methods")
	flag.Var(&packages, "p", "ignore packages")
	flag.BoolVar(&verbose, "v", false, "verbose mode (default: false)")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "usage: getters [options] [filename]\n")
		flag.PrintDefaults()
	}
}

func main() {
	flag.Parse()

	args := flag.Args()
	configFile := getConfigFile(args)

	if configFile != nil {
		// If config file exists and if some flag options are empty, fill the
		// flag options using the config file values.
		if structs.isEmpty() {
			structs.setStrings(configFile.structs())
		}
		if methods.isEmpty() {
			methods.setStrings(configFile.methods())
		}
		if packages.isEmpty() {
			packages.setStrings(configFile.packages())
		}
	}

	suffix = getSuffix(suffix, configFile)
	fset := token.NewFileSet()

	pkgs, err := parser.ParseDir(fset, ".", sourceFilter(suffix), 0)
	if err != nil {
		log.Fatal(err)
	}

	for pkgName, pkg := range pkgs {
		if packages.index[pkgName] {
			continue
		}

		t := &Tmpl{
			filename:       pkgName + suffix,
			Package:        pkgName,
			Imports:        make(map[string]string),
			ignoreStructs:  structs.index,
			ignoreMethods:  methods.index,
			ignorePackages: packages.index,
		}

		for filename, f := range pkg.Files {
			logf("Processing %v...", filename)
			if err := t.processAST(f); err != nil {
				log.Fatal(err)
			}
		}

		logf("Dumping %v...", pkgName)
		if err := t.dump(); err != nil {
			log.Fatal(err)
		}
	}

	logf("Done.")
}

func sourceFilter(sf string) func(os.FileInfo) bool {
	return func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go") && !strings.HasSuffix(fi.Name(), sf)
	}
}

func getSuffix(sf string, ig *ConfigFile) string {
	if sf == "" && ig != nil && ig.Suffix != "" {
		return ig.Suffix
	}
	return defaultSuffix
}

func getConfigFile(args []string) *ConfigFile {
	var file *os.File
	var err error
	if len(args) >= 1 {
		// Find out if file exist and return configFile if it exists
		file, err = os.Open(args[0])
		if err != nil {
			logf("%v, error %v", args[0], err)
		}
	}
	if file == nil {
		// Attempt to get .getters.yml
		file, err = os.Open(defaultConfigFile)
		if err != nil && !os.IsNotExist(err) {
			logf("%v, error %v", defaultConfigFile, err)
			return nil // Abort mission
		}
	}
	// We've found the file, now time to read it.
	defer file.Close()
	var configFile ConfigFile
	data, err := ioutil.ReadAll(file)
	err = yaml.Unmarshal(data, &configFile)
	if err != nil {
		log.Fatalf("Unable to read file %v, error %v", file, err)
	}
	return &configFile
}

type Options struct {
	index map[string]bool
}

func (o *Options) String() string {
	var s []string
	for k, _ := range o.index {
		s = append(s, k)
	}
	return strings.Join(s, ",")
}

func (o *Options) Set(value string) error {
	return o.setStrings(strings.Split(value, ","))
}

func (o *Options) setStrings(v []string) error {
	for _, str := range v {
		o.index[str] = true
	}
	return nil
}

func (o *Options) isEmpty() bool {
	return len(o.index) == 0
}

type ConfigFile struct {
	Suffix string  `yaml:"suffix"`
	Ignore *ignore `yaml:"ignore"`
}

func (c *ConfigFile) structs() []string {
	if c == nil || c.Ignore == nil || c.Ignore.Structs == nil {
		return nil
	}
	return c.Ignore.Structs
}

func (c *ConfigFile) methods() []string {
	if c == nil || c.Ignore == nil || c.Ignore.Methods == nil {
		return nil
	}
	return c.Ignore.Methods
}

func (c *ConfigFile) packages() []string {
	if c == nil || c.Ignore == nil || c.Ignore.Packages == nil {
		return nil
	}
	return c.Ignore.Packages
}

type ignore struct {
	Structs  []string `yaml:"structs"`
	Methods  []string `yaml:"methods"`
	Packages []string `yaml:"packages"`
}

type Tmpl struct {
	filename       string
	Package        string
	Imports        map[string]string
	Getters        []*getter
	ignoreStructs  map[string]bool
	ignoreMethods  map[string]bool
	ignorePackages map[string]bool
}

type getter struct {
	SortVal      string // Lower-case version of "ReceiverType.FieldName".
	ReceiverVar  string // The one-letter variable name to match the ReceiverType.
	ReceiverType string
	FieldName    string
	FieldType    string
	ZeroValue    string
	NamedStruct  bool // Getter for named struct.
}

func (g getter) String() string {
	return fmt.Sprintf(`%v.Get%v() %v returns %v`, g.ReceiverType, g.FieldName, g.FieldType, g.ZeroValue)
}

func newGetter(receiverType, fieldName, fieldType, zeroValue string, namedStruct bool) *getter {
	return &getter{
		SortVal:      strings.ToLower(receiverType) + "." + strings.ToLower(fieldName),
		ReceiverVar:  strings.ToLower(receiverType[:1]),
		ReceiverType: receiverType,
		FieldName:    fieldName,
		FieldType:    fieldType,
		ZeroValue:    zeroValue,
		NamedStruct:  namedStruct,
	}
}

func (t *Tmpl) processAST(f *ast.File) error {
	for _, decl := range f.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			// Skip unexported identifiers.
			if !ts.Name.IsExported() {
				logf("Struct %v is unexported; skipping.", ts.Name)
				continue
			}
			// Check if the struct will be ignored.
			if t.ignoreStructs[ts.Name.Name] {
				logf("Struct %v is ignored; skipping.", ts.Name)
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			for _, field := range st.Fields.List {
				se, ok := field.Type.(*ast.StarExpr)
				if len(field.Names) == 0 || !ok {
					continue
				}

				fieldName := field.Names[0]
				// Skip unexported identifiers.
				if !fieldName.IsExported() {
					logf("Field %v is unexported; skipping.", fieldName)
					continue
				}
				// Check if "struct.method" will be ignored.
				if key := fmt.Sprintf("%v.Get%v", ts.Name, fieldName); t.ignoreMethods[key] {
					logf("Method %v is ignored; skipping.", key)
					continue
				}

				switch x := se.X.(type) {
				case *ast.ArrayType:
					t.addArrayType(x, ts.Name.String(), fieldName.String())
				case *ast.Ident:
					t.addIdent(x, ts.Name.String(), fieldName.String())
				case *ast.MapType:
					t.addMapType(x, ts.Name.String(), fieldName.String())
				case *ast.SelectorExpr:
					t.addSelectorExpr(x, ts.Name.String(), fieldName.String())
				default:
					logf("processAST: type %q, field %q, unknown %T: %+v", ts.Name, fieldName, x, x)
				}
			}
		}
	}
	return nil
}

func (t *Tmpl) dump() error {
	if len(t.Getters) == 0 {
		logf("No getters for %v; skipping.", t.filename)
		return nil
	}

	// Sort getters by ReceiverType.FieldName.
	sort.Sort(byName(t.Getters))

	var buf bytes.Buffer
	if err := sourceTmpl.Execute(&buf, t); err != nil {
		return err
	}

	clean, err := format.Source(buf.Bytes())
	if err != nil {
		return err
	}

	logf("Writing %v...", t.filename)
	return ioutil.WriteFile(t.filename, clean, 0644)
}

func (t *Tmpl) addArrayType(x *ast.ArrayType, receiverType, fieldName string) {
	var eltType string
	switch elt := x.Elt.(type) {
	case *ast.Ident:
		eltType = elt.String()
	default:
		logf("addArrayType: type %q, field %q: unknown elt type: %T %+v; skipping.", receiverType, fieldName, elt, elt)
		return
	}

	t.Getters = append(t.Getters, newGetter(receiverType, fieldName, "[]"+eltType, "nil", false))
}

func (t *Tmpl) addIdent(x *ast.Ident, receiverType, fieldName string) {
	var zeroValue string
	var namedStruct = false

	switch x.String() {
	case "int", "int32", "int64", "uint", "uint32", "uint64", "float32", "float64":
		zeroValue = "0"
	case "string":
		zeroValue = `""`
	case "bool":
		zeroValue = "false"
	default:
		zeroValue = "nil"
		namedStruct = true
	}

	t.Getters = append(t.Getters, newGetter(receiverType, fieldName, x.String(), zeroValue, namedStruct))
}

func (t *Tmpl) addMapType(x *ast.MapType, receiverType, fieldName string) {
	var keyType string
	switch key := x.Key.(type) {
	case *ast.Ident:
		keyType = key.String()
	default:
		logf("addMapType: type %q, field %q: unknown key type: %T %+v; skipping.", receiverType, fieldName, key, key)
		return
	}

	var valueType string
	switch value := x.Value.(type) {
	case *ast.Ident:
		valueType = value.String()
	default:
		logf("addMapType: type %q, field %q: unknown value type: %T %+v; skipping.", receiverType, fieldName, value, value)
		return
	}

	fieldType := fmt.Sprintf("map[%v]%v", keyType, valueType)
	zeroValue := fmt.Sprintf("map[%v]%v{}", keyType, valueType)
	t.Getters = append(t.Getters, newGetter(receiverType, fieldName, fieldType, zeroValue, false))
}

func (t *Tmpl) addSelectorExpr(x *ast.SelectorExpr, receiverType, fieldName string) {
	if strings.ToLower(fieldName[:1]) == fieldName[:1] { // Non-exported field.
		return
	}

	var xX string
	if xx, ok := x.X.(*ast.Ident); ok {
		xX = xx.String()
	}

	switch xX {
	case "time", "json":
		if xX == "json" {
			t.Imports["encoding/json"] = "encoding/json"
		} else {
			t.Imports[xX] = xX
		}
		fieldType := fmt.Sprintf("%v.%v", xX, x.Sel.Name)
		zeroValue := fmt.Sprintf("%v.%v{}", xX, x.Sel.Name)
		if xX == "time" && x.Sel.Name == "Duration" {
			zeroValue = "0"
		}
		t.Getters = append(t.Getters, newGetter(receiverType, fieldName, fieldType, zeroValue, false))
	default:
		logf("addSelectorExpr: xX %q, type %q, field %q: unknown x=%+v; skipping.", xX, receiverType, fieldName, x)
	}
}

func logf(fmt string, args ...interface{}) {
	if verbose {
		log.Printf(fmt, args...)
	}
}

type byName []*getter

func (b byName) Len() int           { return len(b) }
func (b byName) Less(i, j int) bool { return b[i].SortVal < b[j].SortVal }
func (b byName) Swap(i, j int)      { b[i], b[j] = b[j], b[i] }

const source = `// Code generated by github.com/tors/getters; DO NOT EDIT.
package {{.Package}}
{{with .Imports}}
import (
  {{- range . -}}
  "{{.}}"
  {{end -}}
)
{{end}}
{{range .Getters}}
{{if .NamedStruct}}
// Get{{.FieldName}} returns the {{.FieldName}} field.
func ({{.ReceiverVar}} *{{.ReceiverType}}) Get{{.FieldName}}() *{{.FieldType}} {
  if {{.ReceiverVar}} == nil {
    return {{.ZeroValue}}
  }
  return {{.ReceiverVar}}.{{.FieldName}}
}
{{else}}
// Get{{.FieldName}} returns the {{.FieldName}} field if it's non-nil, zero value otherwise.
func ({{.ReceiverVar}} *{{.ReceiverType}}) Get{{.FieldName}}() {{.FieldType}} {
  if {{.ReceiverVar}} == nil || {{.ReceiverVar}}.{{.FieldName}} == nil {
    return {{.ZeroValue}}
  }
  return *{{.ReceiverVar}}.{{.FieldName}}
}
{{end}}
{{end}}
`
