package generator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/pkg/errors"
	extensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	extensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	extensionsscheme "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset/scheme"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

const generatedCrdsPattern = "{{ .ResourceKind }}{{ .KindSuffix }}.json"

func Generate(customResourceDefinitionPath string, outputDir string) (files []string, err error) {

	if !filepath.IsAbs(outputDir) {
		outputDir, err = filepath.Abs(outputDir)
		if err != nil {
			workdir, err := os.Getwd()
			if err != nil {
				return files, errors.Wrap(err, "failed to get workdir for relative outputdir")
			}
			outputDir = filepath.Join(workdir, outputDir)
		}
	}

	schemas, err := GenerateString(customResourceDefinitionPath)
	if err != nil {
		return files, err
	}

	msg := ""
	for name, schema := range schemas {
		outfile := filepath.Join(outputDir, name)
		err = os.MkdirAll(filepath.Dir(outfile), 0777)
		if err != nil {
			msg += fmt.Sprintf("\t%v\n", err)
			continue
		}
		err = writeFile([]byte(schema), outfile)
		if err != nil {
			msg += fmt.Sprintf("\t%v\n", err)
			continue
		}
		files = append(files, outfile)
	}

	if msg != "" {
		return files, fmt.Errorf("Failed to write following files:\n%v", msg)
	}
	return files, nil
}

func GenerateString(customResourceDefinitionPath string) (schemas map[string]string, err error) {
	msg := fmt.Sprintf("error on crd '%v'", customResourceDefinitionPath)
	crdContents, err := readCRDFromPath(customResourceDefinitionPath)
	if err != nil {
		return schemas, errors.Wrap(err, msg)
	}

	crdObj, err := decodeCRD(crdContents)
	if err != nil {
		return schemas, errors.Wrap(err, msg)
	}

	schemas, err = generateSchemaFromCRD(crdObj)
	if err != nil {
		return schemas, errors.Wrap(err, msg)
	}
	return schemas, nil
}

func decodeCRD(raw []byte) (interface{}, error) {
	scheme := runtime.NewScheme()
	extensionsscheme.AddToScheme(scheme)
	crd, _, err := serializer.NewCodecFactory(scheme).UniversalDeserializer().Decode(raw, nil, nil)
	return crd, err
}

func generateSchemaFromCRD(crdObj interface{}) (res map[string]string, err error) {
	schemas := make(map[string]interface{})
	switch v := crdObj.(type) {
	case *extensionsv1.CustomResourceDefinition:
		crd := crdObj.(*extensionsv1.CustomResourceDefinition)
		for _, version := range crd.Spec.Versions {
			name, err := generateFilename(generatedCrdsPattern, crd.Spec.Names.Kind, fmt.Sprintf("%v/%v", crd.Spec.Group, version.Name))
			if err != nil {
				return res, err
			}
			if version.Schema != nil {
				schemas[name] = *version.Schema.OpenAPIV3Schema
			}
		}
	case *extensionsv1beta1.CustomResourceDefinition:
		crd := crdObj.(*extensionsv1beta1.CustomResourceDefinition)
		name, err := generateFilename(generatedCrdsPattern, crd.Spec.Names.Kind, fmt.Sprintf("%v/%v", crd.Spec.Group, crd.Spec.Version))
		if err != nil {
			return res, err
		}
		if crd.Spec.Validation != nil {
			schemas[name] = *crd.Spec.Validation.OpenAPIV3Schema
		}
	default:
		return res, fmt.Errorf("Unknown CRD version %v", v)
	}

	if len(schemas) == 0 {
		return res, fmt.Errorf("No validation specified")
	}

	msg := ""
	res = make(map[string]string)
	for name, schema := range schemas {
		b, err := json.MarshalIndent(schema, "", "  ")
		if err != nil {
			msg += fmt.Sprintf("Failed to marschal schema: %v\n", err)
		} else {
			res[name] = string(b)
		}
	}
	if msg != "" {
		return res, fmt.Errorf(msg)
	}
	return res, nil
}

// copied from github.com/yannh/kubeconform/pkg/registry.schemaPath method because filenaming is important for validation schema
// with yannhs kubeconform validator
func generateFilename(tpl, resourceKind, resourceAPIVersion string) (string, error) {
	groupParts := strings.Split(resourceAPIVersion, "/")
	versionParts := strings.Split(groupParts[0], ".")

	kindSuffix := "-" + strings.ToLower(versionParts[0])
	if len(groupParts) > 1 {
		kindSuffix += "-" + strings.ToLower(groupParts[1])
	}

	tmpl, err := template.New("tpl").Parse(tpl)
	if err != nil {
		return "", err
	}

	tplData := struct {
		ResourceKind       string
		ResourceAPIVersion string
		Group              string
		KindSuffix         string
	}{
		strings.ToLower(resourceKind),
		groupParts[len(groupParts)-1],
		groupParts[0],
		kindSuffix,
	}

	var buf bytes.Buffer
	err = tmpl.Execute(&buf, tplData)
	if err != nil {
		return "", err
	}

	return buf.String(), nil
}

func writeFile(b []byte, outfile string) error {

	_, err := os.Stat(outfile)
	if err == nil {
		if err := os.Remove(outfile); err != nil {
			return errors.Wrap(err, "failed to remove file")
		}
	}

	d, _ := path.Split(outfile)
	_, err = os.Stat(d)
	if os.IsNotExist(err) {
		if err = os.MkdirAll(d, 0755); err != nil {
			return errors.Wrap(err, "failed to mkdir")
		}
	}

	err = ioutil.WriteFile(outfile, b, 0644)
	if err != nil {
		return errors.Wrap(err, "failed to write file")
	}

	return nil
}

func readCRDFromPath(specPath string) ([]byte, error) {
	if !isURL(specPath) {
		if _, err := os.Stat(specPath); os.IsNotExist(err) {
			return nil, fmt.Errorf("%s was not found", specPath)
		}

		b, err := ioutil.ReadFile(specPath)
		if err != nil {
			return nil, errors.Wrap(err, "failed to read file")
		}

		return b, nil
	}
	req, err := http.NewRequest("GET", specPath, nil)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}
	req.Header.Set("User-Agent", "Replicated_CRDToOpenApiSchema/v1alpha1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute request")
	} else if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(resp.Status)
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read response body")
	}

	return body, nil
}

func isURL(str string) bool {
	parsed, err := url.ParseRequestURI(str)
	if err != nil {
		return false
	}

	return parsed.Scheme != ""
}
