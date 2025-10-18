/*
Copyright 2025 Kubotal

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package misc

import (
	"fmt"
	"io"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

func LoadYaml(fileName string, target interface{}) error {
	content, err := os.ReadFile(fileName)
	if err != nil {
		return err
	}
	cnt, err := ExpandEnv(string(content))
	if err != nil {
		return fmt.Errorf("error in '%s': %w", fileName, err)
	}
	dec := yaml.NewDecoder(strings.NewReader(cnt))
	dec.KnownFields(true)
	err = dec.Decode(target)
	//err = yaml.Unmarshal([]byte(cnt), target)
	if err != nil {
		if err != io.EOF { // EOF is not an error. Just an empty file (with or without comment)
			return fmt.Errorf("error while unmarshalling '%s': %w", fileName, err)
		}
	}
	return nil
}

func ParseYaml(content string, target interface{}) error {
	dec := yaml.NewDecoder(strings.NewReader(content))
	dec.KnownFields(true)
	err := dec.Decode(target)
	//err = yaml.Unmarshal([]byte(cnt), target)
	if err != nil {
		if err != io.EOF { // EOF is not an error. Just an empty file (with or without comment)
			return fmt.Errorf("error while unmarshalling configuration: %w", err)
		}
	}
	return nil
}
