package packagefs

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"

	"github.com/santhosh-tekuri/jsonschema/v5"
)

func Schema(raw json.RawMessage) (*jsonschema.Schema, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	if len(raw) > 64<<10 {
		return nil, errors.New("schema exceeds size limit")
	}
	compiler := jsonschema.NewCompiler()
	compiler.LoadURL = func(string) (io.ReadCloser, error) { return nil, errors.New("external schema references are disabled") }
	if err := compiler.AddResource("urn:shitbotultra:package", bytes.NewReader(raw)); err != nil {
		return nil, err
	}
	return compiler.Compile("urn:shitbotultra:package")
}

func ValidateJSON(schema *jsonschema.Schema, raw json.RawMessage) error {
	if len(raw) == 0 || len(raw) > 2<<20 {
		return errors.New("JSON data must be 1 byte to 2 MiB")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	var value any
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return errors.New("trailing JSON data")
	}
	if schema != nil {
		return schema.Validate(value)
	}
	return nil
}
