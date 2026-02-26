package main

import (
	"go/parser"
	"go/token"
	"log"
	"os"
	"strings"
)

func main() {
	args := os.Args[1:]
	for i := 0; i < len(args); i++ {
		serviceRoot := args[i]
		actionsFolder := serviceRoot + "/actions"
		pluggerPathParts := strings.Split(serviceRoot, "/")
		pluggerName := strings.Join(pluggerPathParts[2:], "/")
		code := `
		package plugger_` + pluggerPathParts[len(pluggerPathParts)-1] + `

		import (
			"reflect"
			iaction "kasper/src/abstract/models/action"
			"kasper/src/abstract/models/core"

		`
		entries, err := os.ReadDir(actionsFolder)
		if err != nil {
			log.Fatal(err)
		}
		var serviceNames []string
		for _, e := range entries {
			serviceName := e.Name()
			build(pluggerName, serviceName, serviceRoot)
			serviceNames = append(serviceNames, serviceName)
			code += `
			plugger_` + serviceName + ` "kasper/src/` + pluggerName + `/pluggers/` + serviceName + `"
			action_` + serviceName + ` "kasper/src/` + pluggerName + `/actions/` + serviceName + `"
			`
		}
		code += `
		)
		
		func PlugThePlugger(core core.ICore, plugger interface{}) {
			s := reflect.TypeOf(plugger)
			for i := 0; i < s.NumMethod(); i++ {
				f := s.Method(i)
				if f.Name != "Install" {
					result := f.Func.Call([]reflect.Value{reflect.ValueOf(plugger)})
					action := result[0].Interface().(iaction.IAction)
					core.Actor().InjectAction(action)
				}
			}
		}
	
		func PlugAll(core core.ICore, modelExtender map[string]map[string]iaction.ExtendedField) {
		`
		for _, serviceName := range serviceNames {
			code += `
				a_` + serviceName + ` := &action_` + serviceName + `.Actions{App: core}
				p_` + serviceName + ` := plugger_` + serviceName + `.New(a_` + serviceName + `, core)
				PlugThePlugger(core, p_` + serviceName + `)
				p_` + serviceName + `.Install(a_` + serviceName + `, modelExtender)
			`
		}
		code += `
		}
		`
		err2 := os.MkdirAll(serviceRoot+"/main", os.ModePerm)
		if err2 != nil {
			log.Fatal(err2)
			return
		}
		writeToFile(serviceRoot+"/main/"+pluggerPathParts[len(pluggerPathParts)-1]+".go", code)
	}
}

func build(pluggerName string, serviceName string, serviceRoot string) {

	var sourcePath = serviceRoot + "/actions/" + serviceName + "/" + serviceName + ".go"
	var resultFolder = serviceRoot + "/pluggers/" + serviceName

	err := os.MkdirAll(resultFolder, os.ModePerm)
	if err != nil {
		log.Fatal(err)
		return
	}

	fSet := token.NewFileSet()

	// Parse src
	parsedAst, err := parser.ParseFile(fSet, sourcePath, nil, parser.ParseComments)
	if err != nil {
		log.Fatal(err)
		return
	}

	var funcNames []string
	for _, co := range parsedAst.Comments {
		comment := strings.Trim(strings.Trim(co.Text(), "\n"), " ")
		cParts := strings.Split(comment, " ")
		if len(cParts) > 0 {
			funcNames = append(funcNames, cParts[0])
		}
	}

	code := `
	package plugger_` + serviceName + `

	import (
		"kasper/src/abstract/models/core"
		"kasper/src/shell/utils"
	    iaction "kasper/src/abstract/models/action"
		actions "kasper/src/` + pluggerName + `/actions/` + serviceName + `"
	)
	
	type Plugger struct {
		Id      *string
		Actions *actions.Actions
		Core core.ICore
	}
	`
	for _, funcName := range funcNames {
		code += `
		func (c *Plugger) ` + funcName + `() iaction.IAction {
			return utils.ExtractSecureAction(c.Core, c.Actions.` + funcName + `)
		}
		`
	}
	code +=
		`
	func (c *Plugger) Install(a *actions.Actions, extra ...any) *Plugger {
		err := actions.Install(a, extra...)
		if err != nil {
			panic(err)
		}
		return c
	}

	func New(actions *actions.Actions, core core.ICore) *Plugger {
		id := "` + serviceName + `"
		return &Plugger{Id: &id, Actions: actions, Core: core}
	}
	`
	writeToFile(resultFolder+"/"+serviceName+".go", code)
}

func writeToFile(path string, textContent string) {
	dest, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		log.Fatal(err)
	}
	defer func(dest *os.File) {
		err := dest.Close()
		if err != nil {
			log.Fatal(err)
		}
	}(dest)
	if _, err = dest.Write([]byte(textContent)); err != nil {
		log.Fatal(err)
	}
}
