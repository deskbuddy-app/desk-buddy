//go:build ignore

// One-time source migration of display Format calls. No parser/time-zone rules change.
package main
import("go/ast";"go/parser";"go/token";"os";"sort";"strconv")
func main(){for _,name:=range os.Args[1:]{b,e:=os.ReadFile(name);if e!=nil{panic(e)};fs:=token.NewFileSet();f,e:=parser.ParseFile(fs,name,b,0);if e!=nil{panic(e)}
type edit struct{a,b int;s string};var edits []edit
ast.Inspect(f,func(n ast.Node)bool{c,ok:=n.(*ast.CallExpr);if !ok||len(c.Args)!=1{return true};s,ok:=c.Fun.(*ast.SelectorExpr);if !ok||s.Sel.Name!="Format"{return true};lit,ok:=c.Args[0].(*ast.BasicLit);if !ok{return true};layout,_:=strconv.Unquote(lit.Value);switch layout{case "Monday 2 January","Mon 2 Jan","January 2006","Mon","2 January","PM":default:return true};a:=fs.Position(c.Pos()).Offset;z:=fs.Position(c.End()).Offset;x:=string(b[fs.Position(s.X.Pos()).Offset:fs.Position(s.X.End()).Offset]);edits=append(edits,edit{a,z,"displayDate("+x+", "+lit.Value+")"});return false});sort.Slice(edits,func(i,j int)bool{return edits[i].a>edits[j].a});for _,ed:=range edits{b=append(append(append([]byte{},b[:ed.a]...),[]byte(ed.s)...),b[ed.b:]...)};if len(edits)>0{if e:=os.WriteFile(name,b,0644);e!=nil{panic(e)}}}}
