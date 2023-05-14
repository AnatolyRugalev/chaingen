package chaingen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"log"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"golang.org/x/tools/go/packages"
)

type Chaingen struct {
	opts Options
}

func New(opts Options) Chaingen {
	return Chaingen{
		opts: opts,
	}
}

// Builder represents a struct type that is considered to be a builder.
// Builder has at least one method that returns altered Builder copy (chaining method).
// Every non-chaining method is considered as finalizer
type Builder struct {
	PkgPath            string
	Package            *packages.Package
	Type               *types.Named
	Annotations        []string
	Struct             *types.Struct
	Methods            []Method
	GeneratedMethods   []Method
	GeneratedFunctions []Method
	Children           []*BuilderRef
	MethodNames        map[string]*Method
	File               *ast.File
	FilePath           string
	Rendered           bool
	Depth              int
}

type BuilderRef struct {
	Name             string
	IsMethod         bool
	FieldAnnotation  string
	Builder          *Builder
	GeneratedMethods []Method
}

type Import struct {
	Alias   string
	Package *types.Package
}

type Method struct {
	Name     string
	Alias    string
	Pos      token.Pos
	Variadic bool
	Exported bool
	Builder  *Builder
	Ref      *BuilderRef

	Recv           FuncParam
	Params         []FuncParam
	Results        []FuncParam
	Prefixes       []string
	Postfixes      []string
	WrapperName    string
	Pointer        bool
	Picked         bool
	VariadicUnwrap bool
	PkgExport      bool
}

func (m Method) String() string {
	return m.Builder.Type.Obj().Name() + "." + m.Name
}

type Function struct {
	Name     string
	Alias    string
	Variadic bool
	Exported bool
	Builder  *Builder
	Ref      *BuilderRef

	Params  []FuncParam
	Results []FuncParam
}

type FuncParam struct {
	Name   string
	Type   types.Type
	Prefix string
	Suffix string
}

func NewMethod(builder *Builder, f *types.Func, sig *types.Signature) Method {
	m := Method{
		Name:     f.Name(),
		Alias:    f.Name(),
		Pos:      f.Pos(),
		Variadic: sig.Variadic(),
		Builder:  builder,
		Exported: f.Exported(),
		Recv: FuncParam{
			Name: sig.Recv().Name(),
			Type: sig.Recv().Type(),
		},
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		m.Params = append(m.Params, FuncParam{
			Name: param.Name(),
			Type: param.Type(),
		})
	}
	for i := 0; i < sig.Results().Len(); i++ {
		param := sig.Results().At(i)
		m.Results = append(m.Results, FuncParam{
			Name: param.Name(),
			Type: param.Type(),
		})
	}
	return m
}

func (b *Builder) ReceiverName() string {
	var name string
	for _, m := range b.Methods {
		if m.Recv.Name != "" {
			name = m.Recv.Name
			break
		}
	}
	if name == "" {
		name = strings.ToLower(b.Type.Obj().Name()[0:1])
	}
	return name
}

func (b *Builder) ReceiverType() types.Type {
	for _, m := range b.Methods {
		return m.Recv.Type
	}
	// TODO: this works poorly with type params
	return b.Type
}

func (b *Builder) ReceiverTypeName(ptr bool) string {
	s := strings.Builder{}
	if ptr {
		s.WriteRune('*')
	}
	s.WriteString(b.Type.Obj().Name())
	tps := b.Type.TypeParams()
	var tpss []string
	for i := 0; i < tps.Len(); i++ {
		tp := tps.At(i)
		tpss = append(tpss, tp.String())
	}
	if len(tpss) > 0 {
		s.WriteString("[" + strings.Join(tpss, ", ") + "]")
	}
	return s.String()
}

func (b *Builder) Ref(builder *Builder) *BuilderRef {
	for _, child := range b.Children {
		if child.Builder == builder {
			return child
		}
	}
	return nil
}

func (b *Builder) RenderSignature(file *File, method Method, unwrap bool) (string, []string) {
	var inputParams []string
	var callParams []string
	for i, param := range method.Params {
		last := i == len(method.Params)-1
		if last && method.Variadic {
			typ := file.TypeIdentifier(param.Type.(*types.Slice).Elem())
			out := param.Name + "..."
			if unwrap {
				param.Prefix = strings.ReplaceAll(param.Prefix, "__TYPE__", typ)
				out = param.Prefix + out + param.Suffix
			}
			inputParams = append(inputParams, param.Name+" ..."+typ)
			callParams = append(callParams, out)
		} else {
			typ := file.TypeIdentifier(param.Type)
			out := param.Name
			if unwrap {
				param.Prefix = strings.ReplaceAll(param.Prefix, "__TYPE__", typ)
				out = param.Prefix + out + param.Suffix
			}
			inputParams = append(inputParams, param.Name+" "+file.TypeIdentifier(param.Type))
			callParams = append(callParams, out)
		}
	}
	var outputParams []string
	for i, param := range method.Results {
		typeName := file.TypeIdentifier(param.Type)
		name := param.Name
		if name != "" {
			name += " "
		}
		if i == 0 && method.Pointer && method.IsChaining() {
			typeName = "*" + typeName
		}
		outputParams = append(outputParams, name+typeName)
	}
	outputParamsStr := strings.Join(outputParams, ", ")
	if len(outputParams) > 1 || (len(method.Results) == 0 && method.Results[0].Name != "") {
		outputParamsStr = "(" + outputParamsStr + ")"
	}
	return method.Alias + `(` + strings.Join(inputParams, ", ") + `) ` + outputParamsStr, callParams

}

func (b *Builder) RenderFunction(file *File, method Method) {
	signature, callParams := b.RenderSignature(file, method, false)
	file.L()
	doc := method.Doc()
	if doc != nil {
		for _, line := range doc.List {
			file.L(line.Text)
		}
	}
	file.L("func " + signature + "{ ")
	if len(method.Results) > 0 {
		file.P("return ")
	}
	file.P("new(" + file.TypeIdentifier(method.Recv.Type) + ")." + method.Alias + "(" + strings.Join(callParams, ", ") + ")")
	file.L("")
	file.L("}")
}

func (b *Builder) RenderMethod(file *File, method Method) {
	signature, callParams := b.RenderSignature(file, method, true)
	file.L()
	doc := method.Doc()
	if doc != nil {
		for _, line := range doc.List {
			file.L(line.Text)
		}
	}
	recv := method.Recv.Name
	file.L(`func (` + recv + ` ` + b.ReceiverTypeName(method.Pointer) + `) ` + signature + " {")
	if method.VariadicUnwrap {
		file.L("out := make(" + file.TypeIdentifier(method.Results[0].Type) + ", len(in))")
		file.L("for i := range in {")
		file.L("out[i] = in[i]." + method.Params[0].Suffix)
		file.L("}")
		file.L("return out")
	} else {
		ref := recv + "." + method.Ref.Name
		for _, prefix := range method.Prefixes {
			file.L(prefix)
		}
		for _, postfix := range method.Postfixes {
			file.L("defer func() {")
			file.L(postfix)
			file.L("}()")
		}
		result := ref + "." + method.Name + "(" + strings.Join(callParams, ", ") + ")"
		if method.WrapperName != "" {
			result = recv + "." + method.WrapperName + "(" + result + ")"
		}
		if method.IsChaining() {
			if method.Ref.IsMethod {
				file.L("return " + result)
			} else {
				file.L("\t" + ref + ` = ` + result)
				file.L("\treturn " + recv)
			}
		} else {
			if len(method.Results) > 0 {
				file.P("return ")
			}
			file.P(result)
			file.L("")
		}
	}
	file.L("}")
}

func (m Method) IsChaining() bool {
	if m.WrapperName != "" {
		return false
	}
	if len(m.Results) != 1 {
		return false
	}
	return m.Recv.Type.String() == m.Results[0].Type.String()
}

const generatedPrefix = "Code generated by chaingen. DO NOT EDIT."

func (m Method) Doc() *ast.CommentGroup {
	pkg := m.Builder.Package
	methodPos := pkg.Fset.Position(m.Pos)
	for _, file := range pkg.Syntax {
		for _, cg := range file.Comments {
			commentPos := pkg.Fset.Position(cg.End())
			if commentPos.Filename == methodPos.Filename && commentPos.Line == methodPos.Line-1 {
				if m.Name != m.Alias {
					firstLine := cg.List[0].Text
					if strings.HasPrefix(firstLine, "// "+m.Name+" ") {
						firstLine = "// " + m.Alias + " " + firstLine[len(m.Name)+4:]
						cg.List[0].Text = firstLine
					}
				}
				return cg
			}
		}
	}
	return nil
}

type Options struct {
	Src           string
	TypeName      string
	Recursive     bool
	FileSuffix    string
	ErrOnConflict bool
	StructTag     string
	BuildTag      string
}

type File struct {
	Builders []*Builder
	Package  *packages.Package
	File     *ast.File
	Path     string
	BuildTag string

	Imports       map[string]Import
	ImportAliases map[string]*Import
	Body          bytes.Buffer
}

func (f *File) Render(w io.Writer) error {
	output := "//go:build !" + f.BuildTag + "\n"
	output += "// +build !" + f.BuildTag + "\n\n"
	output += "// " + generatedPrefix + "\n\n"
	output += "package " + f.Package.Name + "\n\n"
	if len(f.Imports) > 0 {
		output += "import (\n"
		for _, i := range f.Imports {
			alias := ""
			if i.Alias != i.Package.Name() {
				alias = i.Alias + " "
			}
			output += "\t" + alias + `"` + i.Package.Path() + `"` + "\n"
		}
		output += ")\n"
	}
	_, err := w.Write([]byte(output + f.Body.String()))
	return err
}

func (f *File) P(s ...string) {
	f.Body.WriteString(strings.Join(s, ""))
}

func (f *File) L(s ...string) {
	f.P(append(s, "\n")...)
}

func (f *File) TypeIdentifier(typ types.Type) string {
	if typ == nil {
		panic("oi")
	}
	switch t := typ.(type) {
	case *types.Named:
		pkg := t.Obj().Pkg()
		pkgPath := ""
		if pkg != nil && pkg.Path() != f.Package.PkgPath {
			pkgPath = f.PackageIdentifier(pkg) + "."
		}

		typeArgs := ""
		if ta := t.TypeArgs(); ta != nil {
			var args []string
			for i := 0; i < ta.Len(); i++ {
				args = append(args, f.TypeIdentifier(ta.At(i)))
			}
			typeArgs = "[" + strings.Join(args, ", ") + "]"
		} else if tp := t.TypeParams(); tp != nil {
			var args []string
			for i := 0; i < tp.Len(); i++ {
				args = append(args, f.TypeIdentifier(tp.At(i)))
			}
			typeArgs = "[" + strings.Join(args, ", ") + "]"
		}
		return pkgPath + t.Obj().Name() + typeArgs
	case *types.Slice:
		return "[]" + f.TypeIdentifier(t.Elem())
	case *types.Map:
		return "map[" + f.TypeIdentifier(t.Key()) + "]" + f.TypeIdentifier(t.Elem())
	case *types.Pointer:
		return "*" + f.TypeIdentifier(t.Elem())
	default:
		return t.String()
	}
}

func (f *File) PackageIdentifier(pkg *types.Package) string {
	i, ok := f.Imports[pkg.Path()]
	if ok {
		return i.Alias
	}
	i = Import{
		Alias:   pkg.Name(),
		Package: pkg,
	}
	f.Imports[pkg.Path()] = i
	suffix := 0
	for {
		_, ok := f.ImportAliases[i.Alias]
		if !ok {
			break
		}
		suffix++
		i.Alias = fmt.Sprintf("%s%d", pkg.Name(), suffix)
	}
	f.ImportAliases[i.Alias] = &i
	return i.Alias
}

var unwrappers = map[string]Method{}

func (b *Builder) generateMethods() {
	// this func should be called only after all children are evaluated

	// Collect all methods of all children
	// We should also take generated methods of the children
	myMethodsMap := make(map[string]Method, len(b.Methods))
	visited := map[string]struct{}{}
	for _, method := range b.Methods {
		myMethodsMap[method.Alias] = method
		visited[method.Alias] = struct{}{}
	}
	for _, child := range b.Children {
		allMethods := append(child.Builder.Methods, child.Builder.GeneratedMethods...)
		// Evaluate all annotations, then assign results to GeneratedMethods
		pool := make(map[string]Method, len(allMethods))
		for _, m := range allMethods {
			m.Ref = child
			m.Pointer = false
			m.WrapperName = ""
			m.Prefixes = []string{}
			m.Postfixes = []string{}
			m.Picked = false
			pool[m.Alias] = m
		}
		modifiers := strings.Split(child.FieldAnnotation, ",")
		for _, modifier := range modifiers {
			if len(modifier) == 0 {
				continue
			}
			parts := strings.Split(modifier, "=")
			withPrivate := false
			switch {
			case modifier == "!":
				withPrivate = true
			case modifier == "*":
				for _, m := range pool {
					m.Picked = m.Exported || withPrivate
					pool[m.Alias] = m
				}
			case modifier[0] == '-':
				glob := NewGlob(modifier[1:])
				for _, m := range pool {
					if glob.Match(m.Recv.Type, m.Alias) {
						m.Picked = false
						pool[m.Alias] = m
					}
				}
			case strings.HasPrefix(modifier, "wrap("):
				selector := parts[0][5 : len(parts[0])-1]
				glob := NewGlob(selector)
				wrappers := strings.Split(parts[1], "|")
				for _, wrapperName := range wrappers {
					parentMethod, ok := myMethodsMap[wrapperName]
					if !ok {
						continue
					}
					for _, method := range pool {
						if !glob.Match(method.Recv.Type, method.Alias) {
							continue
						}
						compatible := parentMethod.Variadic
						if len(parentMethod.Params) == len(method.Results) {
							compatible = true
							for i := 0; i < len(parentMethod.Params); i++ {
								if parentMethod.Params[i].Type.String() != method.Results[i].Type.String() {
									compatible = false
									break
								}
							}
						}
						if !compatible {
							continue
						}
						generated := method
						generated.Picked = method.Exported || withPrivate
						generated.WrapperName = wrapperName
						generated.Results = parentMethod.Results
						pool[generated.Name] = generated
					}
				}
			case strings.HasPrefix(modifier, "unwrap"):
				names := strings.Split(parts[1], "|")
				for _, unwrap := range names {
					parentMethod, ok := myMethodsMap[unwrap]
					if !ok {
						continue
					}
					unwrappers[parentMethod.Results[0].Type.String()] = parentMethod
				}
			case strings.HasPrefix(modifier, "export("):
				selector := parts[0][7 : len(parts[0])-1]
				glob := NewGlob(selector)
				for _, method := range pool {
					if glob.Match(method.Recv.Type, method.Alias) {
						method.PkgExport = true
					}
					pool[method.Alias] = method
				}
			case strings.HasPrefix(modifier, "ptr("):
				selector := parts[0][4 : len(parts[0])-1]
				glob := NewGlob(selector)
				for _, method := range pool {
					if glob.Match(method.Recv.Type, method.Alias) {
						method.Pointer = true
					}
					pool[method.Alias] = method
				}
			case strings.HasPrefix(modifier, "pre("):
				selector := parts[0][4 : len(parts[0])-1]
				glob := NewGlob(selector)
				for _, method := range pool {
					if glob.Match(method.Recv.Type, method.Alias) {
						method.Prefixes = append(method.Prefixes, parts[1])
					}
					pool[method.Alias] = method
				}
			case strings.HasPrefix(modifier, "post("):
				selector := parts[0][5 : len(parts[0])-1]
				glob := NewGlob(selector)
				for _, method := range pool {
					if glob.Match(method.Recv.Type, method.Alias) {
						method.Postfixes = append(method.Postfixes, parts[1])
					}
					pool[method.Alias] = method
				}
			default:
				left := NewGlob(parts[0])
				var right *Glob
				pick := true
				if len(parts) > 1 {
					if parts[1] == "-" {
						pick = false
					} else {
						rightGlob := NewGlob(parts[1])
						right = &rightGlob
					}
				}
				newPool := make(map[string]Method, len(pool))
				for _, method := range pool {
					if left.Match(method.Recv.Type, method.Alias) {
						method.Alias = left.Replace(method.Alias, right)
						method.Picked = pick && (method.Exported || withPrivate)
					}
					newPool[method.Alias] = method
				}
				pool = newPool
			}
		}
		for _, m := range pool {
			if !m.Picked {
				continue
			}
			_, ok := visited[m.Alias]
			if ok {
				continue
			}
			visited[m.Alias] = struct{}{}
			recv := FuncParam{
				Name: b.ReceiverName(),
				Type: b.ReceiverType(),
			}
			if m.PkgExport {
				m.Recv = recv
				b.GeneratedFunctions = append(b.GeneratedFunctions, m)
			}

			if !child.IsMethod && m.IsChaining() {
				param := recv
				param.Name = ""
				m.Results = []FuncParam{
					param,
				}
			}
			m.Recv = recv
			b.GeneratedMethods = append(b.GeneratedMethods, m)
		}
	}
	pool := make(map[string]Method, len(b.Methods))
	for _, m := range b.Methods {
		pool[m.Alias] = m
	}
	for _, modifier := range b.Annotations {
		if len(modifier) == 0 {
			continue
		}
		if strings.HasPrefix(modifier, "export(") {
			parts := strings.Split(modifier, ":")
			selector := parts[0][7 : len(parts[0])-1]
			glob := NewGlob(selector)
			for _, method := range pool {
				if glob.Match(method.Recv.Type, method.Alias) {
					method.PkgExport = method.Exported
				}
				pool[method.Alias] = method
			}
		}
	}
	for _, m := range pool {
		if m.PkgExport {
			b.GeneratedFunctions = append(b.GeneratedFunctions, m)
		}
	}
}

func (b *Builder) applyUnwrap() {
	var add []Method
	variadicUnwraps := make(map[string]struct{})
	for mi, m := range b.GeneratedMethods {
		for i, param := range m.Params {
			if sl, ok := param.Type.(*types.Slice); ok {
				unwrap, ok := unwrappers[sl.Elem().String()]
				if !ok {
					continue
				}

				param.Type = types.NewSlice(unwrap.Recv.Type)
				param.Prefix = "new(__TYPE__).gen_" + unwrap.Name + "("
				param.Suffix = ")"
				if m.Variadic {
					param.Suffix += "..."
				}
				unwrapType := unwrap.Recv.Type.String()
				methodType := m.Recv.Type.String()
				if _, ok := variadicUnwraps[sl.Elem().String()]; !ok && unwrapType == methodType {
					variadicUnwraps[sl.Elem().String()] = struct{}{}
					variadicMethod := Method{
						VariadicUnwrap: true,
						Name:           "gen_" + unwrap.Name,
						Alias:          "gen_" + unwrap.Name,
						Builder:        b,
						Recv: FuncParam{
							Name: "",
							Type: sl.Elem(),
						},
						Variadic: true,
						Params: []FuncParam{
							{
								Name:   "in",
								Type:   types.NewSlice(unwrap.Recv.Type),
								Suffix: unwrap.Name + "()",
							},
						},
						Results: []FuncParam{
							{
								Type: types.NewSlice(unwrap.Results[0].Type),
							},
						},
					}
					add = append(add, variadicMethod)
				}
			} else {
				unwrap, ok := unwrappers[param.Type.String()]
				if !ok {
					continue
				}
				param.Type = unwrap.Recv.Type
				param.Suffix = "." + unwrap.Name + "()"
			}
			m.Params[i] = param
		}
		b.GeneratedMethods[mi] = m
	}
	b.GeneratedMethods = append(b.GeneratedMethods, add...)
}

type Glob struct {
	TypeName string
	Const    string
	Prefix   string
	Suffix   string
	Middle   string
}

func NewGlob(glob string) Glob {
	dot := strings.Index(glob, ".")
	typeName := ""
	if dot >= 0 {
		typeName = glob[:dot]
		glob = glob[dot+1:]
	}
	if glob == "*" {
		return Glob{
			TypeName: typeName,
		}
	}
	asterisk := strings.Index(glob, "*")
	if asterisk < 0 {
		return Glob{
			TypeName: typeName,
			Const:    glob,
		}
	}
	if asterisk == 0 && glob[len(glob)-1] == '*' {
		return Glob{
			TypeName: typeName,
			Middle:   glob[1 : len(glob)-1],
		}
	}
	return Glob{
		TypeName: typeName,
		Prefix:   glob[:asterisk],
		Suffix:   glob[asterisk+1:],
	}
}

func (g Glob) Match(typ types.Type, str string) bool {
	var typeName string
	named, ok := typ.(*types.Named)
	if ok {
		typeName = named.Obj().Name()
	}
	if g.TypeName != "" && g.TypeName != typeName {
		return false
	}
	if g.Const != "" {
		return str == g.Const
	}
	if g.Middle != "" {
		return strings.Contains(str, g.Middle)
	}
	match := true
	if g.Prefix != "" {
		match = match && len(str) > len(g.Prefix) && strings.HasPrefix(str, g.Prefix)
	}
	if g.Suffix != "" {
		match = match && len(str) > len(g.Suffix) && strings.HasSuffix(str, g.Suffix)
	}
	return match
}

func (g Glob) Replace(methodName string, right *Glob) string {
	left := g
	switch {
	case right == nil:
		return methodName
	case right.Const != "":
		return right.Const
	case left.Middle != "":
		pos := strings.Index(methodName, g.Middle)
		leftMatchPrefix := methodName[:pos]
		leftMatchSuffix := methodName[pos+len(g.Middle):]
		if right.Middle != "" {
			return leftMatchPrefix + right.Middle + leftMatchSuffix
		}
		return right.Prefix + leftMatchPrefix + leftMatchSuffix + right.Suffix
	default:
		match := methodName[len(left.Prefix):]
		match = match[:len(match)-len(left.Suffix)]
		if right.Middle != "" {
			if len(left.Prefix) == 0 {
				return match + right.Middle
			}
			return right.Middle + match
		}
		return right.Prefix + match + right.Suffix
	}
}

func (c Chaingen) render(files map[string]*File, builder *Builder) error {
	file, ok := files[builder.FilePath]
	if !ok {
		file = &File{
			Package:       builder.Package,
			File:          builder.File,
			BuildTag:      c.opts.BuildTag,
			Path:          builder.FilePath,
			Imports:       map[string]Import{},
			ImportAliases: map[string]*Import{},
		}
		files[builder.FilePath] = file
	}
	file.Builders = append(file.Builders, builder)
	if builder.Rendered {
		return nil
	}
	builder.Rendered = true
	for _, m := range builder.GeneratedMethods {
		if !m.Exported && m.Builder.PkgPath != builder.PkgPath {
			continue
		}
		if conflict, ok := builder.MethodNames[m.Alias]; ok {
			if c.opts.ErrOnConflict {
				return fmt.Errorf("method naming conflict for %s.%s: %s and %s", builder.Type.Obj().Name(), m.Alias, conflict.String(), m.String())
			}
			continue
		}
		builder.RenderMethod(file, m)
		builder.MethodNames[m.Alias] = &m
	}
	for _, f := range builder.GeneratedFunctions {
		builder.RenderFunction(file, f)
	}

	return nil
}

func (c Chaingen) Generate() error {
	if c.opts.Src == "" {
		return fmt.Errorf("source dir is not set")
	}
	if c.opts.FileSuffix == "" {
		return fmt.Errorf("file suffix is not set")
	}
	pkgs, err := packages.Load(&packages.Config{
		Dir:        c.opts.Src,
		Mode:       packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports | packages.NeedModule,
		BuildFlags: []string{"-tags=" + c.opts.BuildTag},
	})
	if err != nil {
		return fmt.Errorf("error loading Go packages from %s: %w", c.opts.Src, err)
	}

	var errors []string
	for _, p := range pkgs {
		for _, imp := range p.Imports {
			for _, err := range imp.Errors {
				errors = append(errors, err.Error())
			}
		}
		for _, err := range p.Errors {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("errors occurred loading source code:\n%s\n", strings.Join(errors, "\n"))
	}

	type B struct {
		T *types.Named
		P *packages.Package
	}

	var found []B

	names := strings.Split(c.opts.TypeName, ",")
	for _, p := range pkgs {
		for _, name := range names {
			typ := builderType(objToType(p.Types.Scope().Lookup(name)))
			if typ != nil {
				found = append(found, B{
					T: typ,
					P: p,
				})
			}
		}
	}
	if len(found) == 0 {
		return fmt.Errorf("unable to find builder type %q in %s", c.opts.TypeName, c.opts.Src)
	}

	builders := map[*types.Named]*Builder{}
	for _, b := range found {
		err := c.NewBuilder(builders, b.P, b.T)
		if err != nil {
			return fmt.Errorf("error creating builder: %w", err)
		}
	}

	files := make(map[string]*File)
	for _, b := range found {
		builder := builders[b.T]
		err := c.render(files, builder)
		if err != nil {
			return fmt.Errorf("error generating code: %w", err)
		}
	}
	for _, file := range files {
		if file.Body.Len() == 0 {
			continue
		}
		buf := bytes.Buffer{}
		err = file.Render(&buf)
		if err != nil {
			return err
		}
		src := file.Path
		dest := src[:len(src)-3] + c.opts.FileSuffix
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			log.Printf("error formatting file: %s", err.Error())
			formatted = buf.Bytes()
		}
		err = os.WriteFile(dest, formatted, 0755)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(c.opts.Src, dest)
		log.Printf("generated file: %s", rel)
	}
	return nil
}

type BuilderSpec struct {
	Type       *types.Type
	Annotation string
}

func (c Chaingen) NewBuilder(builders map[*types.Named]*Builder, pkg *packages.Package, n *types.Named) error {
	_, err := c.newBuilder(builders, pkg, n, 0)
	return err
}

func (c Chaingen) newBuilder(builders map[*types.Named]*Builder, pkg *packages.Package, typ *types.Named, depth int) (*Builder, error) {
	if b, ok := builders[typ]; ok {
		return b, nil
	}
	builder := &Builder{
		Depth:       depth,
		Package:     pkg,
		PkgPath:     pkg.PkgPath,
		Type:        typ,
		MethodNames: map[string]*Method{},
	}
	if s, ok := typ.Underlying().(*types.Struct); ok {
		builder.Struct = s
	}
	builders[typ] = builder

	builderFile := pkg.Fset.File(builder.Type.Obj().Pos())
	for _, file := range pkg.Syntax {
		astFile := pkg.Fset.File(file.Pos())
		if astFile.Name() == builderFile.Name() {
			builder.File = file
			builder.FilePath = astFile.Name()
			break
		}
	}
	if builder.File == nil {
		return nil, fmt.Errorf("could not discover file for builder %q", typ.Obj().Name())
	}

	for i := 0; i < typ.NumMethods(); i++ {
		fun := typ.Method(i)
		sig, ok := fun.Type().(*types.Signature)
		if !ok {
			continue
		}
		m := NewMethod(builder, fun, sig)
		builder.Methods = append(builder.Methods, m)
	}
	// Look up builder comment-based annotations

	typePos := pkg.Fset.Position(builder.Type.Obj().Pos())
	for _, file := range pkg.Syntax {
		for _, cg := range file.Comments {
			commentPos := pkg.Fset.Position(cg.End())
			if commentPos.Filename == builder.FilePath && commentPos.Line == typePos.Line-1 {
				for _, comment := range cg.List {
					annotation := strings.Trim(strings.TrimLeft(comment.Text, "/"), " ")
					tag, ok := reflect.StructTag(annotation).Lookup(c.opts.StructTag)
					if ok {
						builder.Annotations = append(builder.Annotations, tag)
					}
				}
			}
		}
	}

	if builder.Struct != nil {
		for i := 0; i < builder.Struct.NumFields(); i++ {
			field := builder.Struct.Field(i)
			fieldAnnotation, _ := reflect.StructTag(builder.Struct.Tag(i)).Lookup(c.opts.StructTag)
			if fieldAnnotation == "-" {
				continue
			}
			typ := builderType(field.Type())
			if typ == nil {
				continue
			}
			name := typ.Obj().Name()
			if !field.Embedded() {
				name = field.Name()
			}
			childPkg := pkg
			pkgPath := typ.Obj().Pkg().Path()
			if pkgPath != pkg.PkgPath {
				childPkg = pkg.Imports[pkgPath]
			}
			if childPkg == nil {
				continue
			}
			child, err := c.newBuilder(builders, childPkg, typ, depth+1)
			if err != nil {
				return nil, fmt.Errorf("error creating builder for field %s: %w", field.Name(), err)
			}
			ref := &BuilderRef{
				Name:            name,
				FieldAnnotation: fieldAnnotation,
				Builder:         child,
			}
			builder.Children = append(builder.Children, ref)
		}
	}
	for _, annotation := range builder.Annotations {
		if strings.HasPrefix(annotation, "ext(") {
			methodName := annotation[4:strings.Index(annotation, ")")]
			for _, method := range builder.Methods {
				if method.Name != methodName {
					continue
				}
				typ := builderType(method.Results[0].Type)
				if typ == nil {
					break
				}

				childPkg := pkg
				pkgPath := typ.Obj().Pkg().Path()
				if pkgPath != pkg.PkgPath {
					childPkg = pkg.Imports[pkgPath]
				}
				if childPkg == nil {
					continue
				}
				child, err := c.newBuilder(builders, childPkg, typ, depth+1)
				if err != nil {
					return nil, fmt.Errorf("error creating external builder %s: %w", methodName, err)
				}
				var fieldAnnotation string
				parts := strings.Split(annotation, ":")
				if len(parts) == 2 {
					fieldAnnotation = parts[1]
				}
				builder.Children = append(builder.Children, &BuilderRef{
					Name:            methodName + "()",
					IsMethod:        true,
					FieldAnnotation: fieldAnnotation,
					Builder:         child,
				})
				break
			}
		}
	}
	builder.generateMethods()
	builder.applyUnwrap()

	return builder, nil
}

func objToType(obj types.Object) types.Type {
	if obj == nil {
		return nil
	}
	typName, ok := obj.(*types.TypeName)
	if !ok {
		return nil
	}
	return typName.Type()
}

func builderType(typ types.Type) *types.Named {
	if typ == nil {
		return nil
	}
	if n, ok := typ.(*types.Named); ok {
		_, ok := n.Underlying().(*types.Struct)
		if ok {
			return n
		}
		_, ok = n.Underlying().(*types.Slice)
		if ok {
			return n
		}
		_, ok = n.Underlying().(*types.Signature)
		if ok {
			return n
		}
	}
	if p, ok := typ.Underlying().(*types.Pointer); ok {
		return builderType(p.Elem())
	}
	return nil
}
