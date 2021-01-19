// Copyright 2020 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package openapi

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/go-openapi/spec"
	"k8s.io/kubectl/pkg/cmd/util"
	"sigs.k8s.io/kustomize/kyaml/errors"
	"sigs.k8s.io/kustomize/kyaml/openapi"
	"sigs.k8s.io/kustomize/kyaml/openapi/kustomizationapi"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

const (
	SchemaSourceBuiltin = "builtin"
	SchemaSourceFile    = "file"
	SchemaSourceCluster = "cluster"
)

// ConfigureOpenAPI sets the openAPI schema in kyaml. It can either
// fetch the schema from a cluster, read it from file, or just the
// schema built into kyaml.
func ConfigureOpenAPI(factory util.Factory, k8sSchemaSource, k8sSchemaPath string) error {
	switch k8sSchemaSource {
	case SchemaSourceCluster:
		openAPISchema, err := FetchOpenAPISchemaFromCluster(factory)
		if err != nil {
			return fmt.Errorf("error fetching schema from cluster: %v", err)
		}
		return ConfigureOpenAPISchema(openAPISchema)
	case SchemaSourceFile:
		openAPISchema, err := ReadOpenAPISchemaFromDisk(k8sSchemaPath)
		if err != nil {
			return fmt.Errorf("error reading file at path %s: %v",
				k8sSchemaPath, err)
		}
		return ConfigureOpenAPISchema(openAPISchema)
	case SchemaSourceBuiltin:
		return nil
	default:
		return fmt.Errorf("unknown schema source %s. Must be one of file, cluster, builtin",
			k8sSchemaSource)
	}
}

func FetchOpenAPISchemaFromCluster(f util.Factory) ([]byte, error) {
	restClient, err := f.RESTClient()
	if err != nil {
		return nil, err
	}
	data, err := restClient.Get().AbsPath("/openapi/v2").
		SetHeader("Accept", "application/json").Do(context.Background()).Raw()
	if err != nil {
		return nil, err
	}
	return data, nil
}

func ReadOpenAPISchemaFromDisk(path string) ([]byte, error) {
	return ioutil.ReadFile(path)
}

func ConfigureOpenAPISchema(openAPISchema []byte) error {
	openapi.SuppressBuiltInSchemaUse()

	err := openapi.AddSchema(openAPISchema)
	if err != nil {
		return err
	}
	// Also add make sure the Kustomize openAPI is always added regardless
	// of where we got the Kubernetes openAPI schema.
	// TODO: Refactor the openapi package in kyaml so we don't need to
	// know the name of the kustomize asset here.
	return openapi.AddSchema(kustomizationapi.MustAsset("kustomizationapi/swagger.json"))
}

func SchemaFromFile(path string) (*spec.Schema, error) {
	object, err := parseOpenAPI(path)
	if err != nil {
		return nil, err
	}

	return schemaFromNode(object)
}

// parseOpenAPI reads openAPIPath yaml and converts it to RNode
func parseOpenAPI(openAPIPath string) (*yaml.RNode, error) {
	b, err := ioutil.ReadFile(openAPIPath)
	if err != nil {
		return nil, err
	}

	object, err := yaml.Parse(string(b))
	if err != nil {
		return nil, errors.Errorf("invalid file %q: %v", openAPIPath, err)
	}
	return object, nil
}

// addSchemaUsingField parses the OpenAPI definitions from the specified field.
// If field is the empty string, use the whole document as OpenAPI.
func schemaFromNode(object *yaml.RNode) (*spec.Schema, error) {
	oAPI, err := object.String()
	if err != nil {
		return nil, err
	}

	// convert the yaml openAPI to a JSON string by unmarshalling it to an
	// interface{} and the marshalling it to a string
	var o interface{}
	err = yaml.Unmarshal([]byte(oAPI), &o)
	if err != nil {
		return nil, err
	}
	j, err := json.Marshal(o)
	if err != nil {
		return nil, err
	}

	var sc spec.Schema
	err = sc.UnmarshalJSON(j)
	if err != nil {
		return nil, err
	}

	return &sc, nil
}
