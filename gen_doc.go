package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/xuri/excelize/v2"
)

func main() {
	pkgs := parseAll()

	generateExcel(pkgs)
	generatePlantUML(pkgs)
}

func parseAll() map[string]*ast.Package {
	fset := token.NewFileSet()
	pkgs := make(map[string]*ast.Package)

	filepath.Walk("./", func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}

		if info.IsDir() &&
			(strings.Contains(path, "vendor") ||
				strings.Contains(path, ".git") ||
				strings.Contains(path, "proto")) {
			return filepath.SkipDir
		}

		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
		if err != nil {
			return nil
		}

		pkgName := file.Name.Name
		if _, ok := pkgs[pkgName]; !ok {
			pkgs[pkgName] = &ast.Package{
				Name:  pkgName,
				Files: map[string]*ast.File{},
			}
		}

		pkgs[pkgName].Files[path] = file
		return nil
	})

	return pkgs
}

func generateExcel(pkgs map[string]*ast.Package) {
	f := excelize.NewFile()
	sheet := "Docs"
	f.SetSheetName("Sheet1", sheet)

	headers := []string{"Entity", "Kind", "Field", "Type", "Visibility", "Description"}
	for i, h := range headers {
		cell, _ := excelize.CoordinatesToCellName(i+1, 1)
		f.SetCellValue(sheet, cell, h)
	}

	row := 2

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {

				if gen, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range gen.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}

						desc := getDoc(gen.Doc)

						if st, ok := ts.Type.(*ast.StructType); ok {
							writeRow(f, sheet, row, ts.Name.Name, "struct", "", "", visibility(ts.Name.Name), desc)
							row++

							for _, field := range st.Fields.List {
								fieldDesc := getDoc(field.Doc)
								if fieldDesc == "" {
									fieldDesc = getDoc(field.Comment)
								}

								fieldType := exprToString(field.Type)

								for _, name := range field.Names {
									writeRow(f, sheet, row, ts.Name.Name, "field", name.Name, fieldType, visibility(name.Name), fieldDesc)
									row++
								}
							}
						}

						if it, ok := ts.Type.(*ast.InterfaceType); ok {
							writeRow(f, sheet, row, ts.Name.Name, "interface", "", "", visibility(ts.Name.Name), desc)
							row++

							for _, m := range it.Methods.List {
								methodType := exprToString(m.Type)

								for _, name := range m.Names {
									writeRow(f, sheet, row, ts.Name.Name, "method", name.Name, methodType, visibility(name.Name), "")
									row++
								}
							}
						}
					}
				}

				if fn, ok := decl.(*ast.FuncDecl); ok {
					desc := getDoc(fn.Doc)

					if fn.Recv != nil {
						recv := cleanType(exprToString(fn.Recv.List[0].Type))
						writeRow(f, sheet, row, recv, "method", fn.Name.Name, "", visibility(fn.Name.Name), desc)
					} else {
						writeRow(f, sheet, row, fn.Name.Name, "func", "", "", visibility(fn.Name.Name), desc)
					}
					row++
				}
			}
		}
	}

	f.SaveAs("docs.xlsx")
	log.Println("Готово: docs.xlsx")
}

func generatePlantUML(pkgs map[string]*ast.Package) {
	out, _ := os.Create("diagram.puml")
	defer out.Close()

	fmt.Fprintln(out, "@startuml")

	// отключаем иконки + задаем один цвет
	fmt.Fprintln(out, "skinparam classAttributeIconSize 0")
	fmt.Fprintln(out, "skinparam classBackgroundColor #FFF2CC")
	fmt.Fprintln(out, "skinparam interfaceBackgroundColor #FFF2CC")

	structs := map[string]bool{}
	interfaces := map[string][]string{}
	structMethods := map[string][]string{}

	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {

				if gen, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range gen.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}

						if _, ok := ts.Type.(*ast.StructType); ok {
							structs[ts.Name.Name] = true
						}

						if it, ok := ts.Type.(*ast.InterfaceType); ok {
							for _, m := range it.Methods.List {
								for _, name := range m.Names {
									interfaces[ts.Name.Name] = append(interfaces[ts.Name.Name], name.Name)
								}
							}
						}
					}
				}

				if fn, ok := decl.(*ast.FuncDecl); ok && fn.Recv != nil {
					recv := cleanType(exprToString(fn.Recv.List[0].Type))
					structMethods[recv] = append(structMethods[recv], fn.Name.Name)
				}
			}
		}
	}

	// классы
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {

				if gen, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range gen.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}

						if st, ok := ts.Type.(*ast.StructType); ok {
							fmt.Fprintf(out, "class %s {\n", ts.Name.Name)

							for _, field := range st.Fields.List {
								for _, name := range field.Names {
									fmt.Fprintf(out, "  %s %s %s\n",
										umlVis(name.Name),
										name.Name,
										exprToString(field.Type),
									)
								}
							}

							for _, m := range structMethods[ts.Name.Name] {
								fmt.Fprintf(out, "  %s %s()\n", umlVis(m), m)
							}

							fmt.Fprintln(out, "}")
						}

						if it, ok := ts.Type.(*ast.InterfaceType); ok {
							fmt.Fprintf(out, "interface %s {\n", ts.Name.Name)

							for _, m := range it.Methods.List {
								for _, name := range m.Names {
									fmt.Fprintf(out, "  %s %s()\n", umlVis(name.Name), name.Name)
								}
							}

							fmt.Fprintln(out, "}")
						}
					}
				}
			}
		}
	}

	// связи
	for _, pkg := range pkgs {
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				if gen, ok := decl.(*ast.GenDecl); ok {
					for _, spec := range gen.Specs {
						ts, ok := spec.(*ast.TypeSpec)
						if !ok {
							continue
						}

						st, ok := ts.Type.(*ast.StructType)
						if !ok {
							continue
						}

						for _, field := range st.Fields.List {
							t := cleanType(exprToString(field.Type))
							if structs[t] {
								fmt.Fprintf(out, "%s --> %s\n", ts.Name.Name, t)
							}
						}
					}
				}
			}
		}
	}

	// implements
	for s, methods := range structMethods {
		for iface, ifaceMethods := range interfaces {
			if implements(methods, ifaceMethods) {
				fmt.Fprintf(out, "%s ..|> %s\n", s, iface)
			}
		}
	}

	fmt.Fprintln(out, "@enduml")
	log.Println("Готово: diagram.puml")
}

func writeRow(f *excelize.File, sheet string, row int, entity, kind, field, typ, vis, desc string) {
	values := []string{entity, kind, field, typ, vis, desc}
	for i, v := range values {
		cell, _ := excelize.CoordinatesToCellName(i+1, row)
		f.SetCellValue(sheet, cell, v)
	}
}

func visibility(name string) string {
	if ast.IsExported(name) {
		return "public"
	}
	return "private"
}

func umlVis(name string) string {
	if ast.IsExported(name) {
		return "+"
	}
	return "-"
}

func getDoc(doc *ast.CommentGroup) string {
	if doc == nil {
		return ""
	}
	var lines []string
	for _, c := range doc.List {
		text := strings.TrimPrefix(c.Text, "//")
		text = strings.TrimSpace(text)
		lines = append(lines, text)
	}
	return strings.Join(lines, " ")
}

func exprToString(expr ast.Expr) string {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name
	case *ast.StarExpr:
		return "*" + exprToString(t.X)
	case *ast.ArrayType:
		return "[]" + exprToString(t.Elt)
	case *ast.SelectorExpr:
		return exprToString(t.X) + "." + t.Sel.Name
	case *ast.FuncType:
		return "func(...)"
	default:
		return "complex"
	}
}

func cleanType(t string) string {
	t = strings.TrimPrefix(t, "*")
	t = strings.TrimPrefix(t, "[]")
	if strings.Contains(t, ".") {
		parts := strings.Split(t, ".")
		t = parts[len(parts)-1]
	}
	return t
}

func implements(structMethods, ifaceMethods []string) bool {
	set := map[string]bool{}
	for _, m := range structMethods {
		set[m] = true
	}
	for _, m := range ifaceMethods {
		if !set[m] {
			return false
		}
	}
	return len(ifaceMethods) > 0
}
