# Kubernetes CRD to OpenAPISchema

This project is a CLI that will convert `kind: CustomResourceDefinition`s to a JSON Schema file that's now compatible to use with [kubeconform](https://github.com/yannh/kubeconform) to check your CRs.

## Motivation

As part of the Replicated vendor tools, we want to help ensure that valid YAML is created for every release. As more CRDs are used in applications, the schema has become more dynamic. Many projects publish CRDs without also publishing an OpenAPISchema for the project.

## Usage

```
crd-to-openapischema <url or path to schema>
```

