package crudgen

import (
	"bytes"
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	contractsorm "github.com/goravel/framework/contracts/database/orm"
)

type Generator struct {
	orm  contractsorm.Orm
	root string
}

type generatedFile struct {
	path    string
	content string
	goFile  bool
}

func NewGenerator(orm contractsorm.Orm, root string) *Generator {
	return &Generator{orm: orm, root: root}
}

func (g *Generator) Generate(opts Options) error {
	table, err := inspectTable(g.orm, opts)
	if err != nil {
		return err
	}

	files, err := renderFiles(table)
	if err != nil {
		return err
	}
	if err := g.ensureWritable(files, opts.Force); err != nil {
		return err
	}
	return g.writeFiles(files)
}

func renderFiles(table Table) ([]generatedFile, error) {
	files := []generatedFile{
		{path: "app/models/" + table.FileName + ".go", content: modelTemplate, goFile: true},
		{path: "app/repositories/" + table.Module + "/" + table.FileName + "_repository.go", content: repositoryTemplate, goFile: true},
		{path: "app/http/request/" + table.Module + "/" + table.FileName + "_request.go", content: requestTemplate, goFile: true},
		{path: "app/http/controllers/admin/" + table.Module + "/" + table.FileName + "_controller.go", content: controllerTemplate, goFile: true},
		{path: "routes/" + table.Module + "_" + table.FileName + "_routes.go", content: routeTemplate, goFile: true},
	}
	for i := range files {
		content, err := executeTemplate(files[i].content, table)
		if err != nil {
			return nil, err
		}
		if files[i].goFile {
			content, err = formatGo(content)
			if err != nil {
				return nil, err
			}
		}
		files[i].content = content
	}
	return files, nil
}

func executeTemplate(source string, data Table) (string, error) {
	tpl, err := template.New("crud").Funcs(template.FuncMap{
		"lower": strings.ToLower,
	}).Parse(source)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := tpl.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func formatGo(content string) (string, error) {
	out, err := format.Source([]byte(content))
	if err != nil {
		return "", fmt.Errorf("format generated go: %w", err)
	}
	return string(out), nil
}

func (g *Generator) ensureWritable(files []generatedFile, force bool) error {
	if force {
		return nil
	}
	for _, file := range files {
		if _, err := os.Stat(filepath.Join(g.root, file.path)); err == nil {
			return fmt.Errorf("%w: %s", ErrFileExists, file.path)
		}
	}
	return nil
}

func (g *Generator) writeFiles(files []generatedFile) error {
	for _, file := range files {
		path := filepath.Join(g.root, file.path)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(file.content), 0644); err != nil {
			return err
		}
	}
	return nil
}
