package input

import (
	"encoding/json"
	"os"

	"gopkg.in/yaml.v3"
)

func DecodePayload(f string, payload *InputSchema) error {
	var err error
	if f == "yaml" {
		err = yaml.NewDecoder(os.Stdin).Decode(&payload)
	} else {
		err = json.NewDecoder(os.Stdin).Decode(&payload)
	}
	return err
}
