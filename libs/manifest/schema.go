package manifest

import (
	"bytes"
	"embed"
	"errors"
	"fmt"
	"slices"
	"strings"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

//go:embed schema/*.schema.json
var schemaFS embed.FS

// Versions lists every supported manifestVersion, oldest first.
var Versions = []int{1}

// Schema returns the raw JSON Schema for a manifest version.
func Schema(version int) ([]byte, error) {
	if !slices.Contains(Versions, version) {
		return nil, unsupported(version)
	}
	return schemaFS.ReadFile(fmt.Sprintf("schema/v%d.schema.json", version))
}

var compiled = sync.OnceValues(func() (map[int]*jsonschema.Schema, error) {
	out := map[int]*jsonschema.Schema{}
	c := jsonschema.NewCompiler()
	for _, v := range Versions {
		raw, err := Schema(v)
		if err != nil {
			return nil, err
		}
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
		if err != nil {
			return nil, fmt.Errorf("schema v%d: %w", v, err)
		}
		url := fmt.Sprintf("https://hub.localhost/schemas/app.manifest.v%d.json", v)
		if err := c.AddResource(url, doc); err != nil {
			return nil, fmt.Errorf("schema v%d: %w", v, err)
		}
		if out[v], err = c.Compile(url); err != nil {
			return nil, fmt.Errorf("schema v%d: %w", v, err)
		}
	}
	return out, nil
})

func unsupported(version int) error {
	return fmt.Errorf("unsupported manifestVersion %d (this hub supports %v)", version, Versions)
}

// validate checks JSON-encoded manifest data against its version's schema.
func validate(version int, raw []byte) error {
	schemas, err := compiled()
	if err != nil {
		return err
	}
	sch, ok := schemas[version]
	if !ok {
		return unsupported(version)
	}
	inst, err := jsonschema.UnmarshalJSON(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	err = sch.Validate(inst)
	var verr *jsonschema.ValidationError
	if errors.As(err, &verr) {
		list := problems(verr)
		slices.Sort(list)
		return &Error{Problems: list}
	}
	return err
}

// Error lists every schema problem found in a manifest.
type Error struct {
	Problems []string
}

func (e *Error) Error() string {
	return "invalid manifest:\n  - " + strings.Join(e.Problems, "\n  - ")
}

var printer = message.NewPrinter(language.English)

// problems flattens a validation error tree into "<field>: <message>" lines.
func problems(e *jsonschema.ValidationError) []string {
	if len(e.Causes) > 0 {
		var out []string
		for _, c := range e.Causes {
			out = append(out, problems(c)...)
		}
		return out
	}
	field := strings.Join(e.InstanceLocation, ".")
	if field == "" {
		field = "(root)"
	}
	return []string{field + ": " + e.ErrorKind.LocalizedString(printer)}
}
