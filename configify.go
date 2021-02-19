package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
	"go/printer"
	"go/token"
	"go/types"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"text/template"
	"unicode"

	"golang.org/x/tools/go/packages"
)

var (
	genType   = flag.String("type", "", "the config type to generate stubs for")
	buildTags = flag.String("tags", "", "comma-separated list of build tags to apply")
	output    = flag.String("output", "", "output file name; default srcdir/<type>_option.go")
)

// Usage is a replacement usage function for the flags package.
func Usage() {
	fmt.Fprintf(os.Stderr, "Usage of configify:\n")
	fmt.Fprintf(os.Stderr, "\tconfigify [flags] -type T [directory]\n")
	//fmt.Fprintf(os.Stderr, "\tconfigify [flags] -type T files... # Must be a single package\n")
	fmt.Fprintf(os.Stderr, "For more information, see:\n")
	fmt.Fprintf(os.Stderr, "\thttps://pkg.go.dev/github.com/madvikinggod/configify\n")
	fmt.Fprintf(os.Stderr, "Flags:\n")
	flag.PrintDefaults()
}

func main() {
	flag.Usage = Usage
	flag.Parse()
	if len(*genType) == 0 {
		flag.Usage()
		os.Exit(2)
	}

	var tags []string
	if len(*buildTags) > 0 {
		tags = strings.Split(*buildTags, ",")
	}

	args := flag.Args()
	if len(args) == 0 {
		args = []string{"."}
	}

	g := &Generator{}

	g.parsePackage(args, tags)

	g.generate(*genType)

	src := g.format()

	var dir string
	if len(args) == 1 && isDirectory(args[0]) {
		dir = args[0]
	} else {
		if len(tags) != 0 {
			log.Fatal("-tags option applies only to directories, not when files are specified")
		}
		dir = filepath.Dir(args[0])
	}

	// Write to file.
	outputName := *output
	if outputName == "" {
		baseName := fmt.Sprintf("%s_string.go", *genType)
		outputName = filepath.Join(dir, strings.ToLower(baseName))
	}
	err := ioutil.WriteFile(outputName, src, 0644)
	if err != nil {
		log.Fatalf("writing output: %s", err)
	}
}

// isDirectory reports whether the named file is a directory.
func isDirectory(name string) bool {
	info, err := os.Stat(name)
	if err != nil {
		log.Fatal(err)
	}
	return info.IsDir()
}

// Generator holds the state of the analysis. Primarily used to buffer
// the output for format.Source.
type Generator struct {
	buf bytes.Buffer
	pkg *Package
}

// File holds a single parsed file and associated data.
type File struct {
	pkg  *Package  // Package to which this file belongs.
	fileset *token.FileSet // Parsed Fileset to which this file belongs.
	file *ast.File // Parsed AST.
	// These fields are reset for each type being generated.
	typeName string  // Name of the constant type.
	values   []Value // Accumulator for constant values of that type.
}
type Package struct {
	name  string
	defs  map[*ast.Ident]types.Object
	files []*File
}

// Value represents a declared constant.
type Value struct {
	originalName string // The name of the constant.

	originalType string // The Type of the config
	valueType    ValueType
}
type ValueType int

const (
	baseType ValueType = iota
	pointerType
	sliceType
	mapType
)

// parsePackage analyzes the single package constructed from the patterns and tags.
// parsePackage exits if there is an error.
func (g *Generator) parsePackage(patterns []string, tags []string) {
	cfg := &packages.Config{
		Mode: packages.LoadSyntax,
		// TODO: Need to think about constants in test files. Maybe write type_string_test.go
		// in a separate pass? For later.
		Tests:      false,
		BuildFlags: []string{fmt.Sprintf("-tags=%s", strings.Join(tags, " "))},
	}
	pkgs, err := packages.Load(cfg, patterns...)
	if err != nil {
		log.Fatal(err)
	}
	if len(pkgs) != 1 {
		log.Fatalf("error: %d packages found", len(pkgs))
	}
	g.addPackage(pkgs[0])
}

// addPackage adds a type checked Package and its syntax files to the generator.
func (g *Generator) addPackage(pkg *packages.Package) {
	g.pkg = &Package{
		name:  pkg.Name,
		defs:  pkg.TypesInfo.Defs,
		files: make([]*File, len(pkg.Syntax)),
	}

	for i, file := range pkg.Syntax {
		g.pkg.files[i] = &File{
			file: file,
			pkg:  g.pkg,
			fileset: pkg.Fset,
		}
	}
}

//go:embed header.inc
var templateHeader string

//go:embed baseType.inc
var templateBaseType string

//go:embed pointerType.inc
var templatePointerType string

//go:embed sliceType.inc
var templateSliceType string

func (g *Generator) generate(typeName string) {
	tempHeader := template.Must(template.New("header").Parse(templateHeader))
	temps := map[ValueType]*template.Template{
		baseType: template.Must(template.New("baseType").Parse(templateBaseType)),
		pointerType: template.Must(template.New("pointerType").Parse(templatePointerType)),
		sliceType: template.Must(template.New("pointerType").Parse(templateSliceType)),
	}

	values := make([]Value, 0, 100)
	for _, file := range g.pkg.files {
		// Set the state for this run of the walker.
		file.typeName = typeName
		file.values = nil
		if file.file != nil {
			ast.Inspect(file.file, file.structType)
			values = append(values, file.values...)
		}

	}

	tempHeader.Execute(&g.buf, map[string]string{
		"package": g.pkg.name,
		"cfgType": typeName,
	})

	for _, value := range values {
		temp, ok := temps[value.valueType]
		if !ok { continue}
		temp.Execute(&g.buf, map[string]string{
			"origName":  value.originalName,
			"nameLower": lowerName(value.originalName),
			"nameUpper": upperName(value.originalName),
			"origType":  value.originalType,
		})
	}
}

func lowerName(name string) string {
	a := []rune(name)
	a[0] = unicode.ToLower(a[0])
	return string(a)
}
func upperName(name string) string {
	a := []rune(name)
	a[0] = unicode.ToUpper(a[0])
	return string(a)
}

func (f *File) structType(node ast.Node) bool {
	if node == nil {
		return true
	}

	ident, ok := node.(*ast.Ident)
	if !ok || ident.Name != f.typeName {
		return true
	}
	if ident.Obj == nil {
		return true
	}
	ts, ok := ident.Obj.Decl.(*ast.TypeSpec)
	if !ok {
		return true
	}
	st, ok := ts.Type.(*ast.StructType)

	f.values = []Value{}
	for _, field := range st.Fields.List {
		val, err := f.fromField(field)
		if err != nil {
			log.Printf("Warning: %s", err)
		}
		if err == nil {
			f.values = append(f.values, val)
		}
	}

	return true
}

func (f *File) fromField(field *ast.Field) (Value, error) {
	if len(field.Names) != 1 {
		return Value{}, fmt.Errorf("No support for embedded types currently")
	}

	var errStr string

	switch ft := field.Type.(type) {
	case *ast.Ident:
		return Value{
			originalName: field.Names[0].Name,
			originalType: ft.Name,
			valueType:    baseType,
		}, nil
	case *ast.ArrayType:
		buf := &bytes.Buffer{}
		printer.Fprint(buf, f.fileset, ft)
		valueType := sliceType
		if ft.Len != nil { //This is an array use the basic type
			valueType = baseType
		}
		return Value{
			originalName: field.Names[0].Name,
			originalType: buf.String(),
			valueType:    valueType,
		}, nil
	case *ast.MapType:
		buf := &bytes.Buffer{}
		printer.Fprint(buf, f.fileset, ft)
		return Value{
			originalName: field.Names[0].Name,
			originalType: buf.String(),
			valueType:    mapType,
		}, nil
	case *ast.FuncType:
		buf := &bytes.Buffer{}
		printer.Fprint(buf, f.fileset, ft)
		return Value{
			originalName: field.Names[0].Name,
			originalType: buf.String(),
			valueType:    baseType,
		}, nil
	case *ast.StarExpr:
		buf := &bytes.Buffer{}
		printer.Fprint(buf, f.fileset, ft.X)
		return Value{
			originalName: field.Names[0].Name,
			originalType: buf.String(),
			valueType: pointerType,
		}, nil
	case *ast.SelectorExpr:
		//TODO: rebuild interface Interfaces need to be imported need to track those
		errStr = fmt.Sprintf("%T", ft)
	default:
		errStr = fmt.Sprintf("Unknown type %T", ft)

	}

	return Value{}, fmt.Errorf("%s not supported yet", errStr)
}

func (g *Generator) format() []byte {
	src, err := format.Source(g.buf.Bytes())
	if err != nil {
		// Should never happen, but can arise when developing this code.
		// The user can compile the output to see the error.
		log.Printf("warning: internal error: invalid Go generated: %s", err)
		log.Printf("warning: compile the package to analyze the error")
		return g.buf.Bytes()
	}
	return src
}
