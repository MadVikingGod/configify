package main

import (
	"bytes"
	_ "embed"
	"flag"
	"fmt"
	"go/ast"
	"go/format"
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

type Package struct {
	name    string
	defs    map[*ast.Ident]types.Object
	types   *types.Package
	imports map[string]*packages.Package
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
	interfaceType
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
		name:    pkg.Name,
		defs:    pkg.TypesInfo.Defs,
		types:   pkg.Types,
		imports: pkg.Imports,
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

//go:embed interfaceType.inc
var templateInterfaceType string

func (g *Generator) generate(typeName string) {
	tempHeader := template.Must(template.New("header").Parse(templateHeader))
	temps := map[ValueType]*template.Template{
		baseType:      template.Must(template.New("baseType").Parse(templateBaseType)),
		pointerType:   template.Must(template.New("pointerType").Parse(templatePointerType)),
		sliceType:     template.Must(template.New("sliceType").Parse(templateSliceType)),
		interfaceType: template.Must(template.New("InterfaceType").Parse(templateInterfaceType)),
	}

	values := make([]Value, 0, 100)
	imports := map[string]struct{}{}

	for e, v := range g.pkg.defs {
		if e.Name == typeName {
			struc := v.Type().(*types.Named).Underlying().(*types.Struct)
			for i := 0; i < struc.NumFields(); i++ {
				field := struc.Field(i)
				if field.Embedded() {
					continue
				}
				typeString := types.TypeString(field.Type(), types.RelativeTo(field.Pkg()))
				var imp string
				if indx := strings.LastIndex(typeString, "."); indx > 0 {
					imp = typeString[0:indx]
				}
				if indx := strings.LastIndex(typeString, "/"); indx > 0 {
					typeString = typeString[indx+1:]
				}

				var valueType ValueType
				switch typ := field.Type().Underlying().(type) {
				case *types.Basic:
					valueType = baseType
				case *types.Array:
					valueType = baseType
				case *types.Slice:
					valueType = sliceType
				case *types.Map:
					valueType = mapType
				case *types.Signature:
					valueType = baseType
				case *types.Pointer:
					if len(imp) > 0 {
						imp = imp[1:]
					}
					typeString = typeString[1:]
					valueType = pointerType
				case *types.Interface:
					valueType = interfaceType
				case *types.Struct:
					valueType = baseType
				default:
					fmt.Printf("%s - %s - %T %#v\n", field.Name(), typeString, typ, typ)
					continue
				}

				if len(imp) > 0 {
					imports[imp] = struct{}{}
				}

				values = append(values, Value{
					originalName: field.Name(),
					originalType: typeString,
					valueType:    valueType,
				})
			}

		}

	}

	importList := make([]string, 0, len(imports))
	for imp := range imports {
		importList = append(importList, imp)
	}

	tempHeader.Execute(&g.buf, map[string]interface{}{
		"package": g.pkg.name,
		"cfgType": typeName,
		"imports": importList,
	})

	for _, value := range values {
		temp, ok := temps[value.valueType]
		if !ok {
			continue
		}
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
