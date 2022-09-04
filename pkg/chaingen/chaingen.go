package chaingen

import (
	"bytes"
	"fmt"
	"go/ast"
	"go/format"
	"go/token"
	"go/types"
	"io"
	"os"
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
	Name        string
	PkgPath     string
	Package     *packages.Package
	Type        *types.Named
	Struct      *types.Struct
	Methods     []Method
	Children    []*Builder
	MethodNames map[string]*Method
	File        *ast.File
	FilePath    string
}

type Import struct {
	Alias   string
	Package *types.Package
}

type Method struct {
	Name  string
	Scope *types.Scope
	Pos   token.Pos

	Recv    MethodParam
	Params  []MethodParam
	Results []MethodParam
}

type MethodParam struct {
	Name string
	Type types.Type
}

func NewMethod(f *types.Func, sig *types.Signature) Method {
	m := Method{
		Name:  f.Name(),
		Pos:   f.Pos(),
		Scope: f.Scope(),
		Recv: MethodParam{
			Name: sig.Recv().Name(),
			Type: sig.Recv().Type(),
		},
	}
	for i := 0; i < sig.Params().Len(); i++ {
		param := sig.Params().At(i)
		m.Params = append(m.Params, MethodParam{
			Name: param.Name(),
			Type: param.Type(),
		})
	}
	for i := 0; i < sig.Results().Len(); i++ {
		param := sig.Results().At(i)
		m.Results = append(m.Results, MethodParam{
			Name: param.Name(),
			Type: param.Type(),
		})
	}
	return m
}

func (b *Builder) ReceiverName() string {
	if len(b.Methods) > 0 {
		return b.Methods[0].Recv.Name
	}
	return strings.ToLower(b.Type.Obj().Name()[0:1])
}

func (b *Builder) RenderChainMethod(file *File, child *Builder, method Method) {
	b.MethodNames[method.Name] = &method
	ref := b.ReceiverName() + "." + child.Name
	var inputParams []string
	var callParams []string
	for _, param := range method.Params {
		inputParams = append(inputParams, param.Name+" "+file.TypeIdentifier(param.Type))
		callParams = append(callParams, param.Name)
	}
	file.L()
	doc := child.MethodDoc(method)
	if doc != nil {
		for _, line := range doc.List {
			file.L(line.Text)
		}
	}
	file.L(`func (` + b.ReceiverName() + ` ` + b.Type.Obj().Name() + `) ` + method.Name + `(` + strings.Join(inputParams, ", ") + `) ` + b.Type.Obj().Name() + " {")
	file.L("\t" + ref + ` = ` + ref + `.` + method.Name + `(` + strings.Join(callParams, ", ") + `)`)
	file.L("\treturn " + b.ReceiverName())
	file.L("}")
}

func (b *Builder) RenderFinalizer(file *File, child *Builder, method Method) {
	b.MethodNames[method.Name] = &method
	ref := b.ReceiverName() + "." + child.Name
	var inputParams []string
	var callParams []string
	for _, param := range method.Params {
		inputParams = append(inputParams, param.Name+" "+file.TypeIdentifier(param.Type))
		callParams = append(callParams, param.Name)
	}
	var outputParams []string
	for _, param := range method.Results {
		typeName := file.TypeIdentifier(param.Type)
		if typeName == "" {
			// TODO: error
			return
		}
		name := param.Name
		if name != "" {
			name += " "
		}
		outputParams = append(outputParams, name+typeName)
	}
	outputParamsStr := strings.Join(outputParams, ", ")
	if len(outputParams) > 1 {
		outputParamsStr = "(" + outputParamsStr + ")"
	}
	file.L()
	doc := child.MethodDoc(method)
	if doc != nil {
		for _, line := range doc.List {
			file.L(line.Text)
		}
	}
	file.L(`func (` + b.ReceiverName() + ` ` + b.Type.Obj().Name() + `) ` + method.Name + `(` + strings.Join(inputParams, ", ") + `) ` + outputParamsStr + " {")
	file.L("\treturn " + ref + `.` + method.Name + `(` + strings.Join(callParams, ", ") + `)`)
	file.L("}")
}

func (b *Builder) IsChainMethod(method Method) bool {
	if len(method.Results) != 1 {
		return false
	}
	return method.Results[0].Type == b.Type
}

func (b *Builder) IsFinalizer(method Method) bool {
	return !b.IsChainMethod(method)
}

const generatedPrefix = "Code generated by chaingen. DO NOT EDIT."

func (b *Builder) IsGenerated(m Method) bool {
	fileScope := m.Scope.Parent()
	for node, scope := range b.Package.TypesInfo.Scopes {
		file, ok := node.(*ast.File)
		if !ok {
			continue
		}
		if scope != fileScope {
			continue
		}
		return strings.HasPrefix(file.Doc.Text(), generatedPrefix)
	}
	return false
}

func (b *Builder) MethodDoc(m Method) *ast.CommentGroup {
	fileScope := m.Scope.Parent()
	fileTok := b.Package.Fset.File(fileScope.Pos())
	for node, scope := range b.Package.TypesInfo.Scopes {
		file, ok := node.(*ast.File)
		if !ok {
			continue
		}
		if scope != fileScope {
			continue
		}
		funcLine := fileTok.Position(m.Pos).Line
		for _, comment := range file.Comments {
			commentLine := fileTok.Position(comment.End()).Line
			if commentLine == funcLine-1 {
				return comment
			}
		}
		return nil
	}
	return nil
}

type Options struct {
	Src        string
	TypeName   string
	Recursive  bool
	FileSuffix string
}

type File struct {
	Builders []*Builder
	Package  *packages.Package
	File     *ast.File
	Path     string

	Imports       map[string]Import
	ImportAliases map[string]*Import
	Body          bytes.Buffer
}

func (f *File) Render(w io.Writer) error {
	output := "// " + generatedPrefix + "\n"
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
	switch t := typ.(type) {
	case *types.Named:
		pkg := t.Obj().Pkg()
		if pkg.Path() == f.Package.PkgPath {
			return t.Obj().Name()
		}
		return f.PackageIdentifier(pkg) + "." + t.Obj().Name()
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

func (c Chaingen) Render(builders []*Builder) (map[string]*File, error) {
	files := make(map[string]*File)
	for _, builder := range builders {
		c.render(files, builder)
	}
	return files, nil
}

func (b *Builder) HasChainMethods() bool {
	for _, m := range b.Methods {
		if b.IsChainMethod(m) && !b.IsGenerated(m) {
			return true
		}
	}
	return false
}

func (c Chaingen) render(files map[string]*File, builder *Builder) []Method {
	file, ok := files[builder.File.Name.Name]
	if !ok {
		file = &File{
			Package:       builder.Package,
			File:          builder.File,
			Path:          builder.FilePath,
			Imports:       map[string]Import{},
			ImportAliases: map[string]*Import{},
		}
		files[file.File.Name.Name] = file
	}
	var methods []Method
	for _, child := range builder.Children {
		if c.opts.Recursive {
			methods = append(methods, c.render(files, child)...)
		}
		renderMethods := append(child.Methods, methods...)
		for _, m := range renderMethods {
			if _, ok := builder.MethodNames[m.Name]; ok {
				// skip conflicting method names
				continue
			}
			switch {
			case child.IsChainMethod(m):
				builder.RenderChainMethod(file, child, m)
				methods = append(methods, Method{
					Name: m.Name,
					Results: []MethodParam{
						{
							Type: builder.Type,
						},
					},
					Recv:   m.Recv,
					Pos:    m.Pos,
					Scope:  m.Scope,
					Params: m.Params,
				})
			case child.IsFinalizer(m):
				builder.RenderFinalizer(file, child, m)
				methods = append(methods, m)
			}
		}
	}
	file.Builders = append(file.Builders, builder)
	return methods
}

func (c Chaingen) Generate() error {
	if c.opts.Src == "" {
		return fmt.Errorf("source dir is not set")
	}
	if c.opts.FileSuffix == "" {
		return fmt.Errorf("file suffix is not set")
	}
	pkgs, err := packages.Load(&packages.Config{
		Dir:  c.opts.Src,
		Mode: packages.NeedName | packages.NeedSyntax | packages.NeedTypes | packages.NeedTypesInfo | packages.NeedDeps | packages.NeedImports,
	})
	if err != nil {
		return fmt.Errorf("error loading Go packages from %s: %w", c.opts.Src, err)
	}

	found := make(map[*types.Named]*packages.Package)
	if c.opts.TypeName == "" {
		for _, p := range pkgs {
			for _, name := range p.Types.Scope().Names() {
				typ, _ := toStruct(p.Types.Scope().Lookup(name))
				if typ != nil {
					found[typ] = p
				}
			}
		}
		if len(found) == 0 {
			return fmt.Errorf("unable to find structs in %s", c.opts.Src)
		}
	} else {
		for _, p := range pkgs {
			typ, _ := toStruct(p.Types.Scope().Lookup(c.opts.TypeName))
			if typ != nil {
				found[typ] = p
				break
			}
		}
		if len(found) == 0 {
			return fmt.Errorf("unable to find struct type %q in %s", c.opts.TypeName, c.opts.Src)
		}
	}

	var builders []*Builder
	for typ, pkg := range found {
		builder, err := NewBuilder(pkg, typ)
		if err != nil {
			return fmt.Errorf("error creating builder: %w", err)
		}
		builders = append(builders, builder)
	}

	files, err := c.Render(builders)
	if err != nil {
		return fmt.Errorf("error generating code: %w", err)
	}
	for _, file := range files {
		buf := bytes.Buffer{}
		err = file.Render(&buf)
		if err != nil {
			return err
		}
		src := file.Path
		dest := src[:len(src)-3] + c.opts.FileSuffix
		formatted, err := format.Source(buf.Bytes())
		if err != nil {
			return err
		}
		err = os.WriteFile(dest, formatted, 0755)
		if err != nil {
			return err
		}
	}
	return nil
}

func NewBuilder(pkg *packages.Package, n *types.Named) (*Builder, error) {
	return newBuilder(pkg, n, "")
}

func newBuilder(pkg *packages.Package, typ *types.Named, name string) (*Builder, error) {
	builder := &Builder{
		Name:        name,
		Package:     pkg,
		Type:        typ,
		Struct:      typ.Underlying().(*types.Struct),
		MethodNames: map[string]*Method{},
	}

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
		m := NewMethod(fun, sig)
		if builder.IsGenerated(m) {
			continue
		}
		builder.Methods = append(builder.Methods, m)
		builder.MethodNames[m.Name] = &m
	}

	for i := 0; i < builder.Struct.NumFields(); i++ {
		field := builder.Struct.Field(i)
		typ, _ := toStruct(field)
		typ, ok := field.Type().(*types.Named)
		if !ok {
			continue
		}
		if !isStruct(typ) {
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
		child, err := newBuilder(childPkg, typ, name)
		if err != nil {
			return nil, fmt.Errorf("error creating builder for field %s: %w", field.Name(), err)
		}
		builder.Children = append(builder.Children, child)
	}

	return builder, nil
}

func toStruct(obj types.Object) (*types.Named, *types.Struct) {
	if obj == nil {
		return nil, nil
	}
	typName, ok := obj.(*types.TypeName)
	if !ok {
		return nil, nil
	}
	typ, ok := typName.Type().(*types.Named)
	if !ok {
		return nil, nil
	}
	str, ok := typ.Underlying().(*types.Struct)
	if !ok {
		return nil, nil
	}
	return typ, str
}

func isStruct(typ *types.Named) bool {
	_, ok := typ.Underlying().(*types.Struct)
	return ok
}
