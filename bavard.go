// Copyright 2020 ConsenSys Software Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package bavard contains helper functions to generate consistent code from text/template templates
// it is used by github.com/consensys/gurvy && github.com/consensys/gnark && github.com/consensys/goff
package bavard

import (
	"bufio"
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// Bavard root object to configure the code generation from text/template
type Bavard struct {
	verbose     bool
	fmt         bool
	imports     bool
	packageName string
	packageDoc  string
	license     string
	generated   string
	buildTag    string
	funcs       template.FuncMap
}

// Generate will concatenate templates and create output file from executing the resulting text/template
// see other package functions to add options (package name, licensing, build tags, ...)
func Generate(output string, templates []string, data interface{}, options ...func(*Bavard) error) error {
	var b Bavard

	// default settings
	b.imports = true
	b.fmt = true
	b.verbose = true
	b.generated = "bavard"

	// handle options
	for _, option := range options {
		if err := option(&b); err != nil {
			return err
		}
	}

	// create output dir if not exist
	_ = os.MkdirAll(filepath.Dir(output), os.ModePerm)

	// create output file
	file, err := os.Create(output)
	if err != nil {
		return err
	}
	if b.verbose {
		fmt.Printf("generating %-70s\n", output)
	}

	if b.buildTag != "" {
		if _, err := file.WriteString("// +build " + b.buildTag + "\n"); err != nil {
			return err
		}
	}

	if b.license != "" {
		if _, err := file.WriteString(b.license + "\n"); err != nil {
			return err
		}
	}
	if _, err := file.WriteString(fmt.Sprintf("// Code generated by %s DO NOT EDIT\n\n", b.generated)); err != nil {
		return err
	}

	if b.packageName != "" {
		if b.packageDoc != "" {
			if _, err := file.WriteString("// Package " + b.packageName + " "); err != nil {
				return err
			}
			if _, err := file.WriteString(b.packageDoc + "\n"); err != nil {
				return err
			}
		}
		if _, err := file.WriteString("package " + b.packageName + "\n\n"); err != nil {
			return err
		}
	}

	// parse templates
	fnHelpers := helpers()
	for k, v := range b.funcs {
		fnHelpers[k] = v
	}
	tmpl := template.Must(template.New("").
		Funcs(fnHelpers).
		Parse(aggregate(templates)))

	// execute template
	if err = tmpl.Execute(file, data); err != nil {
		file.Close()
		return err
	}
	file.Close()

	// format generated code
	if b.fmt {
		switch filepath.Ext(output) {
		case ".go":
			cmd := exec.Command("gofmt", "-s", "-w", output)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			if err := cmd.Run(); err != nil {
				return err
			}
		case ".s":
			// a quick and dirty formatter, not even in place

			// 1- create result buffer
			var result bytes.Buffer

			// 2- open file
			file, err := os.Open(output)
			if err != nil {
				return err
			}

			scanner := bufio.NewScanner(file)
			prevLine := false
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				isJump := line == ""
				if (isJump && !prevLine) || !isJump {
					result.WriteString(line)
					result.WriteByte('\n')
				}
				if strings.HasPrefix(line, "TEXT ") {
					break
				}
				prevLine = isJump
			}
			prevLine = false
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				isJump := line == ""
				if (isJump && !prevLine) || !isJump {
					result.WriteString("    " + line)
					result.WriteByte('\n')
				}
				prevLine = isJump
			}

			if err := scanner.Err(); err != nil {
				file.Close()
				return err
			}
			file.Close()

			err = ioutil.WriteFile(output, result.Bytes(), 0644)
			if err != nil {
				return err
			}
		}

	}

	// run goimports on generated code
	if b.imports {
		cmd := exec.Command("goimports", "-w", output)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			return err
		}
	}
	return nil
}

func aggregate(values []string) string {
	var sb strings.Builder
	for _, v := range values {
		sb.WriteString(v)
	}
	return sb.String()
}

// Apache2Header returns a Apache2 header string
func Apache2Header(copyrightHolder string, year int) string {
	apache2 := `
	// Copyright %d %s
	//
	// Licensed under the Apache License, Version 2.0 (the "License");
	// you may not use this file except in compliance with the License.
	// You may obtain a copy of the License at
	//
	//     http://www.apache.org/licenses/LICENSE-2.0
	//
	// Unless required by applicable law or agreed to in writing, software
	// distributed under the License is distributed on an "AS IS" BASIS,
	// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
	// See the License for the specific language governing permissions and
	// limitations under the License.
	`
	return fmt.Sprintf(apache2, year, copyrightHolder)
}

// Apache2 returns a bavard option to be used in Generate writing an apache2 licence header in the generated file
func Apache2(copyrightHolder string, year int) func(*Bavard) error {
	return func(b *Bavard) error {
		b.license = Apache2Header(copyrightHolder, year)
		return nil
	}
}

// GeneratedBy returns a bavard option to be used in Generate writing a standard
// "Code generated by 'label' DO NOT EDIT"
func GeneratedBy(label string) func(*Bavard) error {
	return func(b *Bavard) error {
		b.generated = label
		return nil
	}
}

// BuildTag returns a bavard option to be used in Generate adding build tags string on top of the generated file
func BuildTag(buildTag string) func(*Bavard) error {
	return func(b *Bavard) error {
		b.buildTag = buildTag
		return nil
	}
}

// Package returns a bavard option adding package name and optional package documentation in the generated file
func Package(name string, doc ...string) func(*Bavard) error {
	return func(b *Bavard) error {
		b.packageName = name
		if len(doc) > 0 {
			b.packageDoc = doc[0]
		}
		return nil
	}
}

// Verbose returns a bavard option to be used in Generate. If set to true, will print to stdout during code generation
func Verbose(v bool) func(*Bavard) error {
	return func(b *Bavard) error {
		b.verbose = v
		return nil
	}
}

// Format returns a bavard option to be used in Generate. If set to true, will run gofmt on generated file.
// Or simple tab alignment on .s files
func Format(v bool) func(*Bavard) error {
	return func(b *Bavard) error {
		b.fmt = v
		return nil
	}
}

// Import returns a bavard option to be used in Generate. If set to true, will run goimports
func Import(v bool) func(*Bavard) error {
	return func(b *Bavard) error {
		b.imports = v
		return nil
	}
}

// Funcs returns a bavard option to be used in Generate. See text/template FuncMap for more info
func Funcs(funcs template.FuncMap) func(*Bavard) error {
	return func(b *Bavard) error {
		b.funcs = funcs
		return nil
	}
}
